// Package terminal drives the outer terminals cld runs in, behind one interface.
//
// Keys are named the way tmux names them: "Enter", "S-Enter", "C-q", "C-b", or one character.
// Methods fail the test on errors; a terminal that cannot do something skips the test instead,
// giving the reason.
package terminal

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Modes is terminal state a program may leave behind when it exits.
type Modes struct {
	AltScreen bool `json:"alt_screen"`
	Mouse     bool `json:"mouse"` // any mouse reporting
}

// Terminal is an outer terminal: a pty and whatever emulates the screen and keyboard around it.
type Terminal interface {
	Name() string
	// Start runs argv on a fresh pty of this terminal, with env as its whole environment.
	Start(argv []string, env map[string]string, dir string)
	// Keys types the keys one after another, as a user would.
	Keys(keys ...string)
	WheelUp()
	Focus(focused bool)
	// Title is the window or tab title the terminal shows.
	Title() string
	// Screen is the visible text.
	Screen() string
	Modes() Modes
	// Clipboard is the text last copied to the clipboard through OSC 52.
	Clipboard() string
	// Running reports whether the program Start ran is still running.
	Running() bool
	Close()
}

// New creates the named terminal; it is closed when the test ends.
func New(t testing.TB, name string, s *sandbox.Sandbox) Terminal {
	t.Helper()
	var created Terminal
	switch name {
	case "tmux":
		created = newTmux(t, s)
	case "jediterm":
		created = newJediTerm(t, s)
	default:
		t.Fatalf("unknown terminal %q", name)
	}
	t.Cleanup(created.Close)
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

func (u unsupported) Focus(bool) {
	u.t.Helper()
	u.t.Skipf("%s: cannot inject focus changes", u.name)
}

func (u unsupported) Clipboard() string {
	u.t.Helper()
	u.t.Skipf("%s: cannot read the clipboard", u.name)
	return ""
}
