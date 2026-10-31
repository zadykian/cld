package tabs

import (
	"context"
	"errors"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Restore reads Ghostty and reports what it finds. This release has the dry-run's report alone:
// Ghostty's version, pid and start (decision 59.11).
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
	procs, err := Processes(ctx)
	if err != nil {
		return nil, err
	}
	return &Report{Lines: []string{c.ghostty(snap, app, procs)}}, nil
}

// failure is err as a restore ends with it: Ghostty's errors with cld's advice, status 1.
func failure(err error) error {
	if e, ok := errors.AsType[*ghostty.Error](err); ok {
		return e.Fail()
	}
	return err
}
