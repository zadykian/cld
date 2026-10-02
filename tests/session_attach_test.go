package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// join attaches beside the terminals on the session, which stay on the same claude (decision 23).
// C-q d detaches its own terminal alone, and join --detach-others every other.
func TestJoinBesideOtherClients(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "shared")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)

	second := startCld(t, s, "tmux", nil, "join", "-s", "shared")
	waitClients(t, s, 2)
	waitScreen(t, second, "probe --name cld-shared")
	if !first.Running() {
		t.Error("the first client was detached")
	}
	mark := probe.Mark()
	first.Keys("x")
	probe.WaitInput(mark, "x")
	third := startCld(t, s, "tmux", nil, "join", "-s", "shared")
	waitClients(t, s, 3)
	waitScreen(t, third, "probe --name cld-shared")
	second.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "the second client to be detached", func() bool {
		return !second.Running()
	})
	clients := s.Clients()
	if !slices.Equal(clients, []string{"cld-shared", "cld-shared"}) || !first.Running() ||
		!third.Running() {
		t.Errorf("clients attached to %q after C-q d, want the first and third, to cld-shared",
			clients)
	}

	fourth := startCld(t, s, "tmux", nil, "join", "-s", "shared", "--detach-others")
	sandbox.WaitFor(t, 10*time.Second, "the other clients to be detached", func() bool {
		return !first.Running() && !third.Running()
	})
	waitScreen(t, fourth, "probe --name cld-shared")
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-shared"}) || !fourth.Running() {
		t.Errorf("clients attached to %q, want one, to cld-shared", clients)
	}
	if probes := s.Probes(); len(probes) != 1 || !probe.Alive() {
		t.Errorf("%d claude processes, want the original one", len(probes))
	}
}

// Joining from another directory leaves claude, and the session, where they are.
func TestJoinFromElsewhereKeepsClaude(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-n", "app", "-s", "task")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })

	elsewhere := filepath.Join(s.Root, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	second := startCldIn(t, s, "tmux", elsewhere, nil, "join", "-n", "app", "-s", "task")
	waitScreen(t, second, "probe --name cld-app-task")
	if probes := s.Probes(); len(probes) != 1 || !probe.Alive() || probe.Cwd != s.Work {
		t.Errorf("%d claude processes after joining, want the original one in %s",
			len(probes), s.Work)
	}
	if path := s.Format("cld-app-task", "#{session_path}"); path != s.Work {
		t.Errorf("session directory %s after joining, want %s", path, s.Work)
	}
}

// A reattach repaints claude's screen from tmux's own copy of it, the same text in the same
// attributes, in a terminal of another size too. It does so after claude pushed its keyboard modes
// again, as after an external editor. claude has the alternate screen, mouse and wheel back.
func TestReattachRepaintsTheSameScreen(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "paint")
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
		term.Start(s.CldArgv("join", "-s", "paint"), s.Env, s.Work)
		waitScreen(t, term, "repainted")
		if repainted := cells(term.Styled()); !slices.Equal(repainted, painted) {
			t.Errorf("reattached at %dx%d:\n%s\nwant\n%s", size[0], size[1],
				strings.Join(repainted, "\n"), strings.Join(painted, "\n"))
		}
		if modes := term.Modes(); !modes.AltScreen || !modes.Mouse {
			t.Errorf("modes after reattaching at %dx%d %+v, "+
				"want the alternate screen and mouse reporting on", size[0], size[1], modes)
		}
		mark := probe.Mark()
		term.WheelUp()
		probe.WaitInput(mark, "\x1b[<64;")
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool {
			return !term.Running()
		})
	}
}

// tmux draws what is not ASCII as "_" on a terminal it does not take for UTF-8, as over ssh to a
// host whose sshd takes no LANG. The clients of join, creating, resuming or attaching, and of the
// list's Enter take it for UTF-8 whatever the locale (docs/design/findings/tmux-terminal.md).
// They show what claude draws unchanged: here its arguments, where --resume puts SESSION.
func TestClientsTakeUTF8(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	const conversation = "✳ ⏺ café │"
	// attached waits for the terminal to attach to session cld-NAME, and checks that tmux takes it
	// for UTF-8 and, where claude draws some, shows what is not ASCII.
	attached := func(how string, term terminal.Terminal, name string, shows bool) {
		t.Helper()
		waitClients(t, s, 1)
		if flag := s.MustTmux("cld-"+name, "list-clients", "-F", "#{client_utf8}"); flag != "1" {
			t.Errorf("%s: client_utf8 %q under LANG=C, want 1", how, flag)
		}
		if shows {
			waitScreen(t, term, "--resume "+conversation)
		}
	}
	detach := func(term terminal.Terminal) {
		t.Helper()
		term.Keys("C-q", "d")
		waitClients(t, s, 0)
	}

	term := startCld(t, s, "tmux", notUTF8, "join", "-s", "n")
	attached("join creating", term, "n", false)
	detach(term)
	term = startCld(t, s, "tmux", notUTF8, "join", "-s", "r", "--resume", conversation)
	attached("join --resume", term, "r", true)
	detach(term)
	term = startCld(t, s, "tmux", notUTF8, "join", "-s", "r")
	attached("join", term, "r", true)
	detach(term)
	term = terminal.New(t, "tmux", s)
	startList(t, s, term, listScript, notUTF8)
	waitScreen(t, term, listHints)
	term.Keys("Down", "Enter")
	attached("the list's Enter", term, "r", true)
}
