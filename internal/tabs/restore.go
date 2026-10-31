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
	if !snap.Running {
		return &Report{Lines: []string{c.ghostty(snap, ghostty.App{}, nil)}}, nil
	}
	app, err := snap.Ghostty()
	if err != nil {
		return nil, failure(err)
	}
	o, err := Observe(ctx, snap)
	if err != nil {
		return nil, failure(err)
	}
	if err := merge(o); err != nil {
		return nil, err
	}
	return &Report{Lines: []string{c.ghostty(snap, app, o.Procs)}}, nil
}

// merge folds o into the record in cld's state directory, and saves it.
func merge(o *Observation) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	r, old, err := load(dir)
	if err != nil {
		return err
	}
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
