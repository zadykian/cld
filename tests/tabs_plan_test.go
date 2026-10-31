package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What a restore reopens (decision 59.6): the losses it picks, as its options ask, and what it
// leaves.

// shellLoss is a loss of run, of cause at time at: a window of one tab of the pane id. The record
// last saw that pane a shell in /Users/me/ID.
func shellLoss(run, cause string, at time.Time, id string) *tabs.Loss {
	return &tabs.Loss{At: at, Run: run, Cause: cause, Windows: []tabs.Window{{Tabs: []tabs.Tab{
		{Selected: true, Shape: "made", Tree: &tabs.Node{Pane: id,
			Copy: &tabs.Pane{Kind: "local", Cwd: "/Users/me/" + id}}}}}}}
}

// selectRecord is a record at now of m's run, whose one window holds the shell a. Its losses are
// named by their one pane. q is the window of a run that quit 3 hours ago. Then windows closed
// 50, 25, 20, 16, 14 and 5 minutes ago, a split 40, a tab 3, and a window restored 2 minutes ago.
func selectRecord(m *mac.Model, a string, now time.Time) *tabs.Record {
	r := recordOf(m, map[string]*tabs.Pane{a: {Kind: "local", Since: now}},
		tabs.Window{Tabs: []tabs.Tab{{Selected: true, Shape: "made", Tree: leaf(a)}}})
	r.Previous = "1@2025-01-01T00:00:00Z"
	ago := func(minutes int) time.Time { return now.Add(-time.Duration(minutes) * time.Minute) }
	split, tab := shellLoss(r.Run.ID, "pane", ago(40), "s40"), shellLoss(r.Run.ID, "tab", ago(3), "t3")
	split.Anchor, tab.Anchor = &tabs.Anchor{Pane: a, Split: "right"}, &tabs.Anchor{Pane: a, Index: 2}
	restored := shellLoss(r.Run.ID, "closed", ago(2), "r2")
	restored.Restored = &now
	r.Losses = []*tabs.Loss{shellLoss(r.Previous, "quit", ago(180), "q"),
		shellLoss(r.Run.ID, "closed", ago(50), "w50"), split,
		shellLoss(r.Run.ID, "closed", ago(25), "w25"), shellLoss(r.Run.ID, "closed", ago(20), "w20"),
		shellLoss(r.Run.ID, "closed", ago(16), "w16"), shellLoss(r.Run.ID, "closed", ago(14), "w14"),
		shellLoss(r.Run.ID, "closed", ago(5), "w5"), tab, restored}
	return r
}

// The dry-run lists what a restore without options would reopen, and each loss it leaves with the
// option that reopens it.
func TestTabsPlanLeft(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	r := selectRecord(m, a, time.Now().UTC().Truncate(time.Second))
	writeRecord(t, s, r)
	at := func(i int) string { return clockAt(r.Losses[i].At) }
	checkExit(t, "restore tabs --dry-run", dryRun(t, s), 0, fmt.Sprintf(
		"Ghostty 1.4.0, pid %d, started 2025-01-02 09:12\n", m.PID)+
		"The watcher: none runs; cld restore tabs --watch in a Ghostty pane starts one\n"+
		"Would reopen 1 window, lost as Ghostty quit at "+at(0)+":\n  1 tab: a shell in /Users/me/q\n"+
		"Would reopen 1 window, closed at "+at(6)+":\n  1 tab: a shell in /Users/me/w14\n"+
		"Would reopen 1 window, closed at "+at(7)+":\n  1 tab: a shell in /Users/me/w5\n"+
		"Left:\n  a window of 1 tab, closed at "+at(1)+": --older or --all\n"+
		"  a split, closed at "+at(2)+": --closed with --older or --all\n"+
		"  a window of 1 tab, closed at "+at(3)+": --older or --all\n"+
		"  a window of 1 tab, closed at "+at(4)+": --older or --all\n"+
		"  a window of 1 tab, closed at "+at(5)+": --older or --all\n"+
		"  a tab, closed at "+at(8)+": --closed\n", "")
}

// A quit after which Ghostty brought every window back lost nothing: a restore reopens nothing,
// and runs no apply.
func TestTabsPlanEmptyQuit(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	now := time.Now().UTC().Truncate(time.Second)
	r := recordOf(m, map[string]*tabs.Pane{a: {Kind: "local", Since: now}},
		tabs.Window{Tabs: []tabs.Tab{{Selected: true, Shape: "made", Tree: leaf(a)}}})
	r.Previous = "1@2025-01-01T00:00:00Z"
	r.Losses = []*tabs.Loss{{At: now.Add(-time.Minute), Run: r.Previous, Cause: "quit",
		Windows: []tabs.Window{}}}
	writeRecord(t, s, r)
	checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0, "", noWatcher)
	for _, call := range s.OsascriptCalls() {
		if call.Script != "snapshot" {
			t.Errorf("osascript ran %s, want only a snapshot", call.Script)
		}
	}
	if l := readRecord(t, s).Losses; len(l) != 1 || l[0].Restored != nil {
		t.Errorf("losses %+v, want the empty quit's, not restored", l)
	}
}

// A window closed, whose panes' logins live while Ghostty's undo can bring it back, waits: a
// restore reopens nothing, and notes it.
func TestTabsPlanPending(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b := addPane(m, "~"), addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(b))}
	tick(t, s, m)
	m.Windows = m.Windows[:1]
	r := tick(t, s, m)
	if len(r.Pending) != 1 {
		t.Fatalf("pending %s, want the closed window", drawLosses(r.Pending, named(a, b)))
	}
	checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0, "",
		"cld: a window closed at "+clockAt(r.Pending[0].At)+" waits for Ghostty's undo: run "+
			"cld restore tabs again once it has passed\n"+noWatcher)
}
