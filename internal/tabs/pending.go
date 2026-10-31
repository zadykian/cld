package tabs

import (
	"iter"
	"slices"
	"strings"
)

// vanish makes a pending loss of each window, tab and split of r's windows whose panes o holds
// nowhere, and takes those panes out of the record's. Ghostty keeps a closed pane's login while
// its undo can bring the pane back, so the loss counts only once the login goes.
func vanish(r *Record, o *Observation) {
	var gone []string
	for _, w := range r.Windows {
		for _, l := range closedIn(w, o.gone, r.Panes) {
			l.At, l.Run = o.Time, o.Run
			r.Pending = append(r.Pending, l)
		}
		for _, id := range windowLeaves(w) {
			if o.gone(id) {
				gone = append(gone, id)
			}
		}
	}
	for _, id := range gone {
		delete(r.Panes, id)
	}
}

// closedIn is what closed in window w, gone telling a closed pane: w whole where its panes are
// all gone. Else each tab whose panes are all gone is a loss, with its place, and each split
// likewise, with its side. The losses hold copies of the panes as the record last saw them.
func closedIn(w Window, gone func(id string) bool, panes map[string]*Pane) []*Loss {
	ids := windowLeaves(w)
	if len(ids) == 0 || slices.ContainsFunc(ids, func(id string) bool { return !gone(id) }) {
		return closedTabs(w, gone, panes)
	}
	return []*Loss{{Cause: CauseClosed, Windows: []Window{copyWindow(w, panes)}}}
}

// closedTabs is what closed in the tabs of window w, whose panes are not all gone.
func closedTabs(w Window, gone func(id string) bool, panes map[string]*Pane) []*Loss {
	var losses []*Loss
	for i, tab := range w.Tabs {
		if allGone(tab.Tree, gone) {
			losses = append(losses, &Loss{Cause: CauseTab, Anchor: anchorFor(w, i, gone),
				Windows: []Window{{Tabs: []Tab{copyTab(tab, panes)}}}})
			continue
		}
		for _, c := range goneSubtrees(tab.Tree, gone) {
			lost := Tab{Shape: tab.Shape, Tree: copyTree(c.tree, panes)}
			if isLeaf(c.tree) {
				lost.Shape = ShapeMade
			}
			if slices.Contains(Leaves(c.tree), tab.Focused) {
				lost.Focused = tab.Focused
			}
			losses = append(losses, &Loss{Cause: CausePane,
				Anchor:  &Anchor{Pane: c.anchor, Split: c.side},
				Windows: []Window{{Tabs: []Tab{lost}}}})
		}
	}
	return losses
}

// anchorFor is where tab i of window w goes back: its place from 1, beside the first open pane of
// the nearest tab before it, else after it. Nil means no tab of w has an open pane.
func anchorFor(w Window, i int, gone func(id string) bool) *Anchor {
	for j := i - 1; j >= 0; j-- {
		if pane := firstOpen(w.Tabs[j].Tree, gone); pane != "" {
			return &Anchor{Pane: pane, Index: i + 1}
		}
	}
	for j := i + 1; j < len(w.Tabs); j++ {
		if pane := firstOpen(w.Tabs[j].Tree, gone); pane != "" {
			return &Anchor{Pane: pane, Index: i + 1}
		}
	}
	return nil
}

// copyWindow is a copy of window w whose leaves carry their panes in panes, nil stripping them.
func copyWindow(w Window, panes map[string]*Pane) Window {
	tabs := make([]Tab, len(w.Tabs))
	for i, tab := range w.Tabs {
		tabs[i] = copyTab(tab, panes)
	}
	return Window{Tabs: tabs}
}

// copyTab is a copy of tab t whose leaves carry their panes in panes, nil stripping them.
func copyTab(t Tab, panes map[string]*Pane) Tab {
	t.Tree = copyTree(t.Tree, panes)
	return t
}

// undo drops each pending loss of which o shows a pane in a window: Ghostty's undo brought it
// back, and nothing was lost. Its panes shown go back into r.Panes as they were, keeping Since
// and Last. Its windows, stripped, are what undo returns, so that their trees stay (treeFor).
func undo(r *Record, o *Observation) []Window {
	var undone []Window
	kept := r.Pending[:0]
	for _, l := range r.Pending {
		if !showsAny(o, l) {
			kept = append(kept, l)
			continue
		}
		for id, p := range lossCopies(l) {
			if _, in := r.Panes[id]; p != nil && !in && o.shows(id) {
				r.Panes[id] = p
			}
		}
		for _, w := range l.Windows {
			undone = append(undone, copyWindow(w, nil))
		}
	}
	r.Pending = kept
	return undone
}

// settlePending moves to r.Losses each pending loss none of whose logins procs still shows: the
// close is past Ghostty's undo. A loss keeps the time of its close. A pane's login lives as long
// as Ghostty keeps its surface, as the default undo-timeout showed. UNVERIFIED until #177's check 9
// (decision 59.12): other undo-timeout values.
func settlePending(r *Record, procs Table) {
	kept := r.Pending[:0]
	for _, l := range r.Pending {
		if lives(l, procs) {
			kept = append(kept, l)
		} else {
			r.Losses = append(r.Losses, l)
		}
	}
	r.Pending = kept
}

// lives reports whether procs shows the login of a pane of loss l on that pane's tty.
func lives(l *Loss, procs Table) bool {
	for _, p := range lossCopies(l) {
		if p != nil && p.Login != 0 && procs.Has(p.Login, strings.TrimPrefix(p.TTY, "/dev/")) {
			return true
		}
	}
	return false
}

// showsAny reports whether a window of o shows a pane of loss l.
func showsAny(o *Observation, l *Loss) bool {
	for id := range lossCopies(l) {
		if o.shows(id) {
			return true
		}
	}
	return false
}

// lossCopies are loss l's leaves: each terminal id, and the pane as last seen, nil where none.
func lossCopies(l *Loss) iter.Seq2[string, *Pane] { return windowCopies(l.Windows) }

// windowCopies are the leaves of windows, a loss's: each terminal id, and its pane's copy.
func windowCopies(windows []Window) iter.Seq2[string, *Pane] {
	return func(yield func(string, *Pane) bool) {
		for _, w := range windows {
			for _, tab := range w.Tabs {
				if !leafCopies(tab.Tree, yield) {
					return
				}
			}
		}
	}
}

// leafCopies yields the leaves of tree n, and reports whether yield asked for more.
func leafCopies(n *Node, yield func(string, *Pane) bool) bool {
	switch {
	case n == nil:
		return true
	case isLeaf(n):
		return n.Pane == "" || yield(n.Pane, n.Copy)
	}
	return leafCopies(n.First, yield) && leafCopies(n.Second, yield)
}
