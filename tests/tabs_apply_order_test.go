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

// Which builds a restore makes, and in which order (decision 59.6): tabs back at their places. A
// pane that two losses hold is rebuilt once, and one open again not at all.

// tabLoss is the loss at time at of a tab of one pane, id, a shell in /Users/me/ID, which went
// back at index beside the pane anchor.
func tabLoss(run string, at time.Time, id, anchor string, index int) *tabs.Loss {
	l := shellLoss(run, "tab", at, id)
	l.Anchor = &tabs.Anchor{Pane: anchor, Index: index}
	return l
}

// Two tabs closed from one window go back at their places, whichever closed first. A tab's index
// is its place as it closed, among the tabs that closed after it, so the newest goes back first.
// Tabs that closed at once go back by index.
func TestTabsApplyTabOrder(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name          string
		first, second string // the tabs closed, A or B, the older first
		at            [2]int // their indexes as they closed
		same          bool   // they closed at once
		reopened      [2]int // the second's index, then the first's
	}{
		{"A, then B", "A", "B", [2]int{2, 2}, false, [2]int{2, 2}},
		{"B, then A", "B", "A", [2]int{3, 2}, false, [2]int{2, 3}},
		{"both at once", "B", "A", [2]int{3, 2}, true, [2]int{2, 3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			x, y := addPane(m, "~"), addPane(m, "~")
			m.Terminals[x].Cwd, m.Terminals[y].Cwd = "/Users/me/x", "/Users/me/y"
			m.Windows = []mac.Window{windowOf(tabOf(x), tabOf(y))}
			r := tick(t, s, m)
			now := time.Now().UTC().Truncate(time.Second)
			older, newer := now.Add(-3*time.Minute), now.Add(-2*time.Minute)
			if test.same {
				older = newer
			}
			r.Losses = []*tabs.Loss{tabLoss(r.Run.ID, older, test.first, x, test.at[0]),
				tabLoss(r.Run.ID, newer, test.second, x, test.at[1])}
			writeRecord(t, s, r)
			checkExit(t, "restore tabs --closed", s.RunCld(tabsEnv, "restore", "tabs", "--closed"),
				0, fmt.Sprintf("Reopened a tab at %d in its window, closed at %s\n"+
					"Reopened a tab at %d in its window, closed at %s\n", test.reopened[0],
					clockAt(newer), test.reopened[1], clockAt(older)), noWatcher)
			if got := tabDirs(loadModel(t, s)); got != "x A B y" {
				t.Errorf("Ghostty's window holds %s, want x A B y", got)
			}
		})
	}
}

// tabDirs names the tabs of m's front window by their first panes' directories.
func tabDirs(m *mac.Model) string {
	var names []string
	for _, tab := range m.Windows[0].Tabs {
		names = append(names, filepath.Base(m.Terminals[tab.Tree.Leaves()[0]].Cwd))
	}
	return strings.Join(names, " ")
}

// A pane that two losses hold, as a window Ghostty brought back late closed again, is rebuilt
// once, from the newer. Losses of which nothing remains to build, their panes open again, run no
// apply. Each restore marks both losses restored.
func TestTabsApplyPruned(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, stdout string
		open         bool // the losses' panes are open again
	}{
		{"a pane two losses hold", "Reopened a window of 1 tab and 2 panes, closed at ", false},
		{"panes open again", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			a, b := addPane(m, "~"), addPane(m, "~")
			m.Windows = []mac.Window{windowOf(tabOf(a), tabOf(b))}
			r := tick(t, s, m)
			x, y := "x", "y"
			if test.open {
				x, y = a, b
			}
			now := time.Now().UTC().Truncate(time.Second)
			older := shellLoss(r.Run.ID, "closed", now.Add(-10*time.Minute), x)
			newer := shellLoss(r.Run.ID, "closed", now.Add(-5*time.Minute), x)
			tree := &newer.Windows[0].Tabs[0].Tree
			*tree = split("right", *tree, &tabs.Node{Pane: y,
				Copy: &tabs.Pane{Kind: "local", Cwd: "/Users/me/y"}})
			r.Losses = []*tabs.Loss{older, newer}
			writeRecord(t, s, r)
			if test.stdout != "" {
				test.stdout += clockAt(newer.At) + "\n"
			}
			checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0, test.stdout,
				noWatcher)
			scripts := []string{}
			for _, call := range s.OsascriptCalls() {
				scripts = append(scripts, call.Script)
			}
			if applied := slices.Contains(scripts, "apply"); applied == test.open {
				t.Errorf("osascript ran %q, want an apply %t", scripts, !test.open)
			}
			if got := restoredOf(readRecord(t, s)); got != x+" "+x {
				t.Errorf("restored %s, want both losses", got)
			}
		})
	}
}
