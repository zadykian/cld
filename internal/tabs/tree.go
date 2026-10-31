package tabs

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
