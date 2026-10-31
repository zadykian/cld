package tests

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A tab's tree from one tick to the next (decision 59.8): kept through zooms and closes. Each pane
// is a real pty, sized as in tabs_layout_test.go.

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
