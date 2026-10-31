package tabs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/zadykian/cld/internal/fail"
)

// watchArgv is the words after cld that run the watcher, which Start runs.
var watchArgv = []string{"restore", "tabs", "--watching"}

// logLimit is watch.log's size past which Start empties it.
const logLimit = 1 << 20

// Start starts the watcher: cld restore tabs --watching, in a session of its own (decision 59.3).
// It refuses to run outside a Ghostty pane: the watcher's osascript asks macOS for the grant of
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
	cmd.Stderr, cmd.Dir = log, "/"
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
