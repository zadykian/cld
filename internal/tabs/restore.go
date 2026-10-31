package tabs

import (
	"context"
	"errors"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Restore reads Ghostty and merges what it shows into the record, as a tick does, then reports.
// This release has the dry-run's report alone: what the record holds (decision 59.11).
func Restore(ctx context.Context) (*Report, error) {
	report := &Report{}
	seen, err := observe(ctx, report)
	if err != nil {
		return nil, err
	}
	dryRun(ctx, seen, report)
	return report, nil
}

// observed is what a restore saw: the record after its merge, and Ghostty, with o nil where
// Ghostty does not run.
type observed struct {
	record *Record
	app    ghostty.App
	o      *Observation
}

// observe reads Ghostty and merges what it shows into the record in cld's state directory, as a
// tick does, and saves it. Where Ghostty does not run, the merge is that of a run that brings
// nothing back, and writes nothing. A record cld cannot read is moved aside, with a warning in
// report.
func observe(ctx context.Context, report *Report) (*observed, error) {
	snap, err := ghostty.Take(ctx)
	if err != nil {
		return nil, failure(err)
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	r, old, warning, err := load(dir)
	if err != nil {
		return nil, err
	}
	report.warn(warning)
	if !snap.Running {
		Merge(r, &Observation{Time: time.Now().UTC().Truncate(time.Second)})
		return &observed{record: r}, nil
	}
	app, err := snap.Ghostty()
	if err != nil {
		return nil, failure(err)
	}
	o, err := Observe(ctx, snap, ownTTY())
	if err != nil {
		return nil, failure(err)
	}
	Merge(r, o)
	return &observed{record: r, app: app, o: o}, save(dir, r, old)
}

// ownTTY tells the tty of the pane cld runs in, which a restore never types into: nil where cld's
// stdin is no terminal.
func ownTTY() func(tty string) bool {
	dev, ok := ownDevice()
	if !ok {
		return nil
	}
	return func(tty string) bool { return sameDevice(tty, dev) }
}

// failure is err as a restore ends with it: Ghostty's errors with cld's advice, status 1.
func failure(err error) error {
	if e, ok := errors.AsType[*ghostty.Error](err); ok {
		return e.Fail()
	}
	return err
}
