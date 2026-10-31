package main

import (
	"slices"

	"github.com/zadykian/cld/tests/internal/mac"
)

// How the fake Ghostty makes an apply's terminals, tabs and windows, and where it puts them.

// grow builds a tree at the terminal id, made for its first leaf, as apply.js's grow does. The
// key goes in first, so a split that fails still reports that terminal. A split makes its second
// half's first terminal, then each half grows from its own.
func (a *applier) grow(n *node, id string, keys map[string]string) error {
	first, err := firstLeaf(n)
	if err != nil {
		return err
	}
	keys[first.Key] = id
	if n.Leaf != nil {
		return nil
	}
	second, err := firstLeaf(n.Second)
	if err != nil {
		return err
	}
	made, err := a.splitAt(id, n.Split, second)
	if err != nil {
		return err
	}
	if err := a.grow(n.First, id, keys); err != nil {
		return err
	}
	return a.grow(n.Second, made, keys)
}

// open makes the terminal of a new window or tab, for the tab's first leaf, and is its id. The
// caller puts it in its window.
func (a *applier) open(tab tabBuild) (string, error) {
	l, err := firstLeaf(tab.Tree)
	if err != nil {
		return "", err
	}
	if err := a.pace(l); err != nil {
		return "", err
	}
	if a.off {
		return "", errOff
	}
	return a.add(l, errNoTerminal)
}

// splitAt splits the terminal id towards direction for a leaf, as Ghostty's split does, and is the
// new terminal's id. The new terminal takes the half that direction names, and the focus.
func (a *applier) splitAt(id, direction string, l *leaf) (string, error) {
	if err := a.pace(l); err != nil {
		return "", err
	}
	w, t := a.locate(id)
	half, known := halves[direction]
	switch {
	case w < 0:
		return "", errNoObject
	case !known:
		return "", errDirection
	}
	made, err := a.add(l, errSplit)
	if err != nil {
		return "", err
	}
	tab := &a.m.Windows[w].Tabs[t]
	tab.Tree = replace(tab.Tree, id, func(old *mac.Node) *mac.Node {
		if half.first {
			return &mac.Node{Split: half.split, First: &mac.Node{Pane: made}, Second: old}
		}
		return &mac.Node{Split: half.split, First: old, Second: &mac.Node{Pane: made}}
	})
	tab.Focused = made
	return made, nil
}

// pace steps before a leaf's line, as apply.js's open and splitAt do.
func (a *applier) pace(l *leaf) error {
	if l.Line == "" {
		return nil
	}
	return a.step()
}

// add makes a leaf's terminal, in its directory with its line and a newline as the initial input,
// as apply.js's surface configuration gives them. The model's FailAt-th fails with failure.
func (a *applier) add(l *leaf, failure applyError) (string, error) {
	if a.made++; a.made == a.m.FailAt {
		return "", failure
	}
	input := ""
	if l.Line != "" {
		input = l.Line + "\n"
	}
	return a.m.AddTerminal(l.Cwd, input), nil
}

// locate is the window and the tab holding the terminal id, where Ghostty finds it by its id.
func (a *applier) locate(id string) (window, tab int) {
	if a.off {
		return -1, -1
	}
	return a.m.Locate(id)
}

// newTab puts the terminal id's tab after the selected tab of window w, or last, as Ghostty's new
// tab does, and selects it, its window in front.
func (a *applier) newTab(w int, id string) {
	tabs := a.m.Windows[w].Tabs
	at := slices.IndexFunc(tabs, func(tab mac.Tab) bool { return tab.Selected }) + 1
	if at == 0 {
		at = len(tabs)
	}
	a.m.Windows[w].Tabs = slices.Insert(tabs, at, mac.Tab{Focused: id, Tree: &mac.Node{Pane: id}})
	front(a.m, w, at)
}

// moveTo moves the terminal id's tab to index, where that is above 0, clamped as macOS clamps
// move_tab. It returns the index read back, from 1. A move the model drops goes once more, as
// apply.js sends it.
func (a *applier) moveTo(id string, index int) int {
	w, t := a.m.Locate(id)
	target := min(index, len(a.m.Windows[w].Tabs))
	for attempt := 0; target > 0 && target != t+1 && attempt < 2; attempt++ {
		if a.m.DropMoves > 0 {
			a.m.DropMoves--
			continue
		}
		tab := a.m.Windows[w].Tabs[t]
		rest := slices.Delete(a.m.Windows[w].Tabs, t, t+1)
		t = target - 1
		a.m.Windows[w].Tabs = slices.Insert(rest, t, tab)
	}
	return t + 1
}

// focus focuses the terminal id as Ghostty's focus does, selecting its tab, its window in front.
func (a *applier) focus(id string) {
	if w, t := a.m.Locate(id); w >= 0 {
		a.m.Windows[w].Tabs[t].Focused = id
		front(a.m, w, t)
	}
}

// replace is the tree with the leaf of the terminal id replaced by what with makes of it.
func replace(n *mac.Node, id string, with func(*mac.Node) *mac.Node) *mac.Node {
	switch {
	case n == nil || (n.Pane != "" && n.Pane != id):
		return n
	case n.Pane == id:
		return with(n)
	}
	return &mac.Node{Split: n.Split, First: replace(n.First, id, with),
		Second: replace(n.Second, id, with)}
}

// front selects the tab t of the window w, and moves that window to the front.
func front(m *mac.Model, w, t int) {
	window := m.Windows[w]
	for i := range window.Tabs {
		window.Tabs[i].Selected = i == t
	}
	m.Windows = slices.Insert(slices.Delete(m.Windows, w, w+1), 0, window)
}
