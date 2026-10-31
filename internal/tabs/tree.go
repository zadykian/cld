package tabs

import "slices"

// splitRight is Ghostty's split word for a split whose halves lie side by side.
const splitRight = "right"

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

// sameLeaves reports whether tree n's leaves are ids, in that order.
func sameLeaves(n *Node, ids []string) bool { return slices.Equal(Leaves(n), ids) }

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
	return &c
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
