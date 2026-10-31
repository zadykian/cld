package tests

import (
	"math"
	"slices"
	"testing"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A tab's split tree from its panes' sizes (decision 59.8). Ghostty's dictionary gives a tab's
// panes left or top first, and no split's direction. Each pane here is a real pty with the winsize
// Ghostty would give it, which the dry-run's merge reads as a tick's does.

// room is the space Ghostty divides among a tab's panes: its points, the display's scale and a
// cell's pixels. pad is window-padding-x and -y, in points a side, and balance
// window-padding-balance.
type room struct {
	w, h, scale, pad float64
	cellW, cellH     int
	balance          bool
}

// retina is a tab of 1200x800 points on a display of scale 2, a cell of 16x34 pixels, and
// Ghostty's default padding.
var retina = room{w: 1200, h: 800, scale: 2, pad: 2, cellW: 16, cellH: 34}

// cut is a split of first and second in direction, first's share of the room ratio.
func cut(direction string, ratio float64, first, second *tabs.Node) *tabs.Node {
	return &tabs.Node{Split: direction, Ratio: ratio, First: first, Second: second}
}

// sizes is the winsize Ghostty gives each pane of tree, named by its leaves, in the room.
func (r room) sizes(tree *tabs.Node) map[string]tabs.Size {
	sizes := map[string]tabs.Size{}
	r.divide(tree, r.w, r.h, sizes)
	return sizes
}

// divide adds to sizes those of tree's panes in w x h points. A split's first part takes its
// share less half the divider's point, in whole points, and the second the rest less the other
// half (SplitView.swift).
func (r room) divide(tree *tabs.Node, w, h float64, sizes map[string]tabs.Size) {
	switch tree.Split {
	case "":
		sizes[tree.Pane] = r.winsize(int(w*r.scale), int(h*r.scale))
	case "right":
		first := math.Floor(w*tree.Ratio - 0.5)
		r.divide(tree.First, first, h, sizes)
		r.divide(tree.Second, w-first-0.5, h, sizes)
	default:
		first := math.Floor(h*tree.Ratio - 0.5)
		r.divide(tree.First, w, first, sizes)
		r.divide(tree.Second, w, h-first-0.5, sizes)
	}
}

// winsize is a pty's in a surface of width x height pixels: the surface less its padding. A
// balanced padding grows to leave whole cells, half on each side (src/renderer/size.zig).
func (r room) winsize(width, height int) tabs.Size {
	pad := int(2 * r.pad * r.scale)
	size := tabs.Size{Width: width - pad, Height: height - pad}
	size.Cols, size.Rows = size.Width/r.cellW, size.Height/r.cellH
	if r.balance {
		size.Width = width - (width-size.Cols*r.cellW)/2*2
		size.Height = height - (height-size.Rows*r.cellH)/2*2
	}
	return size
}

// ptyPanes are a test's panes by name, each a shell at its prompt on a pty of the test's, and
// their names by their ids, for drawWindows.
type ptyPanes struct {
	ids, names map[string]string
	ptys       map[string]*sandbox.Pty
}

// addPtyPanes adds to m a pane for each of names, in that order.
func addPtyPanes(t *testing.T, m *mac.Model, names ...string) *ptyPanes {
	t.Helper()
	p := &ptyPanes{ids: map[string]string{}, names: map[string]string{},
		ptys: map[string]*sandbox.Pty{}}
	for _, name := range names {
		p.ids[name] = addPane(m, "~")
		p.names[p.ids[name]], p.ptys[name] = name, ptyPane(t, m, p.ids[name])
	}
	return p
}

// size gives each pane named in sizes its size there.
func (p *ptyPanes) size(sizes map[string]tabs.Size) {
	for name, size := range sizes {
		p.ptys[name].SetSize(size.Rows, size.Cols, size.Width, size.Height)
	}
}

// tab is the model's tab of tree, whose leaves name the panes; the first is focused.
func (p *ptyPanes) tab(tree *tabs.Node) mac.Tab {
	var node func(n *tabs.Node) *mac.Node
	node = func(n *tabs.Node) *mac.Node {
		if n.Pane != "" {
			return &mac.Node{Pane: p.ids[n.Pane]}
		}
		return &mac.Node{Split: n.Split, First: node(n.First), Second: node(n.Second)}
	}
	return mac.Tab{Focused: p.ids[tabs.Leaves(tree)[0]], Tree: node(tree)}
}

// ratios are tree's splits' ratios, first the outer split's, then each half's.
func ratios(tree *tabs.Node) []float64 {
	if tree == nil || tree.Pane != "" {
		return nil
	}
	return slices.Concat([]float64{tree.Ratio}, ratios(tree.First), ratios(tree.Second))
}

// checkTab reports what a step left, unless r holds one window of one tab, drawn as want, whose
// splits' ratios are wantRatios.
func checkTab(t *testing.T, step string, r *tabs.Record, names map[string]string, want string,
	wantRatios ...float64,
) {
	t.Helper()
	checkRecord(t, step, r, names, want, "", "")
	if len(r.Windows) == 1 && len(r.Windows[0].Tabs) == 1 {
		if got := ratios(r.Windows[0].Tabs[0].Tree); !slices.Equal(got, wantRatios) {
			t.Errorf("%s: the ratios are %v, want %v", step, got, wantRatios)
		}
	}
}

// layoutCase is a tab in a room, drawn by its tree, and the tree and ratios a tick records of it.
type layoutCase struct {
	name   string
	room   room
	tree   *tabs.Node
	want   string
	ratios []float64
}

// runLayout runs each case in a sandbox of its own: one tick of its tab, sized as its room has it.
func runLayout(t *testing.T, cases []layoutCase) {
	t.Helper()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			p := addPtyPanes(t, m, tabs.Leaves(test.tree)...)
			p.size(test.room.sizes(test.tree))
			m.Windows = []mac.Window{windowOf(p.tab(test.tree))}
			checkTab(t, test.name, tick(t, s, m), p.names, test.want, test.ratios...)
		})
	}
}

// A tab's splits come back as a row for now: the sizes of its panes are read, and merge.go does
// not use them yet.
func TestTabsLayout(t *testing.T) {
	t.Parallel()
	a, b, c := leaf("a"), leaf("b"), leaf("c")
	runLayout(t, []layoutCase{
		{"three in a row", retina, cut("right", 1.0/3, a, cut("right", 0.5, b, c)),
			"*right(a,right(b,c))/flat", []float64{0, 0}},
	})
}
