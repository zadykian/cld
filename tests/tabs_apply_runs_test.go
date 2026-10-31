package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A restore's runs around apply: an apply that gives no result, and a Ghostty the restore opens.

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
