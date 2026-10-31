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
