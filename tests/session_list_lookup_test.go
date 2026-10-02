package tests

import (
	"slices"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listJoinLeaveLookup checks that Esc and Ctrl+C leave while Enter looks the session up too,
// printing the table: the lookup's tmux is killed.
func listJoinLeaveLookup(t *testing.T) {
	t.Helper()
	for _, key := range []string{"Escape", "C-c"} {
		t.Run("quit at enter with "+key, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a")
			lookup := holdLookup(t, s, "a")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, listScript, lookup.env)
			waitScreen(t, term, listHints)
			term.Keys("Enter")
			held := lookup.held(t)
			term.Keys(key)
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			if !gone(held) {
				t.Error("the lookup's tmux outlived cld")
			}
			afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
				"a     detached  now          "+s.Work+"\n")
			list.checkRestored(t, term)
			if clients := s.Clients(); len(clients) != 0 {
				t.Errorf("clients attached to %q, want none", clients)
			}
		})
	}
}

// listJoinKeysDuringLookup checks that other keys do nothing while Enter looks the session up:
// Down leaves the selection, and a second Enter looks nothing up, so the first lookup joins its
// session. Each key draws a frame, which tells the test that the list has taken it.
func listJoinKeysDuringLookup(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	lookup := holdLookup(t, s, "a")
	term := startCld(t, s, "tmux", lookup.env, "list")
	waitScreen(t, term, listHints)
	term.Keys("Enter")
	lookup.held(t)
	waitFrames(t, term, 2)
	term.Keys("Down")
	waitFrames(t, term, 3)
	if row := selectedRow(term); row != "a" {
		t.Errorf("row %q selected during the lookup, want a", row)
	}
	term.Keys("Enter")
	waitFrames(t, term, 4)
	if count := lookup.begun(t); count != 1 {
		t.Errorf("cld-a looked up %d times during the lookup, want once: "+
			"the second Enter looked it up again", count)
	}
	lookup.release(t)
	waitScreen(t, term, "probe --name cld-a")
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a"}) {
		t.Errorf("clients attached to %q, want one, to cld-a", clients)
	}
	// join looks the session up again, under the record's lock, once the list has handed over.
	if count := lookup.begun(t); count != 2 {
		t.Errorf("cld-a looked up %d times, want twice: Enter's, then join's", count)
	}
}
