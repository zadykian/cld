package tabs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/ghostty"
)

// recordWait is how long a restore waits for the record's lock, which a tick holds for an instant.
const recordWait = 10 * time.Second

// Options are what cld restore tabs is asked to do.
type Options struct {
	DryRun bool // report what a restore would do, and change nothing in Ghostty
	Older  bool // reopen also the newest burst of the losses a restore without options leaves
	All    bool // reopen every loss
	Closed bool // reopen the tabs and splits closed alone too
}

// Restore is cld restore tabs (decision 59.6). It merges what Ghostty shows into the record, as a
// tick does, then types into the panes Ghostty kept, under both locks. A dry-run reports what it
// would do instead.
func Restore(ctx context.Context, opts Options) (*Report, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	unlock, err := lock(dir, opts)
	if err != nil {
		return nil, err
	}
	report := &Report{}
	seen, err := observe(ctx, dir, report)
	if err != nil {
		unlock()
		return nil, err
	}
	p, left := restorePlan(ctx, seen, opts)
	if opts.DryRun {
		unlock()
		dryRun(ctx, dir, seen, p, left, opts, report)
		return report, nil
	}
	res := apply(ctx, p, report)
	unlock()
	finish(dir, seen, p, res, report)
	return report, nil
}

// lock takes a restore's locks in dir: restore's, which keeps a second restore from typing into
// the same panes, but for a dry-run; then the record's. It returns what lets both go.
func lock(dir string, opts Options) (unlock func(), err error) {
	var restore *os.File
	if !opts.DryRun {
		file, locked, err := tryLock(filepath.Join(dir, restoreFile))
		if err != nil {
			return nil, err
		}
		if !locked {
			return nil, fail.Runtime("another cld restore tabs runs")
		}
		restore = file
	}
	unlockRecord, err := lockRecord(dir, recordWait)
	release := func() {
		if restore != nil {
			_ = restore.Close() //nolint:errcheck // the close lets the lock go
		}
	}
	if err != nil {
		release()
		return nil, err
	}
	return func() { unlockRecord(); release() }, nil
}

// observed is what a restore saw: the record after its merge, and Ghostty, with o nil where
// Ghostty does not run.
type observed struct {
	record *Record
	app    ghostty.App
	o      *Observation
}

// observe reads Ghostty and merges what it shows into the record in dir, as a tick does, under the
// record's lock. Where Ghostty does not run, the merge is that of a run that brings nothing back,
// and writes nothing.
func observe(ctx context.Context, dir string, report *Report) (*observed, error) {
	snap, err := ghostty.Take(ctx)
	if err != nil {
		return nil, failure(err)
	}
	r, old, warning, err := load(dir)
	if err != nil {
		return nil, err
	}
	report.warn(warning)
	if !snap.Running {
		Merge(r, &Observation{Time: time.Now().UTC().Truncate(time.Second)}, lastTick(dir))
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
	Merge(r, o, lastTick(dir))
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

// restorePlan picks what a restore reopens of the record seen, as opts asks, and plans it against
// Ghostty as seen. It asks Ghostty's terminfo cache once per host. left is what it leaves.
func restorePlan(ctx context.Context, seen *observed, opts Options) (*Planned, []*Loss) {
	chosen, left := selectLosses(seen.record, opts, time.Now())
	bin, ok := ghostty.Binary(seen.app)
	cached := func(host string) bool { return ok && ghostty.TerminfoCached(ctx, bin, host) }
	return plan(seen.record, seen.o, chosen, cached), left
}
