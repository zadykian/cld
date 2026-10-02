package tests

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// join in a pane of cld's servers moves the terminal a key was typed in last on that session to
// the session it names, saying nothing (decision 51.5). The terminal's cld join runs with its own
// environment, in the directory join ran in, and records where it came from. What join would
// refuse, it refuses in the pane, as it does with no terminal to move.
func TestJoinMovesTheTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	r := moveRig{s: s}
	r.first = startCld(t, s, "tmux", map[string]string{"CLD_TERMINAL": "first"}, "join", "-s", "a")
	waitClients(t, s, 1)
	r.second = startCld(t, s, "tmux", map[string]string{"CLD_TERMINAL": "second"},
		"join", "-s", "a")
	waitClients(t, s, 2)
	waitScreen(t, r.second, "probe --name cld-a")
	r.claude = map[string]string{"CLAUDE_CODE_SESSION_ID": "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0",
		"CLAUDE_CODE_CHILD_SESSION": "1"}
	maps.Copy(r.claude, paneOf(t, s, "a"))
	r.probe = claudeOf(t, s, "a")

	r.moveFirst(t)
	r.createFromSecond(t)
	r.fromB(t)
	none := "cld: no terminal is attached to this session for join to move; " +
		"run cld join in a terminal (see cld help join)\n"
	result := s.RunCld(r.claude, "join", "-s", "b")
	if result.Code != 1 || result.Stdout != "" || result.Stderr != none {
		t.Errorf("join -s b with no terminal on a: exit %d, stdout %q, stderr %q, "+
			"want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, none)
	}
	r.fromShell(t)
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-0", "cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want 0, a, b and c, each running", sessions)
	}
}

// moveRig is TestJoinMovesTheTerminal's sandbox, its two terminals, attached to a, and claude a,
// with the environment it runs commands with.
type moveRig struct {
	s             *sandbox.Sandbox
	first, second terminal.Terminal
	probe         *sandbox.Probe
	claude        map[string]string
}

// checkMoved checks that a join that moved a terminal exited 0, printing nothing.
func checkMoved(t *testing.T, what string, result sandbox.Result) {
	t.Helper()
	if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 0 and no output",
			what, result.Code, result.Stdout, result.Stderr)
	}
}

// moveFirst types a key in the first terminal and runs join -s b in claude's pane: the first moves
// to b, and the second stays on a.
func (r moveRig) moveFirst(t *testing.T) {
	t.Helper()
	mark := r.probe.Mark()
	r.first.Keys("x")
	r.probe.WaitInput(mark, "x")
	checkMoved(t, "join -s b in claude's pane", r.s.RunCld(r.claude, "join", "-s", "b"))
	waitScreen(t, r.first, "probe --name cld-b")
	waitClients(t, r.s, 2)
	clients := r.s.Clients()
	if !slices.Equal(clients, []string{"cld-a", "cld-b"}) || !r.first.Running() ||
		!r.second.Running() {
		t.Errorf("clients attached to %q, want the second on cld-a and the first on cld-b", clients)
	}
	if last := lastOf(r.s, "b"); last != "a" {
		t.Errorf("b records %q as the last session, want a", last)
	}
}

// createFromSecond types a key in the second terminal and runs a join that creates c, from a
// directory whose name leaves nothing of a NAME and goes encoded to the terminal. claude c starts
// there, with the second terminal's variables and not claude a's.
func (r moveRig) createFromSecond(t *testing.T) {
	t.Helper()
	dir := filepath.Join(r.s.Work, `' #;$`)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mark := r.probe.Mark()
	r.second.Keys("y")
	r.probe.WaitInput(mark, "y")
	checkMoved(t, "join -s c in claude's pane",
		r.s.RunCldIn(dir, r.claude, "join", "-s", "c", "--", "hello"))
	waitScreen(t, r.second, "probe --name cld-c")
	c := claudeOf(t, r.s, "c")
	if c.Cwd != dir {
		t.Errorf("claude c runs in %s, want %s", c.Cwd, dir)
	}
	if c.Env["CLD_TERMINAL"] != "second" || c.Env["CLAUDE_CODE_SESSION_ID"] != "" ||
		c.Env["CLAUDE_CODE_CHILD_SESSION"] != "" {
		t.Errorf("claude c has CLD_TERMINAL %q, CLAUDE_CODE_SESSION_ID %q and "+
			"CLAUDE_CODE_CHILD_SESSION %q, want the second terminal's, and none of claude a's",
			c.Env["CLD_TERMINAL"], c.Env["CLAUDE_CODE_SESSION_ID"],
			c.Env["CLAUDE_CODE_CHILD_SESSION"])
	}
	if !slices.Contains(c.Argv, "hello") {
		t.Errorf("claude c has arguments %q, want hello among them", c.Argv)
	}
	if last := lastOf(r.s, "c"); last != "a" {
		t.Errorf("c records %q as the last session, want a", last)
	}
	waitClients(t, r.s, 2)
	if clients := r.s.Clients(); !slices.Equal(clients, []string{"cld-b", "cld-c"}) {
		t.Errorf("clients attached to %q, want [cld-b cld-c]", clients)
	}
}

// fromB runs join in b's pane: refused, where claude shows why and the terminal stays on b, and
// then without -s, where the terminal's cld join makes a session under the next index, 0 here.
func (r moveRig) fromB(t *testing.T) {
	t.Helper()
	inB := paneOf(t, r.s, "b")
	lost := lostRefusal("c", "--new", "-s c")
	result := r.s.RunCld(inB, "join", "-s", "c", "--new")
	if result.Code != 1 || result.Stderr != lost {
		t.Errorf("join -s c --new in b's pane: exit %d, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stderr, lost)
	}
	if !r.first.Running() || !slices.Contains(r.s.Clients(), "cld-b") {
		t.Errorf("the first terminal left b, clients attached to %q", r.s.Clients())
	}
	checkMoved(t, "join in b's pane", r.s.RunCld(inB, "join"))
	waitScreen(t, r.first, "probe --name cld-0")
	if last := lastOf(r.s, "0"); last != "b" {
		t.Errorf("0 records %q as the last session, want b", last)
	}
}

// fromShell runs join in a shell in a window of c, which goes by the shell's terminal, the
// second.
func (r moveRig) fromShell(t *testing.T) {
	t.Helper()
	shell := filepath.Join(r.s.Root, "shell")
	r.s.MustTmux("cld-c", append([]string{"new-window", "-t", "=cld-c:", "-c", r.s.Work,
		"sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, shell, "env", "-u", "TMUX_PANE"},
		r.s.CldArgv("join", "-s", "b")...)...)
	waitScreen(t, r.second, "probe --name cld-b")
	code := waitExitFile(t, "cld join in a shell to return", shell)
	stderr, err := os.ReadFile(shell + ".err")
	if err != nil || code != "0\n" || len(stderr) != 0 {
		t.Errorf("cld join in a shell: exit %s, stderr %q, want exit 0 and no output",
			strings.TrimSpace(code), stderr)
	}
	if clients := r.s.Clients(); !slices.Equal(clients, []string{"cld-0", "cld-b"}) {
		t.Errorf("clients attached to %q, want [cld-0 cld-b]", clients)
	}
}

// Two joins at once, from claude's panes in two sessions, into one session that does not run: both
// terminals move there, and the session is made once. One terminal's cld join makes it, and the
// other finds its start mark, waits and attaches (decision 50.4). The first is held as its tmux is
// about to make the session.
func TestJoinMovesToOneSessionAtOnce(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	tmux := holdTmux(t, s, "the first terminal's tmux making cld-x", "*' new-session -s cld-x '*")
	first := startCld(t, s, "tmux", tmux.env, "join", "-s", "a")
	waitClients(t, s, 1)
	second := startCld(t, s, "tmux", tmux.env, "join", "-s", "b")
	waitClients(t, s, 2)
	inA, inB := paneOf(t, s, "a"), paneOf(t, s, "b")
	tmux.start(t)
	if result := s.RunCld(inA, "join", "-s", "x"); result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join -s x in a's pane: exit %d, stderr %q", result.Code, result.Stderr)
	}
	tmux.held(t)
	if result := s.RunCld(inB, "join", "-s", "x"); result.Code != 0 || result.Stderr != "" {
		t.Fatalf("join -s x in b's pane: exit %d, stderr %q", result.Code, result.Stderr)
	}
	time.Sleep(time.Second)
	if begun := tmux.begun(t); begun != 1 {
		t.Errorf("%d tmux commands began to make cld-x while the first was held, want 1", begun)
	}
	tmux.release(t)
	sandbox.WaitFor(t, 10*time.Second, "both terminals on cld-x", func() bool {
		return slices.Equal(s.Clients(), []string{"cld-x", "cld-x"})
	})
	waitScreen(t, first, "probe --name cld-x")
	waitScreen(t, second, "probe --name cld-x")
	claudeOf(t, s, "x")
	time.Sleep(500 * time.Millisecond)
	if probes := s.Probes(); len(probes) != 3 {
		t.Errorf("%d claudes started, want 3: a's, b's and one of x", len(probes))
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-x"}) {
		t.Errorf("sessions %q, want [cld-a cld-b cld-x]", sessions)
	}
}

// claudeOf is the claude of session name, among the probes started so far, once it has started.
func claudeOf(t *testing.T, s *sandbox.Sandbox, name string) *sandbox.Probe {
	t.Helper()
	var found *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, "claude of cld-"+name+" to start", func() bool {
		for _, probe := range s.Probes() {
			if len(probe.Argv) > 1 && probe.Argv[1] == "cld-"+name {
				found = probe
			}
		}
		return found != nil
	})
	return found
}

// paneOf is the TMUX and TMUX_PANE of claude's pane in session name, with which claude runs a
// command: ! cld join among them.
func paneOf(t *testing.T, s *sandbox.Sandbox, name string) map[string]string {
	t.Helper()
	probe := claudeOf(t, s, name)
	pane := map[string]string{"TMUX": probe.Env["TMUX"], "TMUX_PANE": probe.Env["TMUX_PANE"]}
	if !strings.HasPrefix(pane["TMUX"], filepath.Join(s.SocketDir(), "cld-"+name)+",") ||
		pane["TMUX_PANE"] == "" {
		t.Fatalf("claude %s has TMUX %q and TMUX_PANE %q", name, pane["TMUX"], pane["TMUX_PANE"])
	}
	return pane
}

// lastOf is what session name records as the session a move took its terminal from, or nothing
// where tmux cannot show it.
func lastOf(s *sandbox.Sandbox, name string) string {
	out, err := s.Tmux("cld-"+name, "show", "-v", "-t", "=cld-"+name+":", "@cld-last")
	if err != nil {
		return ""
	}
	return out
}
