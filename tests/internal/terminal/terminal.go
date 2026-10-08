// Package terminal drives the outer terminals cld runs in, behind one interface.
//
// Keys are named the way tmux names them: "Enter", "S-Enter", "Up", "Down", "Escape", "C-q",
// "C-b", or one character. The baseline terminal also types "S-Up", "M-j" and "M-Escape" as xterm
// sends them, and "M-Up" as rxvt does (see xtermInput).
// Methods fail the test on errors; a terminal that cannot do something skips the test instead,
// giving the reason.
package terminal

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Modes is terminal state a program may leave behind when it exits.
type Modes struct {
	AltScreen bool `json:"alt_screen"`
	Mouse     bool `json:"mouse"`  // any mouse reporting
	Cursor    bool `json:"cursor"` // the cursor is visible
}

// Terminal is an outer terminal: a pty and whatever emulates the screen and keyboard around it.
type Terminal interface {
	Name() string
	// Start runs argv on a fresh pty of this terminal, with env as its whole environment.
	Start(argv []string, env map[string]string, dir string)
	// Keys types the keys one after another, as a user would.
	Keys(keys ...string)
	// Hold types key as a key held down: once, again after delay, and then repeats times more, one
	// every interval, as the terminal repeats it. The terminal times the keys itself, so that the
	// test, held up, does not space them out; Hold returns once it has typed them all.
	Hold(key string, delay, interval time.Duration, repeats int)
	// Paste pastes text the way the terminal's own paste command does.
	Paste(text string)
	WheelUp()
	// Click presses a mouse button over the screen and lets it go. key names the press the way
	// tmux does, with the modifiers held: "C-MouseDown1" is a Ctrl+click, "M-MouseDown3" an
	// Alt+right-click.
	Click(key string)
	Focus(focused bool)
	// Resize sets the size in cells; called before Start, the size the terminal starts with.
	Resize(columns, rows int)
	// Freeze stops the terminal, as a busy one, until thaw is called: it neither reads what the
	// program writes nor answers it until then.
	Freeze() (thaw func())
	// Title is the window or tab title the terminal shows.
	Title() string
	// Screen is the visible text.
	Screen() string
	// Styled is the visible text with SGR sequences for its attributes, as tmux capture-pane -e
	// writes it: equal screens give equal strings.
	Styled() string
	// Output is everything the program has written to the terminal so far.
	Output() []byte
	Modes() Modes
	// Clipboard is the text last copied to the clipboard through OSC 52.
	Clipboard() string
	// Running reports whether the program Start ran is still running, or the terminal has yet to
	// take in what it wrote. Once it reports false, the screen and the modes are final.
	Running() bool
	Close()
}

// newGhostty creates the ghostty terminal, set in a build with the tag ghostty alone (see
// ghostty.go).
var newGhostty func(testing.TB, *sandbox.Sandbox) Terminal

// ghosttyDir is CLD_GHOSTTY_DIR, where tests/ghostty/build-lib installs what the ghostty driver
// reads: Ghostty's version and terminfo entry.
var ghosttyDir = sync.OnceValues(func() (string, error) {
	if dir := os.Getenv("CLD_GHOSTTY_DIR"); dir != "" {
		return dir, nil
	}
	return "", errors.New("CLD_GHOSTTY_DIR is unset (see tests/ghostty/build-lib)")
})

// GhosttyReady says why the ghostty driver cannot run, or returns nil: TestMain checks it once
// where CLD_TERMINALS lists ghostty.
func GhosttyReady() error {
	if newGhostty == nil {
		return errors.New("ghostty: the tests were built without -tags ghostty, which make test adds")
	}
	_, err := ghosttyDir()
	return err
}

// New creates the named terminal, and closes it when the test ends.
func New(tb testing.TB, name string, s *sandbox.Sandbox) Terminal {
	tb.Helper()
	var created Terminal
	switch name {
	case "tmux":
		created = newTmux(tb, s)
	case "jediterm":
		created = newJediTerm(tb, s)
	case "ghostty":
		created = newGhostty(tb, s)
	default:
		tb.Fatalf("unknown terminal %q", name)
	}
	tb.Cleanup(created.Close)
	return created
}

// unsupported provides the optional operations for terminals that cannot perform them.
type unsupported struct {
	t    testing.TB
	name string
}

func (u unsupported) Name() string { return u.name }

func (u unsupported) WheelUp() {
	u.t.Helper()
	u.t.Skipf("%s: cannot inject mouse wheel events", u.name)
}

func (u unsupported) Click(string) {
	u.t.Helper()
	u.t.Skipf("%s: cannot inject mouse clicks", u.name)
}

func (u unsupported) Hold(string, time.Duration, time.Duration, int) {
	u.t.Helper()
	u.t.Skipf("%s: cannot time a key held down", u.name)
}

func (u unsupported) Focus(bool) {
	u.t.Helper()
	u.t.Skipf("%s: cannot inject focus changes", u.name)
}

func (u unsupported) Resize(int, int) {
	u.t.Helper()
	u.t.Skipf("%s: cannot resize", u.name)
}

func (u unsupported) Freeze() func() {
	u.t.Helper()
	u.t.Skipf("%s: cannot freeze", u.name)
	return nil
}

func (u unsupported) Styled() string {
	u.t.Helper()
	u.t.Skipf("%s: cannot read attributes", u.name)
	return ""
}

func (u unsupported) Clipboard() string {
	u.t.Helper()
	u.t.Skipf("%s: cannot read the clipboard", u.name)
	return ""
}

// mouseModifiers are the modifiers of a mouse key as tmux names them, and their bits in the button
// of an xterm mouse report.
var mouseModifiers = map[string]int{"S-": 4, "M-": 8, "C-": 16}

// mouseButton is the button of an xterm mouse report for key, a press named the way tmux names it
// (see Click). Buttons count from 0, and each modifier adds a bit.
func mouseButton(tb testing.TB, key string) int {
	tb.Helper()
	button := 0
	for len(key) > 2 && mouseModifiers[key[:2]] != 0 {
		button |= mouseModifiers[key[:2]]
		key = key[2:]
	}
	number, found := strings.CutPrefix(key, "MouseDown")
	if n, err := strconv.Atoi(number); found && err == nil && n >= 1 && n <= 3 {
		return button | (n - 1)
	}
	tb.Fatalf("no mouse press %q", key)
	return 0
}
