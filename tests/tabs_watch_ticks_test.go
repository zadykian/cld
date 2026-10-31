package tests

import (
	"os"
	"path/filepath"
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

// A dry-run waits 10 s for the record's lock, which another cld holds, then fails.
func TestTabsWatchBusy(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
	tick(t, s, m)
	lock, err := os.OpenFile(filepath.Join(tabsDir(s), "lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() }) //nolint:errcheck // closed already, but at a failure
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, recordPath(s))
	checkFailed(t, dryRun(t, s), 1, "cld: cannot lock cld's record of Ghostty's tabs: another "+
		"cld restore tabs holds it\n")
	if readFile(t, recordPath(s)) != before {
		t.Error("tabs.json changed while another cld held its lock")
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

// watchTime is watch's time in s, which each merge of the watcher's sets.
func watchTime(t *testing.T, s *sandbox.Sandbox) time.Time {
	t.Helper()
	info, err := os.Stat(watchPath(s))
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}
