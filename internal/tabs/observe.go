package tabs

import (
	"context"
	"fmt"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Observation is a snapshot of Ghostty, with ps's rows and the panes' sizes read after it: what a
// merge folds into the record.
type Observation struct {
	Time    time.Time // in UTC, to the second
	Run     string    // Ghostty's run, PID@START (see runID)
	Version string
	Ghostty int              // Ghostty's pid
	Windows []ghostty.Window // front to back
	Hidden  map[string]bool  // the app-wide terminals in no window, such as the quick terminal
	Seen    map[string]Seen  // by terminal id, every window's
	Sizes   map[string]Size  // by terminal id, where TIOCGWINSZ read one
	Procs   Table
	Own     string // the terminal cld runs in, "" for none
}

// Observe reads ps and the panes' sizes around snap, which must hold a running Ghostty. own
// reports whether a tty is cld's own, nil for none. That pane is a shell at its prompt whatever
// ps shows, as the shell runs cld there (decision 59.9). A Ghostty whose ps row is gone, or
// started at another time, is a *ghostty.Error of kind Gone: the snapshot came as it quit.
func Observe(ctx context.Context, snap *ghostty.Snapshot, own func(tty string) bool) (
	*Observation, error,
) {
	app, err := snap.Ghostty()
	if err != nil {
		return nil, err
	}
	o := &Observation{Time: time.Now().UTC().Truncate(time.Second), Version: snap.Version,
		Ghostty: app.PID, Windows: snap.Windows, Hidden: map[string]bool{},
		Seen: map[string]Seen{}, Sizes: map[string]Size{}}
	if o.Procs, err = Processes(ctx); err != nil {
		return nil, err
	}
	run, ok := runID(app.PID, o.Procs)
	if !ok {
		return nil, &ghostty.Error{Kind: ghostty.Gone}
	}
	o.Run = run
	for _, window := range snap.Windows {
		for _, tab := range window.Tabs {
			for _, t := range tab.Terminals {
				o.see(t, own)
			}
		}
	}
	for _, id := range snap.Terminals {
		if _, in := o.Seen[id]; !in {
			o.Hidden[id] = true
		}
	}
	return o, nil
}

// see classifies terminal t and reads its size.
func (o *Observation) see(t ghostty.Terminal, own func(tty string) bool) {
	seen := classify(t, o.Procs, o.Ghostty)
	if o.Own == "" && own != nil && t.TTY != "" && own(t.TTY) {
		o.Own = t.ID
		seen = Seen{Kind: KindLocal, Title: seen.Title, Login: seen.Login}
	}
	o.Seen[t.ID] = seen
	if size := winsize(t.TTY); size != nil {
		o.Sizes[t.ID] = *size
	}
}

// runID is Ghostty's run, by its pid and its start as ps gives it, and false where ps lacks the
// pid. ps names the start, not Ghostty's launch date, so that the watcher's check that the run
// goes on compares ps with ps.
func runID(pid int, procs Table) (string, bool) {
	p, ok := procs.ByPID(pid)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d@%s", pid, p.Start.UTC().Format(time.RFC3339)), true
}
