package tabs

import "math"

// padSlack is the pixels each pane takes off a run's size along an axis, beyond one pane. Padding
// takes 8, 2 points a side at a scale of 2, and the divider 1, half a point. Rounding and balanced
// padding take the other 7 (renderer/size.zig, Config.zig, SplitView.swift). UNVERIFIED until
// #177's check 10 (decision 59.12): the padding, the divider and the scale on a Mac.
const padSlack = 16

// wantAspect is the width over height a tab's tree is taken to have where its sizes fit trees of
// other shapes: the one nearest it wins (decision 59.8).
const wantAspect = 1.4

// maxParses bounds the parses kept of a run of leaves. A run of n equal panes has one per way of
// tiling them: 8 at most for n up to 24.
const maxParses = 8

// parse is a tree of a run of a tab's leaves, with its size in pixels and the panes it spans,
// across then down. Along a split both sum; beside it, they are those of one half. An open size
// leaves out a pane whose size is unknown.
type parse struct {
	tree *Node
	size [2]int
	n    [2]int
	open [2]bool
}

// region is the parses of a run of leaves, and its leaves' largest cell in pixels.
type region struct {
	parses []parse
	cell   [2]float64
}

// search finds a tab's tree: it holds the tab's leaves, their sizes, and the regions parsed so far
// by their first and last leaf.
type search struct {
	ids     []string
	sizes   []Size
	regions map[[2]int]*region
}

// infer is the split tree of a tab of the panes ids, left or top first, from their sizes in sizes
// (decision 59.8). False means a pane lacks a size, or no tree fits them.
func infer(ids []string, sizes map[string]Size) (*Node, bool) {
	best, _, ok := bestParse(ids, sizes)
	return best.tree, ok
}

// revise is the tree the sizes give a tab of the panes ids whose recorded tree is base. Nil means
// base stays: the sizes fit no tree, or they fit base, at the size of the tree they give
// (decision 59.8).
func revise(base *Node, ids []string, sizes map[string]Size) *Node {
	best, cell, ok := bestParse(ids, sizes)
	if !ok {
		return nil
	}
	if got, ok := fit(base, sizes, cell); ok && same(got, best, cell) {
		return nil
	}
	return best.tree
}

// bestParse is the parse of the panes ids nearest wantAspect, and their largest cell. False means
// a pane lacks a size, or no tree fits them.
func bestParse(ids []string, sizes map[string]Size) (parse, [2]float64, bool) {
	s := &search{ids: ids, regions: map[[2]int]*region{}}
	for _, id := range ids {
		size, ok := sizes[id]
		if !ok || !valid(size) {
			return parse{}, [2]float64{}, false
		}
		s.sizes = append(s.sizes, size)
	}
	all := s.region(0, len(ids)-1)
	best, ok := nearest(all.parses)
	return best, all.cell, ok
}

// region is the region of leaves a to b. Its parses are a leaf, or its splits into two runs: side
// by side ones first, then stacked ones, each from the shortest first run on.
func (s *search) region(a, b int) *region {
	if reg, ok := s.regions[[2]int{a, b}]; ok {
		return reg
	}
	reg := &region{}
	for i := a; i <= b; i++ {
		size := s.sizes[i]
		reg.cell[0] = max(reg.cell[0], float64(size.Width)/float64(size.Cols))
		reg.cell[1] = max(reg.cell[1], float64(size.Height)/float64(size.Rows))
	}
	if a == b {
		reg.parses = []parse{leafParse(s.ids[a], s.sizes[a])}
	}
	for _, direction := range []string{splitRight, splitDown} {
		for k := a; k < b; k++ {
			s.try(reg, direction, a, k, b)
		}
	}
	s.regions[[2]int{a, b}] = reg
	return reg
}

// try adds to reg each parse of leaves a to k split in direction from one of k+1 to b that meets
// it.
func (s *search) try(reg *region, direction string, a, k, b int) {
	for _, p := range s.region(a, k).parses {
		for _, q := range s.region(k+1, b).parses {
			if joint, ok := join(direction, p, q, reg.cell); ok {
				joint.tree = joined(direction, p, q)
				reg.add(joint)
			}
		}
	}
}

// join is the size of p and q split in direction, false where they do not meet across it. Along
// it their sizes and counts sum; across, the half spanning more panes gives them, or the first. A
// half open across has the rest of the other's size, which must pass what near allows.
func join(direction string, p, q parse, cell [2]float64) (parse, bool) {
	i := axis(direction)
	j := 1 - i
	var joint parse
	joint.size[i], joint.n[i] = p.size[i]+q.size[i], p.n[i]+q.n[i]
	joint.open[i] = p.open[i] || q.open[i]
	switch {
	case p.open[j] || q.open[j]:
		closed, part := q, p
		if q.open[j] {
			closed, part = p, q
		}
		if !beyond(closed.size[j], part.size[j], closed.n[j], part.n[j], cell[j]) {
			return parse{}, false
		}
		joint.size[j], joint.n[j] = closed.size[j], max(p.n[j], q.n[j])
	case !near(p.size[j], q.size[j], p.n[j], q.n[j], cell[j]):
		return parse{}, false
	default:
		joint.size[j], joint.n[j] = deeper(p.size[j], p.n[j], q.size[j], q.n[j])
	}
	return joint, true
}

// fit is tree n laid out on sizes, false where a pane lacks a size or two halves do not meet.
func fit(n *Node, sizes map[string]Size, cell [2]float64) (parse, bool) {
	if isLeaf(n) {
		size, ok := sizes[n.Pane]
		return leafParse(n.Pane, size), ok && valid(size)
	}
	p, ok := fit(n.First, sizes, cell)
	if !ok {
		return parse{}, false
	}
	q, ok := fit(n.Second, sizes, cell)
	if !ok {
		return parse{}, false
	}
	return join(n.Split, p, q, cell)
}

// near reports whether sizes x and y, spanning nx and ny panes, are one length: within a cell,
// and padSlack for each pane one spans more than the other.
func near(x, y, nx, ny int, cell float64) bool {
	return math.Abs(float64(x-y)) <= cell+float64(abs(nx-ny))*padSlack
}

// beyond reports whether size x, spanning nx panes, passes y, spanning ny, by more than near
// allows.
func beyond(x, y, nx, ny int, cell float64) bool {
	return float64(x-y) > cell+float64(abs(nx-ny))*padSlack
}

// same reports whether parses p and q are of one size, as near tells along each axis.
func same(p, q parse, cell [2]float64) bool {
	return near(p.size[0], q.size[0], p.n[0], q.n[0], cell[0]) &&
		near(p.size[1], q.size[1], p.n[1], q.n[1], cell[1])
}

// deeper is the size and count of the half that spans more panes, or of the first half. So a
// split's size across its direction goes with its count.
func deeper(x, nx, y, ny int) (int, int) {
	if ny > nx {
		return y, ny
	}
	return x, nx
}

// joined is p and q split in direction. Its ratio is p's share of their sizes along it, rounded
// to two places.
func joined(direction string, p, q parse) *Node {
	i := axis(direction)
	ratio := math.Round(float64(p.size[i])/float64(p.size[i]+q.size[i])*100) / 100
	return &Node{Split: direction, Ratio: ratio, First: p.tree, Second: q.tree}
}

// add keeps parse p, unless reg holds maxParses or one of p's size over as many panes. So of two
// trees of one size, such as a grid's, the first found stays: side by side before stacked.
func (reg *region) add(p parse) {
	if len(reg.parses) == maxParses {
		return
	}
	for _, q := range reg.parses {
		if q.n == p.n && same(q, p, reg.cell) {
			return
		}
	}
	reg.parses = append(reg.parses, p)
}

// nearest is the parse whose width over height is nearest wantAspect. False means there is none.
func nearest(parses []parse) (parse, bool) {
	var best parse
	gap := math.Inf(1)
	for _, p := range parses {
		if d := math.Abs(math.Log(float64(p.size[0]) / float64(p.size[1]) / wantAspect)); d < gap {
			best, gap = p, d
		}
	}
	return best, best.tree != nil
}

// leafParse is pane id's parse, of its size.
func leafParse(id string, size Size) parse {
	return parse{tree: &Node{Pane: id}, size: [2]int{size.Width, size.Height}, n: [2]int{1, 1}}
}

// valid reports whether size has cells and pixels, as Ghostty's winsize of a pane has.
func valid(size Size) bool {
	return size.Cols > 0 && size.Rows > 0 && size.Width > 0 && size.Height > 0
}

// axis is direction's index in a parse's sizes: 0 for right, across, and 1 for down.
func axis(direction string) int {
	if direction == splitRight {
		return 0
	}
	return 1
}

// abs is n's absolute value.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
