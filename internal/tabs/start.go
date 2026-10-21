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

// watchArgv is the words after cld that run the watcher, which Start runs and the watcher execs.
var watchArgv = []string{"restore", "tabs", "--watching"}

// The watcher's fd 3 is watch, which Start hands it locked. handover is how long the watcher waits
// for that lock where Start handed it over unlocked. logLimit is watch.log's size past which Start
// empties it.
const (
	watchFD  = 3
	handover = 30 * time.Second
	logLimit = 1 << 20
)

// cannotStart starts Start's error where the watcher's process does not start, and a restore's
// warning where Start fails.
const cannotStart = "cannot start the watcher: "

// Start starts the watcher unless one runs: cld restore tabs --watching, in a session of its own,
// with watch's lock on fd 3 (decision 59.3). It reports false where one runs. It refuses to run
// outside a Ghostty pane: the watcher's osascript asks macOS for the grant of the app it descends
// from.
func Start() (started bool, err error) {
	if os.Getenv("TERM_PROGRAM") != "ghostty" {
		return false, fail.Runtime(
			"restore tabs --watch runs in a Ghostty pane: TERM_PROGRAM is not ghostty")
	}
	dir, err := Dir()
	if err != nil {
		return false, err
	}
	watch, err := openWatch(dir)
	if err != nil || watch == nil {
		return false, err
	}
	defer watch.Close() //nolint:errcheck // the watcher holds its own copy
	log, err := openLog(dir)
	if err != nil {
		return false, err
	}
	defer log.Close() //nolint:errcheck // the watcher holds its own copy
	exe, err := os.Executable()
	if err != nil {
		return false, fail.Runtime("cannot find the file cld runs from: " + err.Error())
	}
	// stdin and stdout are /dev/null. The directory is /, so that the watcher keeps no pane's
	// directory, or its volume, in use. UNVERIFIED until #177's checks 2 and 16 (decision 59.12).
	// macOS charges the watcher's Apple Events to Ghostty once its pane closes, and after its exec.
	cmd := exec.Command(exe, watchArgv...)
	cmd.Args[0] = "cld"
	cmd.Stderr, cmd.ExtraFiles, cmd.Dir = log, []*os.File{watch}, "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf(cannotStart+"%w", err)
	}
	return true, cmd.Process.Release()
}

// openWatch opens dir's watch for a new watcher, locked. A watcher that holds it, but whose
// Ghostty is gone, ends at its next tick. Then watch comes back unlocked, and the new watcher
// waits for the lock (see holdsLock). Nil means a watcher runs, or another --watch starts one: it
// holds the lock while watch still names a watcher that ended.
func openWatch(dir string) (*os.File, error) {
	path := filepath.Join(dir, watchFile)
	watch, locked, err := tryLock(path)
	if err != nil || locked {
		return watch, err
	}
	if w, app := readWatch(path); app == 0 || alive(app) || !alive(w) {
		return nil, nil
	}
	watch, err = os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return nil, reason(err)
	}
	return watch, nil
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

// readWatch reads watch at path: the watcher's pid and Ghostty's, 0 for a line it lacks.
func readWatch(path string) (watcher, app int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(string(data), "\n")
	watcher, _ = strconv.Atoi(lines[0]) //nolint:errcheck // a line that is no pid is none
	if len(lines) > 1 {
		app, _ = strconv.Atoi(lines[1]) //nolint:errcheck // as above
	}
	return watcher, app
}

// alive reports whether process pid exists: kill(pid, 0) finds it, or may not signal it.
func alive(pid int) bool {
	return pid > 0 && !errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}

// Status is the watcher, as a dry-run reports it.
type Status struct {
	PID      int       // 0: none runs
	LastTick time.Time // watch's time
	LastLine string    // watch.log's last line, "" for none
}

// WatcherStatus is the watcher of the record in dir, read without a lock: a --watch that met the
// lock while held here would start none. A pid reused since its watcher ended misleads it.
func WatcherStatus(dir string) Status {
	path := filepath.Join(dir, watchFile)
	status := Status{LastLine: lastLine(filepath.Join(dir, logFile))}
	if pid, _ := readWatch(path); alive(pid) {
		status.PID = pid
		if info, err := os.Stat(path); err == nil {
			status.LastTick = info.ModTime()
		}
	}
	return status
}

// lastLine is the last line of the file at path, read from its last 4 KiB, "" for none.
func lastLine(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close() //nolint:errcheck // a file only read loses nothing
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	from := max(info.Size()-4096, 0)
	tail := make([]byte, info.Size()-from)
	n, _ := file.ReadAt(tail, from) //nolint:errcheck // what was read is the tail
	text := strings.TrimRight(string(tail[:n]), "\n")
	return text[strings.LastIndexByte(text, '\n')+1:]
}
