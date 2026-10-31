package tests

import (
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Which builds a restore makes, and in which order (decision 59.6): a pane that two losses hold is
// rebuilt once, and one open again not at all.

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
