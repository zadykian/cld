package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Ghostty's quits, which a merge sees as a new run: what came back, what is lost, and each
// pane's last. Then the bounds on the record's losses.

// relaunched is when the relaunched Ghostty of the quit tests started.
var relaunched = ghosttyStart.Add(24 * time.Hour)

// quitMac is a Ghostty of three windows, a, right(b, c) and d, recorded by a first tick. a is a cld
// pane in ~/src, b ssh, c a shell and d a cld pane in a worktree. It returns the panes' ids.
func quitMac(t *testing.T, s *sandbox.Sandbox) (*mac.Model, []string) {
	t.Helper()
	m := ghosttyApp(t, s)
	ids := []string{addPane(m, "✳ cld-app", ssh("ws")), addPane(m, "me@ws: ~", ssh("ws")),
		addPane(m, "~"), addPane(m, "✳ cld-x [w]", ssh("-p", "2222", "ws2"))}
	m.Terminals[ids[0]].Cwd = filepath.Join(s.Home, "src")
	m.Windows = []mac.Window{windowOf(tabOf(ids[0])), windowOf(tabOf(ids[1], ids[2])),
		windowOf(tabOf(ids[3]))}
	tick(t, s, m)
	return m, ids
}

// Where Ghostty brings every window back, the quit loses nothing. Each pane that was on ssh to a
// host keeps that binding as last, which the dry-run lists to join again. Where Ghostty does not
// run, the dry-run reads the record as if it brought nothing back, and writes nothing.
func TestTabsQuitBroughtAllBack(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m, ids := quitMac(t, s)
	before := readFile(t, recordPath(s))
	saveModel(t, s, &mac.Model{})
	merged := time.Now()
	result := dryRun(t, s)
	want := "Ghostty is not running: the record as if it brought nothing back\n" +
		"Losses:\n  3 windows of 3 tabs and 4 panes, lost as Ghostty quit at "
	if result.Code != 0 || !strings.HasPrefix(result.Stdout, want) ||
		!slices.Contains([]string{clockAt(merged) + "\n", clockAt(time.Now()) + "\n"},
			strings.TrimPrefix(result.Stdout, want)) || readFile(t, recordPath(s)) != before {
		t.Errorf("no Ghostty: exit %d, stdout\n%s\nwant %q and the time, and tabs.json as it was",
			result.Code, result.Stdout, want)
	}
	n := relaunch(m, 4242, relaunched, ids...)
	r := tick(t, s, n)
	checkRecord(t, "all back", r, named(ids...), "*a/made | *right(b,c)/flat | *d/made", "",
		"quit: ")
	if r.Run.ID != runOf(n) || r.Previous != runOf(m) {
		t.Errorf("run %s, previous %s, want %s and %s", r.Run.ID, r.Previous, runOf(n), runOf(m))
	}
	quit := r.Losses[len(r.Losses)-1].At
	for i, want := range []*tabs.Last{
		{Host: "ws", Session: "app", Cut: "quit", Run: runOf(m), At: quit},
		{Host: "ws", Cut: "quit", Run: runOf(m), At: quit},
		nil,
		{Host: "ws2", Session: "x", Worktree: true, Cut: "quit", Run: runOf(m), At: quit},
	} {
		if got := r.Panes[ids[i]].Last; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("pane %d: last %+v, want %+v", i, got, want)
		}
	}
	at := clockAt(quit)
	checkReport(t, s, "Panes to join again:\n"+
		"  in ~/src: session 'app' on ws, as Ghostty quit at "+at+"\n"+
		"  in /Users/me: ssh to ws, as Ghostty quit at "+at+"\n"+
		"  in /Users/me: session 'x' on ws2, as Ghostty quit at "+at+"\n"+
		"Ghostty brought back every window as it quit at "+at+"\n")
}

// The windows Ghostty does not bring back are the quit's one loss, with their panes as they were.
func TestTabsQuitBroughtSomeBack(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name          string
		back          []int // the panes back, by place
		windows, want string
		report        string
	}{
		{"some", []int{0, 3}, "*a/made | *d/made", "quit: *right(b,c)/flat",
			"a window of 1 tab and 2 panes"},
		{"none", nil, "", "quit: *a/made | *right(b,c)/flat | *d/made",
			"3 windows of 3 tabs and 4 panes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m, ids := quitMac(t, s)
			var back []string
			for _, i := range test.back {
				back = append(back, ids[i])
			}
			r := tick(t, s, relaunch(m, 4242, relaunched, back...))
			checkRecord(t, test.name, r, named(ids...), test.windows, "", test.want)
			if len(r.Losses) == 1 {
				checkBackLast(t, r, ids, back, r.Losses[0].At, runOf(m))
				checkCopy(t, r.Losses[0], ids[1], shown{Kind: "ssh", Host: "ws", Title: "me@ws: ~"})
				checkReport(t, s, "Losses:\n  "+test.report+", lost as Ghostty quit at "+
					clockAt(r.Losses[0].At)+"\n")
			}
			for _, id := range ids[1:3] {
				if r.Panes[id] != nil {
					t.Errorf("a pane lost in the quit is still the record's: %+v", r.Panes[id])
				}
			}
		})
	}
}

// checkBackLast reports the panes back, of quitMac's ids, unless each has the last that the quit
// of run at time at gave it.
func checkBackLast(t *testing.T, r *tabs.Record, ids, back []string, at time.Time, run string) {
	t.Helper()
	want := map[string]*tabs.Last{
		ids[0]: {Host: "ws", Session: "app", Cut: "quit", Run: run, At: at},
		ids[3]: {Host: "ws2", Session: "x", Worktree: true, Cut: "quit", Run: run, At: at},
	}
	for _, id := range back {
		if p := r.Panes[id]; p == nil || fmt.Sprint(p.Last) != fmt.Sprint(want[id]) {
			t.Errorf("pane %s back: %+v, want last %+v", id, p, want[id])
		}
	}
}

// A window still waiting for Ghostty's undo as Ghostty quits joins the quit's loss. A tab waiting
// so stays a tab's loss, which only --closed reopens.
func TestTabsQuitWhilePending(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b, c, d := addPane(m, "~"), addPane(m, "~"), addPane(m, "~"), addPane(m, "~")
	names := named(a, b, c, d)
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b)), windowOf(tabOf(c)), windowOf(tabOf(d))}
	tick(t, s, m)
	m.Windows = []mac.Window{windowOf(tabOf(a)), windowOf(tabOf(d))}
	checkRecord(t, "closed", tick(t, s, m), names, "*a/made | *d/made",
		"tab 2 beside a: b/made\nclosed: *c/made", "")
	r := tick(t, s, relaunch(m, 4242, relaunched, a, d))
	checkRecord(t, "quit", r, names, "*a/made | *d/made", "",
		"tab 2 beside a: b/made\nquit: *c/made")
}

// A last that a quit set clears at the next run change, unless the pane was on ssh again by then.
func TestTabsQuitClearsLast(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a, b := addPane(m, "✳ cld-a", ssh("ws")), addPane(m, "✳ cld-b", ssh("ws"))
	m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b))}
	tick(t, s, m)
	second := relaunch(m, 4242, relaunched, a, b)
	tick(t, s, second)
	front(second, b, ssh("ws"))
	second.Terminals[b].Name = "✳ cld-c"
	tick(t, s, second)
	r := tick(t, s, relaunch(second, 4343, relaunched.Add(time.Hour), a, b))
	if last := r.Panes[a].Last; last != nil {
		t.Errorf("a: last %+v, want none after the second quit", last)
	}
	if last := r.Panes[b].Last; last == nil || last.Session != "c" || last.Run != runOf(second) {
		t.Errorf("b: last %+v, want session c of the second run", last)
	}
}

// A quit's time is when the watcher saw Ghostty's run end, else its last tick in that run, else the
// merge's. A tick from before the run's start was an earlier run's watcher's.
func TestTabsQuitTime(t *testing.T) {
	t.Parallel()
	ended := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	ticked := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	for _, test := range []struct {
		name       string
		ended      bool
		tick, want time.Time // zero: no watch file, and the merge's time
	}{
		{"run.ended", true, ticked, ended},
		{"the last tick", false, ticked, ticked},
		{"a tick before the run", false, ghosttyStart.Add(-time.Hour), time.Time{}},
		{"the merge", false, time.Time{}, time.Time{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			a := addPane(m, "✳ cld-a", ssh("ws"))
			m.Windows = []mac.Window{windowOf(tabOf(a))}
			r := recordOf(m, map[string]*tabs.Pane{a: {Kind: "cld", Host: "ws", Session: "a"}},
				tabs.Window{Tabs: []tabs.Tab{{Selected: true, Shape: "made", Tree: leaf(a)}}})
			if test.ended {
				r.Run.Ended = ended
			}
			writeRecord(t, s, r)
			if !test.tick.IsZero() {
				watch := filepath.Join(tabsDir(s), "watch")
				s.WriteFile(watch, "")
				if err := os.Chtimes(watch, test.tick, test.tick); err != nil {
					t.Fatal(err)
				}
			}
			before := time.Now().UTC().Truncate(time.Second)
			r = tick(t, s, relaunch(m, 4242, relaunched))
			if len(r.Losses) != 1 || !quitAt(r.Losses[0].At, test.want, before) {
				t.Errorf("losses %s, want a quit at %v", drawLosses(r.Losses, named(a)), test.want)
			}
		})
	}
}

// quitAt reports whether a quit at got is at want, or where want is zero, at a merge since before.
func quitAt(got, want, before time.Time) bool {
	if want.IsZero() {
		return !got.Before(before) && !got.After(time.Now())
	}
	return got.Equal(want)
}

// The record keeps 32 losses at most, the newest, of this run and the one before, none older than
// 30 days.
func TestTabsQuitBounds(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	now := time.Now().UTC().Truncate(time.Second)
	r := recordOf(m, map[string]*tabs.Pane{a: {Kind: "local"}})
	r.Previous = "1@2025-01-01T00:00:00Z"
	loss := func(run string, ago time.Duration) *tabs.Loss {
		return &tabs.Loss{At: now.Add(-ago), Run: run, Cause: "closed",
			Windows: []tabs.Window{{Tabs: []tabs.Tab{{Shape: "made", Tree: leaf("gone")}}}}}
	}
	r.Losses = []*tabs.Loss{loss("2@2025-01-01T00:00:00Z", time.Minute),
		loss(r.Run.ID, 31*24*time.Hour), loss(r.Previous, time.Second)}
	for i := range 33 {
		r.Losses = append(r.Losses, loss(r.Run.ID, time.Duration(40-i)*time.Minute))
	}
	writeRecord(t, s, r)
	var got []string
	for _, l := range tick(t, s, m).Losses {
		got = append(got, fmt.Sprintf("%s %v", l.Run, now.Sub(l.At)))
	}
	var want []string
	for i := 2; i < 33; i++ {
		want = append(want, fmt.Sprintf("%s %v", r.Run.ID, time.Duration(40-i)*time.Minute))
	}
	want = append(want, r.Previous+" 1s")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("losses\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
