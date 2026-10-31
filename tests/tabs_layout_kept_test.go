package tests

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/zadykian/cld/internal/ghostty"
	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A tab's tree from one tick to the next (decision 59.8): kept through zooms and closes, and
// revised where the sizes fit another tree. Each pane is a real pty, sized as in
// tabs_layout_test.go.

// writeMade writes s's record of m: one window of one tab, tree, which cld made. Its leaves name
// p's panes, and its ratios are unknown.
func writeMade(t *testing.T, s *sandbox.Sandbox, m *mac.Model, p *ptyPanes, tree *tabs.Node) {
	t.Helper()
	panes := map[string]*tabs.Pane{}
	var node func(n *tabs.Node) *tabs.Node
	node = func(n *tabs.Node) *tabs.Node {
		if n.Pane != "" {
			panes[p.ids[n.Pane]] = &tabs.Pane{Kind: "local"}
			return leaf(p.ids[n.Pane])
		}
		return split(n.Split, node(n.First), node(n.Second))
	}
	writeRecord(t, s, recordOf(m, panes, tabs.Window{Tabs: []tabs.Tab{{Selected: true,
		Shape: "made", Tree: node(tree)}}}))
}

// A tab whose sizes fit no tree is a row: a resize read half done. A row tries its sizes again at
// the next tick. A tree stays where the sizes fit none, as with this zoom. A pane that closes
// leaves the rest of the tree, as Ghostty removes it, zoomed or not.
func TestTabsLayoutFallback(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	p := addPtyPanes(t, m, "a", "b", "c")
	tree := cut("right", 0.5, leaf("a"), cut("down", 0.5, leaf("b"), leaf("c")))
	wider := room{w: 1600, h: 800, scale: 2, pad: 2, cellW: 16, cellH: 34}
	p.size(retina.sizes(tree))
	resized := wider.sizes(tree)
	delete(resized, "c")
	p.size(resized)
	m.Windows = []mac.Window{windowOf(p.tab(tree))}
	checkTab(t, "a resize half read", tick(t, s, m), p.names, "*right(a,right(b,c))/flat", 0, 0)
	p.size(wider.sizes(tree))
	checkTab(t, "the resize read", tick(t, s, m), p.names, "*right(a,down(b,c))/inferred",
		0.5, 0.5)
	p.size(wider.sizes(leaf("b")))
	m.Windows = []mac.Window{windowOf(p.tabAt(tree, "b"))}
	checkTab(t, "b zoomed", tick(t, s, m), p.names, "*right(a,down(b,c))/inferred", 0.5, 0.5)
	m.Windows = []mac.Window{windowOf(p.tab(cut("down", 0.5, leaf("b"), leaf("c"))))}
	checkRecord(t, "a closed, b zoomed", tick(t, s, m), p.names, "*down(b,c)/inferred",
		"pane left of b: a/made", "")
}

// A tab first seen zoomed takes the tree its stale sizes fit, until the zoom ends.
func TestTabsLayoutZoomedAtFirst(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	p := addPtyPanes(t, m, "a", "b", "c")
	tree := cut("right", 0.5, leaf("a"), cut("down", 0.5, leaf("b"), leaf("c")))
	p.size(retina.sizes(tree))
	p.size(retina.sizes(leaf("a")))
	m.Windows = []mac.Window{windowOf(p.tab(tree))}
	checkTab(t, "a zoomed", tick(t, s, m), p.names, "*down(a,right(b,c))/inferred", 0.67, 0.5)
	p.size(retina.sizes(tree))
	checkTab(t, "unzoomed", tick(t, s, m), p.names, "*right(a,down(b,c))/inferred", 0.5, 0.5)
}

// Ghostty's Move Split can change a tree and keep its panes' order, and focuses the pane it moved,
// here the last. The sizes then give the new tree, unless they fit one cld made: see
// TestTabsLayoutMade. A moved pane as large as the rest is no zoom.
func TestTabsLayoutMoved(t *testing.T) {
	t.Parallel()
	a, b, c := leaf("a"), leaf("b"), leaf("c")
	for _, test := range []struct {
		name          string
		made          bool
		before, after *tabs.Node
		want          string
		ratios        []float64
	}{
		{"beside a to below it", false, cut("right", 0.5, a, b), cut("down", 0.5, a, b),
			"*down(a,b)/inferred", []float64{0.5}},
		{"made, below b to beside it", true, cut("right", 0.5, a, cut("down", 0.5, b, c)),
			cut("right", 0.5, a, cut("right", 0.5, b, c)), "*right(a,right(b,c))/inferred",
			[]float64{0.5, 0.5}},
		{"below b to beside it, then resized", false, cut("right", 0.5, a,
			cut("down", 0.5, b, c)), cut("right", 0.5, cut("right", 0.5, a, b), c),
			"*right(a,right(b,c))/inferred", []float64{0.25, 0.33}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			p := addPtyPanes(t, m, tabs.Leaves(test.before)...)
			p.size(retina.sizes(test.before))
			m.Windows = []mac.Window{windowOf(p.tab(test.before))}
			if test.made {
				writeMade(t, s, m, p, test.before)
			} else {
				tick(t, s, m)
			}
			p.size(retina.sizes(test.after))
			leaves := tabs.Leaves(test.after)
			m.Windows = []mac.Window{windowOf(p.tabAt(test.after, leaves[len(leaves)-1]))}
			checkTab(t, test.name, tick(t, s, m), p.names, test.want, test.ratios...)
		})
	}
}

// A tree cld built stays while its panes do and their sizes fit it, even where they fit another
// too, and its ratios stay unknown. A new split makes the tab's tree from the sizes.
func TestTabsLayoutMade(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	p := addPtyPanes(t, m, "a", "b", "c")
	writeMade(t, s, m, p, cut("down", 0, leaf("a"), leaf("b")))
	split := cut("down", 0.7, leaf("a"), leaf("b"))
	p.size(retina.sizes(split))
	m.Windows = []mac.Window{windowOf(p.tab(split))}
	checkTab(t, "made", tick(t, s, m), p.names, "*down(a,b)/made", 0)
	beside := cut("right", 0.5, leaf("a"), leaf("b"))
	p.size(retina.sizes(beside))
	m.Windows = []mac.Window{windowOf(p.tab(beside))}
	checkTab(t, "made, moved side by side", tick(t, s, m), p.names, "*down(a,b)/made", 0)
	split = cut("down", 0.7, leaf("a"), cut("right", 0.5, leaf("b"), leaf("c")))
	p.size(retina.sizes(split))
	m.Windows = []mac.Window{windowOf(p.tab(split))}
	checkTab(t, "c split from b", tick(t, s, m), p.names, "*down(a,right(b,c))/inferred",
		0.7, 0.5)
}

// A closed window goes back with the tree its panes' sizes gave it. The plan apply.js gets
// carries that tree, and the record holds it as made once Ghostty has built it.
func TestTabsLayoutRestore(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	p := addPtyPanes(t, m, "a", "b", "c", "d")
	tree := cut("right", 0.5, leaf("b"), cut("down", 0.5, leaf("c"), leaf("d")))
	p.size(retina.sizes(tree))
	p.size(retina.sizes(leaf("a")))
	m.Windows = []mac.Window{windowOf(p.tab(leaf("a"))), windowOf(p.tab(tree))}
	tick(t, s, m)
	m.Windows = m.Windows[:1]
	endLogins(m, p.ids["b"], p.ids["c"], p.ids["d"])
	r := tick(t, s, m)
	checkRecord(t, "closed", r, p.names, "*a/made", "", "closed: *right(b,down(c,d))/inferred")
	if len(r.Losses) != 1 {
		return
	}
	checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0,
		"Reopened a window of 1 tab and 3 panes, closed at "+clockAt(r.Losses[0].At)+"\n",
		noWatcher)
	if got := planTree(t, s); got != "right(b0l0,down(b0l1,b0l2))" {
		t.Errorf("apply.js got the tree %s, want right(b0l0,down(b0l1,b0l2))", got)
	}
	names := p.names
	for _, w := range loadModel(t, s).Windows {
		for i, id := range w.Tabs[0].Tree.Leaves() {
			if id != p.ids["a"] {
				names[id] = string(rune('b' + i))
			}
		}
	}
	const want = "*right(b,down(c,d))/made | *a/made"
	if got := drawWindows(readRecord(t, s).Windows, names); got != want {
		t.Errorf("the record holds %s, want %s", got, want)
	}
}

// planTree draws the tree of the one build in the plan s's restore gave apply.js, by its leaves'
// keys.
func planTree(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	calls := s.OsascriptCalls()
	i := slices.IndexFunc(calls, func(c mac.OsascriptCall) bool { return c.Script == "apply" })
	if i < 0 {
		t.Fatalf("osascript's calls %+v, want an apply", calls)
	}
	var plan ghostty.Plan
	if err := json.Unmarshal(calls[i].Input, &plan); err != nil {
		t.Fatalf("apply's plan %s: %v", calls[i].Input, err)
	}
	if len(plan.Builds) != 1 || len(plan.Builds[0].Tabs) != 1 {
		t.Fatalf("apply's plan %s, want one build of one tab", calls[i].Input)
	}
	var draw func(n *ghostty.Node) string
	draw = func(n *ghostty.Node) string {
		if n.Leaf != nil {
			return n.Leaf.Key
		}
		return fmt.Sprintf("%s(%s,%s)", n.Split, draw(n.First), draw(n.Second))
	}
	return draw(plan.Builds[0].Tabs[0].Tree)
}
