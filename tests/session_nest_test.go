package tests

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// Inside another tmux (TMUX set) cld nests, on its own server, its client taking the terminal for
// UTF-8 (decision 2). cld looks for its own panes on its own servers only. It nests in a pane of
// the default server, of the one cld 0.3.0 shared, of cld-x.y and of an unmarked cld-outer. In the
// last, list is interactive too.
func TestNestsInsideAnotherTmux(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	elsewhere := filepath.Join(s.Root, "elsewhere", "default") + ",1,0"
	startCld(t, s, "tmux", map[string]string{"TMUX": elsewhere, "LANG": "C"}, "join")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-0"}) {
		t.Errorf("sessions %q, want [cld-0]", sessions)
	}
	if utf8 := s.MustTmux("cld-0", "list-clients", "-F", "#{client_utf8}"); utf8 != "1" {
		t.Errorf("client_utf8 %q under LANG=C, want 1", utf8)
	}
	for i, server := range []string{"default", "cld", "cld-x.y", "cld-outer"} {
		checkNestsInPane(t, s, server, "in"+strconv.Itoa(i))
	}
	s.MustTmux("cld-outer", append([]string{"new-session", "-d", "-s", "list", "-c", s.Work},
		s.CldArgv("list")...)...)
	sandbox.WaitFor(t, 10*time.Second, "the interactive list in a pane of server cld-outer",
		func() bool {
			screen, err := s.Tmux("cld-outer", "capture-pane", "-p", "-t", "=list:")
			return err == nil && strings.Contains(screen, "enter to join")
		})
}

// checkNestsInPane runs cld join -s name in a pane of server, on its own pty, with the TMUX tmux
// sets for it, and checks that it attaches.
func checkNestsInPane(t *testing.T, s *sandbox.Sandbox, server, name string) {
	t.Helper()
	out := filepath.Join(s.Root, name)
	s.MustTmux(server, append([]string{"-f", "/dev/null", "new-session", "-d", "-s", "outer",
		"-c", s.Work, "sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, out},
		s.CldArgv("join", "-s", name)...)...)
	sandbox.WaitFor(t, 10*time.Second, "cld in a pane of server "+server+" to attach or return",
		func() bool {
			_, err := os.Stat(out + ".code")
			return err == nil || slices.Contains(s.Clients(), "cld-"+name)
		})
	if !slices.Contains(s.Clients(), "cld-"+name) {
		stderr, err := os.ReadFile(out + ".err")
		if err != nil {
			stderr = []byte(err.Error())
		}
		t.Errorf("cld join in a pane of server %s did not attach: %q", server, stderr)
	}
}

// A dead pane keeps the name of its closed pty, which the system hands to the next terminal
// opened. tmux refuses a client on a pty of that name (decision 2), but cld's client, with an empty
// TMUX, attaches all the same. Not parallel: a terminal another test opens could take the name
// first.
func TestNestsOnADeadPanesPty(t *testing.T) {
	s := sandbox.New(t)
	// Session main, on a server marked as cld marks its own, and a dead pane on its server:
	// remain-on-exit keeps the pane, dead, once false has exited.
	s.MustTmux("cld-main", "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
		"new-session", "-d", "-s", "cld-main", "sleep", "600")
	s.MustTmux("cld-main", "set", "-g", "remain-on-exit", "on", ";",
		"new-session", "-d", "-s", "dead", "false")
	sandbox.WaitFor(t, 10*time.Second, "the pane to die", func() bool {
		return s.MustTmux("cld-main", "list-panes", "-t", "=dead", "-F", "#{pane_dead}") == "1"
	})
	dead := s.MustTmux("cld-main", "list-panes", "-t", "=dead", "-F", "#{pane_tty}")

	// A pane of another tmux: the terminal writes down its pty and runs join. printf ends the
	// line: uutils' tty (0.8.0) prints the name without a newline.
	env := map[string]string{"TMUX": filepath.Join(s.Root, "elsewhere", "default") + ",1,0"}
	maps.Copy(env, s.Env)
	ttyFile := filepath.Join(s.Root, "tty")
	term := terminal.New(t, "tmux", s)
	term.Start(append([]string{"sh", "-c",
		`tty=$(tty) && printf '%s\n' "$tty" >"$0" && exec "$@" join -s main`, ttyFile},
		s.CldArgv()...), env, s.Work)
	var tty []byte
	sandbox.WaitFor(t, 10*time.Second, "the terminal's pty", func() bool {
		var err error
		tty, err = os.ReadFile(ttyFile)
		return err == nil && strings.HasSuffix(string(tty), "\n")
	})
	if got := strings.TrimSuffix(string(tty), "\n"); got != dead {
		t.Skipf("the terminal got %s rather than the dead pane's %s", got, dead)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld join to attach", func() bool {
		return slices.Equal(s.Clients(), []string{"cld-main"}) || !term.Running()
	})
	if !term.Running() {
		t.Fatalf("cld join on %s failed: %q", dead, term.Output())
	}
}

// In a live pane of one of cld's servers, join moves the terminal on the pane's session (decision
// 51.5). With none attached it refuses, attaching, creating or bringing back alike, before claude
// --version. cld finds the server by the socket the pane's TMUX names, after a change of
// TMUX_TMPDIR too.
func TestJoinInItsOwnPaneWithNoTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	moved := filepath.Join(s.Root, "moved")
	if err := os.Mkdir(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, test := range ownPaneJoins(moved) {
		checkOwnPaneJoin(t, s, strconv.Itoa(i), test)
	}
	sessions := s.Sessions()
	if slices.ContainsFunc(sessions, func(session string) bool {
		return strings.HasSuffix(session, "cld-c") || strings.HasSuffix(session, "cld-d") ||
			strings.HasSuffix(session, "cld-0")
	}) {
		t.Errorf("sessions %q, want no cld-0, cld-c or cld-d", sessions)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a", "cld-b"}) {
		t.Errorf("clients attached to %q, want the first ones, to cld-a and cld-b", clients)
	}
}

// ownPaneJoin is a join of TestJoinInItsOwnPaneWithNoTerminal, and its refusal.
type ownPaneJoin struct {
	// env is what the pane runs cld with besides the server's environment.
	env  []string
	args []string
	want string
}

// ownPaneJoins are the joins TestJoinInItsOwnPaneWithNoTerminal runs, one with TMUX_TMPDIR moved.
// A claude too old is the terminal's cld join's to report, so join refuses it here as any other.
func ownPaneJoins(moved string) []ownPaneJoin {
	nested := "cld: no terminal is attached to this session for join to move; " +
		"run cld join in a terminal (see cld help join)\n"
	old := "CLD_FAKE_CLAUDE_VERSION=2.1.231 (Claude Code)"
	return []ownPaneJoin{
		{nil, []string{"join", "-s", "a"}, nested},
		{nil, []string{"join", "-s", "b"}, nested},
		{nil, []string{"join", "-s", "c"}, nested},
		{nil, []string{"join", "-s", "c", "--resume", "x"}, nested},
		{nil, []string{"join"}, nested},
		// tmux -L cld-a would look for the socket in the directory TMUX_TMPDIR names now.
		{[]string{"TMUX_TMPDIR=" + moved}, []string{"join", "-s", "a"}, nested},
		{[]string{old}, []string{"join", "-s", "d"}, nested},
		{[]string{old}, []string{"join", "-s", "d", "--resume", "x"}, nested},
	}
}

// checkOwnPaneJoin runs the join in a pane on session a's server, on its own pty, with the TMUX
// tmux sets for it. It checks that the join fails with the refusal.
func checkOwnPaneJoin(t *testing.T, s *sandbox.Sandbox, index string, test ownPaneJoin) {
	t.Helper()
	out := filepath.Join(s.Root, index)
	argv := append(append([]string{"env"}, test.env...), s.CldArgv(test.args...)...)
	s.MustTmux("cld-a", append([]string{"new-session", "-d", "-s", "in-" + index,
		"sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, out}, argv...)...)
	command := strings.Join(append(append(slices.Clone(test.env), "cld"), test.args...), " ")
	code := waitExitFile(t, command+" to return", out)
	stderr, err := os.ReadFile(out + ".err")
	if err != nil || code != "1\n" || string(stderr) != test.want {
		t.Errorf("%s: exit %s, stderr %q, want exit 1, stderr %q",
			command, strings.TrimSpace(code), stderr, test.want)
	}
}
