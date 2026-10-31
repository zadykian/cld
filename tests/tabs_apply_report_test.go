package tests

import (
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What a restore's report says beyond a plain apply: a tab off its index.

// A tab that Ghostty leaves off its index, its moves dropped, is a warning, with status 1.
func TestTabsApplyMissedIndex(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b := addPane(m, "~"), addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b))}
	r := tick(t, s, m)
	m.DropMoves = 2
	saveModel(t, s, m)
	lost := tabLoss(r.Run.ID, time.Now().UTC().Add(-time.Minute).Truncate(time.Second), "t", a, 1)
	r.Losses = []*tabs.Loss{lost}
	writeRecord(t, s, r)
	checkExit(t, "restore tabs --closed", s.RunCld(tabsEnv, "restore", "tabs", "--closed"), 1,
		"Reopened a tab at 2 in its window, closed at "+clockAt(lost.At)+"\n",
		"cld: warning: reopened a tab at 2, not at 1\n"+noWatcher)
}
