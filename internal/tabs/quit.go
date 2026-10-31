package tabs

import (
	"slices"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/session"
)

// The record's bounds on its losses: at most maxLosses, from this run and the one before. None is
// older than lossSpan, after which cld forgets an ended session's entry (decision 40.6).
const (
	maxLosses = 32
	lossSpan  = session.Expiry
)

// runChange is Ghostty's quit of r's run, seen as o shows another (decision 59.4). What o shows
// none of the panes of is the quit's loss, empty where Ghostty brought everything back. A window
// pending at the quit joins it, and a pending tab or split stays a loss of its own kind.
func runChange(r *Record, o *Observation, lastTick time.Time) {
	ended := r.Run.ID
	at := quitTime(r.Run, lastTick, o.Time)
	windows := quitWindows(r, o)
	for _, l := range r.Pending {
		if l.Cause == CauseClosed {
			windows = append(windows, l.Windows...)
		} else {
			r.Losses = append(r.Losses, l)
		}
	}
	r.Losses = append(r.Losses, &Loss{At: at, Run: ended, Cause: CauseQuit,
		Windows: nonNil(windows)})
	r.Previous, r.Run, r.Pending = ended, Run{ID: o.Run, Version: o.Version}, nil
}

// quitTime is when run quit: when the watcher saw its pid go, else its last tick, else now. A tick
// before the run's start was a watcher's of an earlier run, which no watcher of this one followed.
func quitTime(run Run, lastTick, now time.Time) time.Time {
	switch {
	case !run.Ended.IsZero():
		return run.Ended.UTC().Truncate(time.Second)
	case !lastTick.IsZero() && !lastTick.Before(runStart(run.ID)):
		return lastTick.UTC().Truncate(time.Second)
	}
	return now
}

// runStart is the START of run id PID@START, zero where id has none.
func runStart(id string) time.Time {
	_, start, _ := strings.Cut(id, "@")
	at, _ := time.Parse(time.RFC3339, start) //nolint:errcheck // a run with no start has zero
	return at
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

// bound keeps r's losses within the record's bounds at time now, oldest first. Pending losses are
// never bounded, as each settles within Ghostty's undo or at the next run change.
func bound(r *Record, now time.Time) {
	r.Losses = slices.DeleteFunc(r.Losses, func(l *Loss) bool {
		ours := l.Run != "" && (l.Run == r.Run.ID || l.Run == r.Previous)
		return !ours || now.Sub(l.At) > lossSpan
	})
	slices.SortStableFunc(r.Losses, func(a, b *Loss) int { return a.At.Compare(b.At) })
	if over := len(r.Losses) - maxLosses; over > 0 {
		r.Losses = slices.Delete(r.Losses, 0, over)
	}
}
