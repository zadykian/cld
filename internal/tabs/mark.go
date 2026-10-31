package tabs

import (
	"slices"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// mark records in r, at time now, what apply did of plan p, under the record's lock (decision
// 59.6). res is apply's result, nil where it gave none. Each build's terminals enter the record
// as panes at their prompt, in the windows it made.
func mark(r *Record, p *Planned, res *ghostty.Result, now time.Time) {
	if res == nil {
		return
	}
	now = now.UTC().Truncate(time.Second)
	for i := range min(len(res.Typed), len(p.typed)) {
		if t := res.Typed[i]; t.Error == "" && !t.Busy {
			typedAt(r.Panes[p.typed[i].pane], now)
		}
	}
	made, built := map[*Loss]bool{}, map[*Loss]bool{}
	for _, b := range p.builds {
		built[b.loss] = true
	}
	for i := range min(len(res.Builds), len(p.builds)) {
		if len(res.Builds[i].Keys) > 0 {
			made[p.builds[i].loss] = true
			enter(r, p.script.Builds[i], p.builds[i], res.Builds[i], now)
		}
	}
	for _, l := range p.chosen {
		if made[l] || !built[l] {
			l.Restored = new(now)
		}
	}
}

// typedAt notes on pane p that a restore typed its Last's line at time now. A tick leaves the Last
// until the line has had its time to join (decision 59.4).
func typedAt(p *Pane, now time.Time) {
	if p != nil && p.Last != nil {
		p.Last.Typed = new(now)
	}
}

// enter puts into r the terminals that build b of info's loss made, as res maps them. Each is a
// pane at its prompt, with the Last its typed line set, so that a failed ssh is retried in place.
// Their tree, as cld made it, goes where b put it, so that a tick keeps it.
func enter(r *Record, b ghostty.Build, info building, res ghostty.BuildResult, now time.Time) {
	for key, id := range res.Keys {
		c, planned := info.copies[key]
		if !planned {
			continue
		}
		pane := &Pane{Kind: KindLocal, Since: now}
		if c != nil { // a leaf with no copy has no binding either
			pane.Cwd = c.Cwd
			if bind, ok := info.bindings[key]; ok {
				pane.Last = &Last{Host: bind.Host, Session: bind.Session, Worktree: c.Worktree,
					Cut: cutOf(info.loss), Run: r.Run.ID, At: now, Typed: new(now)}
			}
		}
		r.Panes[id] = pane
	}
	tabs := madeTabs(b.Tabs, res.Keys)
	switch {
	case len(tabs) == 0:
	case b.Kind == buildPane:
		graftSplit(r, b, tabs[0])
	case b.Kind == buildTab:
		insertTab(r, b, res, tabs[0])
	default:
		r.Windows = slices.Insert(r.Windows, 0, Window{Tabs: tabs})
	}
}

// cutOf is the cut of a binding a restore of loss l typed again: a quit's, or a close's.
func cutOf(l *Loss) string {
	if l.Cause == CauseQuit {
		return CutQuit
	}
	return CutClosed
}

// madeTabs are the tabs of builds as keys made them: each tree less the leaves it lacks, with
// the terminals' ids, and shape made. A tab none of whose leaves was made goes.
func madeTabs(builds []ghostty.TabBuild, keys map[string]string) []Tab {
	var tabs []Tab
	for _, tb := range builds {
		if tree := madeTree(tb.Tree, keys); tree != nil {
			tabs = append(tabs, Tab{Selected: tb.Selected, Focused: keys[tb.Focused],
				Shape: ShapeMade, Tree: tree})
		}
	}
	return tabs
}

// madeTree is tree n as keys made it, nil for none of it. apply.js makes a split's second half
// before the rest of the first, and stops at a failure. So the tree less what it did not make is
// the tree Ghostty holds.
func madeTree(n *ghostty.Node, keys map[string]string) *Node {
	switch {
	case n == nil:
		return nil
	case n.Leaf != nil:
		if id, ok := keys[n.Leaf.Key]; ok {
			return &Node{Pane: id}
		}
		return nil
	}
	first, second := madeTree(n.First, keys), madeTree(n.Second, keys)
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	}
	return &Node{Split: n.Split, First: first, Second: second}
}

// insertTab puts tab, which tab build b made, at the index res read back in the window of r
// holding b's anchor, selected, as Ghostty selects a new tab. Without that window, or where it
// opened as a window, the tab is a window in front. An index not read back puts it last.
func insertTab(r *Record, b ghostty.Build, res ghostty.BuildResult, tab Tab) {
	tab.Selected = true
	w := slices.IndexFunc(r.Windows, func(w Window) bool {
		return b.Anchor != "" && slices.Contains(windowLeaves(w), b.Anchor)
	})
	if res.Window || w < 0 {
		r.Windows = slices.Insert(r.Windows, 0, Window{Tabs: []Tab{tab}})
		return
	}
	tabs := r.Windows[w].Tabs
	for i := range tabs {
		tabs[i].Selected = false
	}
	at := len(tabs)
	if res.Index > 0 {
		at = min(res.Index-1, len(tabs))
	}
	r.Windows[w].Tabs = slices.Insert(tabs, at, tab)
}

// graftSplit puts split's tree, which pane build b made, beside b's anchor in r's windows, or as
// a window in front where none holds the anchor.
func graftSplit(r *Record, b ghostty.Build, split Tab) {
	for _, w := range r.Windows {
		for _, tab := range w.Tabs {
			if graft(tab.Tree, b.Anchor, split.Tree, b.Split) {
				return
			}
		}
	}
	split.Selected = true
	r.Windows = slices.Insert(r.Windows, 0, Window{Tabs: []Tab{split}})
}

// graft turns the leaf of tree n holding pane anchor into a split of it and sub, sub on side of
// it. sub is the second half for right and down, the first for left and up, as Ghostty splits.
// False means n holds no such leaf.
func graft(n *Node, anchor string, sub *Node, side string) bool {
	switch {
	case n == nil:
		return false
	case !isLeaf(n):
		return graft(n.First, anchor, sub, side) || graft(n.Second, anchor, sub, side)
	case n.Pane != anchor:
		return false
	}
	leaf := &Node{Pane: n.Pane, Copy: n.Copy}
	switch side {
	case splitLeft:
		*n = Node{Split: splitRight, First: sub, Second: leaf}
	case splitUp:
		*n = Node{Split: splitDown, First: sub, Second: leaf}
	default:
		*n = Node{Split: side, First: leaf, Second: sub}
	}
	return true
}
