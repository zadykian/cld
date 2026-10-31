package tests

import (
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A restore's runs around apply: an apply that gives no result.

// An apply that gives no result fails the restore with status 1, and marks no loss restored and
// no line typed. So does one whose result lacks builds of the plan. Where Ghostty opened panes by
// then, a merge enters them, and the warning counts them. Else it says why alone.
func TestTabsApplyStopped(t *testing.T) {
	t.Parallel()
	const why = "osascript: fake osascript: execution error"
	partial := map[string]string{"TZ": "UTC", "CLD_FAKE_OSASCRIPT_FAIL": "partial"}
	for _, test := range []struct{ mode, why string }{
		{"partial", why},
		{"short", "osascript: apply.js answered 1 of 1 typings and 1 of 5 builds"},
	} {
		t.Run("after a build, "+test.mode, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			applyMac(t, s, time.Now().UTC().Truncate(time.Second))
			env := map[string]string{"TZ": "UTC", "CLD_FAKE_OSASCRIPT_FAIL": test.mode}
			checkExit(t, "restore tabs", s.RunCld(env, "restore", "tabs", "--all", "--closed"), 1,
				"", "cld: warning: the restore stopped after Ghostty opened 1 pane ("+test.why+
					"): run cld restore tabs --dry-run before running it again\n"+
					uncachedNote("ws2")+noWatcher)
			r := readRecord(t, s)
			if p := r.Panes[mac.TerminalID(11)]; p == nil || p.Kind != "local" {
				t.Errorf("the pane made is %+v, want it in the record, a shell", p)
			}
			if last := r.Panes[mac.TerminalID(1)].Last; last == nil || last.Typed != nil {
				t.Errorf("a's last %+v, want it, with nothing typed", last)
			}
			if got := restoredOf(r); got != "" {
				t.Errorf("restored %s, want none", got)
			}
		})
	}
	t.Run("before any", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		_, a := typedMac(t, s)
		checkExit(t, "restore tabs", s.RunCld(partial, "restore", "tabs"), 1, "",
			"cld: warning: the restore stopped: "+why+"\n"+noWatcher)
		if last := readRecord(t, s).Panes[a].Last; last == nil || last.Typed != nil {
			t.Errorf("last %+v, want it, with nothing typed", last)
		}
	})
}
