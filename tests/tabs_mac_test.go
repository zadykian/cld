package tests

import (
	"slices"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
)

// The model Mac that the tabs_* files give cld restore tabs: a Ghostty and its windows.

// ghosttyStart is when the model Ghostty started, as its ps row says: a day no test runs on, so
// that the dry-run writes it with its date.
var ghosttyStart = time.Date(2025, time.January, 2, 9, 12, 0, 0, time.UTC)

// ghosttyAt is a running Ghostty 1.4.0 with no window, of pid, which started at start.
//
//nolint:unparam // the tests that restart Ghostty pass other pids
func ghosttyAt(pid int, start time.Time) *mac.Model {
	return &mac.Model{Running: true, PID: pid, Launched: float64(start.Unix()),
		Executable: ghosttyExecutable, Version: "1.4.0", Terminals: map[string]*mac.Terminal{},
		Processes: []mac.Process{{PID: pid, PPID: 1, PGID: pid, Start: start,
			Args: []string{ghosttyExecutable}}}}
}

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

// addPane adds a pane to m, in no window yet, titled title, and returns its terminal's id.
func addPane(m *mac.Model, title string) string {
	id := m.AddTerminal("/Users/me", "")
	m.Terminals[id].Name = title
	return id
}
