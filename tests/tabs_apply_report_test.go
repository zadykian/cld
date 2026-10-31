package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What a restore's report says beyond a plain apply: a tab whose pane beside it has gone or left
// no index, ssh to no host, one loss left. Then a tab off its index, an apply that fails, and a
// record the restore cannot write.

// reportRecord is a record at now of a Ghostty of one window, the shell a, whose last ssh named no
// host. Tabs closed 4 and 3 minutes ago went back beside a pane that has gone, and beside a with
// no index. A window closed 2 minutes ago showed ssh to no host, and one closed an hour ago is
// left. It returns the record and the times of its losses, those reopened first.
func reportRecord(t *testing.T, s *sandbox.Sandbox) (*tabs.Record, []time.Time) {
	t.Helper()
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	r := tick(t, s, m)
	now := time.Now().UTC().Truncate(time.Second)
	r.Panes[a].Last = &tabs.Last{Cut: "quit", Run: "1@x", At: now}
	ago := func(minutes int) time.Time { return now.Add(-time.Duration(minutes) * time.Minute) }
	gone, beside := tabLoss(r.Run.ID, ago(4), "t1", "gone", 2), tabLoss(r.Run.ID, ago(3), "t2", a, 0)
	window := shellLoss(r.Run.ID, "closed", ago(2), "w")
	window.Windows[0].Tabs[0].Tree.Copy = &tabs.Pane{Kind: "ssh", Cwd: "/Users/me/w"}
	r.Losses = []*tabs.Loss{shellLoss(r.Run.ID, "closed", ago(60), "old"), gone, beside, window}
	writeRecord(t, s, r)
	return r, []time.Time{ago(3), ago(4), ago(2), ago(60)}
}

// reportNotes are the notes on reportRecord's restore: a's ssh, and the window's, to no host.
const reportNotes = "a pane's last ssh named no host cld can type: left the pane in /Users/me " +
	"as it is\n" + "a lost pane showed ssh to no host cld can type: a shell takes its place in " +
	"/Users/me/w\n"

// A tab whose pane beside it has gone opens as a window, and one whose anchor has no index opens
// where Ghostty puts it in its window. A pane kept or lost that showed ssh to no host stays
// unchanged, or opens a shell, with a note. A note counts the one loss left.
func TestTabsApplyReport(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	_, at := reportRecord(t, s)
	result := s.RunCld(tabsEnv, "restore", "tabs", "--closed", "--dry-run")
	want := "Would reopen a tab in its window, closed at " + clockAt(at[0]) +
		":\n  a shell in /Users/me/t2\nWould reopen a tab as a window, closed at " +
		clockAt(at[1]) + ":\n  a shell in /Users/me/t1\nWould reopen 1 window, closed at " +
		clockAt(at[2]) + ":\n  1 tab: a shell in /Users/me/w\nLeft:\n  a window of 1 tab, " +
		"closed at " + clockAt(at[3]) + ": --older or --all\nNotes:\n  " +
		strings.ReplaceAll(strings.TrimSuffix(reportNotes, "\n"), "\n", "\n  ") + "\n"
	if result.Code != 0 || !strings.HasSuffix(result.Stdout, want) {
		t.Errorf("restore tabs --closed --dry-run: exit %d, stdout\n%s\nwant it to end with\n%s",
			result.Code, result.Stdout, want)
	}
	checkExit(t, "restore tabs --closed", s.RunCld(tabsEnv, "restore", "tabs", "--closed"), 0,
		"Reopened a tab at 2 in its window, closed at "+clockAt(at[0])+"\n"+
			"Reopened a tab as a window, closed at "+clockAt(at[1])+"\n"+
			"Reopened a window of 1 tab, closed at "+clockAt(at[2])+"\n",
		"cld: "+strings.ReplaceAll(strings.TrimSuffix(reportNotes, "\n"), "\n", "\ncld: ")+"\n"+
			"cld: 1 lost window is left: cld restore tabs --dry-run lists it\n"+noWatcher)
}

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
