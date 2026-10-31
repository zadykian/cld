package tests

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The watcher's ticks, and the record's lock that each merge takes.

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
