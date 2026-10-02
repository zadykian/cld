package tests

import (
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// claude exiting closes its own session only, and its server: the other session, its client and
// its claude carry on.
func TestClaudeExitClosesOnlyItsSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "join", "-s", "a")
	s.WaitProbes(1)
	b := startCld(t, s, "tmux", nil, "join", "-s", "b")
	probes := map[string]*sandbox.Probe{}
	for _, probe := range s.WaitProbes(2) {
		probes[probe.Argv[1]] = probe
	}
	waitClients(t, s, 2)
	probes["cld-a"].Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld a to exit", func() bool { return !a.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
		t.Errorf("sessions %q, want [cld-b]", sessions)
	}
	if !b.Running() || !probes["cld-b"].Alive() {
		t.Error("session b did not survive session a's claude exiting")
	}
	sandbox.WaitFor(t, 10*time.Second, "session a's server to exit", func() bool {
		_, err := s.Tmux("cld-a", "list-sessions")
		return err != nil
	})
}

// A claude that fails keeps its session (decision 5). The terminal shows claude's last words and
// how to end the session, on the message line and, once a key clears it, on the pane's border.
// list says claude exited, and join refuses to make it anew until kill ends it. A join --resume
// that finds no conversation fails that way, and a pane split off that fails closes, with no hint.
func TestFailedClaudeKeepsSession(t *testing.T) {
	t.Parallel()
	for _, test := range failedClaudeCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			checkFailedClaude(t, test)
		})
	}
}

// failedClaudeCase is a claude of TestFailedClaudeKeepsSession's, which fails.
type failedClaudeCase struct {
	name string
	args []string
	// startup is what claude prints as it fails at startup; fail makes it fail later instead
	startup string
	// how the hint says claude exited: tmux names a signal where the C library has sys_signame
	// (macOS), and numbers it elsewhere
	how  []string
	fail func(t *testing.T, s *sandbox.Sandbox)
}

var failedClaudeCases = []failedClaudeCase{
	{"start", []string{"join", "-s", "bad"}, "Error: cannot start", []string{"status 1"}, nil},
	{"resume", []string{"join", "-s", "bad", "--resume", "x"},
		"No conversation found with session ID: x", []string{"status 1"}, nil},
	{"status", []string{"join", "-s", "bad"}, "", []string{"status 3"}, exitWithStatus3},
	{"signal", []string{"join", "-s", "bad"}, "", []string{"signal 15", "signal term"},
		killWithSIGTERM},
}

// exitWithStatus3 has the claude started exit with status 3.
func exitWithStatus3(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	s.WaitProbes(1)[0].Send("exit 3")
}

// killWithSIGTERM ends the claude started with SIGTERM.
func killWithSIGTERM(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	if err := syscall.Kill(s.WaitProbes(1)[0].PID, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
}

// checkFailedClaude starts the case's claude, which fails, and checks what the terminal shows,
// list and join, and kill.
func checkFailedClaude(t *testing.T, test failedClaudeCase) {
	t.Helper()
	s := sandbox.New(t)
	extra := map[string]string{}
	if test.startup != "" {
		extra["CLD_PROBE_FAIL"] = test.startup
	}
	term := startCld(t, s, "tmux", extra, test.args...)
	if test.fail != nil {
		waitClients(t, s, 1)
		failSplitPane(t, s, term)
		test.fail(t, s)
	}
	if test.startup != "" {
		waitScreen(t, term, test.startup)
	}
	hint := ": cld kill -s bad ends the session, C-q d or cld detach -s bad detaches"
	waitScreen(t, term, hint)
	how := slices.IndexFunc(test.how, func(how string) bool {
		return strings.Contains(term.Screen(), "claude exited with "+how+hint)
	})
	if how < 0 {
		t.Fatalf("the hint does not say claude exited with %s:\n%s",
			strings.Join(test.how, " or "), term.Screen())
	}
	term.Keys("x")
	waitBorderLine(t, term, "claude exited with "+test.how[how]+hint)
	if screen := term.Screen(); !strings.HasPrefix(screen, test.startup) {
		t.Errorf("claude's words are not at the top:\n%s", screen)
	}
	if !term.Running() {
		t.Error("the terminal was detached")
	}
	checkExitedRefusals(t, s)
	if result := s.RunCld(nil, "kill", "-s", "bad"); result.Code != 0 {
		t.Errorf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after kill, want none", sessions)
	}
}

// failSplitPane has another pane of claude's window fail, one claude splits off for a teammate,
// say. It closes as tmux closes it, with no hint: what cld sets for a failed claude goes to
// claude's pane only.
func failSplitPane(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal) {
	t.Helper()
	s.MustTmux("cld-bad", "split-window", "-d", "-t", "=cld-bad:", "exit 5")
	sandbox.WaitFor(t, 10*time.Second, "the split pane to close", func() bool {
		return s.Format("cld-bad", "#{pane_dead}") == "0"
	})
	if strings.Contains(term.Screen(), "claude exited") {
		t.Errorf("a split pane that failed shows the hint:\n%s", term.Screen())
	}
}

// checkExitedRefusals checks that list shows session bad as exited, and that join refuses --new
// and --resume there.
func checkExitedRefusals(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	list := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"bad   exited    now          " + s.Work + "\n"
	result := s.RunCld(nil, "list")
	if result.Code != 0 || result.Stdout != list || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, list)
	}
	for _, option := range []string{"--new", "--resume"} {
		want := lostRefusal("bad", option, "-s bad")
		args := []string{"join", "-s", "bad", option}
		if option == "--resume" {
			args = append(args, "x")
		}
		if result := s.RunCld(nil, args...); result.Code != 1 || result.Stderr != want {
			t.Errorf("%s: exit %d, stderr %q, want exit 1, stderr %q",
				strings.Join(args, " "), result.Code, result.Stderr, want)
		}
	}
}
