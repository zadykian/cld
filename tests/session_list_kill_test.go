package tests

import (
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// In the interactive list, Ctrl+X arms the kill of the selected session (decision 15). A second
// Ctrl+X within two seconds kills it, as cld kill -s NAME does, and Esc keeps it. The list then
// reads the sessions again and stays open, the selection on the row that took the killed row's.
func TestListKill(t *testing.T) {
	t.Parallel()
	t.Run("kill", listKillKills)
	t.Run("esc", listKillEsc)
	t.Run("timeout", listKillTimeout)
	t.Run("read late", listKillReadLate)
	t.Run("other keys", listKillOtherKeys)
	t.Run("enter", listKillEnter)
	t.Run("held down", listKillHeldDown)
	t.Run("last row", listKillLastRow)
	t.Run("exited", listKillExited)
	t.Run("gone", listKillGone)
	t.Run("replaced", listKillReplaced)
	t.Run("lingering server", listKillLingering)
	t.Run("kill-session fails", listKillSessionFails)
	t.Run("read again fails", listKillReadAgainFails)
	t.Run("server exiting", listKillServerExiting)
	t.Run("keys during the kill", listKillKeysDuring)
	listKillLeaveDuring(t)
	t.Run("split window", listKillSplitWindow)
	t.Run("61 columns", listKill61Columns)
}

// listKillKills checks that the kill detaches the terminal on the session, leaving it clean, as
// cld kill does, the armed kill's footer saying so first. The other sessions carry on, and the
// killed one stays in its place, selected, as one that has ended.
func listKillKills(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "c")
	other := startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitClients(t, s, 1)
	for _, probe := range s.WaitProbes(3) {
		if probe.Argv[1] == "cld-b" {
			probes["b"] = probe
		}
	}
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	rows := []string{
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          " + s.Work,
		"> b     attached  now          " + s.Work,
		"  c     detached  now          " + s.Work,
		"",
	}
	waitScreen(t, term, listHints)
	term.Keys("Down")
	waitLines(t, term, append(rows, listHints)...)
	armThen(t, term, func() {
		waitLines(t, term, append(rows, killArmedAttached)...)
		if footer := cells(term.Styled())[5]; footer != "[intensity=2]"+killArmedAttached {
			t.Errorf("the footer is %q, want it dim", footer)
		}
		if !probes["b"].Alive() || !other.Running() {
			t.Error("the first Ctrl+X killed b")
		}
	}, "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     ended     -            "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		endedHints)
	checkKilledB(t, s, probes, other)
	list.checkRaw(t)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
		"a     detached  now          "+s.Work+"\n"+"b     ended     -            "+s.Work+"\n"+
		"c     detached  now          "+s.Work+"\n")
	list.checkRestored(t, term)
}

// checkKilledB checks that the kill ended session b, with its claude, and detached its terminal,
// other, leaving it clean, and that the claudes of a and c run on.
func checkKilledB(
	t *testing.T, s *sandbox.Sandbox, probes map[string]*sandbox.Probe, other terminal.Terminal,
) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, "claude b to exit", func() bool {
		return !probes["b"].Alive()
	})
	sandbox.WaitFor(t, 10*time.Second, "b's terminal to be detached", func() bool {
		return !other.Running()
	})
	if modes := other.Modes(); modes.AltScreen || modes.Mouse {
		t.Errorf("modes of b's terminal after the kill %+v, want none", modes)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
		t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
	}
	for _, name := range []string{"a", "c"} {
		if !probes[name].Alive() {
			t.Errorf("claude %s exited", name)
		}
	}
}

// listKillEsc checks that Esc disarms the kill and does nothing more: the list stays open, and
// the next Ctrl+X arms the kill again.
func listKillEsc(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	rows := []string{
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          " + s.Work,
		"  b     detached  now          " + s.Work,
		"",
	}
	waitLines(t, term, append(rows, listHints)...)
	armThen(t, term, func() { waitLines(t, term, append(rows, killArmed)...) }, "Escape")
	waitLines(t, term, append(rows, listHints)...)
	term.Keys("C-x")
	waitLines(t, term, append(rows, killArmed)...)
	if list.exited() {
		t.Error("the list closed")
	}
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
		t.Errorf("sessions %q, claude a alive: %v; want both, a alive",
			sessions, probes["a"].Alive())
	}
}

// listKillTimeout checks that the kill disarms two seconds after the first Ctrl+X, and that a
// Ctrl+X after that arms it again.
func listKillTimeout(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := startCld(t, s, "tmux", nil, "list")
	rows := []string{
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          " + s.Work,
		"  b     detached  now          " + s.Work,
		"",
	}
	waitLines(t, term, append(rows, listHints)...)
	pressed := time.Now()
	term.Keys("C-x")
	waitLines(t, term, append(rows, killArmed)...)
	waitLines(t, term, append(rows, listHints)...)
	// Seeing the footer change takes a while: two seconds more is slack for load.
	if waited := time.Since(pressed); waited < 2*time.Second || waited > 4*time.Second {
		t.Errorf("the kill disarmed %v after Ctrl+X, want two seconds", waited)
	}
	term.Keys("C-x")
	waitLines(t, term, append(rows, killArmed)...)
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
		t.Errorf("sessions %q, claude a alive: %v; want both, a alive",
			sessions, probes["a"].Alive())
	}
}

// listKillEnter checks that Enter, once Ctrl+X has armed the kill, disarms it and joins the
// selected session, which the kill leaves alone.
func listKillEnter(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	armThen(t, term, func() { waitScreen(t, term, killArmed) }, "Enter")
	waitScreen(t, term, "probe --name cld-b")
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("clients attached to %q, want one, to cld-b", clients)
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
	for name, probe := range probes {
		if !probe.Alive() {
			t.Errorf("claude %s exited", name)
		}
	}
}

// listKill61Columns checks the footer in 61 columns, a cell short of the hints' 62, on an attached
// row as on another. The arrows' hint goes, so that the others, esc to quit last, show whole. The
// armed kill's on such a row, 58 cells, shows whole too.
func listKill61Columns(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitClients(t, s, 1)
	s.WaitProbes(2)
	term := terminal.New(t, "tmux", s)
	term.Resize(61, 24)
	term.Start(s.CldArgv("list"), s.Env, s.Work)
	rows := func(selected string) []string {
		lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
		for _, row := range []string{
			"a     detached  now          ", "b     attached  now          ",
		} {
			marker := " "
			if row[:1] == selected {
				marker = ">"
			}
			lines = append(lines, cutTo(marker+" "+row+s.Work, 61))
		}
		return append(lines, "")
	}
	hints := "enter to join · ctrl+x to kill · esc to quit"
	waitLines(t, term, append(rows("a"), hints)...)
	term.Keys("Down")
	waitLines(t, term, append(rows("b"), hints)...)
	term.Keys("C-x")
	waitLines(t, term, append(rows("b"), killArmedAttached)...)
}
