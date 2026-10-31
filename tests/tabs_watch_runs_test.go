package tests

import (
	"testing"

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
