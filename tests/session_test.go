package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// How cld uses its tmux server, independent of the outer terminal: cld runs in the baseline
// terminal (a pane of an outer tmux server).

func TestSessionNames(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		session string
	}{
		{[]string{"new"}, "cld-main"},
		{[]string{"new", "-n", "review"}, "cld-review"},
		{[]string{"new", "--name", "Fix_42-b"}, "cld-Fix_42-b"},
		{[]string{"new", "--name=x"}, "cld-x"},
	} {
		t.Run(test.session, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			startCld(t, s, "tmux", nil, test.args...)
			probe := s.WaitProbes(1)[0]
			if sessions := s.Sessions(); !slices.Equal(sessions, []string{test.session}) {
				t.Errorf("sessions %q, want [%s]", sessions, test.session)
			}
			if want := []string{"--name", test.session}; !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != s.Work {
				t.Errorf("claude runs in %s, want %s", probe.Cwd, s.Work)
			}
		})
	}
}

// new -w hands the worktree to claude: claude gets --worktree NAME, with settings that make it
// branch a new worktree from HEAD, and starts where cld runs; it then makes or reopens the
// worktree itself and moves into it.
func TestNewWorktree(t *testing.T) {
	t.Parallel()
	const fromHead = `{"worktree":{"baseRef":"head"}}`
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"new", "-w"}, []string{"--name", "cld-main", "--settings", fromHead, "--worktree", "main"}},
		{[]string{"new", "-n", "feat", "--worktree"}, []string{"--name", "cld-feat", "--settings", fromHead, "--worktree", "feat"}},
		{[]string{"new", "-w", "--name=feat"}, []string{"--name", "cld-feat", "--settings", fromHead, "--worktree", "feat"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			sub := filepath.Join(s.Work, "sub")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			startCldIn(t, s, "tmux", sub, nil, test.args...)
			probe := s.WaitProbes(1)[0]
			if !slices.Equal(probe.Argv, test.want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, test.want)
			}
			if probe.Cwd != sub {
				t.Errorf("claude starts in %s, want %s", probe.Cwd, sub)
			}
		})
	}
}

// Outside a git work tree new -w fails before starting anything: claude would say so in a
// session left to kill or, with tmux 3.3 and 3.4, in a pane that closes as claude exits.
func TestNewWorktreeRequiresRepository(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: --worktree needs a git repository, and " + s.Work + " is not in one\n"
	if result := s.RunCld(nil, "new", "-n", "feat", "-w"); result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// new refuses a session that exists, before touching it: the attached client and its claude
// carry on.
func TestNewRefusesExistingSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "dup")
	s.WaitProbes(1)
	waitClients(t, s, 1)

	result := s.RunCld(nil, "new", "-n", "dup")
	if want := "cld: session 'dup' exists; attach to it with cld join -n dup\n"; result.Code != 1 || result.Stderr != want {
		t.Errorf("exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}
	if result.Stdout != "" {
		t.Errorf("printed %q before failing", result.Stdout)
	}
	if probes := s.Probes(); len(probes) != 1 || !first.Running() {
		t.Errorf("%d claude processes, first client running: %v; want the original one, attached", len(probes), first.Running())
	}
}

// join finds only the session it names: no server, another session, or one whose name starts
// with NAME is no session.
func TestJoinRequiresSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: no session 'rev'; create it with cld new -n rev\n"
	if result := s.RunCld(nil, "join", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("without a server: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}

	startCld(t, s, "tmux", nil, "new", "-n", "review")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	if result := s.RunCld(nil, "join", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("beside cld-review: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}
	waitClients(t, s, 1)
}

// list shows cld's sessions: the name, whether a terminal is attached, and the directory claude
// is in now, also under a locale that is not UTF-8. Without a server there is nothing to show,
// and it shows nothing.
func TestList(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("without a server: exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}

	elsewhere, moved := filepath.Join(s.Root, "elsewhere"), filepath.Join(s.Work, "café")
	for _, dir := range []string{elsewhere, moved} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	startCld(t, s, "tmux", nil, "new", "-n", "b")
	b := s.WaitProbes(1)[0]
	detached := startCldIn(t, s, "tmux", elsewhere, nil, "new", "-n", "long_name-1")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	detached.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !detached.Running() })
	// A session on cld's server that cld did not create is not cld's to show.
	s.MustTmux("new-session", "-d", "-s", "other", "sleep", "60")
	b.Send("cd " + moved)
	sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
		return s.Format("cld-b", "#{pane_current_path}") == moved
	})

	want := "NAME         STATE     DIRECTORY\n" +
		"b            attached  " + moved + "\n" +
		"long_name-1  detached  " + elsewhere + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
	// tmux writes to a client whose locale is not UTF-8 with "_" for what it cannot print: the
	// tabs, and the "é".
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	if result := s.RunCld(notUTF8, "list"); result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("LANG=C: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, want)
	}
}

// kill ends the session it names and the claude in it; the terminal attached to it is detached
// and left clean, and the other sessions carry on. With the last session the server exits.
func TestKill(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "new", "-n", "b")
	probes := map[string]*sandbox.Probe{}
	for _, probe := range s.WaitProbes(2) {
		probes[probe.Argv[1]] = probe
	}
	waitClients(t, s, 2)

	if result := s.RunCld(nil, "kill", "-n", "a"); result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and no output", result.Code, result.Stdout, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["cld-a"].Alive() })
	sandbox.WaitFor(t, 10*time.Second, "cld a to return", func() bool { return !a.Running() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
		t.Errorf("sessions %q, want [cld-b]", sessions)
	}
	if modes := a.Modes(); modes.AltScreen || modes.Mouse {
		t.Errorf("terminal modes after the kill %+v, want none", modes)
	}
	if !probes["cld-b"].Alive() {
		t.Error("claude b did not survive session a's kill")
	}

	if result := s.RunCld(nil, "kill", "-n", "b"); result.Code != 0 {
		t.Errorf("exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "the server to exit", func() bool {
		_, err := s.Tmux("list-sessions")
		return err != nil
	})
}

// kill ends only the session it names: no server, another session, or one whose name starts
// with NAME is no session.
func TestKillRequiresSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: no session 'rev' (see cld list)\n"
	if result := s.RunCld(nil, "kill", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("without a server: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}

	startCld(t, s, "tmux", nil, "new", "-n", "review")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if result := s.RunCld(nil, "kill", "-n", "rev"); result.Code != 1 || result.Stderr != want {
		t.Errorf("beside cld-review: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-review"}) || !probe.Alive() {
		t.Errorf("sessions %q, claude alive: %v; want cld-review running", sessions, probe.Alive())
	}
}

func TestJoinDetachesOtherClient(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "shared")
	s.WaitProbes(1)
	waitClients(t, s, 1)

	second := startCld(t, s, "tmux", nil, "join", "-n", "shared")
	sandbox.WaitFor(t, 10*time.Second, "the first client to be detached", func() bool { return !first.Running() })
	waitScreen(t, second, "probe --name cld-shared")
	if !second.Running() {
		t.Error("the second client is not attached")
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claude processes, want 1", len(probes))
	}
}

// Joining from another directory leaves claude, and the session, where they are.
func TestJoinFromElsewhereKeepsClaude(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "task")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	elsewhere := filepath.Join(s.Root, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	second := startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "task")
	waitScreen(t, second, "probe --name cld-task")
	if probes := s.Probes(); len(probes) != 1 || !probe.Alive() || probe.Cwd != s.Work {
		t.Errorf("%d claude processes after joining, want the original one in %s", len(probes), s.Work)
	}
	if path := s.Format("cld-task", "#{session_path}"); path != s.Work {
		t.Errorf("session directory %s after joining, want %s", path, s.Work)
	}
}

// A reattach repaints claude's screen from tmux's own copy of it: the same text in the same
// attributes - also after claude pushed its keyboard modes again, as it does after an external
// editor, and in a terminal of another size - and claude has the alternate screen, mouse
// reporting and the wheel back.
func TestReattachRepaintsTheSameScreen(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "paint")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Send("rekey")
	waitScreen(t, first, "repainted")
	painted := cells(first.Styled())
	if underlined(painted) {
		t.Errorf("claude's screen is underlined:\n%s", strings.Join(painted, "\n"))
	}
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	for _, size := range [][2]int{{120, 40}, {100, 30}} {
		term := terminal.New(t, "tmux", s)
		term.Resize(size[0], size[1])
		term.Start(s.CldArgv("join", "-n", "paint"), s.Env, s.Work)
		waitScreen(t, term, "repainted")
		if repainted := cells(term.Styled()); !slices.Equal(repainted, painted) {
			t.Errorf("reattached at %dx%d:\n%s\nwant\n%s", size[0], size[1], strings.Join(repainted, "\n"), strings.Join(painted, "\n"))
		}
		if modes := term.Modes(); !modes.AltScreen || !modes.Mouse {
			t.Errorf("modes after reattaching at %dx%d %+v, want the alternate screen and mouse reporting on", size[0], size[1], modes)
		}
		mark := probe.Mark()
		term.WheelUp()
		probe.WaitInput(mark, "\x1b[<64;")
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
}

// claude exiting closes its own session only: the other session, its client and its claude
// carry on.
func TestClaudeExitClosesOnlyItsSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(1)
	b := startCld(t, s, "tmux", nil, "new", "-n", "b")
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
}

// From tmux 3.5 a claude that fails keeps its session: the terminal stays attached and shows
// claude's last words and how to end the session, list says claude exited, and new refuses the
// name until kill ends the session. With tmux 3.3 and 3.4 the session closes, as it does when
// claude exits with status 0 (see above).
func TestFailedClaudeKeepsSession(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		// how the hint says claude exited: tmux names a signal where the C library has
		// sys_signame (macOS), and numbers it elsewhere
		how  []string
		fail func(t *testing.T, s *sandbox.Sandbox)
	}{
		{"start", []string{"status 1"}, nil},
		{"status", []string{"status 3"}, func(t *testing.T, s *sandbox.Sandbox) {
			s.WaitProbes(1)[0].Send("exit 3")
		}},
		{"signal", []string{"signal 15", "signal term"}, func(t *testing.T, s *sandbox.Sandbox) {
			if err := syscall.Kill(s.WaitProbes(1)[0].PID, syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			extra := map[string]string{}
			if test.fail == nil {
				extra["CLD_PROBE_FAIL"] = "Error: cannot start"
			}
			term := startCld(t, s, "tmux", extra, "new", "-n", "bad")
			if test.fail != nil {
				waitClients(t, s, 1)
				test.fail(t, s)
			}
			if !keepsFailedSessions(t) {
				sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
				if sessions := s.Sessions(); len(sessions) != 0 {
					t.Errorf("sessions %q, want none", sessions)
				}
				return
			}
			if test.fail == nil {
				waitScreen(t, term, "Error: cannot start")
			}
			hint := ": C-q d detaches, cld kill -n bad ends the session"
			waitScreen(t, term, hint)
			if !slices.ContainsFunc(test.how, func(how string) bool {
				return strings.Contains(term.Screen(), "claude exited with "+how+hint)
			}) {
				t.Errorf("the hint does not say claude exited with %s:\n%s", strings.Join(test.how, " or "), term.Screen())
			}
			if !term.Running() {
				t.Error("the terminal was detached")
			}

			list := "NAME  STATE     DIRECTORY\n" +
				"bad   exited    " + s.Work + "\n"
			if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != list || result.Stderr != "" {
				t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s", result.Code, result.Stderr, result.Stdout, list)
			}
			want := "cld: session 'bad' exists, but its claude exited; end it with cld kill -n bad\n"
			if result := s.RunCld(nil, "new", "-n", "bad"); result.Code != 1 || result.Stderr != want {
				t.Errorf("new: exit %d, stderr %q, want exit 1, stderr %q", result.Code, result.Stderr, want)
			}
			if result := s.RunCld(nil, "kill", "-n", "bad"); result.Code != 0 {
				t.Errorf("kill: exit %d, stderr %q", result.Code, result.Stderr)
			}
			sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q after kill, want none", sessions)
			}
		})
	}
}

// From tmux 3.5, a claude that fails with no terminal attached leaves the hint to join, which
// shows it on the message line: from the hook, tmux would keep it and show it in view-mode over
// the next session any terminal attaches to. Joining a live session shows no hint. With tmux 3.3
// and 3.4 the session closes.
func TestClaudeFailingDetached(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "new", "-n", "bad")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })
	probe.Send("exit 1")
	if !keepsFailedSessions(t) {
		sandbox.WaitFor(t, 10*time.Second, "the session to close", func() bool { return len(s.Sessions()) == 0 })
		return
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return s.Format("cld-bad", "#{pane_dead}") == "1" })

	other := startCld(t, s, "tmux", nil, "new", "-n", "other")
	s.WaitProbes(2)
	waitClients(t, s, 1)
	if mode := s.Format("cld-other", "#{pane_mode}"); mode != "" {
		t.Errorf("a new session opens in %s:\n%s", mode, other.Screen())
	}

	hint := "claude exited with status 1: C-q d detaches, cld kill -n bad ends the session"
	joined := startCld(t, s, "tmux", nil, "join", "-n", "bad")
	waitScreen(t, joined, hint)
	if screen := joined.Screen(); !strings.HasSuffix(strings.TrimRight(screen, " \n"), "\n"+hint) {
		t.Errorf("the hint is not on the message line:\n%s", screen)
	}
	if mode := s.Format("cld-bad", "#{pane_mode}"); mode != "" {
		t.Errorf("the joined session is in %s", mode)
	}

	live := startCld(t, s, "tmux", nil, "join", "-n", "other")
	waitScreen(t, live, "probe --name cld-other")
	if screen := live.Screen(); strings.Contains(screen, "claude exited") {
		t.Errorf("joining a live session shows the hint:\n%s", screen)
	}
}

func TestSessionsShareOneServer(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new", "-n", "a")
	startCld(t, s, "tmux", nil, "new", "-n", "b")
	s.WaitProbes(2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

func TestIgnoresUserTmuxConfig(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new")
	s.WaitProbes(1)
	if left := s.MustTmux("show", "-gv", "status-left"); left == "POISONED" {
		t.Error("~/.tmux.conf was loaded")
	}
	socket := filepath.Join(s.Root, fmt.Sprintf("tmux-%d", os.Getuid()), "default")
	if _, err := os.Stat(socket); err == nil {
		t.Error("cld started the default tmux server")
	}
}

func TestServerOptions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new")
	s.WaitProbes(1)
	remain := "off"
	if keepsFailedSessions(t) {
		remain = "failed"
	}
	for _, option := range []struct{ scope, name, value string }{
		{"-sv", "extended-keys", "on"},
		{"-sv", "focus-events", "on"},
		{"-gv", "mouse", "on"},
		{"-gv", "allow-passthrough", "on"},
		{"-gv", "status", "off"},
		{"-gv", "prefix", "C-q"},
		{"-gwv", "remain-on-exit", remain},
		{"-gwv", "remain-on-exit-format", ""},
	} {
		if value := s.MustTmux("show", option.scope, option.name); value != option.value {
			t.Errorf("%s is %q, want %q", option.name, value, option.value)
		}
	}
	if features := s.MustTmux("show", "-sv", "terminal-features"); !slices.Contains(strings.Split(features, "\n"), "xterm*:extkeys") {
		t.Errorf("terminal-features lacks xterm*:extkeys:\n%s", features)
	}
	// "list-keys -T prefix C-q" would be shorter, but tmux 3.7 prints nothing for it.
	bindings := strings.Split(s.MustTmux("list-keys", "-T", "prefix"), "\n")
	if !slices.ContainsFunc(bindings, func(binding string) bool {
		return slices.Equal(strings.Fields(binding), []string{"bind-key", "-T", "prefix", "C-q", "send-prefix"})
	}) {
		t.Errorf("C-q C-q is not bound to send-prefix:\n%s", strings.Join(bindings, "\n"))
	}
}

// cld sets its options every time it creates a session; none of them may pile up on a
// long-lived server.
func TestRepeatedRunsDoNotStackOptions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for i, name := range []string{"a", "b", "c"} {
		startCld(t, s, "tmux", nil, "new", "-n", name)
		s.WaitProbes(i + 1)
	}
	waitClients(t, s, 3)
	features := strings.Split(s.MustTmux("show", "-sv", "terminal-features"), "\n")
	if count := len(slices.DeleteFunc(features, func(f string) bool { return f != "xterm*:extkeys" })); count != 1 {
		t.Errorf("%d xterm*:extkeys entries after three sessions, want 1", count)
	}
	if hooks := strings.Split(s.MustTmux("show-hooks", "-g", "pane-died"), "\n"); len(hooks) != 1 {
		t.Errorf("%d pane-died hooks after three sessions, want 1:\n%s", len(hooks), strings.Join(hooks, "\n"))
	}
}

// claude trusts TERMINAL_EMULATOR over TERM_PROGRAM=tmux, and the server keeps the environment of
// the client that started it: without cld's env -u, a server started from a JetBrains terminal
// would hand TERMINAL_EMULATOR to every later session, attached from anywhere.
func TestClaudeNeverSeesTerminalEmulator(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", map[string]string{"TERMINAL_EMULATOR": "JetBrains-JediTerm"}, "new", "-n", "ide")
	s.WaitProbes(1)
	startCld(t, s, "tmux", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "new", "-n", "iterm")
	for _, probe := range s.WaitProbes(2) {
		if value, found := probe.Env["TERMINAL_EMULATOR"]; found {
			t.Errorf("claude %q sees TERMINAL_EMULATOR=%s", probe.Argv, value)
		}
	}
}

func TestNestsInsideAnotherTmux(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	elsewhere := filepath.Join(s.Root, "elsewhere", "default") + ",1,0"
	startCld(t, s, "tmux", map[string]string{"TMUX": elsewhere}, "new")
	s.WaitProbes(1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-main"}) {
		t.Errorf("sessions %q, want [cld-main]", sessions)
	}
}

// In a live pane of cld's server - claude's external editor, say - a session attached would show
// inside itself: new and join refuse, saying how to get out, and the terminal attached before
// stays.
func TestRefusesToNestInItsOwnPane(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "new", "-n", "a")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	want := "cld: this terminal is inside a cld session; detach with C-q d first\n"
	for _, args := range [][]string{{"join", "-n", "a"}, {"new", "-n", "b"}} {
		// The pane runs cld on its own pty, with the TMUX tmux sets for it.
		out := filepath.Join(s.Root, args[0])
		s.MustTmux(append([]string{"new-session", "-d", "-s", "in-" + args[0],
			"sh", "-c", `"$@" 2>"$0.err"; echo $? >"$0.code"`, out}, s.CldArgv(args...)...)...)
		var code []byte
		sandbox.WaitFor(t, 10*time.Second, "cld "+args[0]+" to return", func() bool {
			code, _ = os.ReadFile(out + ".code")
			return strings.HasSuffix(string(code), "\n")
		})
		if stderr, _ := os.ReadFile(out + ".err"); string(code) != "1\n" || string(stderr) != want {
			t.Errorf("cld %s: exit %s, stderr %q, want exit 1, stderr %q", args[0], strings.TrimSpace(string(code)), stderr, want)
		}
	}
	if sessions := s.Sessions(); slices.Contains(sessions, "cld-b") {
		t.Errorf("sessions %q, want no cld-b", sessions)
	}
	if clients := s.MustTmux("list-clients", "-F", "#{session_name}"); clients != "cld-a" {
		t.Errorf("clients attached to %q, want the first one, to cld-a", clients)
	}
}

// keepsFailedSessions reports whether cld keeps a failed claude's session with the tmux under
// test: from 3.5 on, and in development builds (see bin/cld).
func keepsFailedSessions(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		t.Fatal(err)
	}
	version := regexp.MustCompile(`(\d+)\.(\d+)`).FindStringSubmatch(string(out))
	if version == nil {
		return true
	}
	major, _ := strconv.Atoi(version[1])
	minor, _ := strconv.Atoi(version[2])
	return major > 3 || major == 3 && minor >= 5
}

// gitInit makes dir a git repository.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func waitClients(t *testing.T, s *sandbox.Sandbox, count int) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%d attached client(s)", count), func() bool {
		clients, err := s.Tmux("list-clients", "-F", "#{client_name}")
		return err == nil && len(strings.Fields(clients)) == count
	})
}

func waitScreen(t *testing.T, term terminal.Terminal, text string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%q on the screen", text), func() bool {
		return strings.Contains(term.Screen(), text)
	})
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// attribute names the attribute each SGR code sets or clears.
var attribute = map[string]string{
	"1": "intensity", "2": "intensity", "22": "intensity",
	"3": "italic", "23": "italic",
	"4": "underline", "21": "underline", "24": "underline",
	"5": "blink", "6": "blink", "25": "blink",
	"7": "inverse", "27": "inverse",
	"8": "hidden", "28": "hidden",
	"9": "strike", "29": "strike",
	"53": "overline", "55": "overline",
	"38": "fg", "39": "fg", "48": "bg", "49": "bg", "58": "underline-colour", "59": "underline-colour",
}

// clearing lists the SGR codes that turn their attribute off.
var clearing = map[string]bool{
	"22": true, "23": true, "24": true, "4:0": true, "25": true, "27": true, "28": true,
	"29": true, "55": true, "39": true, "49": true, "59": true,
}

// cells turns a Styled screen into lines of runs, "[attribute=code ...]text", whatever the
// SGR sequences that happened to draw them: equal cells give equal lines. Trailing blank cells
// and lines are dropped, so that screens of different sizes compare by what is drawn on them.
func cells(styled string) []string {
	state := map[string]string{} // capture-pane -e carries attributes over line ends
	var lines []string
	for _, row := range strings.Split(styled, "\n") {
		var attrs, texts []string // one per run
		matches := sgr.FindAllStringSubmatch(row, -1)
		for i, text := range sgr.Split(row, -1) {
			if i > 0 {
				apply(state, matches[i-1][1])
			}
			if text == "" {
				continue
			}
			if n := len(attrs); n > 0 && attrs[n-1] == describe(state) {
				texts[n-1] += text
			} else {
				attrs, texts = append(attrs, describe(state)), append(texts, text)
			}
		}
		if n := len(attrs); n > 0 && attrs[n-1] == "" {
			if texts[n-1] = strings.TrimRight(texts[n-1], " "); texts[n-1] == "" {
				attrs, texts = attrs[:n-1], texts[:n-1]
			}
		}
		var line strings.Builder
		for i := range attrs {
			line.WriteString("[" + attrs[i] + "]" + texts[i])
		}
		lines = append(lines, line.String())
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// apply updates the attributes of the cells after an SGR sequence with parameters params.
func apply(state map[string]string, params string) {
	codes := strings.Split(params, ";")
	for i := 0; i < len(codes); i++ {
		code := codes[i]
		if code == "" || code == "0" {
			clear(state)
			continue
		}
		base, _, _ := strings.Cut(code, ":")
		// A colour in the ; form takes the codes after it: 5;N or 2;R;G;B.
		if (base == "38" || base == "48" || base == "58") && base == code {
			n := 4
			if i+1 < len(codes) && codes[i+1] == "5" {
				n = 2
			}
			code = strings.Join(codes[i:min(i+n+1, len(codes))], ";")
			i += n
		}
		name, known := attribute[base]
		switch {
		case !known:
		case clearing[code]:
			delete(state, name)
		default:
			state[name] = code
		}
	}
}

func describe(state map[string]string) string {
	var pairs []string
	for name, code := range state {
		pairs = append(pairs, name+"="+code)
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// underlined reports whether any cell of a screen from cells is underlined.
func underlined(lines []string) bool {
	return slices.ContainsFunc(lines, func(line string) bool {
		return strings.Contains(line, "underline=")
	})
}
