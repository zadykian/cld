package tests

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// The interactive list's rename of an ended session that a restore or a join is bringing back,
// which renames nothing (decisions 48.6, 50.4 and 57).

// The list's rename of a session restore is bringing back waits for the record's lock, held as
// restore's tmux is held until the rename waits (see waitForLock). It then finds the session
// running, and leaves its entry, environment and run mark.
func TestRenameRacingRestore(t *testing.T) {
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
	waitFooter(t, term, endedHints)
	renameTo(term, "y")
	waitForLock(t, s, "the list's rename to wait for the record's lock")
	tmux.release(t)
	checkExit(t, "restore", restore(), 0, "Restored session 'x' in "+s.Work+"\n", "")
	// The row runs again, so the editor stays closed.
	waitFooter(t, term, "session 'x' runs again")
	checkNotRenamed(t, s)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
		t.Errorf("sessions %q, want [cld-x]", sessions)
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
}

// A join on another terminal is making an ended session, its tmux held once cld has let the lock
// go. The list's rename of the session finds the join's start mark: "session 'x' is starting".
// The join then attaches to the session it made, whose entry, environment and run mark stay.
func TestRenameRacingJoin(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "x", s.Work, firstID)
	tmux := holdTmux(t, s, "the join's tmux making cld-x", "*' new-session -s cld-x '*")
	tmux.start(t)
	join := startCld(t, s, "tmux", tmux.env, "join", "-s", "x")
	tmux.held(t)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	renameTo(term, "y")
	waitFooter(t, term, "rename to y  · session 'x' is starting")
	if !exists(companionFile(s, "x", ".start")) || exists(entryFile(s, "y")) {
		t.Error("the rename moved the files of the session the join is making")
	}
	tmux.release(t)
	waitClients(t, s, 1)
	if !join.Running() {
		t.Errorf("join is not attached:\n%s", join.Screen())
	}
	checkNotRenamed(t, s)
	term.Keys("Escape")
	waitFooter(t, term, endedHints)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
}

// checkNotRenamed checks that session x in s keeps its entry, environment and run mark, and that y
// has no files.
func checkNotRenamed(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	if readEntry(s, "x") == "" || !exists(companionFile(s, "x", ".env")) {
		t.Error("the rename took the entry or the environment of the session brought back")
	}
	checkMarks(t, s, "after the rename", map[string]bool{"x": true})
	for _, ext := range recordFiles {
		if exists(companionFile(s, "y", ext)) {
			t.Errorf("y%s made", ext)
		}
	}
}
