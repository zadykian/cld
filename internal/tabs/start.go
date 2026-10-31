package tabs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
)

// watchArgv is the words after cld that run the watcher, which Start runs.
var watchArgv = []string{"restore", "tabs", "--watching"}

// The watcher's fd 3 is watch, which Start hands it locked. handover is how long the watcher waits
// for that lock where Start handed it over unlocked. logLimit is watch.log's size past which Start
// empties it.
const (
	watchFD  = 3
	handover = 30 * time.Second
	logLimit = 1 << 20
)

// Start starts the watcher: cld restore tabs --watching, in a session of its own, with watch's
// lock on fd 3 (decision 59.3). It starts none where a watcher holds the lock. It refuses to run outside a Ghostty pane: the watcher's osascript asks macOS for the grant of
// the app it descends from.
func Start() error {
	if os.Getenv("TERM_PROGRAM") != "ghostty" {
		return fail.Runtime(
			"restore tabs --watch runs in a Ghostty pane: TERM_PROGRAM is not ghostty")
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	watch, locked, err := tryLock(filepath.Join(dir, watchFile))
	if err != nil || !locked {
		return err
	}
	defer watch.Close() //nolint:errcheck // the watcher holds its own copy
	log, err := openLog(dir)
	if err != nil {
		return err
	}
	defer log.Close() //nolint:errcheck // the watcher holds its own copy
	exe, err := os.Executable()
	if err != nil {
		return fail.Runtime("cannot find the file cld runs from: " + err.Error())
	}
	cmd := exec.Command(exe, watchArgv...)
	cmd.Args[0] = "cld"
	cmd.Stderr, cmd.ExtraFiles, cmd.Dir = log, []*os.File{watch}, "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start the watcher: %w", err)
	}
	return cmd.Process.Release()
}

// openLog opens watch.log in dir to append the watcher's stderr, emptied first where over
// logLimit.
func openLog(dir string) (*os.File, error) {
	path := filepath.Join(dir, logFile)
	if info, err := os.Stat(path); err == nil && info.Size() > logLimit {
		if err := os.Truncate(path, 0); err != nil {
			return nil, reason(err)
		}
	}
	log, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, reason(err)
	}
	return log, nil
}

// holdsLock is fd 3 as the watcher's watch in dir, once the watcher holds its lock: at once where
// Start locked it, else once the watcher before ends. Nil means another watcher won it within
// handover. fd 3 that is not watch is a refusal: only Start runs the watcher.
func holdsLock(dir string) (*os.File, error) {
	path := filepath.Join(dir, watchFile)
	var fd, file unix.Stat_t
	if unix.Fstat(watchFD, &fd) != nil || unix.Stat(path, &file) != nil || fd.Dev != file.Dev ||
		fd.Ino != file.Ino {
		return nil, fail.Runtime("restore tabs --watching runs only as --watch starts it")
	}
	deadline := time.Now().Add(handover)
	for {
		err := unix.Flock(watchFD, unix.LOCK_EX|unix.LOCK_NB)
		switch {
		case err == nil:
			// Wrapped once, for the process's life: the GC would close a dropped *os.File's fd,
			// and the lock with it.
			return os.NewFile(watchFD, path), nil
		case !errors.Is(err, unix.EWOULDBLOCK):
			return nil, fmt.Errorf("%s: %w", path, err)
		case !time.Now().Before(deadline):
			return nil, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
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

// alive reports whether process pid exists: kill(pid, 0) finds it, or may not signal it.
func alive(pid int) bool {
	return pid > 0 && !errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}
