package tests

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// cld 0.3.0 and earlier ran every session on one server, -L cld, which cld leaves alone (decision
// 13.4). list does not show its sessions, and detach and kill find none there. join makes a
// session of that name on a server of its own: here first without a terminal, which it refuses.
func TestLeavesTheSharedServerAlone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	for _, test := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"list"}, 0, ""},
		{[]string{"join", "-s", "old"}, 1, "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"kill", "-s", "old"}, 1, "cld: no session 'old' (see cld list)\n"},
		{[]string{"detach", "-s", "old"}, 1, "cld: no session 'old' (see cld list)\n"},
	} {
		result := s.RunCld(nil, test.args...)
		if result.Code != test.code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr, test.code,
				test.want)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "old")
	s.WaitProbes(1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-old"}) {
		t.Errorf("sessions on cld's servers %q, want [cld-old]", sessions)
	}
	if old := s.MustTmux("cld", "list-sessions", "-F", "#{session_name}"); old != "cld-old" {
		t.Errorf("sessions on the shared server %q, want the one there", old)
	}
}

// A server named like cld's that cld did not start is none of cld's, as cld marks its own with
// @cld (decision 34). list shows nothing there, and join, detach and kill refuse the name,
// pointing at no kill, and end nothing. A server of cld 0.8.2 or earlier, without the mark but
// with prefix C-q, is cld's.
func TestLeavesAForeignServerAlone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.MustTmux("cld-x", "-f", "/dev/null", "new-session", "-d", "-s", "other", "sleep", "600")
	s.MustTmux("cld-y", "-f", "/dev/null", "new-session", "-d", "-s", "cld-y", "sleep", "600")
	s.MustTmux("cld-old", "-f", "/dev/null", "set", "-g", "prefix", "C-q", ";",
		"new-session", "-d", "-s", "cld-old", "-c", s.Work, "sleep", "600")
	for _, test := range foreignRuns(s.Work) {
		result := s.RunCld(nil, test.args...)
		if result.Code != test.code || result.Stdout != test.stdout ||
			result.Stderr != test.stderr {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr,
				test.code, test.stdout, test.stderr)
		}
	}
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-x/other", "cld-y"}) || len(s.Probes()) != 0 {
		t.Errorf("sessions %q, %d claude processes; want [cld-x/other cld-y] and none",
			sessions, len(s.Probes()))
	}
	checkForeignPaneDetach(t, s)
	checkMarkLostBeforeKill(t, s)
}

// foreignRun is a run of cld by TestLeavesAForeignServerAlone, and what it prints.
type foreignRun struct {
	args           []string
	code           int
	stdout, stderr string
}

// foreignRuns are the runs of cld on the servers cld-x and cld-y, none of cld's, and cld-old, an
// older cld's, whose session started in work.
func foreignRuns(work string) []foreignRun {
	runs := []foreignRun{{[]string{"list"}, 0,
		"NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "old   detached  now          " + work + "\n", ""}}
	for _, name := range []string{"x", "y"} {
		for _, args := range [][]string{
			{"join"}, {"join", "--new"}, {"join", "--resume", "x"}, {"detach"}, {"kill"},
		} {
			runs = append(runs, foreignRun{append(args, "-s", name), 1, "",
				"cld: tmux server cld-" + name + " is not one of cld's; use another name\n"})
		}
	}
	return append(runs,
		foreignRun{[]string{"join", "-s", "old", "--new"}, 1, "",
			lostRefusal("old", "--new", "-s old")},
		foreignRun{[]string{"detach", "-s", "old"}, 0, "", ""},
		foreignRun{[]string{"kill", "-s", "old"}, 0, "", ""})
}

// checkForeignPaneDetach runs detach without -s, as a program in a pane of cld-x runs it, with a
// terminal on session other, and checks that it refuses, detaching nothing.
func checkForeignPaneDetach(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	term := terminal.New(t, "tmux", s)
	term.Start([]string{sandbox.RealTmux, "-L", "cld-x", "attach-session", "-t", "=other"},
		s.Env, s.Work)
	sandbox.WaitFor(t, 10*time.Second, "a terminal on other", func() bool {
		return slices.Contains(s.Clients(), "other")
	})
	pane := map[string]string{"TMUX": filepath.Join(s.SocketDir(), "cld-x") + ",1,0",
		"TMUX_PANE": s.MustTmux("cld-x", "display-message", "-p", "-t", "=other:", "#{pane_id}")}
	const foreign = "cld: tmux server cld-x is not one of cld's; " +
		"name the session with -s SUFFIX (see cld list)\n"
	result := s.RunCld(pane, "detach")
	if result.Code != 1 || result.Stdout != "" || result.Stderr != foreign {
		t.Errorf("detach in a pane of cld-x: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, foreign)
	}
	clients := s.MustTmux("cld-x", "list-clients", "-F", "#{session_name}")
	if clients != "other" || !term.Running() {
		t.Errorf("clients on cld-x %q, the terminal running: %v; want the one on other",
			clients, term.Running())
	}
}

// checkMarkLostBeforeKill checks a server of cld's that has outlived its session, and loses the
// mark after kill read it: the kill's own tmux command checks the mark again (decision 34.2), and
// ends nothing. A tmux first on the PATH takes the mark away.
func checkMarkLostBeforeKill(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	s.MustTmux("cld-z", "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
		"new-session", "-d", "-s", "side", "sleep", "600")
	env := wrapTmux(t, s, "case \"$*\" in *kill-server*)\n\ttmux -L cld-z set -su @cld ;;\nesac\n")
	result := s.RunCld(env, "kill", "-s", "z")
	if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("kill -s z, the mark gone before kill-server: exit %d, stdout %q, stderr %q, "+
			"want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	if sessions := s.MustTmux("cld-z", "list-sessions", "-F", "#{session_name}"); sessions != "side" {
		t.Errorf("sessions on server cld-z %q, want side", sessions)
	}
}
