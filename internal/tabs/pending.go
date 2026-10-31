package tabs

import "slices"

// vanish makes a pending loss of each window, tab and split of r's windows whose panes o holds
// nowhere, and takes those panes out of the record's. Ghostty keeps a closed pane's login while
// its undo can bring the pane back, so the loss counts only once the login goes.
func vanish(r *Record, o *Observation) {
	var gone []string
	for _, w := range r.Windows {
		for _, l := range closedIn(w, o.gone, r.Panes) {
			l.At, l.Run = o.Time, o.Run
			r.Pending = append(r.Pending, l)
		}
		for _, id := range windowLeaves(w) {
			if o.gone(id) {
				gone = append(gone, id)
			}
		}
	}
	for _, id := range gone {
		delete(r.Panes, id)
	}
}

// closedIn is what closed in window w, gone telling a closed pane: w whole where its panes are
// all gone, with copies of the panes as the record last saw them.
func closedIn(w Window, gone func(id string) bool, panes map[string]*Pane) []*Loss {
	ids := windowLeaves(w)
	if len(ids) == 0 || slices.ContainsFunc(ids, func(id string) bool { return !gone(id) }) {
		return nil
	}
	return []*Loss{{Cause: CauseClosed, Windows: []Window{copyWindow(w, panes)}}}
}

// copyWindow is a copy of window w whose leaves carry their panes in panes, nil stripping them.
func copyWindow(w Window, panes map[string]*Pane) Window {
	tabs := make([]Tab, len(w.Tabs))
	for i, tab := range w.Tabs {
		tabs[i] = copyTab(tab, panes)
	}
	return Window{Tabs: tabs}
}

// copyTab is a copy of tab t whose leaves carry their panes in panes, nil stripping them.
func copyTab(t Tab, panes map[string]*Pane) Tab {
	t.Tree = copyTree(t.Tree, panes)
	return t
}
