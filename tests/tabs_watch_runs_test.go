package tests

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The watcher across Ghostty's runs: the handover to a new Ghostty's watcher, and a relaunch once
// the watcher has ended. Then the last tick, which dates a quit that no watcher saw.

// A --watch whose watcher still reads a Ghostty that quit starts the next watcher, which takes
// the lock as the first ends. It reads the new run, of which the first's end is the quit.
func TestTabsWatchHandover(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	startWatch(t, s)
	first := waitTicked(t, s, m)
	quitGhostty(m)
	n := relaunch(m, startSleep(t), relaunched, a)
	saveModel(t, s, n)
	startWatch(t, s)
	second := waitTicked(t, s, n)
	r := readRecord(t, s)
	if second == first || !watching(t, s) || r.Previous != runOf(m) || len(r.Losses) != 1 {
		t.Errorf("watchers %d then %d, the second's lock held %t; the record's runs %s and %s, "+
			"its losses %+v", first, second, watching(t, s), r.Previous, r.Run.ID, r.Losses)
	}
	checkLog(t, s, "Ghostty's run "+runOf(m)+" ended")
}

// Two --watch at once, after a relaunch and once the watcher before has ended, leave one watcher
// on the new run. The other finds the lock held while watch names a watcher that ended. It starts
// none, so no second watcher waits 30 s for the lock, then logs that it gave up.
func TestTabsWatchRelaunch(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	startWatch(t, s)
	old := waitTicked(t, s, m)
	quitGhostty(m)
	sandbox.WaitFor(t, 15*time.Second, "the watcher to end", func() bool {
		return !watching(t, s)
	})
	waitGone(t, old)
	n := relaunch(m, startSleep(t), relaunched, a)
	saveModel(t, s, n)
	first := startCldAsync(t, s, s.Work, watchEnv, "restore", "tabs", "--watch")
	second := startCldAsync(t, s, s.Work, watchEnv, "restore", "tabs", "--watch")
	checkExit(t, "the first --watch", first(), 0, "", "")
	checkExit(t, "the second --watch", second(), 0, "", "")
	pid := waitTicked(t, s, n)
	time.Sleep(31 * time.Second)
	if now, _ := watchPIDs(s); now != pid || !watching(t, s) {
		t.Errorf("the watcher is %d, its lock held %t, want %d", now, watching(t, s), pid)
	}
	checkLog(t, s, "Ghostty's run "+runOf(m)+" ended")
}

// A watcher killed before Ghostty quit notes no end of its run. The next watcher then dates the
// quit by the last tick, watch's time, which its own start keeps.
func TestTabsWatchLastTick(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "✳ cld-x", ssh("ws"))
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	tick(t, s, m)
	killed := startSleep(t)
	stopSleep(killed)
	quitGhostty(m)
	s.WriteFile(watchPath(s), strconv.Itoa(killed)+"\n"+strconv.Itoa(m.PID)+"\n")
	ticked := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(watchPath(s), ticked, ticked); err != nil {
		t.Fatal(err)
	}
	n := relaunch(m, startSleep(t), relaunched)
	saveModel(t, s, n)
	startWatch(t, s)
	waitTicked(t, s, n)
	if r := readRecord(t, s); len(r.Losses) != 1 || !r.Losses[0].At.Equal(ticked) {
		t.Errorf("losses %s, want a quit at %v", drawLosses(r.Losses, named(a)), ticked)
	}
}
