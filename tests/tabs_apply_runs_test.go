package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A restore's runs around apply: an apply that gives no result, a Ghostty the restore opens, and
// the watcher it starts.

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

// Where Ghostty does not run, a restore opens it by its bundle id. It reads Ghostty until two
// snapshots in a row show the same terminals in the same tabs and windows, a third where a window
// opens after the first. Then it joins the panes Ghostty brought back, and reopens the others.
func TestTabsApplyOpens(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		late  bool // a window opens once the first snapshot after open has read Ghostty
		reads int  // the snapshots after open
	}{{"at once", false, 2}, {"a window late", true, 3}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			s.WriteProbeFile(mac.SSHCache, "me@ws\n")
			_, ended := stoppedMac(t, s)
			before := snapshots(t, s)
			if test.late {
				s.WriteProbeFile(mac.DelayFile, "1\n")
			}
			wait := startCldAsync(t, s, s.Work, tabsEnv, "restore", "tabs")
			if test.late {
				sandbox.WaitFor(t, 10*time.Second, "the first read", func() bool {
					return !exists(filepath.Join(s.ProbeDir, mac.DelayFile))
				})
				m := loadModel(t, s)
				m.Windows = append(m.Windows, windowOf(tabOf(addPane(m, "~"))))
				saveModel(t, s, m)
			}
			checkExit(t, "restore tabs", wait(), 0, openedReport(ended), noWatcher)
			checkContent(t, filepath.Join(s.ProbeDir, mac.OpenLog), "-b com.mitchellh.ghostty\n")
			checkContent(t, filepath.Join(s.ProbeDir, mac.NextFile), "")
			var scripts []string
			for _, call := range s.OsascriptCalls()[before:] {
				scripts = append(scripts, call.Script)
			}
			want := append(slices.Repeat([]string{"snapshot"}, 1+test.reads), "apply")
			if !slices.Equal(scripts, want) {
				t.Errorf("osascript ran %q, want %q", scripts, want)
			}
		})
	}
}

// openedReport is what a restore reports of stoppedMac's Ghostty, whose run ended at ended.
func openedReport(ended time.Time) string {
	return "Typed into the pane in /Users/me: session 'app' on ws\n" +
		"Reopened a window of 1 tab, lost as Ghostty quit at " + clockAt(ended) + "\n"
}

// Where Ghostty opens no window, or no terminal of its gets its tty, in 10 s, the restore gives
// up waiting. It fails where Ghostty does not run by then, and goes on with a note where it does.
// Windows that change as a snapshot reads them are read again. Without open, the restore fails.
func TestTabsApplyOpenWaits(t *testing.T) {
	t.Parallel()
	t.Run("windows changing", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		s.WriteProbeFile(mac.SSHCache, "me@ws\n")
		_, ended := stoppedMac(t, s)
		s.WriteProbeFile(mac.FailFile, "changed\n")
		before := snapshots(t, s)
		wait := startCldAsync(t, s, s.Work, tabsEnv, "restore", "tabs")
		waitSnapshots(t, s, before+3)
		if err := os.Remove(filepath.Join(s.ProbeDir, mac.FailFile)); err != nil {
			t.Fatal(err)
		}
		checkExit(t, "restore tabs", wait(), 0, openedReport(ended), noWatcher)
	})
	t.Run("no open", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		saveModel(t, s, &mac.Model{})
		env := map[string]string{"TZ": "UTC", "PATH": s.Tools("osascript", "ps", "ssh", "ghostty")}
		checkFailed(t, s.RunCld(env, "restore", "tabs"), 1,
			"cld: cannot open Ghostty: open is not installed\n")
	})
	t.Run("not started", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		saveModel(t, s, &mac.Model{})
		checkFailed(t, s.RunCld(tabsEnv, "restore", "tabs"), 1,
			"cld: Ghostty did not start in 10 s\n")
		checkContent(t, filepath.Join(s.ProbeDir, mac.OpenLog), "-b com.mitchellh.ghostty\n")
	})
	t.Run("still opening", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		saveModel(t, s, &mac.Model{})
		m := ghosttyApp(t, s)
		a := addPane(m, "~")
		m.Terminals[a].TTY = ""
		m.Windows = []mac.Window{windowOf(tabOf(a))}
		saveNext(t, s, m)
		checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0, "",
			"cld: Ghostty was still opening windows\n"+noWatcher)
	})
}

// A restore run in a Ghostty pane starts the watcher, where none runs. One run elsewhere notes
// that none runs, unless one does.
func TestTabsApplyStartsWatcher(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
	saveModel(t, s, m)
	checkExit(t, "restore tabs elsewhere", s.RunCld(tabsEnv, "restore", "tabs"), 0, "", noWatcher)
	t.Cleanup(func() { killWatchers(t, s) })
	checkExit(t, "restore tabs in a Ghostty pane", s.RunCld(watchEnv, "restore", "tabs"), 0, "",
		"")
	waitTicked(t, s, m)
	checkExit(t, "restore tabs elsewhere, the watcher running",
		s.RunCld(tabsEnv, "restore", "tabs"), 0, "", "")
}

// watchedMac is a Ghostty of one shell, a, whose record lost a window on session x on ws a minute
// ago, and the watcher that reads it. It returns a and the loss.
func watchedMac(t *testing.T, s *sandbox.Sandbox) (string, *tabs.Loss) {
	t.Helper()
	s.WriteProbeFile(mac.SSHCache, "me@ws\n")
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	r := tick(t, s, m)
	lost := shellLoss(r.Run.ID, "closed", time.Now().UTC().Add(-time.Minute).Truncate(time.Second),
		"x")
	lost.Windows[0].Tabs[0].Tree.Copy = &tabs.Pane{Kind: "cld", Host: "ws", Session: "x"}
	r.Losses = []*tabs.Loss{lost}
	writeRecord(t, s, r)
	startWatch(t, s)
	waitTicked(t, s, m)
	return a, lost
}

// A restore holds the record's lock until the record notes what its apply made: a watcher's tick
// meanwhile merges nothing. Then the window it made, and its pane's last, survive the next ticks.
func TestTabsApplyHoldsLock(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a, lost := watchedMac(t, s)
	s.WriteProbeFile(mac.ApplyDelayFile, "8\n")
	wait := startCldAsync(t, s, s.Work, tabsEnv, "restore", "tabs")
	sandbox.WaitFor(t, 10*time.Second, "the apply", func() bool {
		return !exists(filepath.Join(s.ProbeDir, mac.ApplyDelayFile))
	})
	record, ticked := statRecord(t, s), watchTime(t, s)
	waitSnapshots(t, s, snapshots(t, s)+1)
	time.Sleep(time.Second) // for the tick's merge, were the record's lock free
	if now := statRecord(t, s); !os.SameFile(now, record) || !now.ModTime().Equal(
		record.ModTime()) || !watchTime(t, s).Equal(ticked) {
		t.Error("a tick merged while the restore held the record")
	}
	checkExit(t, "restore tabs", wait(), 0,
		"Reopened a window of 1 tab, closed at "+clockAt(lost.At)+"\n", "")
	made, ticked := loadModel(t, s).Windows[0].Tabs[0].Focused, watchTime(t, s)
	sandbox.WaitFor(t, 15*time.Second, "the watcher's next merge", func() bool {
		return watchTime(t, s).After(ticked)
	})
	checkStale(t, s, "after the next merge", a, made)
}

// statRecord is the record of Ghostty's tabs in s, as it stands.
func statRecord(t *testing.T, s *sandbox.Sandbox) os.FileInfo {
	t.Helper()
	info, err := os.Stat(recordPath(s))
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// A watcher's snapshot begun before a restore, and answered after it, is dropped. So the window
// the restore made, and its pane's last, survive the watcher's next ticks.
func TestTabsApplyStale(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a, lost := watchedMac(t, s)
	s.WriteProbeFile(mac.DelayFile, "6\n")
	sandbox.WaitFor(t, 10*time.Second, "the watcher's slow snapshot", func() bool {
		return !exists(filepath.Join(s.ProbeDir, mac.DelayFile))
	})
	checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0,
		"Reopened a window of 1 tab, closed at "+clockAt(lost.At)+"\n", "")
	made := loadModel(t, s).Windows[0].Tabs[0].Focused
	s.WriteProbeFile(mac.FailFile, "changed\n")
	waitSnapshots(t, s, snapshots(t, s)+1)
	checkStale(t, s, "after the slow snapshot", a, made)
	ticked := watchTime(t, s)
	if err := os.Remove(filepath.Join(s.ProbeDir, mac.FailFile)); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 15*time.Second, "the watcher's next merge", func() bool {
		return watchTime(t, s).After(ticked)
	})
	checkStale(t, s, "after the next merge", a, made)
}

// checkStale reports the record in s, as the watcher left it when. It must hold the window that
// TestTabsApplyStale's restore made, in front of a's, and its pane's last.
func checkStale(t *testing.T, s *sandbox.Sandbox, when, a, made string) {
	t.Helper()
	r := readRecord(t, s)
	checkRecord(t, when, r, named(a, made), "*b/made | *a/made", "", "closed: */made")
	if p := r.Panes[made]; p == nil || p.Last == nil || p.Last.Session != "x" {
		t.Errorf("%s: the pane made is %+v, want its last on session x", when, p)
	}
}
