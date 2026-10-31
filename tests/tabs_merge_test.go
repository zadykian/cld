package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The record's merges within one run of Ghostty. Panes move, and windows, tabs and splits close,
// wait for Ghostty's undo, and count as lost once their logins are gone.

// checkRecord reports r, what a step left, unless its windows, its pending losses and its losses
// draw as want's lines do, with names for the panes.
func checkRecord(
	t *testing.T, step string, r *tabs.Record, names map[string]string, want ...string,
) {
	t.Helper()
	got := []string{drawWindows(r.Windows, names), drawLosses(r.Pending, names),
		drawLosses(r.Losses, names)}
	if strings.Join(got, "\n--\n") != strings.Join(want, "\n--\n") {
		t.Errorf("%s: the record holds\n%s\nwant\n%s", step, strings.Join(got, "\n--\n"),
			strings.Join(want, "\n--\n"))
	}
}

// A pane that moves to another tab or window loses nothing, and keeps its since.
func TestTabsMergeMoves(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b, c := addPane(m, "✳ cld-x", ssh("ws")), addPane(m, "~"), addPane(m, "~")
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
		checkRecord(t, step.name, r, names, step.want, "", "")
		for _, id := range []string{a, b, c} {
			if p := r.Panes[id]; p == nil || !p.Since.Equal(first.Panes[id].Since) {
				t.Errorf("%s: pane %s is %+v, want since %v", step.name, names[id], p,
					first.Panes[id].Since)
			}
		}
	}
}

// A closed window waits for Ghostty's undo while its panes' logins live, which a long
// undo-timeout makes several ticks. Then the record counts it lost, with its time of closing and
// its panes as the record last saw them.
func TestTabsMergeClosedWindow(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b, c := addPane(m, "~"), addPane(m, "✳ cld-x", ssh("ws")), addPane(m, "~")
	names := named(a, b, c)
	m.Windows = []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(b, c))}
	tick(t, s, m)
	m.Windows = m.Windows[:1]
	var closed time.Time
	for i := range 3 {
		r := tick(t, s, m)
		checkRecord(t, "closed", r, names, "*a/made", "closed: *right(b,c)/flat", "")
		switch {
		case len(r.Pending) != 1:
		case i == 0:
			closed = r.Pending[0].At
			checkCopy(t, r.Pending[0], b, shown{Kind: "cld", Host: "ws", Session: "x",
				Title: "✳ cld-x"})
		case !r.Pending[0].At.Equal(closed):
			t.Errorf("tick %d: closed at %v, want %v", i, r.Pending[0].At, closed)
		}
		if r.Panes[b] != nil || r.Panes[c] != nil {
			t.Error("the closed window's panes are still the record's")
		}
	}
	checkReport(t, s, "Waiting for Ghostty's undo:\n  a window of 1 tab and 2 panes, closed at "+
		clockAt(closed)+"\n")
	endLogins(m, b, c)
	r := tick(t, s, m)
	checkRecord(t, "its logins gone", r, names, "*a/made", "", "closed: *right(b,c)/flat")
	if len(r.Losses) == 1 && !r.Losses[0].At.Equal(closed) {
		t.Errorf("lost at %v, want %v, when it closed", r.Losses[0].At, closed)
	}
	checkReport(t, s, "Losses:\n  a window of 1 tab and 2 panes, closed at "+clockAt(closed)+"\n")
}

// checkCopy reports loss l unless its leaf id holds a copy of the pane that shows want.
func checkCopy(t *testing.T, l *tabs.Loss, id string, want shown) {
	t.Helper()
	for _, w := range l.Windows {
		for _, tab := range w.Tabs {
			if n := findLeaf(tab.Tree, id); n != nil && n.Copy != nil {
				p := n.Copy
				if got := (shown{Kind: p.Kind, Host: p.Host, Session: p.Session, Title: p.Title,
					Worktree: p.Worktree}); got != want {
					t.Errorf("the loss's copy of %s shows %+v, want %+v", id, got, want)
				}
				return
			}
		}
	}
	t.Errorf("the loss holds no copy of %s", id)
}

// findLeaf is tree n's leaf of pane id, nil for none.
func findLeaf(n *tabs.Node, id string) *tabs.Node {
	switch {
	case n == nil, n.Pane == id:
		return n
	case n.Pane != "":
		return nil
	}
	if found := findLeaf(n.First, id); found != nil {
		return found
	}
	return findLeaf(n.Second, id)
}

// A closed tab waits, then is lost, with its place in its window beside the nearest open tab:
// the one before it, or else the one after.
func TestTabsMergeClosedTab(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b, c := addPane(m, "~"), addPane(m, "~"), addPane(m, "~")
	names := named(a, b, c)
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b), tabOf(c))}
	tick(t, s, m)
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(c))}
	checkRecord(t, "closed", tick(t, s, m), names, "*a/made c/made", "tab 2 beside a: b/made", "")
	endLogins(m, b)
	r := tick(t, s, m)
	checkRecord(t, "lost", r, names, "*a/made c/made", "", "tab 2 beside a: b/made")
	if len(r.Losses) == 1 {
		checkReport(t, s, "Losses:\n  a tab, closed at "+clockAt(r.Losses[0].At)+"\n")
	}
	m.Windows = []mac.Window{windowOf(tabOf(c))}
	checkRecord(t, "the first closed", tick(t, s, m), names, "*c/made", "tab 1 beside c: *a/made",
		"tab 2 beside a: b/made")
}

// A closed split waits, then counts as lost. It goes back beside the first open pane of its
// sibling: right or down of it where the split was the second half, left or up where the first.
func TestTabsMergeClosedSplit(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		tree func(a, b, c *tabs.Node) *tabs.Node
		open []int // the panes left open, by place in a, b, c
		want string
	}{
		{"right", func(a, b, _ *tabs.Node) *tabs.Node { return split("right", a, b) },
			[]int{0}, "pane right of a: b/made"},
		{"left", func(a, b, _ *tabs.Node) *tabs.Node { return split("right", a, b) },
			[]int{1}, "pane left of b: a/made"},
		{"down", func(a, b, _ *tabs.Node) *tabs.Node { return split("down", a, b) },
			[]int{0}, "pane down of a: b/made"},
		{"up", func(a, b, _ *tabs.Node) *tabs.Node { return split("down", a, b) },
			[]int{1}, "pane up of b: a/made"},
		{"a nested split", func(a, b, c *tabs.Node) *tabs.Node {
			return split("right", a, split("down", b, c))
		}, []int{0}, "pane right of a: down(b,c)/made"},
		{"within a nested split", func(a, b, c *tabs.Node) *tabs.Node {
			return split("right", a, split("down", b, c))
		}, []int{0, 1}, "pane down of b: c/made"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			ids := []string{addPane(m, "~"), addPane(m, "~"), addPane(m, "~")}
			tree := test.tree(leaf(ids[0]), leaf(ids[1]), leaf(ids[2]))
			panes := map[string]*tabs.Pane{}
			for _, id := range tabs.Leaves(tree) {
				panes[id] = &tabs.Pane{Kind: "local"}
			}
			writeRecord(t, s, recordOf(m, panes, tabs.Window{Tabs: []tabs.Tab{{Selected: true,
				Shape: "made", Tree: tree}}}))
			m.Windows = []mac.Window{windowOf(tabOf(tabs.Leaves(tree)...))}
			tick(t, s, m)
			var open []string
			for _, i := range test.open {
				open = append(open, ids[i])
			}
			m.Windows = []mac.Window{windowOf(tabOf(open...))}
			r := tick(t, s, m)
			if got := drawLosses(r.Pending, named(ids...)); got != test.want {
				t.Errorf("pending %q, want %q", got, test.want)
			}
		})
	}
}

// Panes in no window that Ghostty still lists are never closed: the quick terminal, never in a
// window, enters no record. A window minimized or hidden stays the record's, last. A ghostty
// +ssh process, which runs Ghostty's own binary, is no new run of Ghostty.
func TestTabsMergeHidden(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b, quick := addPane(m, "~"), addPane(m, "~"), addPane(m, "~")
	names := named(a, b)
	m.Windows, m.Hidden = []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(b))}, []string{quick}
	r := tick(t, s, m)
	checkRecord(t, "shown", r, names, "*a/made | *b/made", "", "")
	run := r.Run.ID
	m.Windows, m.Hidden = []mac.Window{windowOf(tabOf(a))}, []string{quick, b}
	sshCache := m.NewPID()
	m.Processes = append(m.Processes, mac.Process{PID: sshCache, PPID: 1, PGID: sshCache,
		Start: time.Now().UTC().Truncate(time.Second), Args: []string{ghosttyExecutable, "+ssh"}})
	for range 2 {
		checkRecord(t, "minimized", tick(t, s, m), names, "*a/made | *b/made", "", "")
	}
	m.Windows, m.Hidden = []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(b))}, nil
	m.Processes = m.Processes[:len(m.Processes)-1]
	r = tick(t, s, m)
	checkRecord(t, "back, the quick terminal closed", r, names, "*a/made | *b/made", "", "")
	if r.Panes[quick] != nil || r.Run.ID != run {
		t.Errorf("the record holds %+v and run %s, want no quick terminal and run %s",
			r.Panes[quick], r.Run.ID, run)
	}
}

// checkReport reports what the dry-run prints in s where it lacks want.
func checkReport(t *testing.T, s *sandbox.Sandbox, want string) {
	t.Helper()
	result := dryRun(t, s)
	if result.Code != 0 || !strings.Contains(result.Stdout, want) {
		t.Errorf("restore tabs --dry-run: exit %d, stdout\n%s\nwant within it\n%s", result.Code,
			result.Stdout, want)
	}
}
