package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		{nil, "cld-main"},
		{[]string{"review"}, "cld-review"},
		{[]string{"Fix_42-b"}, "cld-Fix_42-b"},
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

func TestReattachDetachesOtherClient(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "shared")
	s.WaitProbes(1)
	waitClients(t, s, 1)

	second := startCld(t, s, "tmux", nil, "shared")
	sandbox.WaitFor(t, 10*time.Second, "the first client to be detached", func() bool { return !first.Running() })
	waitScreen(t, second, "probe --name cld-shared")
	if !second.Running() {
		t.Error("the second client is not attached")
	}
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claude processes, want 1", len(probes))
	}
}

// Reattaching from another directory leaves claude where it runs. tmux 3.7 moves the session's
// directory for new windows to the reattaching client's (new-session -A now honours -c); a cld
// session is one window, so only claude's directory is part of the promise.
func TestReattachFromElsewhereKeepsClaude(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "task")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	elsewhere := filepath.Join(s.Root, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	second := startCldIn(t, s, "tmux", elsewhere, nil, "task")
	waitScreen(t, second, "probe --name cld-task")
	if probes := s.Probes(); len(probes) != 1 || !probe.Alive() || probe.Cwd != s.Work {
		t.Errorf("%d claude processes after reattaching, want the original one in %s", len(probes), s.Work)
	}
}

func TestSessionsShareOneServer(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "a")
	startCld(t, s, "tmux", nil, "b")
	s.WaitProbes(2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

func TestIgnoresUserTmuxConfig(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil)
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
	startCld(t, s, "tmux", nil)
	s.WaitProbes(1)
	for _, option := range []struct{ scope, name, value string }{
		{"-sv", "extended-keys", "on"},
		{"-sv", "focus-events", "on"},
		{"-gv", "mouse", "on"},
		{"-gv", "allow-passthrough", "on"},
		{"-gv", "status", "off"},
		{"-gv", "prefix", "C-q"},
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

// cld sets its options on every run; none of them may pile up on a long-lived server.
func TestRepeatedRunsDoNotStackOptions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "b")
	startCld(t, s, "tmux", nil, "a")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	features := strings.Split(s.MustTmux("show", "-sv", "terminal-features"), "\n")
	if count := len(slices.DeleteFunc(features, func(f string) bool { return f != "xterm*:extkeys" })); count != 1 {
		t.Errorf("%d xterm*:extkeys entries after three runs, want 1", count)
	}
}

// claude trusts TERMINAL_EMULATOR over TERM_PROGRAM=tmux, and the server keeps the environment of
// the client that started it: without cld's env -u, a server started from a JetBrains terminal
// would hand TERMINAL_EMULATOR to every later session, attached from anywhere.
func TestClaudeNeverSeesTerminalEmulator(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", map[string]string{"TERMINAL_EMULATOR": "JetBrains-JediTerm"}, "ide")
	s.WaitProbes(1)
	startCld(t, s, "tmux", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "iterm")
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
	startCld(t, s, "tmux", map[string]string{"TMUX": elsewhere})
	s.WaitProbes(1)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-main"}) {
		t.Errorf("sessions %q, want [cld-main]", sessions)
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
