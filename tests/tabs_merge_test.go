package tests

import (
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The record's merges within one run of Ghostty: panes move between tabs and windows.

// checkRecord reports r, what a step left, unless its windows draw as want does, with names for
// the panes.
func checkRecord(t *testing.T, step string, r *tabs.Record, names map[string]string, want string) {
	t.Helper()
	if got := drawWindows(r.Windows, names); got != want {
		t.Errorf("%s: the record holds\n%s\nwant\n%s", step, got, want)
	}
}

// A pane that moves to another tab or window keeps its since.
func TestTabsMergeMoves(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyAt(812, ghosttyStart)
	a, b, c := addPane(m, "~"), addPane(m, "~"), addPane(m, "~")
	names := named(a, b, c)
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b)), windowOf(tabOf(c))}
	first := tick(t, s, m)
	for _, step := range []struct {
		name    string
		windows []mac.Window
		want    string
	}{
		{"a tab to another window", []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(c), tabOf(b))},
			"*a/made | *c/made b/made"},
		{"a tab to a window of its own", []mac.Window{windowOf(tabOf(b)), windowOf(tabOf(c)),
			windowOf(tabOf(a))}, "*b/made | *c/made | *a/made"},
		{"tabs into one's splits", []mac.Window{windowOf(tabOf(c, a, b))},
			"*right(c,right(a,b))/flat"},
	} {
		m.Windows = step.windows
		r := tick(t, s, m)
		checkRecord(t, step.name, r, names, step.want)
		for _, id := range []string{a, b, c} {
			if p := r.Panes[id]; p == nil || !p.Since.Equal(first.Panes[id].Since) {
				t.Errorf("%s: pane %s is %+v, want since %v", step.name, names[id], p,
					first.Panes[id].Since)
			}
		}
	}
}

// Panes in no window that Ghostty still lists are never closed: the quick terminal, never in a
// window, enters no record. A window minimized or hidden stays the record's, last. A ghostty
// +ssh process, which runs Ghostty's own binary, is no new run of Ghostty.
func TestTabsMergeHidden(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyAt(812, ghosttyStart)
	a, b, quick := addPane(m, "~"), addPane(m, "~"), addPane(m, "~")
	names := named(a, b)
	m.Windows, m.Hidden = []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(b))}, []string{quick}
	r := tick(t, s, m)
	checkRecord(t, "shown", r, names, "*a/made | *b/made")
	run := r.Run.ID
	m.Windows, m.Hidden = []mac.Window{windowOf(tabOf(a))}, []string{quick, b}
	sshCache := m.NewPID()
	m.Processes = append(m.Processes, mac.Process{PID: sshCache, PPID: 1, PGID: sshCache,
		Start: time.Now().UTC().Truncate(time.Second), Args: []string{ghosttyExecutable, "+ssh"}})
	for range 2 {
		checkRecord(t, "minimized", tick(t, s, m), names, "*a/made | *b/made")
	}
	m.Windows, m.Hidden = []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(b))}, nil
	m.Processes = m.Processes[:len(m.Processes)-1]
	r = tick(t, s, m)
	checkRecord(t, "back, the quick terminal closed", r, names, "*a/made | *b/made")
	if r.Panes[quick] != nil || r.Run.ID != run {
		t.Errorf("the record holds %+v and run %s, want no quick terminal and run %s",
			r.Panes[quick], r.Run.ID, run)
	}
}
