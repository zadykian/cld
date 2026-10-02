package tests

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// C10: the session list reads the terminal's own keys, not tmux's, and so does C-q s's popup over
// a session. Whether a JetBrains IDE passes Esc and Ctrl+X on to its terminal depends on its
// keymap, which the driver cannot see.
func TestContractList(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		c := listContract{name: name}
		t.Run("join", c.join)
		t.Run("kill", c.kill)
		t.Run("popup", c.popup)
		t.Run("leave", c.leave)
	})
}

// listContract holds TestContractList's subtests, run in the terminal its name gives.
type listContract struct{ name string }

// join has Down and Enter join the second session, with the terminal handed to tmux unchanged by
// the list, which a detach shows.
func (c listContract) join(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	term := terminal.New(t, c.name, s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	if selected := selectedRow(term); selected != "a" {
		t.Errorf("row %q selected, want a", selected)
	}
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	if title := term.Title(); title != "✳ cld-b" {
		t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	list.checkRestored(t, term)
}

// kill has Ctrl+X twice kill the selected session, which then shows as ended.
func (c listContract) kill(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := terminal.New(t, c.name, s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	armThen(t, term, func() {
		waitScreen(t, term, killArmed)
		if !probes["a"].Alive() {
			t.Error("the first Ctrl+X killed a")
		}
	}, "C-x")
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
	sandbox.WaitFor(t, 10*time.Second, "the list to show a as ended, selected", func() bool {
		return selectedRow(term) == "a" && strings.Contains(term.Screen(), "> a     ended")
	})
	if !probes["b"].Alive() {
		t.Error("claude b exited")
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	list.checkRestored(t, term)
}

// popup opens the list over a session with C-q s, in tmux's popup. Esc closes it, the terminal
// staying on its session; Down and Enter move the terminal to the second session, leaving the
// first running, detached.
func (c listContract) popup(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "b")
	term := startCld(t, s, c.name, nil, "join", "-s", "a")
	waitScreen(t, term, "probe --name cld-a")
	term.Keys("C-q", "s")
	waitScreen(t, term, listHints)
	if selected := selectedRow(term); selected != "a" {
		t.Errorf("row %q selected, want a", selected)
	}
	term.Keys("Escape")
	sandbox.WaitFor(t, 10*time.Second, "the popup to close", func() bool {
		return !strings.Contains(term.Screen(), listHints) &&
			strings.Contains(term.Screen(), "probe --name cld-a")
	})
	term.Keys("C-q", "s")
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	sandbox.WaitFor(t, 10*time.Second, "the terminal on cld-b alone", func() bool {
		return slices.Equal(s.Clients(), []string{"cld-b"})
	})
	if title := term.Title(); title != "✳ cld-b" {
		t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
	}
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["b"].Alive() {
		t.Errorf("sessions %q, want a and b running", sessions)
	}
	term.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to exit", func() bool { return !term.Running() })
	if modes := term.Modes(); modes.AltScreen || modes.Mouse {
		t.Errorf("modes after detaching %+v, want everything off", modes)
	}
}

// leave has Esc put the terminal back as the list found it.
func (c listContract) leave(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	term := terminal.New(t, c.name, s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
		t.Errorf("modes while the list is open %+v, "+
			"want the alternate screen and the cursor hidden", modes)
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	list.checkRestored(t, term)
}
