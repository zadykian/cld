package tests

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// The interactive list's forget of an ended session that a restore or a join is bringing back,
// which forgets nothing (decisions 48.6 and 50.4).

// The list's forget of a session restore is bringing back waits for the record's lock, held as
// restore's tmux is held until the forget waits (see waitForLock). It then finds the session
// running, and leaves its entry, environment and run mark.
func TestForgetRacingRestore(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeRestorable(t, s, "x", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"),
		s.Environ(nil))
	tmux := holdTmux(t, s, "restore's tmux making cld-x", "*' new-session -d -s cld-x '*")
	tmux.start(t)
	restore := startCldAsync(t, s, s.Work, tmux.env, "restore")
	tmux.held(t)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, endedHints)
	armThen(t, term, func() { waitScreen(t, term, forgetArmed) }, "C-x")
	waitForLock(t, s, "the list's forget to wait for the record's lock")
	tmux.release(t)
	checkExit(t, "restore", restore(), 0, "Restored session 'x' in "+s.Work+"\n", "")
	waitScreen(t, term, "session 'x' runs again")
	if readEntry(s, "x") == "" {
		t.Error("the forget took the entry of the session restore brought back")
	}
	if !exists(companionFile(s, "x", ".env")) {
		t.Error("the forget took the environment of the session restore brought back")
	}
	checkMarks(t, s, "after the forget", map[string]bool{"x": true})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
		t.Errorf("sessions %q, want [cld-x]", sessions)
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
}

// A join on another terminal is making an ended session, its tmux held once cld has let the lock
// go. The list's forget of the session finds the join's start mark: "session 'x' is starting".
// The join then attaches to the session it made, whose entry, environment and run mark stay.
func TestForgetRacingJoin(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "x", s.Work, firstID)
	tmux := holdTmux(t, s, "the join's tmux making cld-x", "*' new-session -s cld-x '*")
	tmux.start(t)
	join := startCld(t, s, "tmux", tmux.env, "join", "-s", "x")
	tmux.held(t)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, endedHints)
	armThen(t, term, func() { waitScreen(t, term, forgetArmed) }, "C-x")
	waitScreen(t, term, "session 'x' is starting")
	if !exists(companionFile(s, "x", ".start")) || !exists(companionFile(s, "x", ".env")) {
		t.Error("the forget took the start mark or the environment of the session the join is making")
	}
	tmux.release(t)
	waitClients(t, s, 1)
	if !join.Running() {
		t.Errorf("join is not attached:\n%s", join.Screen())
	}
	if readEntry(s, "x") == "" {
		t.Error("the forget took the entry of the session the join made")
	}
	if !exists(companionFile(s, "x", ".env")) {
		t.Error("the forget took the environment of the session the join made")
	}
	checkMarks(t, s, "after the forget", map[string]bool{"x": true})
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
}
