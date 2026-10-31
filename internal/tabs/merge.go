package tabs

import (
	"iter"
	"maps"
	"slices"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Merge folds o into r as a tick does (decision 59.4). A run it has not seen before takes the
// place of the one before. Then the pending losses o shows are undone, the panes o shows update,
// those it shows nowhere go pending, the windows follow o's, and pending losses whose logins are
// gone count.
func Merge(r *Record, o *Observation) {
	switch {
	case r.Run.ID != o.Run:
		r.Run = Run{ID: o.Run, Version: o.Version}
	case r.Run.Version == "":
		r.Run.Version = o.Version
	}
	undone := undo(r, o)
	mergePanes(r, o)
	vanish(r, o)
	mergeWindows(r, o, undone)
	settlePending(r, o.Procs)
}

// shows reports whether a window of o holds terminal id.
func (o *Observation) shows(id string) bool {
	_, ok := o.Seen[id]
	return ok
}

// gone reports whether o holds terminal id nowhere: in no window, and not hidden either.
// UNVERIFIED until #177's check 7 (decision 59.12): that the quick terminal, and a minimized,
// hidden or other Space's window, are hidden: in the app-wide terminals alone.
func (o *Observation) gone(id string) bool { return !o.shows(id) && !o.Hidden[id] }

// terminals are the terminals of windows, front to back, in tab-bar and leaf order.
func terminals(windows []ghostty.Window) iter.Seq[ghostty.Terminal] {
	return func(yield func(ghostty.Terminal) bool) {
		for _, w := range windows {
			for _, tab := range w.Tabs {
				for _, t := range tab.Terminals {
					if !yield(t) {
						return
					}
				}
			}
		}
	}
}

// mergePanes updates r's pane of each terminal of o's windows, adding the new ones.
func mergePanes(r *Record, o *Observation) {
	for t := range terminals(o.Windows) {
		seen := o.Seen[t.ID]
		p, ok := r.Panes[t.ID]
		if !ok {
			p = &Pane{Since: o.Time}
			r.Panes[t.ID] = p
			if seen.Keep {
				seen = Seen{Kind: KindOther, Title: seen.Title, Login: seen.Login}
			}
		}
		mergePane(p, t, o.Sizes, seen, o.Time)
	}
}

// mergePane updates pane p from terminal t, its size in sizes and seen, at time now. A field the
// tick could not read keeps its value: a dead pane keeps its binding, and a pane ps could not
// place keeps what it showed. Since moves where the host or the session changes.
func mergePane(p *Pane, t ghostty.Terminal, sizes map[string]Size, seen Seen, now time.Time) {
	if t.Cwd != "" {
		p.Cwd = t.Cwd
	}
	if t.TTY != "" {
		p.TTY = t.TTY
	}
	if size, ok := sizes[t.ID]; ok {
		p.Size = &size
	}
	if seen.Login != 0 {
		p.Login = seen.Login
	}
	switch {
	case seen.Keep:
	case seen.Kind == KindDead:
		p.Kind = KindDead
	default:
		if seen.Host != p.Host || seen.Session != p.Session {
			p.Since = now
		}
		p.Kind, p.Host, p.Session = seen.Kind, seen.Host, seen.Session
		p.Worktree, p.Title = seen.Worktree, seen.Title
	}
}

// mergeWindows makes r's windows o's, front to back, then the old windows o shows only hidden
// panes of, last. Each tab keeps its tree where an old or undone tab had its panes in that order. A pane in
// no window then leaves r.Panes.
func mergeWindows(r *Record, o *Observation, undone []Window) {
	old := r.Windows
	windows := make([]Window, 0, len(o.Windows))
	for _, w := range o.Windows {
		var tabs []Tab
		for _, t := range w.Tabs {
			ids := make([]string, len(t.Terminals))
			for i, terminal := range t.Terminals {
				ids[i] = terminal.ID
			}
			if tab, ok := tabFor(ids, t.Selected, t.Focused, old, undone); ok {
				tabs = append(tabs, tab)
			}
		}
		if len(tabs) > 0 {
			windows = append(windows, Window{Tabs: tabs})
		}
	}
	for _, w := range old {
		if kept, ok := hiddenWindow(w, o, old); ok {
			windows = append(windows, kept)
		}
	}
	r.Windows = windows
	inWindows := map[string]bool{}
	for _, w := range windows {
		for _, id := range windowLeaves(w) {
			inWindows[id] = true
		}
	}
	maps.DeleteFunc(r.Panes, func(id string, _ *Pane) bool { return !inWindows[id] })
}

// hiddenWindow is old window w as the record keeps it while o shows none of its panes in a window
// but some hidden: a minimized or hidden window. Its gone panes leave its tabs. False means w is
// no such window, or nothing of w remains.
func hiddenWindow(w Window, o *Observation, old []Window) (Window, bool) {
	ids := windowLeaves(w)
	hidden := func(id string) bool { return o.Hidden[id] }
	if slices.ContainsFunc(ids, o.shows) || !slices.ContainsFunc(ids, hidden) {
		return Window{}, false
	}
	var tabs []Tab
	for _, t := range w.Tabs {
		ids := slices.DeleteFunc(Leaves(t.Tree), o.gone)
		if tab, ok := tabFor(ids, t.Selected, t.Focused, old, nil); ok {
			tabs = append(tabs, tab)
		}
	}
	return Window{Tabs: tabs}, len(tabs) > 0
}

// tabFor is the tab of the panes ids, selected or not, with focused its focused pane where ids
// holds that pane. False means ids is empty.
func tabFor(ids []string, selected bool, focused string, old, undone []Window) (Tab, bool) {
	if len(ids) == 0 {
		return Tab{}, false
	}
	if !slices.Contains(ids, focused) {
		focused = ""
	}
	tree, shape := treeFor(ids, old, undone)
	return Tab{Selected: selected, Focused: focused, Shape: shape, Tree: tree}, true
}

// treeFor is the split tree of a tab of the panes ids, and its shape. A tab of old or undone with
// those panes in that order gives its own, so that a tree cld made stays made, through an undo
// too. Else the panes make a
// row: Ghostty's AppleScript gives no split's direction (decision 59.8).
func treeFor(ids []string, old, undone []Window) (*Node, string) {
	if len(ids) == 1 {
		return &Node{Pane: ids[0]}, ShapeMade
	}
	for _, w := range slices.Concat(old, undone) {
		for _, tab := range w.Tabs {
			if sameLeaves(tab.Tree, ids) {
				return copyTree(tab.Tree, nil), tab.Shape
			}
		}
	}
	return flat(ids), ShapeFlat
}
