package tabs

import (
	"context"
	"fmt"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Observation is a snapshot of Ghostty, with ps's rows read after it: what a merge folds into the
// record.
type Observation struct {
	Time    time.Time // in UTC, to the second
	Run     string    // Ghostty's run, PID@START (see runID)
	Version string
	Ghostty int              // Ghostty's pid
	Windows []ghostty.Window // front to back
	Seen    map[string]Seen  // by terminal id, every window's
	Procs   Table
}

// Observe reads ps around snap, which must hold a running Ghostty. A Ghostty whose ps row is
// gone is a *ghostty.Error of kind Gone: the snapshot came as it quit.
func Observe(ctx context.Context, snap *ghostty.Snapshot) (*Observation, error) {
	app, err := snap.Ghostty()
	if err != nil {
		return nil, err
	}
	o := &Observation{Time: time.Now().UTC().Truncate(time.Second), Version: snap.Version,
		Ghostty: app.PID, Windows: snap.Windows, Seen: map[string]Seen{}}
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
				o.see(t)
			}
		}
	}
	return o, nil
}

// see classifies terminal t.
func (o *Observation) see(t ghostty.Terminal) {
	o.Seen[t.ID] = classify(t)
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
