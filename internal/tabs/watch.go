package tabs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/ghostty"
)

// tick is how often the watcher reads Ghostty, from the end of one read to the start of the next.
const tick = 5 * time.Second

// watcher is the watcher's state. run and pid are Ghostty's run and pid, once a tick knows them.
type watcher struct {
	dir   string
	watch *os.File // kept for the process's life: its lock is the watcher's
	run   string
	pid   int
	log   *logger
}

// Watch is the watcher, cld restore tabs --watching. Each tick it merges what Ghostty shows into
// the record, until Ghostty's run ends (decision 59.3).
func Watch() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	w := &watcher{dir: dir, log: &logger{}}
	watch, locked, err := tryLock(filepath.Join(dir, watchFile))
	if err != nil || !locked {
		if err == nil {
			w.log.line("another watcher holds the record")
		}
		return err
	}
	w.watch = watch
	if err := writeWatch(w.watch, os.Getpid()); err != nil {
		return w.stop(err)
	}
	return w.loop(context.Background())
}

// loop runs the watcher's ticks.
func (w *watcher) loop(ctx context.Context) error {
	src := ghostty.Poll(tick)
	defer src.Close() //nolint:errcheck // Poll's closes nothing
	for {
		if err := src.Wait(ctx); err != nil {
			return w.stop(err)
		}
		ended, err := w.tick(ctx, src)
		switch {
		case err != nil:
			return err
		case ended:
			return w.ended()
		}
	}
}

// tick reads Ghostty once and merges what it shows into the record. It reports whether Ghostty's
// run ended: no Ghostty runs, ps lacks its row, or another run has its pid. Its error is a
// permanent failure's, logged. UNVERIFIED until #177's check 16 (decision 59.12): that a quit
// within a tick relaunches nothing, and leaves nothing half read to merge.
func (w *watcher) tick(ctx context.Context, src ghostty.Source) (bool, error) {
	before := statRecord(w.dir)
	snap, err := src.Next(ctx)
	if err != nil {
		return false, w.failed(err)
	}
	if !snap.Running {
		return true, nil
	}
	o, err := Observe(ctx, snap, nil)
	if e, ok := errors.AsType[*ghostty.Error](err); ok && e.Kind == ghostty.Gone {
		return true, nil
	}
	switch {
	case err != nil:
		return false, w.failed(err)
	case w.run != "" && o.Run != w.run:
		return true, nil
	case w.run == "":
		w.run, w.pid = o.Run, o.Ghostty
		if err := writeWatch(w.watch, os.Getpid(), o.Ghostty); err != nil {
			return false, w.failed(err)
		}
	}
	return false, w.merge(o, before)
}

// merge folds o into the record under its lock. It skips the tick where another cld holds the
// lock, or wrote the record since before, as the snapshot began. o is older than that write, and
// would take the windows a restore just made for closed (decision 59.3).
func (w *watcher) merge(o *Observation, before os.FileInfo) error {
	unlock, err := lockRecord(w.dir, 0)
	if errors.Is(err, errBusy) {
		return nil
	}
	if err != nil {
		return w.failed(err)
	}
	defer unlock()
	if !sameRecord(before, statRecord(w.dir)) {
		return nil
	}
	last := lastTick(w.dir)
	r, old, warning, err := load(w.dir)
	if err != nil {
		return w.failed(err)
	}
	if warning != "" {
		w.log.line(warning)
	}
	Merge(r, o, last)
	if err := save(w.dir, r, old); err != nil {
		return w.failed(err)
	}
	now := time.Now()
	if err := os.Chtimes(w.watch.Name(), now, now); err != nil {
		return w.failed(err)
	}
	w.log.flush()
	return nil
}

// ended is the end of Ghostty's run. The watcher then exits, with status 0.
func (w *watcher) ended() error {
	if w.run == "" {
		w.log.line("Ghostty is not running")
		return nil
	}
	w.log.line("Ghostty's run " + w.run + " ended")
	return nil
}

// failed logs err, a tick's failure. A permanent one stops the watcher, with status 1: macOS or
// Ghostty refuses until the user acts, or a newer cld wrote the record. Another skips the tick.
func (w *watcher) failed(err error) error {
	if ge, ok := errors.AsType[*ghostty.Error](err); ok && ge.Permanent() {
		return w.stop(err)
	}
	if newer := (formatError{}); errors.As(err, &newer) {
		return w.stop(err)
	}
	w.log.failed(err.Error())
	return nil
}

// stop logs err as what stopped the watcher, and ends it with status 1.
func (w *watcher) stop(err error) error {
	w.log.line("stopped: " + err.Error())
	return fail.Status(1)
}

// statRecord is the record in dir as it stands, nil where missing.
func statRecord(dir string) os.FileInfo {
	info, err := os.Stat(filepath.Join(dir, recordFile))
	if err != nil {
		return nil
	}
	return info
}

// sameRecord reports whether a and b, two stats of the record, are of one write: save renames a
// new file over it, so each write has a new inode.
func sameRecord(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// writeWatch writes pids to watch, a line each: the watcher's, then Ghostty's once a tick knows
// it. watch keeps its time, the last tick's.
func writeWatch(watch *os.File, pids ...int) error {
	info, err := watch.Stat()
	if err != nil {
		return err
	}
	var text strings.Builder
	for _, pid := range pids {
		text.WriteString(strconv.Itoa(pid) + "\n")
	}
	if err := watch.Truncate(0); err != nil {
		return err
	}
	if _, err := watch.WriteAt([]byte(text.String()), 0); err != nil {
		return err
	}
	return os.Chtimes(watch.Name(), time.Time{}, info.ModTime())
}
