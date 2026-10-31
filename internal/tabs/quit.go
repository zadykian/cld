package tabs

import "slices"

// runChange is Ghostty's quit of r's run, seen as o shows another (decision 59.4). What o shows
// none of the panes of is the quit's loss, empty where Ghostty brought everything back. A window
// pending at the quit joins it, and a pending tab or split stays a loss of its own kind.
func runChange(r *Record, o *Observation) {
	ended := r.Run.ID
	windows := quitWindows(r, o)
	for _, l := range r.Pending {
		if l.Cause == CauseClosed {
			windows = append(windows, l.Windows...)
		} else {
			r.Losses = append(r.Losses, l)
		}
	}
	r.Losses = append(r.Losses, &Loss{At: o.Time, Run: ended, Cause: CauseQuit,
		Windows: nonNil(windows)})
	r.Previous, r.Run, r.Pending = ended, Run{ID: o.Run, Version: o.Version}, nil
}

// quitWindows takes out of r the windows of which o holds no pane, in a window or hidden, with
// their panes. It returns them with copies of the panes, front to back.
func quitWindows(r *Record, o *Observation) []Window {
	var quit, kept []Window
	back := func(id string) bool { return !o.gone(id) }
	for _, w := range r.Windows {
		ids := windowLeaves(w)
		if slices.ContainsFunc(ids, back) {
			kept = append(kept, w)
			continue
		}
		if len(ids) > 0 {
			quit = append(quit, copyWindow(w, r.Panes))
		}
		for _, id := range ids {
			delete(r.Panes, id)
		}
	}
	r.Windows = kept
	return quit
}
