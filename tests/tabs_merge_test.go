package tests

import (
	"testing"

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
