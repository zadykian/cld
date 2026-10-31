package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What a restore's report says beyond a plain apply: a tab off its index, and an apply that fails.

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

// An apply that meets AppleScript turned off fails each typing, and one that macOS denies, or
// that Ghostty quits, stops. Each warns, with status 1, and the pane keeps its last untyped.
func TestTabsApplyMeets(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ mode, warning string }{
		{"off", "cannot type into the pane in /Users/me: Can't get object. (-1728)"},
		{"denied", "the restore stopped: macOS denies control of Ghostty: in System Settings"},
		{"gone", "the restore stopped: Ghostty quit while cld read it"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			_, a := typedMac(t, s)
			wait := heldRestore(t, s)
			s.WriteProbeFile(mac.FailFile, test.mode+"\n")
			result := wait()
			if warning := "cld: warning: " + test.warning; result.Code != 1 ||
				result.Stdout != "" || !strings.HasPrefix(result.Stderr, warning) {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 and the warning %q",
					result.Code, result.Stdout, result.Stderr, warning)
			}
			if last := readRecord(t, s).Panes[a].Last; last == nil || last.Typed != nil {
				t.Errorf("last %+v, want it, with nothing typed", last)
			}
		})
	}
}
