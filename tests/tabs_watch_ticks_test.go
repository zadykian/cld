package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The watcher's ticks, and the record's lock that each merge takes.

// snapshots is how many snapshots the fake osascript has taken in s.
func snapshots(t *testing.T, s *sandbox.Sandbox) int {
	t.Helper()
	n := 0
	for _, call := range s.OsascriptCalls() {
		if call.Script == "snapshot" {
			n++
		}
	}
	return n
}

// waitSnapshots waits until the fake osascript has taken n snapshots in s.
func waitSnapshots(t *testing.T, s *sandbox.Sandbox, n int) {
	t.Helper()
	sandbox.WaitFor(t, 20*time.Second, "the watcher's ticks", func() bool {
		return snapshots(t, s) >= n
	})
}

// repeated matches the line that counts a repeated error, logged once a tick goes well again.
var repeated = regexp.MustCompile(`^the last error repeated [2-9] times$`)

// A failure that may pass is logged once, and counted while it repeats: the count comes as a
// tick goes well again.
func TestTabsWatchRepeats(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
	saveModel(t, s, m)
	startWatch(t, s)
	waitTicked(t, s, m)
	s.WriteProbeFile(mac.FailFile, "changed\n")
	// The fake logs a call before it reads its failure mode. So the second failing snapshot is over
	// only once a third is logged, as the watcher takes one at a time.
	waitSnapshots(t, s, snapshots(t, s)+3)
	if err := os.Remove(filepath.Join(s.ProbeDir, mac.FailFile)); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 20*time.Second, "the count of the repeated error", func() bool {
		return len(watchLog(t, s)) == 2
	})
	got := watchLog(t, s)
	if got[0] != "Ghostty's windows changed twice while cld read them" ||
		!repeated.MatchString(got[1]) {
		t.Errorf("watch.log holds %q, want the error, then its count", got)
	}
	if !watching(t, s) {
		t.Error("the watcher stopped")
	}
}

// A tick whose snapshot began before another cld wrote the record merges nothing: it would take
// the windows that cld saw for closed. A merge touches watch, so its time tells whether the slow
// tick merged before the dry-run wrote, which would test nothing.
func TestTabsWatchStale(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	startWatch(t, s)
	waitTicked(t, s, m)
	s.WriteProbeFile(mac.DelayFile, "6\n")
	sandbox.WaitFor(t, 10*time.Second, "the watcher's slow snapshot", func() bool {
		_, err := os.Stat(filepath.Join(s.ProbeDir, mac.DelayFile))
		return os.IsNotExist(err)
	})
	ticked := watchTime(t, s)
	b := addPane(m, "~")
	m.Windows = append(m.Windows, windowOf(tabOf(b)))
	tick(t, s, m)
	if !watchTime(t, s).Equal(ticked) {
		t.Fatal("the slow snapshot merged before the dry-run wrote the record")
	}
	s.WriteProbeFile(mac.FailFile, "changed\n")
	waitSnapshots(t, s, snapshots(t, s)+1)
	checkRecord(t, "after the slow snapshot", readRecord(t, s), named(a, b), "*a/made | *b/made",
		"", "")
}

// Once cld update renames a new cld over the one the watcher runs from, the watcher runs the new
// cld in its own process. It keeps its pid, its session and its lock, and reads Ghostty on.
func TestTabsWatchRestarts(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
	saveModel(t, s, m)
	cld := filepath.Join(s.Root, "bin", "cld")
	if err := os.Mkdir(filepath.Dir(cld), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := readFile(t, sandbox.Cld)
	s.WriteProgram(cld, binary, 0o755)
	t.Cleanup(func() { killWatchers(t, s) })
	start := exec.Command(cld, "restore", "tabs", "--watch")
	start.Env, start.Dir = s.Environ(watchEnv), s.Work
	checkExit(t, "restore tabs --watch", runCommand(t, start), 0, "", "")
	pid := waitTicked(t, s, m)
	s.WriteProgram(cld+".new", binary, 0o755)
	if err := os.Rename(cld+".new", cld); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 15*time.Second, "the restart", func() bool {
		return len(watchLog(t, s)) > 0
	})
	waitSnapshots(t, s, snapshots(t, s)+1)
	if now, _ := watchPIDs(s); now != pid || !watcherRuns(t, pid) || !watching(t, s) {
		t.Errorf("the watcher is %d, its lock held %t, want %d running cld restore tabs "+
			"--watching", now, watching(t, s), pid)
	}
	checkLog(t, s, "restarting as the new cld")
}

// watchTime is watch's time in s, which each merge of the watcher's sets.
func watchTime(t *testing.T, s *sandbox.Sandbox) time.Time {
	t.Helper()
	info, err := os.Stat(watchPath(s))
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}

// The watcher never waits for the record's lock: while another cld holds it, a tick every 5 s
// merges nothing and logs nothing. A dry-run waits 10 s for it, then fails. The first tick after
// the lock goes merges.
func TestTabsWatchBusy(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
	saveModel(t, s, m)
	startWatch(t, s)
	waitTicked(t, s, m)
	lock, err := os.OpenFile(filepath.Join(tabsDir(s), "lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() }) //nolint:errcheck // closed already, but at a failure
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	held, before, n := time.Now(), readFile(t, recordPath(s)), snapshots(t, s)
	b := addPane(m, "~")
	m.Windows = append(m.Windows, windowOf(tabOf(b)))
	saveModel(t, s, m)
	checkFailed(t, dryRun(t, s), 1, "cld: cannot lock cld's record of Ghostty's tabs: another "+
		"cld restore tabs holds it\n")
	// A watcher that waited 10 s for the lock at a tick would take its third snapshot 20 s in, at
	// the soonest.
	sandbox.WaitFor(t, time.Until(held.Add(19*time.Second)), "3 ticks under the lock",
		func() bool { return snapshots(t, s) >= n+3 })
	if readFile(t, recordPath(s)) != before {
		t.Error("tabs.json changed while another cld held its lock")
	}
	checkLog(t, s)
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 15*time.Second, "a merge once the lock went", func() bool {
		return readRecord(t, s).Panes[b] != nil
	})
	checkLog(t, s)
}
