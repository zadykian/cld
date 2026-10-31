package tabs

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// apply runs plan p's script in Ghostty, then records what it made in the record seen, under the
// locks Restore holds (decision 59.6). A plan with nothing to do runs nothing, and its result is
// empty. The result is nil where the apply gave none. A record it cannot write is a warning, as
// what the apply made is made: the report still says so.
func apply(
	ctx context.Context, dir string, seen *observed, p *Planned, report *Report,
) *ghostty.Result {
	res := &ghostty.Result{}
	if script := p.Script(); len(script.Typed) > 0 || len(script.Builds) > 0 {
		var err error
		if res, err = ghostty.Apply(ctx, script); err != nil {
			stopped(ctx, dir, seen, err, report)
		}
	}
	mark(seen.record, p, res, time.Now())
	if _, err := save(dir, seen.record, seen.saved); err != nil {
		report.warn("cannot record what the restore made, so the next one may reopen it: " +
			err.Error())
	}
	return res
}

// stopped reports an apply that gave no result, err saying why: Ghostty quit, macOS withdrew its
// grant, or osascript failed or ran out of time. apply.js prints only at its end, so it may have
// built by then. Where Ghostty now holds terminals that it lacked before, a merge enters them, so
// that the next restore's dedupe counts those that joined.
func stopped(ctx context.Context, dir string, seen *observed, err error, report *Report) {
	why := failure(err).Error()
	snap, err := ghostty.Take(ctx)
	n := 0
	if err == nil && snap.Running {
		n = appeared(seen.o, snap)
	}
	if n == 0 {
		report.warn("the restore stopped: " + why)
		return
	}
	if o, err := Observe(ctx, snap, ownTTY()); err == nil {
		Merge(seen.record, o, lastTick(dir))
	}
	report.warn(fmt.Sprintf("the restore stopped after Ghostty opened %s (%s): run cld restore"+
		" tabs --dry-run before running it again", count(n, "pane"), why))
}

// appeared counts the terminals of snap that observation o held nowhere.
func appeared(o *Observation, snap *ghostty.Snapshot) int {
	n := 0
	for _, id := range snap.Terminals {
		if o == nil || o.gone(id) {
			n++
		}
	}
	return n
}

// finish ends a restore that ran plan p against the record seen, apply giving res: its report and
// notes, the losses waiting for Ghostty's undo among them, then the watcher, started where cld
// runs in a Ghostty pane.
func finish(dir string, seen *observed, p *Planned, res *ghostty.Result, report *Report) {
	c := newClock()
	c.restored(seen.record, p, res, report)
	for _, l := range seen.record.Pending {
		report.note(noun(l) + " " + c.when(l) + " waits for Ghostty's undo: run cld restore tabs" +
			" again once it has passed")
	}
	report.Notes = append(report.Notes, p.notes(c)...)
	if os.Getenv("TERM_PROGRAM") != "ghostty" {
		if WatcherStatus(dir).PID == 0 {
			report.note("no watcher runs here: the next ssh in a Ghostty pane starts one")
		}
	} else if _, err := Start(); err != nil {
		report.warn(cannotStart + strings.TrimPrefix(err.Error(), cannotStart))
	}
}

// restored reports a restore that ran plan p against record r, apply giving res, nil where it
// gave none. It has a line per pane typed into and per build made, and a warning per failure.
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
	for i := range min(len(res.Builds), len(p.builds)) {
		c.built(r, p.script.Builds[i], p.builds[i], res.Builds[i], report)
	}
}

// built reports build b of info's loss, as res says apply made it, and as mark entered it in r.
func (c clock) built(r *Record, b ghostty.Build, info building, res ghostty.BuildResult,
	report *Report,
) {
	l := info.loss
	if res.Error != "" {
		failed := "cannot reopen " + noun(l) + " " + c.when(l) + ": " + res.Error
		if made := len(res.Keys); made > 0 {
			failed += fmt.Sprintf(" (%d of its panes opened)", made)
		}
		report.warn(failed)
		return
	}
	what := windowSize(b.Tabs)
	switch {
	case l.Cause == CauseTab && b.Kind == buildTab && !res.Window:
		what = fmt.Sprintf("a tab at %d in its window", res.Index)
	case l.Cause == CausePane && b.Kind == buildPane:
		what = "a split beside its pane"
	case l.Cause == CauseTab || l.Cause == CausePane:
		what = noun(l) + " as a window"
	}
	report.Lines = append(report.Lines, "Reopened "+what+", "+c.when(l))
	if want := min(b.Index, tabsBeside(r, b.Anchor)); b.Kind == buildTab && !res.Window &&
		want > 0 && res.Index != want {
		report.warn(fmt.Sprintf("reopened a tab at %d, not at %d", res.Index, want))
	}
}

// tabsBeside counts the tabs of the window of r that holds pane anchor: a tab moved past them
// goes last, as Ghostty clamps the move.
func tabsBeside(r *Record, anchor string) int {
	for _, w := range r.Windows {
		if slices.Contains(windowLeaves(w), anchor) {
			return len(w.Tabs)
		}
	}
	return 0
}
