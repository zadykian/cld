package tabs

import (
	"context"
	"errors"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Restore reads Ghostty and merges what it shows into the record, as a tick does, then reports.
// This release has the dry-run's report alone: Ghostty's version, pid and start (decision 59.11).
// Where Ghostty does not run, it merges nothing.
func Restore(ctx context.Context) (*Report, error) {
	snap, err := ghostty.Take(ctx)
	if err != nil {
		return nil, failure(err)
	}
	c := clock{now: time.Now()}
	report := &Report{}
	if !snap.Running {
		report.Lines = []string{c.ghostty(snap, ghostty.App{}, nil)}
		report.section("Notes:", settingsNotes(ctx, ghostty.App{}))
		return report, nil
	}
	app, err := snap.Ghostty()
	if err != nil {
		return nil, failure(err)
	}
	o, err := Observe(ctx, snap)
	if err != nil {
		return nil, failure(err)
	}
	if err := merge(o, report); err != nil {
		return nil, err
	}
	report.Lines = []string{c.ghostty(snap, app, o.Procs)}
	report.section("Notes:", settingsNotes(ctx, app))
	return report, nil
}

// merge folds o into the record in cld's state directory, and saves it. A record cld cannot read
// is moved aside, with a warning in report.
func merge(o *Observation, report *Report) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	r, old, warning, err := load(dir)
	if err != nil {
		return err
	}
	report.warn(warning)
	Merge(r, o)
	return save(dir, r, old)
}

// failure is err as a restore ends with it: Ghostty's errors with cld's advice, status 1.
func failure(err error) error {
	if e, ok := errors.AsType[*ghostty.Error](err); ok {
		return e.Fail()
	}
	return err
}
