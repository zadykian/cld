package tests

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Ghostty's quits, which a merge sees as a new run: what came back, and what is lost.

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

// Where Ghostty brings every window back, the quit loses nothing. Where Ghostty does not run, the
// dry-run reads the record as if it brought nothing back, and writes nothing.
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
	checkReport(t, s, "Ghostty brought back every window as it quit at "+
		clockAt(r.Losses[len(r.Losses)-1].At)+"\n")
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
