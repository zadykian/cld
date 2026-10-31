package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The watcher's life: the lock it holds, and its ticks. It reads Ghostty every 5 s, so these tests
// wait for them. The helpers that tabs_watch_ticks_test.go shares are here.

// watchEnv is a Ghostty pane's environment beside the sandbox's: --watch starts a watcher there.
var watchEnv = map[string]string{"TZ": "UTC", "TERM_PROGRAM": "ghostty"}

// watchPath is the watcher's file in s: locked for its life, its pid and Ghostty's within.
func watchPath(s *sandbox.Sandbox) string { return filepath.Join(tabsDir(s), "watch") }

// startWatch runs cld restore tabs --watching in s, as a Ghostty pane's environment has it. The
// watchers in s are killed as the test ends.
func startWatch(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	t.Cleanup(func() { killWatchers(t, s) })
	argv := s.CldArgv("restore", "tabs", "--watching")
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Args[0], cmd.Env, cmd.Dir = "cld", s.Environ(watchEnv), s.Work
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }() //nolint:errcheck // the watcher's status is no test's
}

// killWatchers kills the watcher whose pid watch names, while one holds its lock, until none
// does: a watcher waiting for the lock takes it as the one before ends.
func killWatchers(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	for range 5 {
		if !watching(t, s) {
			time.Sleep(300 * time.Millisecond)
			if !watching(t, s) {
				return
			}
		}
		if pid, _ := watchPIDs(s); watcherRuns(t, pid) {
			_ = unix.Kill(pid, unix.SIGKILL) //nolint:errcheck // it may have ended
		}
		sandbox.WaitFor(t, 5*time.Second, "the watcher to end", func() bool {
			return !watching(t, s)
		})
	}
}

// watchPIDs are the pids that watch names in s, the watcher's and Ghostty's, 0 for none.
func watchPIDs(s *sandbox.Sandbox) (watcher, app int) {
	data, err := os.ReadFile(watchPath(s))
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(string(data), "\n")
	watcher, _ = strconv.Atoi(lines[0]) //nolint:errcheck // a file half written names none
	if len(lines) > 1 {
		app, _ = strconv.Atoi(lines[1]) //nolint:errcheck // as above
	}
	return watcher, app
}

// watching reports whether a process holds watch's lock in s. It takes the lock for an instant
// where none does, so a test calls it while no --watch runs.
func watching(t *testing.T, s *sandbox.Sandbox) bool {
	t.Helper()
	file, err := os.OpenFile(watchPath(s), os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close() //nolint:errcheck // the close lets the lock go
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil && !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatal(err)
	}
	return err != nil
}

// watcherRuns reports whether process pid is cld's watcher, as the real ps gives its words.
func watcherRuns(t *testing.T, pid int) bool {
	t.Helper()
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("ps", "-o", "args=", "-p", strconv.Itoa(pid)).Output()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out)) == "cld restore tabs --watching"
}

// waitTicked waits for the watcher in s to read m's Ghostty and write the record, and returns
// the watcher's pid.
func waitTicked(t *testing.T, s *sandbox.Sandbox, m *mac.Model) int {
	t.Helper()
	sandbox.WaitFor(t, 15*time.Second, "the watcher's tick", func() bool {
		_, app := watchPIDs(s)
		data, err := os.ReadFile(recordPath(s))
		return app == m.PID && err == nil && strings.Contains(string(data), runOf(m))
	})
	pid, _ := watchPIDs(s)
	return pid
}

// The watcher holds watch's lock for its life, and merges a tick at once.
func TestTabsWatch(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "✳ cld-x", ssh("ws"))
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	startWatch(t, s)
	waitTicked(t, s, m)
	if !watching(t, s) {
		t.Error("the watcher does not hold watch's lock")
	}
	checkPane(t, "the watcher's tick", readRecord(t, s).Panes[a],
		shown{Kind: "cld", Host: "ws", Session: "x", Title: "✳ cld-x"}, loginOf(m, a))
}
