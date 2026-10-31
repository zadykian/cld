package tabs

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/zadykian/cld/internal/ghostty"
)

// pace spaces apply.js's lines that start an ssh. UNVERIFIED until #177's check 15 (decision
// 59.12): that it keeps sshd's MaxStartups and a fail2ban quiet, and ws's lock waits short.
var pace = ghostty.Pace{Gap: 0.3, Every: 8, Pause: 2}

// The kinds of apply.js's builds, in the order a plan makes them. Tabs go into the windows of
// their anchors first, then windows, the front one last, so that it ends in front.
const (
	buildTab    = "tab"
	buildWindow = "window"
)

var buildRank = map[string]int{buildTab: 0, buildWindow: 1}

// Planned is a restore's plan: the script apply.js runs, and what the report needs of it.
type Planned struct {
	script ghostty.Plan
	typed  []joining  // the pane of each of script.Typed
	builds []building // the loss, leaves and bindings of each of script.Builds
	chosen []*Loss
	// uncached are the hosts whose terminfo Ghostty has not cached, in the order asked.
	uncached []string
}

// joining is a pane Ghostty kept that a restore joins to binding again.
type joining struct {
	pane    string
	binding Binding
	cached  bool // Ghostty has the host's terminfo
}

// building is a build's loss, its leaves' panes as last seen, and the binding of each of its leaves
// that types a line, by key.
type building struct {
	loss     *Loss
	copies   map[string]*Pane
	bindings map[string]Binding
}

// Script is the plan apply.js runs.
func (p *Planned) Script() *ghostty.Plan { return &p.script }

// planner makes a Planned against observation o, asking cached once per host.
type planner struct {
	out    *Planned
	o      *Observation
	cached func(host string) bool
	hosts  map[string]bool
}

// plan is the restore of chosen against record r after its merge of o, nil where Ghostty does not
// run (decision 59.6). Kept panes at their prompt with a Last are typed into, and chosen's windows
// rebuilt. cached tells a host's terminfo cached.
func plan(r *Record, o *Observation, chosen []*Loss, cached func(host string) bool) *Planned {
	if o == nil {
		o = &Observation{}
	}
	pl := &planner{o: o, cached: cached, hosts: map[string]bool{},
		out: &Planned{chosen: chosen, script: ghostty.Plan{Pace: pace, Typed: []ghostty.Typing{},
			Builds: []ghostty.Build{}}}}
	pl.typings(pl.inPlace(r))
	pl.order(pl.leavesOf(chosen))
	return pl.out
}

// keptPane is a pane Ghostty kept, a shell at its prompt, with the binding its Last names.
type keptPane struct {
	pane    string
	pid     int
	binding Binding
}

// inPlace are the panes of o's windows that the snapshot shows a shell at its prompt, with a Last
// in r that names a host.
func (pl *planner) inPlace(r *Record) []keptPane {
	var out []keptPane
	for t := range terminals(pl.o.Windows) {
		seen, p := pl.o.Seen[t.ID], r.Panes[t.ID]
		if seen.Kind != KindLocal || seen.Keep || p == nil || p.Last == nil || p.Last.Host == "" {
			continue
		}
		out = append(out, keptPane{pane: t.ID, pid: t.PID,
			binding: Binding{Host: p.Last.Host, Session: p.Last.Session}})
	}
	return out
}

// typings plans each kept pane: the line that joins its binding again, typed into it.
func (pl *planner) typings(kept []keptPane) {
	for _, k := range kept {
		j := joining{pane: k.pane, binding: k.binding, cached: pl.cachedHost(k.binding.Host)}
		pl.out.typed = append(pl.out.typed, j)
		pl.out.script.Typed = append(pl.out.script.Typed,
			ghostty.Typing{Pane: k.pane, PID: k.pid, Line: Line(j.binding, j.cached)})
	}
}

// pruned is a chosen loss, and its windows less the leaves a restore does not rebuild.
type pruned struct {
	loss    *Loss
	windows []Window
}

// leavesOf is chosen, each with its leaves to rebuild. A leaf open again, in a window of o or
// hidden, is not rebuilt. Nor is one that a newer loss of chosen holds too, which rebuilds it.
func (pl *planner) leavesOf(chosen []*Loss) []pruned {
	out := make([]pruned, len(chosen))
	taken := map[string]bool{}
	drop := func(id string) bool { return taken[id] || !pl.o.gone(id) }
	for i := len(chosen) - 1; i >= 0; i-- {
		out[i].loss = chosen[i]
		for _, w := range chosen[i].Windows {
			if kept, ok := pruneWindow(w, drop); ok {
				out[i].windows = append(out[i].windows, kept)
			}
		}
		for id := range windowCopies(out[i].windows) {
			taken[id] = true
		}
	}
	return out
}

// placement is a build before its keys: its loss, where it goes, and the tabs it makes.
type placement struct {
	loss  *Loss
	build ghostty.Build
	tabs  []Tab
}

// placeLoss is where loss l goes back, a build per window, back to front. A window's close or a
// quit opens windows. A tab opens at its index in the window holding its anchor. Where o does not
// show the anchor, it opens as a window.
func (pl *planner) placeLoss(l pruned) []placement {
	b := ghostty.Build{Kind: buildWindow}
	a := l.loss.Anchor
	switch {
	case l.loss.Cause == CauseTab && a != nil && pl.o.shows(a.Pane):
		b = ghostty.Build{Kind: buildTab, Anchor: a.Pane, Index: a.Index}
	case l.loss.Cause == CauseTab:
		b.Kind = buildTab
	}
	out := make([]placement, 0, len(l.windows))
	for _, w := range slices.Backward(l.windows) {
		out = append(out, placement{loss: l.loss, build: b, tabs: w.Tabs})
	}
	return out
}

// order makes lost's builds: tabs, then windows. Windows go oldest loss first, so that the newest
// loss's front window comes last and ends in front. Tabs go newest first, as an undo does: a
// tab's index is its place as it closed, among the tabs closed later. The tabs of one close go by
// index.
func (pl *planner) order(lost []pruned) {
	var all []placement
	for _, l := range lost {
		all = append(all, pl.placeLoss(l)...)
	}
	slices.SortStableFunc(all, func(a, b placement) int {
		kind := cmp.Compare(buildRank[a.build.Kind], buildRank[b.build.Kind])
		if a.build.Kind == buildTab {
			return cmp.Or(kind, b.loss.At.Compare(a.loss.At),
				cmp.Compare(a.build.Index, b.build.Index))
		}
		return cmp.Or(kind, a.loss.At.Compare(b.loss.At))
	})
	for _, p := range all {
		pl.key(p)
	}
}

// key adds p to the plan as apply.js's build, leaf n of build b keyed b<b>l<n> in leaf order.
// Each tab focuses its recorded leaf, else its first, and a window selects its recorded tab,
// else its first.
func (pl *planner) key(p placement) {
	k := &keyer{pl: pl, prefix: fmt.Sprintf("b%dl", len(pl.out.builds)),
		b: building{loss: p.loss, copies: map[string]*Pane{},
			bindings: map[string]Binding{}}}
	build := p.build
	selected := max(slices.IndexFunc(p.tabs, func(t Tab) bool { return t.Selected }), 0)
	for i, tab := range p.tabs {
		keys := map[string]string{}
		tree := k.node(tab.Tree, keys)
		focused, ok := keys[tab.Focused]
		if !ok {
			focused = firstKey(tree)
		}
		build.Tabs = append(build.Tabs, ghostty.TabBuild{Focused: focused, Tree: tree,
			Selected: i == selected})
	}
	pl.out.script.Builds = append(pl.out.script.Builds, build)
	pl.out.builds = append(pl.out.builds, k.b)
}

// notes are the plan's notes: the hosts whose terminfo Ghostty has not cached.
func (p *Planned) notes() []string {
	var notes []string
	for _, host := range p.uncached {
		notes = append(notes, uncachedNote(host))
	}
	return notes
}
