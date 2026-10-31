package tests

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A tab's tree from one tick to the next (decision 59.8): kept through zooms and closes. Each pane
// is a real pty, sized as in tabs_layout_test.go.

// A tree stays where a zoom gives the sizes no tree. A pane that closes leaves the rest of the
// tree, as Ghostty removes it, zoomed or not.
func TestTabsLayoutFallback(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	p := addPtyPanes(t, m, "a", "b", "c")
	tree := cut("right", 0.5, leaf("a"), cut("down", 0.5, leaf("b"), leaf("c")))
	p.size(retina.sizes(tree))
	m.Windows = []mac.Window{windowOf(p.tab(tree))}
	checkTab(t, "seen", tick(t, s, m), p.names, "*right(a,down(b,c))/inferred", 0.5, 0.5)
	p.size(retina.sizes(leaf("b")))
	m.Windows = []mac.Window{windowOf(p.tabAt(tree, "b"))}
	checkTab(t, "b zoomed", tick(t, s, m), p.names, "*right(a,down(b,c))/inferred", 0.5, 0.5)
	m.Windows = []mac.Window{windowOf(p.tab(cut("down", 0.5, leaf("b"), leaf("c"))))}
	checkRecord(t, "a closed, b zoomed", tick(t, s, m), p.names, "*down(b,c)/inferred",
		"pane left of b: a/made", "")
}
