package main

import (
	"cmp"
	"slices"

	"github.com/zadykian/cld/tests/internal/mac"
)

// apply applies apply.js's plan to m, a Ghostty that runs, in Go and not by apply.js's commands.
// It saves what it typed and built, waits as the apply delay file says, and prints the result,
// unless mode fails it. With partial, it fails as garbage does once it has typed and made the
// first build. With short, it prints what it did by then, as an apply.js that stopped early would.
func apply(dir string, m *mac.Model, p *plan, mode string) error {
	switch failure, canned := applyFailures[mode]; {
	case canned:
		return printJSON(failed(failure))
	case mode == "old":
		refuse("the failure mode old answers no apply: cld runs none on Ghostty 1.3.1")
	case p.Typed == nil:
		garbage("the plan's typed is no list")
	}
	a := &applier{m: m, paced: p.Pace != nil, off: mode == "off"}
	out := result{Typed: []typed{}, Builds: []built{}}
	for _, t := range p.Typed {
		out.Typed = append(out.Typed, a.typeLine(t))
	}
	for i, b := range p.Builds {
		if (mode == "partial" || mode == "short") && i == 1 {
			break
		}
		out.Builds = append(out.Builds, a.build(b))
	}
	if err := mac.Save(dir, m); err != nil {
		return err
	}
	if err := hold(dir, mac.ApplyDelayFile); err != nil {
		return err
	}
	switch {
	case p.Builds == nil:
		garbage("the plan's builds is no list")
	case mode == "partial":
		garbage("")
	}
	return printJSON(out)
}

// applier is apply.js at work on the model, and what it has paced and made so far.
type applier struct {
	m     *mac.Model
	paced bool // the plan has a pace
	off   bool // macos-applescript is off: Ghostty finds no terminal by its id, and makes none
	lines int  // the lines step let through
	made  int  // the terminals asked for, which m.FailAt counts
}

// typeLine types a line into a pane as apply.js does, Ctrl+U first and Enter last, once the pace
// has passed. It types nothing where the pane is gone, or busy: another group than the plan's is
// in front.
func (a *applier) typeLine(t typing) typed {
	if err := a.step(); err != nil {
		return typed{Pane: t.Pane, Error: err.Error()}
	}
	terminal, ok := a.m.Terminals[t.Pane]
	switch {
	case !ok || a.off || !slices.Contains(a.m.IDs(), t.Pane):
		return typed{Pane: t.Pane, Error: errNoObject.Error()}
	case terminal.PID != t.PID:
		return typed{Pane: t.Pane, Busy: true}
	}
	terminal.Typed = append(terminal.Typed, "^U", t.Line, "\r")
	return typed{Pane: t.Pane}
}

// step fails as apply.js's step does where the plan has no pace: before each line but the first.
func (a *applier) step() error {
	if a.lines > 0 && !a.paced {
		return errNoPace
	}
	a.lines++
	return nil
}

// build makes one build in the model, as apply.js does, and is its result: the terminals made by
// leaf key. A build that fails leaves what it made in place, as Ghostty keeps it.
func (a *applier) build(b build) built {
	keys := map[string]string{}
	out, err := built{}, error(errKind)
	switch b.Kind {
	case "window":
		err = a.buildWindow(b, keys)
	case "tab":
		out, err = a.buildTab(b, keys)
	case "pane":
		err = a.buildPane(b, keys)
	}
	if err != nil {
		return built{Keys: keys, Error: err.Error()}
	}
	out.Keys = keys
	return out
}

// buildWindow opens a window in front, then each of the build's tabs after the last, and selects
// the selected one, or the first.
func (a *applier) buildWindow(b build, keys map[string]string) error {
	if len(b.Tabs) == 0 {
		return errNoTab
	}
	var firsts []string
	for _, tab := range b.Tabs {
		id, err := a.open(tab)
		if err != nil {
			return err
		}
		w := 0
		if firsts == nil {
			a.m.Windows = slices.Insert(a.m.Windows, 0, mac.Window{})
		} else {
			w, _ = a.m.Locate(firsts[0])
		}
		firsts = append(firsts, id)
		a.newTab(w, id)
		if err := a.grow(tab.Tree, id, keys); err != nil {
			return err
		}
		a.focus(cmp.Or(keys[tab.Focused], id))
	}
	chosen := max(slices.IndexFunc(b.Tabs, func(tab tabBuild) bool { return tab.Selected }), 0)
	a.focus(cmp.Or(keys[b.Tabs[chosen].Focused], firsts[chosen]))
	return nil
}

// buildTab opens a tab in the window holding the anchor, and moves it to the build's index before
// it grows, as apply.js does. Its result holds the index read back, or, where no window holds the
// anchor, that the tab opened as a window.
func (a *applier) buildTab(b build, keys map[string]string) (built, error) {
	if len(b.Tabs) == 0 {
		return built{}, errNoTab
	}
	tab, home := b.Tabs[0], -1
	if b.Anchor != "" {
		home, _ = a.locate(b.Anchor)
	}
	id, err := a.open(tab)
	if err != nil {
		return built{}, err
	}
	out := built{Window: home < 0}
	if out.Window {
		a.m.Windows = slices.Insert(a.m.Windows, 0, mac.Window{})
		home = 0
	}
	a.newTab(home, id)
	if !out.Window {
		out.Index = a.moveTo(id, b.Index)
	}
	if err := a.grow(tab.Tree, id, keys); err != nil {
		return built{}, err
	}
	a.focus(cmp.Or(keys[tab.Focused], id))
	return out, nil
}

// buildPane splits the anchor's pane for the build's tree, then focuses its focused terminal.
func (a *applier) buildPane(b build, keys map[string]string) error {
	if len(b.Tabs) == 0 {
		return errNoTab
	}
	tab := b.Tabs[0]
	first, err := firstLeaf(tab.Tree)
	if err != nil {
		return err
	}
	id, err := a.splitAt(b.Anchor, b.Split, first)
	if err != nil {
		return err
	}
	if err := a.grow(tab.Tree, id, keys); err != nil {
		return err
	}
	a.focus(cmp.Or(keys[tab.Focused], id))
	return nil
}
