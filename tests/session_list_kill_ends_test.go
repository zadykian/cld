package tests

import (
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listKillLastRow checks that killing the last session leaves it on its row, ended. Forgetting it
// then leaves the list with no sessions, as when its last row has gone elsewhere, and leaving
// prints nothing.
func listKillLastRow(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	const header = "  NAME  STATE     LAST ACTIVE  DIRECTORY"
	term.Keys("C-x", "C-x")
	waitLines(t, term, header, "> a     ended     -            "+s.Work, "", endedHints)
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool {
		return !probes["a"].Alive()
	})
	// A letter first ends the wait after a kill (see held down).
	term.Keys("k", "C-x", "C-x")
	waitLines(t, term, header, "no sessions", "", "esc to quit")
	// Ctrl+X has nothing to arm, or to forget.
	term.Keys("k", "C-x", "C-x")
	waitLines(t, term, header, "no sessions", "", "esc to quit")
	if list.exited() {
		t.Error("the list closed")
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "")
}

// listKillExited checks that an exited session is killed the same way, and the terminal still
// attached to it detached.
func listKillExited(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	failed := startCld(t, s, "tmux", nil, "join", "-s", "b")
	waitClients(t, s, 1)
	for _, probe := range s.WaitProbes(2) {
		if probe.Argv[1] == "cld-b" {
			probe.Send("exit 1")
		}
	}
	waitScreen(t, failed, "claude exited with status 1: cld kill -s b ends the session, "+
		"C-q d or cld detach -s b detaches")
	term := startCld(t, s, "tmux", nil, "list")
	rows := []string{
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          " + s.Work,
		"> b     exited    now          " + s.Work,
		"",
	}
	waitScreen(t, term, listHints)
	term.Keys("Down")
	waitLines(t, term, append(rows, listHints)...)
	armThen(t, term, func() {
		waitLines(t, term, append(rows, killArmedAttached)...)
	}, "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     ended     -            "+s.Work,
		"",
		endedHints)
	sandbox.WaitFor(t, 10*time.Second, "b's terminal to be detached", func() bool {
		return !failed.Running()
	})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q, want [cld-a]", sessions)
	}
}

// listKillGone checks that a session ended by the second Ctrl+X, killed elsewhere here, is
// reported, and that the list reads the sessions again and stays open. The kill elsewhere comes
// before the first Ctrl+X, so that it need not fit in the two seconds.
func listKillGone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b", "c")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	killElsewhere(t, s, "b")
	armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     ended     -            "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		"session 'b' has ended")
	list.checkRaw(t)
	if list.exited() {
		t.Error("the list closed")
	}
	for _, name := range []string{"a", "c"} {
		if !probes[name].Alive() {
			t.Errorf("claude %s exited", name)
		}
	}
}

// listKillReplaced checks that a session ended and made again under its name is another, with
// another claude (decision 15.3). The kill ends nothing, as for a session gone, and the list shows
// the new one.
func listKillReplaced(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	old := detachedSessions(t, s, "a", "b", "c")["b"]
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	killElsewhere(t, s, "b")
	sandbox.WaitFor(t, 10*time.Second, "the first claude b to exit", func() bool {
		return !old.Alive()
	})
	detachedSessions(t, s, "b")
	var replaced *sandbox.Probe
	for _, probe := range s.Probes() {
		if probe.Argv[1] == "cld-b" && probe.PID != old.PID {
			replaced = probe
		}
	}
	if replaced == nil {
		t.Fatal("no second claude b")
	}
	armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     detached  now          "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		"no session 'b'")
	if !replaced.Alive() {
		t.Error("the second claude b exited")
	}
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want [cld-a cld-b cld-c]", sessions)
	}
	if list.exited() {
		t.Error("the list closed")
	}
}

// listKillLingering checks that a session whose server outlived it ends with the server, as cld
// kill ends it, though no pane is left to check against the pids the list read (decision 15.3).
// Its row stays, selected, as one that has ended.
func listKillLingering(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b", "c")
	probes["b"].Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-b/side")
	})
	term := startCld(t, s, "tmux", nil, "list")
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	probes["b"].Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "b's session to end", func() bool {
		return !slices.Contains(s.Sessions(), "cld-b")
	})
	armThen(t, term, func() { waitScreen(t, term, killArmed) }, "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     ended     -            "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		endedHints)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
		t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
	}
}

// listKillSplitWindow checks that the kill goes by the pids of all the session's panes, as tmux
// reports only the active one's. claude's window, split by hand with its other pane selected since
// the list read the sessions, is still the session on the row.
func listKillSplitWindow(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := startCld(t, s, "tmux", nil, "list")
	waitScreen(t, term, listHints)
	s.MustTmux("cld-a", "split-window", "-d", "-t", "=cld-a:", "sleep", "600")
	s.MustTmux("cld-a", "select-pane", "-t", "=cld-a:.1")
	term.Keys("C-x", "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     ended     -            "+s.Work,
		"  b     detached  now          "+s.Work,
		"",
		endedHints)
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool {
		return !probes["a"].Alive()
	})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b"}) {
		t.Errorf("sessions %q, want [cld-b]", sessions)
	}
}

// killElsewhere kills session name, as from elsewhere than the list.
func killElsewhere(t *testing.T, s *sandbox.Sandbox, name string) {
	t.Helper()
	if result := s.RunCld(nil, "kill", "-s", name); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
}
