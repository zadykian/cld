package tests

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// The lines a restore types (decision 59.7), byte for byte, and each run by the baseline terminal.
// The fake ssh runs its remote half in the sandbox, where cld join --existing meets real tmux.

// The lines that join session app-3, whose NAME splits, and x, which -s alone names, on ws.
const (
	splitLine = `ssh -t -o ConnectTimeout=10 ws 'exec $SHELL -lc "cld join --existing -n app ` +
		`-s 3; exec $SHELL -l"'`
	wholeLine = `ssh -t -o ConnectTimeout=10 ws 'exec $SHELL -lc "cd / && cld join --existing ` +
		`-s x; cd; exec $SHELL -l"'`
	uncached = "TERM=xterm-256color command "
)

// typedLines are a pane's last, by host and session, and the line a restore types for it. Ghostty
// has the terminfo of ws and v6 cached, and not ws2's or ws3's. ssh -G gives ws3 port 2222, and v6
// an IPv6 address on port 2200.
var typedLines = []struct{ host, session, line string }{
	{"ws", "app-3", splitLine},
	{"ws", "x", wholeLine},
	{"ws", "", "ssh ws"},
	{"ws2", "app-3", uncached + strings.Replace(splitLine, " ws ", " ws2 ", 1)},
	{"ws2", "", uncached + "ssh ws2"},
	{"ws3", "", uncached + "ssh ws3"},
	{"v6", "", "ssh v6"},
}

// A restore types into each pane at its prompt with a last the line that joins it again. That is
// ssh -t with a login shell that joins the session, from / where -s alone names it, then a shell;
// or ssh alone. TERM=xterm-256color command starts it where Ghostty has no terminfo cached for the
// host, as ssh -G's user@hostname names it; a port but 22 joins the key. Each host is asked once.
func TestTabsLines(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.WriteProbeFile(mac.SSHCache, "me@ws\nme@[fe80::1]:2200\n")
	s.WriteProbeFile(mac.SSHHosts, "ws3 ws3.lan 2222\nv6 fe80::1 2200\n")
	m := ghosttyApp(t, s)
	now := time.Now().UTC().Truncate(time.Second)
	panes, ids, all := map[string]*tabs.Pane{}, []string{}, []mac.Tab{}
	var stdout strings.Builder
	for _, test := range typedLines {
		id := addPane(m, "~")
		panes[id] = &tabs.Pane{Kind: "local", Since: now, Last: &tabs.Last{Host: test.host,
			Session: test.session, Cut: "quit", Run: "1@x", At: now}}
		ids, all = append(ids, id), append(all, tabOf(id))
		stdout.WriteString("Typed into the pane in /Users/me: " + bindingOf(test.host,
			test.session) + "\n")
	}
	m.Windows = []mac.Window{windowOf(all...)}
	saveModel(t, s, m)
	writeRecord(t, s, recordOf(m, panes))
	checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0, stdout.String(),
		uncachedNote("ws2")+uncachedNote("ws3")+noWatcher)
	m = loadModel(t, s)
	for i, test := range typedLines {
		if got := m.Terminals[ids[i]].Typed; !slices.Equal(got, []string{"^U", test.line, "\r"}) {
			t.Errorf("%s on %s: typed %q, want Ctrl+U, %q and Enter", test.session, test.host,
				got, test.line)
		}
	}
	checkContent(t, filepath.Join(s.ProbeDir, mac.GhosttyLog), "+ssh-cache --host=me@ws\n"+
		"+ssh-cache --host=me@ws2\n+ssh-cache --host=me@ws3.lan:2222\n"+
		"+ssh-cache --host=me@[fe80::1]:2200\n")
	var asked []string
	for _, call := range s.SSHCalls() {
		asked = append(asked, strings.Join(call.Argv, " "))
	}
	if want := []string{"-G ws", "-G ws2", "-G ws3", "-G v6"}; !slices.Equal(asked, want) {
		t.Errorf("ssh ran with %q, want %q", asked, want)
	}
}

// noWatcher is the restore's note where no watcher runs and cld runs in no Ghostty pane.
const noWatcher = "cld: no watcher runs here: the next ssh in a Ghostty pane starts one\n"

// uncachedNote is the restore's note on host, whose terminfo Ghostty has not cached.
func uncachedNote(host string) string {
	return "cld: " + host + " has no Ghostty terminfo cached: cld types TERM=xterm-256color " +
		"command ssh; with shell-integration-features = ssh-terminfo, run ssh " + host +
		" once and later restores keep Ghostty's TERM\n"
}

// bindingOf is what a restore's report calls a pane's binding.
func bindingOf(host, session string) string {
	if session == "" {
		return "ssh to " + host
	}
	return "session '" + session + "' on " + host
}

// lineCase is a typed line run where the host has sessions: the session it joins, by its NAME,
// or what cld says as it refuses.
type lineCase struct {
	name, line string
	sessions   func(t *testing.T, s *sandbox.Sandbox) // the host's sessions; nil for none
	joins      string
	resumes    bool // the session has stopped: joining it starts its claude again
	refusal    string
	older      bool // the host's cld knows no --existing
}

// Each line, run as a pane's shell runs it, joins its session on the host: one that runs, or one
// that a stop ended, whose run mark remains. One that a kill or claude's exit ended, or that never
// was, is refused, and the host's shell follows. So does an older cld, which knows no --existing,
// and makes nothing. Where Ghostty has no terminfo, the line's TERM is one the host knows.
func TestTabsLinesJoin(t *testing.T) {
	t.Parallel()
	for _, test := range []lineCase{
		{name: "running", line: splitLine, sessions: sessionsOf("app-3"), joins: "app-3"},
		{name: "in / with no terminfo", line: uncached + wholeLine, sessions: sessionsOf("x"),
			joins: "x"},
		{name: "stopped", line: splitLine, sessions: stoppedSession, joins: "app-3",
			resumes: true},
		{name: "killed", line: splitLine, sessions: endedSession("app-3", killSession),
			refusal: "session 'app-3' has ended: cld join -n app -s 3 resumes it"},
		{name: "exited", line: wholeLine, sessions: endedSession("x", exitClaude),
			refusal: "session 'x' has ended: cld join -s x resumes it"},
		{name: "missing", line: splitLine, refusal: "no session 'app-3' (see cld list)"},
		{name: "an older cld", line: splitLine, older: true,
			refusal: "join: unexpected argument '--existing' (see cld help)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runLine(t, test)
		})
	}
}

// runLine runs test's line in a tmux terminal. An uncached line runs under TERM=dumb, which join
// refuses, as the host lacks Ghostty's terminfo. It detaches from the session the line joins with
// C-q d, then leaves the host's shell with exit.
func runLine(t *testing.T, test lineCase) {
	t.Helper()
	s := sandbox.New(t)
	if test.sessions != nil {
		test.sessions(t, s)
	}
	before, sockets, probes := recordState(t, s), s.Servers(), len(s.Probes())
	cld := filepath.Dir(sandbox.Cld)
	if test.older {
		cld = filepath.Join(s.Root, "old")
		makeDirs(t, cld)
		s.WriteProgram(filepath.Join(cld, "cld"), "#!/bin/sh\necho \"cld: join: unexpected "+
			"argument '--existing' (see cld help)\" >&2\nexit 2\n", 0o755)
	}
	writeProfile(t, s, cld+":"+s.RealTools("tmux"))
	env := maps.Clone(s.Env)
	if strings.HasPrefix(test.line, uncached) {
		env["TERM"] = "dumb"
	}
	term := terminal.New(t, "tmux", s)
	term.Start([]string{"bash", "-c", test.line}, env, s.Work)
	if test.joins == "" {
		waitScreen(t, term, "cld: "+test.refusal)
		leaveShell(t, term)
		checkNothingMade(t, s, before, sockets, probes)
		return
	}
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-" + test.joins}) {
		t.Errorf("clients on %q, want one on cld-%s", clients, test.joins)
	}
	if test.resumes {
		claude := s.WaitProbes(probes + 1)[probes]
		if !optionFollowedBy(claude.Argv, "--resume", firstID) {
			t.Errorf("claude's arguments %q, want --resume %s", claude.Argv, firstID)
		}
	}
	term.Keys("C-q", "d")
	leaveShell(t, term)
	want := probes
	if test.resumes {
		want++
	}
	if started := len(s.Probes()); started != want {
		t.Errorf("%d claudes started, want %d", started, want)
	}
}

// leaveShell waits for the host's shell that the line's exec $SHELL -l starts, shows that it
// answers, and leaves it with exit. The line's ssh, and the terminal's program, then end. A
// detaching tmux client may read the first keys typed after C-q d, so the echo goes again.
func leaveShell(t *testing.T, term terminal.Terminal) {
	t.Helper()
	answered := func() bool { return strings.Contains(term.Screen(), "ws-42") }
	for range 5 {
		term.Keys("echo ws-$((6*7))", "Enter")
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline) &&
			!answered(); {
			time.Sleep(50 * time.Millisecond)
		}
		if answered() {
			break
		}
	}
	waitScreen(t, term, "ws-42")
	term.Keys("exit", "Enter")
	sandbox.WaitFor(t, 10*time.Second, "the line to end", func() bool { return !term.Running() })
}

// sessionsOf makes the host's sessions of names, detached.
func sessionsOf(names ...string) func(t *testing.T, s *sandbox.Sandbox) {
	return func(t *testing.T, s *sandbox.Sandbox) {
		t.Helper()
		detachedSessions(t, s, names...)
	}
}

// stoppedSession makes session app-3 on the host, then ends its server as a reboot would: its run
// mark remains, with its conversation's ID.
func stoppedSession(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	term := startCld(t, s, "tmux", nil, "join", "-n", "app", "-s", "3")
	claude := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	claude.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	s.MustTmux("cld-app-3", "kill-server")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !claude.Alive() })
	waitServerExit(t, s, "app-3")
	sandbox.WaitFor(t, 10*time.Second, "join to end", func() bool { return !term.Running() })
}

// endedSession makes session name on the host, then ends it with end, which removes its run mark.
func endedSession(
	name string, end func(t *testing.T, s *sandbox.Sandbox, name string, claude *sandbox.Probe),
) func(t *testing.T, s *sandbox.Sandbox) {
	return func(t *testing.T, s *sandbox.Sandbox) {
		t.Helper()
		claude := detachedSessions(t, s, name)[name]
		end(t, s, name, claude)
		sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !claude.Alive() })
		waitServerExit(t, s, name)
	}
}

// killSession ends session name with cld kill.
func killSession(t *testing.T, s *sandbox.Sandbox, name string, _ *sandbox.Probe) {
	t.Helper()
	mustKill(t, s, name)
}

// exitClaude ends session name as claude exits with status 0, as /exit does.
func exitClaude(_ *testing.T, _ *sandbox.Sandbox, _ string, claude *sandbox.Probe) {
	claude.Send("exit")
}
