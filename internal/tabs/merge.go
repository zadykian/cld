package tabs

import (
	"iter"
	"maps"

	"github.com/zadykian/cld/internal/ghostty"
)

// Merge folds o into r as a tick does (decision 59.4). A run it has not seen before takes the
// place of the one before. Then the panes o shows update, and those it shows no more leave.
func Merge(r *Record, o *Observation) {
	switch {
	case r.Run.ID != o.Run:
		r.Run = Run{ID: o.Run, Version: o.Version}
	case r.Run.Version == "":
		r.Run.Version = o.Version
	}
	mergePanes(r, o)
}

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

// mergePanes updates r's pane of each terminal of o's windows, adding the new ones, and drops
// those o shows no more.
func mergePanes(r *Record, o *Observation) {
	for t := range terminals(o.Windows) {
		p, ok := r.Panes[t.ID]
		if !ok {
			p = &Pane{Since: o.Time}
			r.Panes[t.ID] = p
		}
		mergePane(p, t, o.Seen[t.ID])
	}
	maps.DeleteFunc(r.Panes, func(id string, _ *Pane) bool { return !o.shows(id) })
}

// shows reports whether a window of o holds terminal id.
func (o *Observation) shows(id string) bool {
	_, ok := o.Seen[id]
	return ok
}

// mergePane updates pane p from terminal t and seen. A field the tick could not read keeps its
// value.
func mergePane(p *Pane, t ghostty.Terminal, seen Seen) {
	if t.Cwd != "" {
		p.Cwd = t.Cwd
	}
	if t.TTY != "" {
		p.TTY = t.TTY
	}
	p.Title = seen.Title
}
