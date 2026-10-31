package tabs

import (
	"context"
	"os"

	"github.com/zadykian/cld/internal/ghostty"
)

// apply types plan p's lines into Ghostty's panes, under the locks Restore holds (decision 59.6).
// A plan with nothing to type runs nothing, and its result is empty. The result is nil where the
// apply gave none.
func apply(ctx context.Context, p *Planned, report *Report) *ghostty.Result {
	res := &ghostty.Result{}
	if typed := p.Script().Typed; len(typed) > 0 {
		var err error
		if res, err = ghostty.Apply(ctx, &ghostty.Plan{Pace: pace, Typed: typed}); err != nil {
			report.warn(failure(err).Error())
		}
	}
	return res
}

// finish ends a restore that ran plan p against the record seen, apply giving res: its report and
// notes.
func finish(dir string, seen *observed, p *Planned, res *ghostty.Result, report *Report) {
	c := newClock()
	c.restored(seen.record, p, res, report)
	report.Notes = append(report.Notes, p.notes()...)
	if os.Getenv("TERM_PROGRAM") != "ghostty" {
		if WatcherStatus(dir).PID == 0 {
			report.note("no watcher runs here: the next ssh in a Ghostty pane starts one")
		}
	}
}

// restored reports a restore that ran plan p against record r, apply giving res, nil where it
// gave none. It has a line per pane typed into, and a warning per failure.
func (c clock) restored(r *Record, p *Planned, res *ghostty.Result, report *Report) {
	if res == nil {
		return
	}
	for i := range min(len(res.Typed), len(p.typed)) {
		j, t := p.typed[i], res.Typed[i]
		in := "the pane in " + c.dir(paneCwd(r, j.pane))
		switch {
		case t.Error != "":
			report.warn("cannot type into " + in + ": " + t.Error)
		case t.Busy:
			report.note("left " + in + " as it is: a program runs there now")
		default:
			report.Lines = append(report.Lines, "Typed into "+in+": "+j.binding.String())
		}
	}
}
