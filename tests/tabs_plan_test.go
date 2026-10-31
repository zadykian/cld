package tests

import (
	"fmt"
	"strings"
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

// restoredOf names r's restored losses, oldest first, by their first pane.
func restoredOf(r *tabs.Record) string {
	var names []string
	for _, l := range r.Losses {
		if l.Restored != nil {
			names = append(names, tabs.Leaves(l.Windows[0].Tabs[0].Tree)[0])
		}
	}
	return strings.Join(names, " ")
}

// selectCase is a restore of selectRecord's losses with args: the losses restored after it, and
// after a second run, "" for none.
type selectCase struct {
	args            []string
	restored, again string
}

// A restore without options reopens the windows lost in the last 15 minutes, 14 minutes ago but
// not 16, and the run before's quit at any age. --older adds a burst at each run, those within 10
// minutes of the newest left, and --all every one. A
// restored loss stays closed, so a second such restore reopens nothing.
func TestTabsPlanSelects(t *testing.T) {
	t.Parallel()
	for _, test := range []selectCase{
		{nil, "q w14 w5 r2", "q w14 w5 r2"},
		{[]string{"--older"}, "q w25 w20 w16 w14 w5 r2", "q w50 w25 w20 w16 w14 w5 r2"},
		{[]string{"--all"}, "q w50 w25 w20 w16 w14 w5 r2", ""},
	} {
		t.Run(strings.Join(append([]string{"restore tabs"}, test.args...), " "), func(t *testing.T) {
			t.Parallel()
			checkSelects(t, test)
		})
	}
}

// checkSelects runs test's restore, twice where it says what the second run restores.
func checkSelects(t *testing.T, test selectCase) {
	t.Helper()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	writeRecord(t, s, selectRecord(m, a, time.Now().UTC().Truncate(time.Second)))
	args := append([]string{"restore", "tabs"}, test.args...)
	result := s.RunCld(tabsEnv, args...)
	if result.Code != 0 || result.Stdout == "" || result.Stderr != noWatcher {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, what reopened, and %q",
			result.Code, result.Stdout, result.Stderr, noWatcher)
	}
	if got := restoredOf(readRecord(t, s)); got != test.restored {
		t.Errorf("restored %s, want %s", got, test.restored)
	}
	if test.again == "" {
		return
	}
	applies := len(s.OsascriptCalls())
	result = s.RunCld(tabsEnv, args...)
	if got := restoredOf(readRecord(t, s)); result.Code != 0 || got != test.again {
		t.Errorf("again: exit %d, restored %s, want exit 0 and %s", result.Code, got,
			test.again)
	}
	if test.args == nil && (result.Stdout != "" || len(s.OsascriptCalls()) != applies+1) {
		t.Errorf("again: stdout %q, %d osascript calls, want nothing reopened, and only a "+
			"snapshot", result.Stdout, len(s.OsascriptCalls())-applies)
	}
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
