package tests

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The plan a restore hands apply.js, run by the fake osascript, and the record after it: what
// apply made, where, and what it failed to.

// applyMac is a Ghostty of one window: a, a shell at its prompt, d, dead, and v, an editor. In its
// record at now, each has a last, session app, which a quit cut 2 hours ago. The record has lost a
// split of a's, a tab beside it, a window of two tabs, and the quit's two windows.
func applyMac(t *testing.T, s *sandbox.Sandbox, now time.Time) (*mac.Model, *tabs.Record) {
	t.Helper()
	s.WriteProbeFile(mac.SSHCache, "me@ws\n")
	m := ghosttyApp(t, s)
	a, d, v := addPane(m, "~"), addPane(m, "~"), addPane(m, "vim", []string{"vim"})
	m.Terminals[d].PID = 0
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(d), tabOf(v))}
	saveModel(t, s, m)
	previous, quit := "1@2025-01-01T00:00:00Z", now.Add(-2*time.Hour)
	last := func() *tabs.Last {
		return &tabs.Last{Host: "ws", Session: "app", Cut: "quit", Run: previous, At: quit}
	}
	r := recordOf(m, map[string]*tabs.Pane{a: {Kind: "local", Since: now, Last: last()},
		d: {Kind: "local", Since: now, Last: last()}, v: {Kind: "local", Since: now, Last: last()}},
		tabs.Window{Tabs: []tabs.Tab{{Selected: true, Shape: "made", Tree: leaf(a)},
			{Shape: "made", Tree: leaf(d)}, {Shape: "made", Tree: leaf(v)}}})
	r.Previous = previous
	copied := func(id string, p tabs.Pane) *tabs.Node {
		p.Cwd = "/Users/me/" + id
		return &tabs.Node{Pane: id, Copy: &p}
	}
	local, run := tabs.Pane{Kind: "local"}, r.Run.ID
	lost := func(at time.Time, cause string, tabList ...tabs.Tab) *tabs.Loss {
		return &tabs.Loss{At: at, Run: run, Cause: cause,
			Windows: []tabs.Window{{Tabs: tabList}}}
	}
	pane := lost(now.Add(-4*time.Minute), "pane", tabs.Tab{Shape: "made", Tree: copied("p", local)})
	pane.Anchor = &tabs.Anchor{Pane: a, Split: "left"}
	tab := lost(now.Add(-3*time.Minute), "tab", tabs.Tab{Focused: "t2", Shape: "made",
		Tree: split("right", copied("t1", tabs.Pane{Kind: "cld", Host: "ws", Session: "x"}),
			copied("t2", local))})
	tab.Anchor = &tabs.Anchor{Pane: a, Index: 1}
	quitLoss := lost(quit, "quit", tabs.Tab{Selected: true, Shape: "made",
		Tree: copied("q1", tabs.Pane{Kind: "cld", Host: "ws", Session: "app-3"})})
	quitLoss.Run = previous
	quitLoss.Windows = append(quitLoss.Windows, tabs.Window{Tabs: []tabs.Tab{{Selected: true,
		Shape: "made", Tree: copied("q2", local)}}})
	r.Losses = []*tabs.Loss{quitLoss, lost(now.Add(-10*time.Minute), "closed",
		tabs.Tab{Focused: "w1", Shape: "made", Tree: copied("w1", tabs.Pane{Kind: "ssh",
			Host: "ws2"})},
		tabs.Tab{Selected: true, Focused: "w3", Shape: "made", Tree: split("down",
			copied("w2", local), copied("w3", local))}), pane, tab}
	writeRecord(t, s, r)
	return m, r
}

// applyNames names applyMac's panes by their terminals' numbers: those it starts with, and those
// the fake Ghostty makes for testdata/tabs/plan.json, in the plan's order.
var applyNames = map[int]string{1: "a", 4: "d", 7: "v", 11: "p", 14: "t1", 17: "t2", 20: "q2",
	23: "q1", 26: "w1", 29: "w2", 32: "w3"}

// applyIDs names applyMac's panes by their terminals' ids, as applyNames does by their numbers.
func applyIDs() map[string]string {
	names := map[string]string{}
	for n, name := range applyNames {
		names[mac.TerminalID(n)] = name
	}
	return names
}

// A restore of every loss types into the shell at its prompt, never the dead pane or the editor.
// Then it builds splits, tabs, then windows back to front, each kind oldest first: the newest ends
// in front. Leaves are keyed b<build>l<leaf>, and tabs keep their focus and selection. The panes
// made enter the record where Ghostty holds them, with the lasts their lines join, as a tick sees.
func TestTabsApplyPlan(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	now := time.Now().UTC().Truncate(time.Second)
	_, before := applyMac(t, s, now)
	result := s.RunCld(tabsEnv, "restore", "tabs", "--all", "--closed")
	at := func(i int) string { return clockAt(before.Losses[i].At) }
	checkExit(t, "restore tabs --all --closed", result, 0,
		"Typed into the pane in /Users/me: session 'app' on ws\n"+
			"Reopened a split beside its pane, closed at "+at(2)+"\n"+
			"Reopened a tab at 1 in its window, closed at "+at(3)+"\n"+
			"Reopened a window of 1 tab, lost as Ghostty quit at "+at(0)+"\n"+
			"Reopened a window of 1 tab, lost as Ghostty quit at "+at(0)+"\n"+
			"Reopened a window of 2 tabs and 3 panes, closed at "+at(1)+"\n",
		uncachedNote("ws2")+noWatcher)
	calls := s.OsascriptCalls()
	want := withIDs(readFile(t, filepath.Join("testdata", "tabs", "plan.json")))
	i := slices.IndexFunc(calls, func(c mac.OsascriptCall) bool { return c.Script == "apply" })
	if i < 0 || !sameJSON(t, calls[i].Input, []byte(want)) {
		t.Errorf("osascript's calls %+v, want an apply of testdata/tabs/plan.json", calls)
	}
	m := loadModel(t, s)
	checkTyped(t, m, map[int][]string{1: {"^U", strings.Replace(wholeLine, "-s x", "-s app", 1),
		"\r"}, 4: nil, 7: nil})
	names := applyIDs()
	const windows = "w1/made *down(w2,w3)/made | *q1/made | *q2/made | " +
		"*right(t1,t2)/made right(p,a)/made d/made v/made"
	r := readRecord(t, s)
	if got := drawWindows(r.Windows, names); got != windows {
		t.Errorf("the record's windows are %s, want %s", got, windows)
	}
	checkMade(t, r, now)
	if got := drawWindows(tick(t, s, m).Windows, names); got != windows {
		t.Errorf("a tick made the record's windows %s, want %s", got, windows)
	}
}

// checkMade reports record r after TestTabsApplyPlan's restore, at now or later. Each pane made
// must have the last its line joins, and the kept ones theirs, and each loss is restored.
func checkMade(t *testing.T, r *tabs.Record, now time.Time) {
	t.Helper()
	lasts := map[int]string{1: "ws app quit typed", 4: "ws app quit", 14: "ws x closed typed",
		23: "ws app-3 quit typed", 26: "ws2  closed typed"}
	for n, name := range applyNames {
		p, want := r.Panes[mac.TerminalID(n)], lasts[n]
		got := ""
		switch {
		case p == nil:
			got = "no pane"
		case p.Last != nil:
			got = fmt.Sprintf("%s %s %s", p.Last.Host, p.Last.Session, p.Last.Cut)
			if typed := p.Last.Typed; typed != nil && !typed.Before(now) {
				got += " typed"
			}
		}
		if got != want {
			t.Errorf("pane %s: last %q, want %q", name, got, want)
		}
	}
	for _, l := range r.Losses {
		if l.Restored == nil || l.Restored.Before(now) {
			t.Errorf("a %s loss restored at %v, want at the restore", l.Cause, l.Restored)
		}
	}
}

// A build that fails keeps what it made, and the restore warns, with status 1. The loss is
// restored where a pane of it opened, and stays for the next restore where none did.
func TestTabsApplyFails(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, warning, windows string
		failAt                 int
		restored               bool
	}{
		{"a split", "Failed to create split. (-10000) (2 of its panes opened)",
			"w1/made *w2/made | ", 8, true},
		{"a window", "no terminal appeared", "", 6, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m, before := applyMac(t, s, time.Now().UTC().Truncate(time.Second))
			m.FailAt = test.failAt
			saveModel(t, s, m)
			result := s.RunCld(tabsEnv, "restore", "tabs", "--all", "--closed")
			warning := "cld: warning: cannot reopen a window closed at " +
				clockAt(before.Losses[1].At) + ": " + test.warning + "\n"
			if result.Code != 1 || !strings.HasPrefix(result.Stderr, warning) {
				t.Errorf("exit %d, stderr %q, want exit 1 and the warning %q", result.Code,
					result.Stderr, warning)
			}
			r := readRecord(t, s)
			want := test.windows + "*q1/made | *q2/made | *right(t1,t2)/made right(p,a)/made " +
				"d/made v/made"
			if got := drawWindows(r.Windows, applyIDs()); got != want {
				t.Errorf("the record's windows are %s, want %s", got, want)
			}
			if restored := r.Losses[1].Restored != nil; restored != test.restored {
				t.Errorf("the window's loss restored %t, want %t", restored, test.restored)
			}
		})
	}
}

// heldRestore starts cld restore tabs in s, and returns once its snapshot has read the model. The
// fake osascript then holds it for a second, while the restore holds its locks. It returns what
// waits for the restore to end.
func heldRestore(t *testing.T, s *sandbox.Sandbox) func() sandbox.Result {
	t.Helper()
	s.WriteProbeFile(mac.DelayFile, "1\n")
	wait := startCldAsync(t, s, s.Work, tabsEnv, "restore", "tabs")
	sandbox.WaitFor(t, 10*time.Second, "the restore's snapshot", func() bool {
		return !exists(filepath.Join(s.ProbeDir, mac.DelayFile))
	})
	return wait
}

// typedMac is a Ghostty of one pane, a shell at its prompt, whose last a quit cut from session
// app on ws, which Ghostty has terminfo for. It returns the pane's id.
func typedMac(t *testing.T, s *sandbox.Sandbox) (*mac.Model, string) {
	t.Helper()
	s.WriteProbeFile(mac.SSHCache, "me@ws\n")
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	now := time.Now().UTC().Truncate(time.Second)
	writeRecord(t, s, recordOf(m, map[string]*tabs.Pane{a: {Kind: "local", Since: now,
		Last: &tabs.Last{Host: "ws", Session: "app", Cut: "quit", Run: "1@x", At: now}}},
		tabs.Window{Tabs: []tabs.Tab{{Selected: true, Shape: "made", Tree: leaf(a)}}}))
	return m, a
}

// A pane in which a program started after the restore's snapshot is busy: apply.js types nothing
// there, and the restore notes it. The pane keeps its last, for the next restore.
func TestTabsApplyBusy(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m, a := typedMac(t, s)
	wait := heldRestore(t, s)
	m.Terminals[a].PID = m.NewPID()
	saveModel(t, s, m)
	checkExit(t, "restore tabs", wait(), 0, "", "cld: left the pane in /Users/me as it is: a "+
		"program runs there now\n"+noWatcher)
	checkTyped(t, loadModel(t, s), map[int][]string{1: nil})
	if last := readRecord(t, s).Panes[a].Last; last == nil || last.Typed != nil {
		t.Errorf("last %+v, want it kept, with nothing typed", last)
	}
}

// A pane a restore typed into keeps its last, now with the time of its typing. A tick leaves it
// until the line has had its time to join.
func TestTabsApplyTyped(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	_, a := typedMac(t, s)
	start := time.Now().UTC().Truncate(time.Second)
	checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0,
		"Typed into the pane in /Users/me: session 'app' on ws\n", noWatcher)
	if last := readRecord(t, s).Panes[a].Last; last == nil || last.Typed == nil ||
		last.Typed.Before(start) {
		t.Errorf("last %+v, want it kept, typed at or after %v", last, start)
	}
}
