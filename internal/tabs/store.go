package tabs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/session"
)

// The files in the record's directory (decision 59.4).
const (
	recordFile  = "tabs.json" // the record
	lockFile    = "lock"      // locked for each read, merge and write of the record
	watchFile   = "watch"     // locked by the watcher for its life; its time is the last tick
	restoreFile = "restore"   // locked by a restore, so that two never type into one pane
	logFile     = "watch.log" // the watcher's stderr
)

// errBusy is lockRecord's refusal where another cld holds the record's lock.
var errBusy = fail.Runtime(
	"cannot lock cld's record of Ghostty's tabs: another cld restore tabs holds it")

// Dir is the record's directory, tabs/ in cld's state directory, made where missing. It sits
// outside sessions/, whose expiry removes the files it does not know.
func Dir() (string, error) {
	state, err := session.StateDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(state, "tabs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", reason(err)
	}
	return dir, nil
}

// load reads the record in dir, and old, its bytes for save. A missing record is a new one, with
// no bytes. One that does not parse, or has format 0, is moved aside: a new one comes back, with
// a warning for the caller to report. A newer format is refused before the parse, which it could
// fail, so that the cld that wrote it finds it still.
func load(dir string) (r *Record, old []byte, warning string, err error) {
	data, err := os.ReadFile(filepath.Join(dir, recordFile))
	if errors.Is(err, fs.ErrNotExist) {
		return newRecord(), nil, "", nil
	}
	if err != nil {
		return nil, nil, "", reason(err)
	}
	var head struct {
		Format int `json:"format"`
	}
	if json.Unmarshal(data, &head) == nil && head.Format > Format {
		return nil, nil, "", formatError{fail.Runtime(fmt.Sprintf(
			"tabs.json has format %d, which this cld cannot read: run cld update", head.Format))}
	}
	r = &Record{}
	if json.Unmarshal(data, r) != nil || r.Format < 1 {
		aside, err := moveAside(dir)
		if err != nil {
			return nil, nil, "", err
		}
		return newRecord(), nil, "moved aside a record cld cannot read: " + aside, nil
	}
	normalize(r)
	return r, data, "", nil
}

// formatError is load's refusal of a record that a newer cld wrote, a *fail.Error within. The
// restore fails with it, and the watcher stops, as no later tick could read the record either.
type formatError struct{ err error }

func (e formatError) Error() string { return e.err.Error() }

func (e formatError) Unwrap() error { return e.err }

// newRecord is the record before any observation.
func newRecord() *Record {
	r := &Record{Format: Format}
	normalize(r)
	return r
}

// normalize gives r an empty list or map for each nil one, so that its bytes do not depend on
// what made it. It drops the nil panes and losses that a record edited by hand may hold.
func normalize(r *Record) {
	if r.Panes == nil {
		r.Panes = map[string]*Pane{}
	}
	for id, p := range r.Panes {
		if p == nil {
			delete(r.Panes, id)
		}
	}
	r.Windows = nonNil(r.Windows)
	isNil := func(l *Loss) bool { return l == nil }
	r.Pending = nonNil(slices.DeleteFunc(r.Pending, isNil))
	r.Losses = nonNil(slices.DeleteFunc(r.Losses, isNil))
	for _, l := range slices.Concat(r.Pending, r.Losses) {
		l.Windows = nonNil(l.Windows)
	}
}

// nonNil is list, or an empty list for nil, which JSON writes as [] rather than null.
func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// save writes r to the record in dir, by a temporary file renamed over it. Where r's bytes are
// old, it writes nothing, so that a record that did not change keeps its file and time.
func save(dir string, r *Record, old []byte) error {
	normalize(r)
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", " ")
	if err := encoder.Encode(r); err != nil {
		return err
	}
	if bytes.Equal(data.Bytes(), old) {
		return nil
	}
	return replace(filepath.Join(dir, recordFile), data.Bytes())
}

// replace replaces file with data: written to a temporary file beside it, readable by the user
// alone, then renamed over it, so that no reader sees it half written.
func replace(file string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".*")
	if err != nil {
		return reason(err)
	}
	_, err = temp.Write(data)
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), file)
	}
	if err != nil {
		_ = os.Remove(temp.Name()) //nolint:errcheck // best effort: the failure is returned
		return reason(err)
	}
	return nil
}

// moveAside renames the record in dir to tabs.json.bad- and the time in UTC, and returns the
// new path.
func moveAside(dir string) (string, error) {
	file := filepath.Join(dir, recordFile)
	aside := file + ".bad-" + time.Now().UTC().Format("20060102T150405Z")
	if err := os.Rename(file, aside); err != nil {
		return "", err
	}
	return aside, nil
}

// lockRecord takes the record's lock in dir, trying every 20 ms for up to wait, and returns what
// lets it go. A lock still held after wait is errBusy, where session.Lock goes on without. A
// restore waits 10 s; the watcher waits not at all at a tick, and 10 s to note its run's end.
func lockRecord(dir string, wait time.Duration) (unlock func(), err error) {
	file, err := os.OpenFile(filepath.Join(dir, lockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, reason(err)
	}
	deadline := time.Now().Add(wait)
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = file.Close() }, nil //nolint:errcheck // the close lets the lock go
		}
		if !errors.Is(err, unix.EWOULDBLOCK) || !time.Now().Before(deadline) {
			_ = file.Close() //nolint:errcheck // a file only opened, to lock
			if errors.Is(err, unix.EWOULDBLOCK) {
				return nil, errBusy
			}
			return nil, fmt.Errorf("%s: %w", file.Name(), err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// tryLock opens the file at path, made where missing, and takes its lock once. It reports false,
// with no file, where another process holds the lock. The lock lasts while the file is open.
func tryLock(path string) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, reason(err)
	}
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return file, true, nil
	}
	_ = file.Close() //nolint:errcheck // a file only opened, to lock
	if errors.Is(err, unix.EWOULDBLOCK) {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("%s: %w", path, err)
}

// lastTick is the watcher's last tick: the time of watch in dir, to the second, or zero where
// watch is missing.
func lastTick(dir string) time.Time {
	info, err := os.Stat(filepath.Join(dir, watchFile))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime().UTC().Truncate(time.Second)
}

// reason is err as the system says it, with the path it names.
func reason(err error) error {
	if pathError, ok := errors.AsType[*fs.PathError](err); ok {
		return errors.New(pathError.Path + ": " + pathError.Err.Error())
	}
	return err
}
