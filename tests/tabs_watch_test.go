package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The watcher's life: how --watch starts it, the lock it holds, and its end as Ghostty quits.
// It reads Ghostty every 5 s, so these tests wait for its ticks. The helpers that
// tabs_watch_ticks_test.go shares are here.

// watchEnv is a Ghostty pane's environment beside the sandbox's: --watch starts a watcher there.
var watchEnv = map[string]string{"TZ": "UTC", "TERM_PROGRAM": "ghostty"}

// watchPath is the watcher's file in s: locked for its life, its pid and Ghostty's within.
func watchPath(s *sandbox.Sandbox) string { return filepath.Join(tabsDir(s), "watch") }

// startWatch runs cld restore tabs --watch in s from a Ghostty pane, which exits 0 printing
// nothing. The watchers in s are killed as the test ends.
func startWatch(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	t.Cleanup(func() { killWatchers(t, s) })
	checkExit(t, "restore tabs --watch", s.RunCld(watchEnv, "restore", "tabs", "--watch"), 0, "",
		"")
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

// logLine is a line of watch.log: the time in UTC, then what happened.
var logLine = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ (.*)$`)

// watchLog is what watch.log in s holds, a line each, without the times.
func watchLog(t *testing.T, s *sandbox.Sandbox) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tabsDir(s), "watch.log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for line := range strings.Lines(string(data)) {
		match := logLine.FindStringSubmatch(strings.TrimSuffix(line, "\n"))
		if match == nil {
			t.Fatalf("watch.log: %q has no time", line)
		}
		lines = append(lines, match[1])
	}
	return lines
}

// checkLog reports watch.log in s where it does not hold want's lines.
func checkLog(t *testing.T, s *sandbox.Sandbox, want ...string) {
	t.Helper()
	if got := watchLog(t, s); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("watch.log holds %q, want %q", got, want)
	}
}

// --watch starts the watcher in a session of its own, which holds watch's lock and merges a tick
// at once. The dry-run reports it. It ends as Ghostty's process does, and the record notes when.
func TestTabsWatch(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "✳ cld-x", ssh("ws"))
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	startWatch(t, s)
	pid := waitTicked(t, s, m)
	if sid, err := unix.Getsid(pid); err != nil || sid != pid || !watching(t, s) {
		t.Errorf("the watcher %d: session %d (%v), its lock held %t", pid, sid, err,
			watching(t, s))
	}
	checkPane(t, "the watcher's tick", readRecord(t, s).Panes[a],
		shown{Kind: "cld", Host: "ws", Session: "x", Title: "✳ cld-x"}, loginOf(m, a))
	checkReport(t, s, fmt.Sprintf("The watcher: pid %d, last read at ", pid))
	quit := time.Now().UTC().Truncate(time.Second)
	quitGhostty(m)
	sandbox.WaitFor(t, 15*time.Second, "the watcher to end", func() bool {
		return !watching(t, s)
	})
	if ended := readRecord(t, s).Run.Ended; ended.Before(quit) || ended.After(time.Now()) {
		t.Errorf("run.ended is %v, want the time Ghostty quit, %v", ended, quit)
	}
	checkLog(t, s, "Ghostty's run "+runOf(m)+" ended")
}

// Two --watch at once, and a third after them, leave one watcher: the one that took the lock.
// Another would have logged that it gave up, after waiting 30 s for the lock.
func TestTabsWatchOnce(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
	saveModel(t, s, m)
	t.Cleanup(func() { killWatchers(t, s) })
	first := startCldAsync(t, s, s.Work, watchEnv, "restore", "tabs", "--watch")
	second := startCldAsync(t, s, s.Work, watchEnv, "restore", "tabs", "--watch")
	checkExit(t, "the first --watch", first(), 0, "", "")
	checkExit(t, "the second --watch", second(), 0, "", "")
	pid := waitTicked(t, s, m)
	startWatch(t, s)
	time.Sleep(31 * time.Second)
	if now, _ := watchPIDs(s); now != pid || !watching(t, s) {
		t.Errorf("the watcher is %d, its lock held %t, want %d", now, watching(t, s), pid)
	}
	checkLog(t, s)
}

// --watch refuses to start a watcher outside a Ghostty pane, where macOS would not give its
// osascript Ghostty's grant.
func TestTabsWatchInGhosttyOnly(t *testing.T) {
	t.Parallel()
	for _, program := range []string{"", "Apple_Terminal"} {
		t.Run(program, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			saveModel(t, s, ghosttyApp(t, s))
			result := s.RunCld(map[string]string{"TERM_PROGRAM": program}, "restore", "tabs",
				"--watch")
			checkFailed(t, result, 1, "cld: restore tabs --watch runs in a Ghostty pane: "+
				"TERM_PROGRAM is not ghostty\n")
			if _, err := os.Stat(watchPath(s)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("watch: %v, want none", err)
			}
		})
	}
}

// --watch empties watch.log where it has grown past 1 MB, before the watcher writes there. A
// smaller log keeps its lines, and the watcher's follow them.
func TestTabsWatchLog(t *testing.T) {
	t.Parallel()
	old := "2026-10-16T09:00:00Z a line of an old watcher\n"
	for _, test := range []struct {
		name  string
		lines int
		want  []string
	}{
		{"past 1 MB", 1 << 15, nil},
		{"smaller", 2, []string{"a line of an old watcher", "a line of an old watcher"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
			saveModel(t, s, m)
			if err := os.MkdirAll(tabsDir(s), 0o700); err != nil {
				t.Fatal(err)
			}
			s.WriteFile(filepath.Join(tabsDir(s), "watch.log"), strings.Repeat(old, test.lines))
			startWatch(t, s)
			waitTicked(t, s, m)
			checkMode(t, watchPath(s), 0o600)
			quitGhostty(m)
			sandbox.WaitFor(t, 15*time.Second, "the watcher to end", func() bool {
				return !watching(t, s)
			})
			checkLog(t, s, append(test.want, "Ghostty's run "+runOf(m)+" ended")...)
		})
	}
}
