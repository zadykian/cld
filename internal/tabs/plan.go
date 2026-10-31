package tabs

import (
	"fmt"
	"slices"

	"github.com/zadykian/cld/internal/ghostty"
)

// pace spaces apply.js's lines that start an ssh. UNVERIFIED until #177's check 15 (decision
// 59.12): that it keeps sshd's MaxStartups and a fail2ban quiet, and ws's lock waits short.
var pace = ghostty.Pace{Gap: 0.3, Every: 8, Pause: 2}

// buildWindow is the kind of apply.js's build that opens a window.
const buildWindow = "window"

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
	pl.order(chosen)
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

// placement is a build before its keys: its loss, and the tabs it makes.
type placement struct {
	loss  *Loss
	build ghostty.Build
	tabs  []Tab
}

// order makes chosen's builds, a window each, back to front as the loss holds them. Oldest loss
// first, so that the newest loss's front window comes last and ends in front.
func (pl *planner) order(chosen []*Loss) {
	var all []placement
	for _, l := range chosen {
		for _, w := range slices.Backward(l.Windows) {
			all = append(all, placement{loss: l, build: ghostty.Build{Kind: buildWindow},
				tabs: w.Tabs})
		}
	}
	slices.SortStableFunc(all, func(a, b placement) int { return a.loss.At.Compare(b.loss.At) })
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
