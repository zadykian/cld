package tabs

import "slices"

// Ghostty's split words: right and down are a split's directions, and left and up the other
// sides of a pane a lost split goes back on.
const (
	splitRight = "right"
	splitDown  = "down"
	splitLeft  = "left"
	splitUp    = "up"
)

// isLeaf reports whether node n is a leaf: a pane, with no halves.
func isLeaf(n *Node) bool { return n.First == nil && n.Second == nil }

// Leaves are the panes of tree n by terminal id, left or top first, as Ghostty lists a tab's
// terminals.
func Leaves(n *Node) []string {
	switch {
	case n == nil:
		return nil
	case isLeaf(n) && n.Pane == "":
		return nil
	case isLeaf(n):
		return []string{n.Pane}
	}
	return append(Leaves(n.First), Leaves(n.Second)...)
}

// windowLeaves are the panes of window w's tabs, in tab-bar and leaf order.
func windowLeaves(w Window) []string {
	var ids []string
	for _, tab := range w.Tabs {
		ids = append(ids, Leaves(tab.Tree)...)
	}
	return ids
}

// within reports whether ids are tree n's leaves, in that order, less none or some.
func within(n *Node, ids []string) bool {
	i := 0
	for _, id := range Leaves(n) {
		if i < len(ids) && id == ids[i] {
			i++
		}
	}
	return i == len(ids)
}

// flat is a row of the panes ids, left to right: right(a, right(b, c)), with no ratio. One id is
// a leaf, and none is nil.
func flat(ids []string) *Node {
	if len(ids) == 0 {
		return nil
	}
	n := &Node{Pane: ids[len(ids)-1]}
	for i := len(ids) - 2; i >= 0; i-- {
		n = &Node{Split: splitRight, First: &Node{Pane: ids[i]}, Second: n}
	}
	return n
}

// copyTree is a copy of tree n whose leaves carry a copy of their panes in panes, as a loss holds
// them: the panes as last seen. A leaf panes lacks carries none, so a nil panes strips the copies.
func copyTree(n *Node, panes map[string]*Pane) *Node {
	if n == nil {
		return nil
	}
	c := &Node{Pane: n.Pane, Split: n.Split, Ratio: n.Ratio, First: copyTree(n.First, panes),
		Second: copyTree(n.Second, panes)}
	if isLeaf(n) {
		c.Copy = clonePane(panes[n.Pane])
	}
	return c
}

// clonePane is a copy of p that shares nothing with it, nil for nil.
func clonePane(p *Pane) *Pane {
	if p == nil {
		return nil
	}
	c := *p
	if p.Size != nil {
		size := *p.Size
		c.Size = &size
	}
	if p.Last != nil {
		last := *p.Last
		if p.Last.Typed != nil {
			typed := *p.Last.Typed
			last.Typed = &typed
		}
		c.Last = &last
	}
	return &c
}

// cut is a split that closed: a subtree of a tab whose panes are all gone, under a parent whose
// other half has one still open. anchor is that half's first open pane. side is where the subtree
// was beside it: the parent's split for the second half, the opposite side for the first.
type cut struct {
	tree   *Node
	anchor string
	side   string
}

// goneSubtrees are the cuts of tree n, left or top first, gone telling a gone pane by its id. n's
// panes must not all be gone, as a whole tab gone is no split's loss.
func goneSubtrees(n *Node, gone func(id string) bool) []cut {
	if n == nil || isLeaf(n) {
		return nil
	}
	var cuts []cut
	if allGone(n.First, gone) {
		cuts = append(cuts, cut{tree: n.First, anchor: firstOpen(n.Second, gone),
			side: opposite(n.Split)})
	} else {
		cuts = append(cuts, goneSubtrees(n.First, gone)...)
	}
	if allGone(n.Second, gone) {
		cuts = append(cuts, cut{tree: n.Second, anchor: firstOpen(n.First, gone), side: n.Split})
	} else {
		cuts = append(cuts, goneSubtrees(n.Second, gone)...)
	}
	return cuts
}

// allGone reports whether tree n has panes, and gone holds for each.
func allGone(n *Node, gone func(id string) bool) bool {
	ids := Leaves(n)
	return len(ids) > 0 && !slices.ContainsFunc(ids, func(id string) bool { return !gone(id) })
}

// firstOpen is tree n's first pane that gone does not hold for, "" for none.
func firstOpen(n *Node, gone func(id string) bool) string {
	for _, id := range Leaves(n) {
		if !gone(id) {
			return id
		}
	}
	return ""
}

// opposite is the side opposite split's: left for right, up for down.
func opposite(split string) string {
	if split == splitDown {
		return splitUp
	}
	return splitLeft
}

// pruneWindow is window w less the leaves drop holds. A tab left with none goes, and false means
// none is left.
func pruneWindow(w Window, drop func(id string) bool) (Window, bool) {
	var tabs []Tab
	for _, tab := range w.Tabs {
		if tab.Tree = prune(tab.Tree, drop); tab.Tree != nil {
			tabs = append(tabs, tab)
		}
	}
	return Window{Tabs: tabs}, len(tabs) > 0
}

// prune is tree n less the leaves drop holds, and those with no pane: a split left with one half
// becomes that half. nil means no leaf is left. n itself does not change.
func prune(n *Node, drop func(id string) bool) *Node {
	switch {
	case n == nil:
		return nil
	case isLeaf(n) && (n.Pane == "" || drop(n.Pane)):
		return nil
	case isLeaf(n):
		return n
	}
	first, second := prune(n.First, drop), prune(n.Second, drop)
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	}
	c := *n
	c.First, c.Second = first, second
	return &c
}
