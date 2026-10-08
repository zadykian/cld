package tests

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// Running cld on a terminal of the tests' and waiting for what it shows, shared by the test files.

// terminals are the terminals the terminal contract runs against, from CLD_TERMINALS.
var terminals = strings.Split(envOr("CLD_TERMINALS", "tmux"), ",")

// envOr is the value of the environment variable name, or fallback where that is empty.
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// forEachTerminal runs body as a subtest per terminal in CLD_TERMINALS.
func forEachTerminal(t *testing.T, body func(t *testing.T, name string)) {
	t.Helper()
	for _, name := range terminals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body(t, name)
		})
	}
}

// startCld runs cld with args in a new terminal of the given kind; extra variables are added to
// the sandbox environment.
func startCld(
	t *testing.T, s *sandbox.Sandbox, name string, extra map[string]string, args ...string,
) terminal.Terminal {
	t.Helper()
	return startCldIn(t, s, name, s.Work, extra, args...)
}

// startCldIn is startCld in dir, in place of the sandbox's work directory.
func startCldIn(
	t *testing.T, s *sandbox.Sandbox, name, dir string, extra map[string]string, args ...string,
) terminal.Terminal {
	t.Helper()
	term := terminal.New(t, name, s)
	env := map[string]string{}
	maps.Copy(env, s.Env)
	maps.Copy(env, extra)
	term.Start(s.CldArgv(args...), env, dir)
	return term
}

// An expectation is what the terminal contract expects of a terminal where terminals differ.
type expectation struct {
	// features tmux must detect in the terminal (#{client_termfeatures}).
	features []string
	// shiftEnter lists what claude may receive for Shift+Enter; plain Enter is always "\r".
	shiftEnter []string
	// pasted is what claude receives for TestContractPaste's paste.
	pasted string
	// termtype starts what the terminal answers XTVERSION with (#{client_termtype}): empty where
	// it answers nothing.
	termtype string
}

// pasted is TestContractPaste's paste as an xterm sends it: bracketed, a line feed as a carriage
// return, and the prefix key unchanged.
const pasted = "\x1b[200~first line\rsecond line \x11d\x1b[201~"

// expectations are each terminal's, by its name in CLD_TERMINALS.
var expectations = map[string]expectation{
	// The outer tmux announces itself through XTVERSION, and the inner tmux knows its features,
	// hyperlinks among them; extkeys comes from cld's terminal-features entry for xterm*.
	"tmux": {
		features:   []string{"clipboard", "extkeys", "focus", "hyperlinks", "mouse", "title"},
		shiftEnter: []string{"\x1b[13;2u", "\x1b[27;2;13~"},
		pasted:     pasted,
		termtype:   "tmux ",
	},
	// JediTerm answers no XTVERSION, so tmux claims clipboard and focus, its defaults for xterm*,
	// which the emulator ignores (see the skipped tests); cld adds extkeys and hyperlinks. JediTerm
	// ignores the modifyOtherKeys request that follows; its own setting makes Shift+Enter ESC CR,
	// which tmux passes on as Meta+Enter.
	"jediterm": {
		features:   []string{"bpaste", "clipboard", "extkeys", "focus", "hyperlinks", "title"},
		shiftEnter: []string{"\x1b\r"},
		pasted:     pasted,
	},
	// Ghostty answers XTVERSION, which tmux does not know, so its features come from Ghostty's
	// terminfo entry, tmux's defaults for xterm* and cld's entry. RGB comes from COLORTERM in tmux
	// 3.7c, not 3.5a. Ghostty sends Shift+Enter as modifyOtherKeys does, even unasked. A bracketed
	// paste keeps its line feeds, and a control character becomes a space.
	"ghostty": {
		features: []string{"bpaste", "ccolour", "clipboard", "cstyle", "extkeys", "focus",
			"hyperlinks", "title"},
		shiftEnter: []string{"\x1b[27;2;13~"},
		pasted:     "\x1b[200~first line\nsecond line  d\x1b[201~",
		termtype:   "ghostty ",
	},
}

// between types keys between two markers and returns what claude received between them.
func between(t *testing.T, term terminal.Terminal, probe *sandbox.Probe, keys ...string) string {
	t.Helper()
	markers := func() int { return bytes.Count(probe.Input(), []byte(">")) }
	before := markers()
	term.Keys(append(append([]string{"<"}, keys...), ">")...)
	sandbox.WaitFor(t, 10*time.Second, "the keys to reach claude",
		func() bool { return markers() > before })
	input := probe.Input()
	end := bytes.LastIndexByte(input, '>')
	start := bytes.LastIndexByte(input[:end], '<')
	return string(input[start+1 : end])
}

// waitClients waits until count clients are attached to cld's servers, all told.
func waitClients(t *testing.T, s *sandbox.Sandbox, count int) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%d attached client(s)", count), func() bool {
		return len(s.Clients()) == count
	})
}

// waitScreen waits until the terminal's screen shows text.
func waitScreen(t *testing.T, term terminal.Terminal, text string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%q on the screen", text), func() bool {
		return strings.Contains(term.Screen(), text)
	})
}

// waitModes waits until the terminal's modes are as ok wants them, and reports them otherwise.
// They can come after the screen: tmux turns every mouse mode off and on again once it has drawn.
func waitModes(
	t *testing.T, term terminal.Terminal, when, want string, ok func(terminal.Modes) bool,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for modes := term.Modes(); !ok(modes); modes = term.Modes() {
		if time.Now().After(deadline) {
			t.Errorf("modes %s %+v, want %s", when, modes, want)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitLines waits until the screen shows exactly lines, from the top, and nothing below them;
// spaces at the end of a line do not count.
func waitLines(t *testing.T, term terminal.Terminal, lines ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := strings.Split(term.Screen(), "\n")
		for i := range got {
			got[i] = strings.TrimRight(got[i], " ")
		}
		for len(got) > 0 && got[len(got)-1] == "" {
			got = got[:len(got)-1]
		}
		if slices.Equal(got, lines) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after 10s waiting for the screen to show\n%s\nit shows\n%s",
				strings.Join(lines, "\n"), strings.Join(got, "\n"))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// detachedSessions creates cld's sessions of the given names, each from a terminal of its own
// that then detaches with C-q d, and returns their claudes by name.
func detachedSessions(
	t *testing.T, s *sandbox.Sandbox, names ...string,
) map[string]*sandbox.Probe {
	t.Helper()
	for _, name := range names {
		term := startCld(t, s, "tmux", nil, "join", "-s", name)
		sandbox.WaitFor(t, 10*time.Second, "a terminal attached to cld-"+name, func() bool {
			return slices.Contains(s.Clients(), "cld-"+name)
		})
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	}
	probes := map[string]*sandbox.Probe{}
	sandbox.WaitFor(t, 10*time.Second, "the claudes to start", func() bool {
		for _, probe := range s.Probes() {
			probes[strings.TrimPrefix(probe.Argv[1], "cld-")] = probe
		}
		return !slices.ContainsFunc(names, func(name string) bool { return probes[name] == nil })
	})
	return probes
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
