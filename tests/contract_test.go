package tests

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// The terminal contract: what cld promises wherever it runs, checked against every terminal in
// CLD_TERMINALS. Where terminals legitimately differ, the difference is an expectation below
// rather than a skip, so a terminal gaining or losing support fails a test.

type expectation struct {
	// features tmux must detect in the terminal (#{client_termfeatures}).
	features []string
	// shiftEnter lists what claude may receive for Shift+Enter; plain Enter is always "\r".
	shiftEnter []string
}

var expectations = map[string]expectation{
	// The outer tmux announces itself through XTVERSION, and the inner tmux knows its features;
	// extkeys comes from cld's terminal-features entry for xterm*.
	"tmux": {
		features:   []string{"clipboard", "extkeys", "focus", "mouse", "title"},
		shiftEnter: []string{"\x1b[13;2u", "\x1b[27;2;13~"},
	},
	// JediTerm answers no XTVERSION, so tmux falls back to its defaults for xterm*: they claim
	// clipboard and focus, which the emulator ignores (see the skipped tests), and cld adds
	// extkeys. The emulator ignores the modifyOtherKeys request that follows; Shift+Enter becomes
	// ESC CR through its own setting, which tmux passes on as Meta+Enter.
	"jediterm": {
		features:   []string{"bpaste", "clipboard", "extkeys", "focus", "title"},
		shiftEnter: []string{"\x1b\r"},
	},
}

// startContract starts cld in the named terminal and waits for claude and the attached client.
func startContract(t *testing.T, name string) (*sandbox.Sandbox, terminal.Terminal, *sandbox.Probe) {
	t.Helper()
	s := sandbox.New(t)
	term := startCld(t, s, name, nil, "contract")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	waitScreen(t, term, "probe --name cld-contract")
	return s, term, probe
}

// between types keys between two markers and returns what claude received between them.
func between(t *testing.T, term terminal.Terminal, probe *sandbox.Probe, keys ...string) string {
	t.Helper()
	markers := func() int { return bytes.Count(probe.Input(), []byte(">")) }
	before := markers()
	term.Keys(append(append([]string{"<"}, keys...), ">")...)
	sandbox.WaitFor(t, 10*time.Second, "the keys to reach claude", func() bool { return markers() > before })
	input := probe.Input()
	end := bytes.LastIndexByte(input, '>')
	start := bytes.LastIndexByte(input[:end], '<')
	return string(input[start+1 : end])
}

// C1: the tab shows the session name, whatever claude sets as its own title.
func TestContractTitle(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, term, probe := startContract(t, name)
		probe.Send("title claude's own title")
		sandbox.WaitFor(t, 10*time.Second, "claude's title in its pane", func() bool {
			return s.Format("cld-contract", "#{pane_title}") == "claude's own title"
		})
		if title := term.Title(); title != "\u2733 cld-contract" {
			t.Errorf("terminal title %q, want %q", title, "\u2733 cld-contract")
		}
	})
}

// C2: what tmux learned about the terminal decides what it forwards to claude.
func TestContractClientFeatures(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, _, _ := startContract(t, name)
		client := s.MustTmux("list-clients", "-F", "#{client_termname}|#{client_termtype}|#{client_termfeatures}")
		t.Logf("client: %s", client)
		detected := strings.Split(strings.SplitN(client, "|", 3)[2], ",")
		for _, feature := range expectations[name].features {
			if !slices.Contains(detected, feature) {
				t.Errorf("tmux does not detect %q in %s", feature, name)
			}
		}
	})
}

// C3: Shift+Enter reaches claude distinct from Enter.
func TestContractShiftEnter(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		if enter := between(t, term, probe, "Enter"); enter != "\r" {
			t.Errorf("Enter arrives as %s, want \"\\r\"", strconv.Quote(enter))
		}
		shiftEnter := between(t, term, probe, "S-Enter")
		if !slices.Contains(expectations[name].shiftEnter, shiftEnter) {
			t.Errorf("Shift+Enter arrives as %s, want one of %q", strconv.Quote(shiftEnter), expectations[name].shiftEnter)
		}
	})
}

// C3: Ctrl keys claude binds pass through; the prefix pressed twice sends itself.
func TestContractControlKeys(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		if got := between(t, term, probe, "C-b", "C-q", "C-q"); got != "\x02\x11" {
			t.Errorf("C-b C-q C-q arrives as %s, want \"\\x02\\x11\"", strconv.Quote(got))
		}
	})
}

// C3, C7: the prefix and d detach, claude keeps running, and the terminal is left clean.
func TestContractDetach(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, term, probe := startContract(t, name)
		if modes := term.Modes(); !modes.AltScreen || !modes.Mouse {
			t.Fatalf("modes while attached %+v, want the alternate screen and mouse reporting on", modes)
		}
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to exit", func() bool { return !term.Running() })
		if modes := term.Modes(); modes.AltScreen || modes.Mouse {
			t.Errorf("modes after detaching %+v, want everything off", modes)
		}
		if !probe.Alive() || !slices.Equal(s.Sessions(), []string{"cld-contract"}) {
			t.Error("claude did not survive the detach")
		}
	})
}

// C4: the mouse wheel reaches claude, which scrolls its transcript with it.
func TestContractMouseWheel(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.WheelUp()
		probe.WaitInput(mark, "\x1b[<64;")
	})
}

// C4: focus changes reach claude.
func TestContractFocus(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.Focus(false)
		probe.WaitInput(mark, "\x1b[O")
		mark = probe.Mark()
		term.Focus(true)
		probe.WaitInput(mark, "\x1b[I")
	})
}

// C5: a copy claude makes through tmux passthrough lands in the terminal's clipboard.
func TestContractClipboard(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		probe.Send("osc52 copied by claude")
		if clipboard := term.Clipboard(); clipboard != "copied by claude" {
			t.Errorf("clipboard %q, want %q", clipboard, "copied by claude")
		}
	})
}
