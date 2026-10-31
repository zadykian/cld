package tests

import (
	"slices"

	"github.com/zadykian/cld/tests/internal/mac"
)

// tabOf is a tab of the panes ids in a row, left to right, the first focused.
func tabOf(ids ...string) mac.Tab {
	var tree *mac.Node
	for _, id := range slices.Backward(ids) {
		leaf := &mac.Node{Pane: id}
		if tree == nil {
			tree = leaf
		} else {
			tree = &mac.Node{Split: "right", First: leaf, Second: tree}
		}
	}
	return mac.Tab{Focused: ids[0], Tree: tree}
}

// windowOf is a window of tabs, the first selected.
func windowOf(tabs ...mac.Tab) mac.Window {
	tabs[0].Selected = true
	return mac.Window{Tabs: tabs}
}
