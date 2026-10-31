package tests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The plan a restore hands apply.js, run by the fake osascript, and the record after it: what
// apply made, where, and what it failed to.

// heldRestore starts cld restore tabs in s, and returns once its snapshot has read the model. The
// fake osascript then holds it for a second, while the restore holds its locks. It returns what
// waits for the restore to end.
func heldRestore(t *testing.T, s *sandbox.Sandbox) func() sandbox.Result {
	t.Helper()
	s.WriteProbeFile(mac.DelayFile, "1\n")
	wait := startCldAsync(t, s, s.Work, tabsEnv, "restore", "tabs")
	sandbox.WaitFor(t, 10*time.Second, "the restore's snapshot", func() bool {
		return !exists(filepath.Join(s.ProbeDir, mac.DelayFile))
	})
	return wait
}

// typedMac is a Ghostty of one pane, a shell at its prompt, whose last a quit cut from session
// app on ws, which Ghostty has terminfo for. It returns the pane's id.
func typedMac(t *testing.T, s *sandbox.Sandbox) (*mac.Model, string) {
	t.Helper()
	s.WriteProbeFile(mac.SSHCache, "me@ws\n")
	m := ghosttyApp(t, s)
	a := addPane(m, "~")
	m.Windows = []mac.Window{windowOf(tabOf(a))}
	saveModel(t, s, m)
	now := time.Now().UTC().Truncate(time.Second)
	writeRecord(t, s, recordOf(m, map[string]*tabs.Pane{a: {Kind: "local", Since: now,
		Last: &tabs.Last{Host: "ws", Session: "app", Cut: "quit", Run: "1@x", At: now}}},
		tabs.Window{Tabs: []tabs.Tab{{Selected: true, Shape: "made", Tree: leaf(a)}}}))
	return m, a
}

// A pane in which a program started after the restore's snapshot is busy: apply.js types nothing
// there, and the restore notes it. The pane keeps its last, for the next restore.
func TestTabsApplyBusy(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m, a := typedMac(t, s)
	wait := heldRestore(t, s)
	m.Terminals[a].PID = m.NewPID()
	saveModel(t, s, m)
	checkExit(t, "restore tabs", wait(), 0, "", "cld: left the pane in /Users/me as it is: a "+
		"program runs there now\n"+noWatcher)
	checkTyped(t, loadModel(t, s), map[int][]string{1: nil})
	if last := readRecord(t, s).Panes[a].Last; last == nil || last.Typed != nil {
		t.Errorf("last %+v, want it kept, with nothing typed", last)
	}
}

// A pane a restore typed into keeps its last, now with the time of its typing. A tick leaves it
// until the line has had its time to join.
func TestTabsApplyTyped(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	_, a := typedMac(t, s)
	start := time.Now().UTC().Truncate(time.Second)
	checkExit(t, "restore tabs", s.RunCld(tabsEnv, "restore", "tabs"), 0,
		"Typed into the pane in /Users/me: session 'app' on ws\n", noWatcher)
	if last := readRecord(t, s).Panes[a].Last; last == nil || last.Typed == nil ||
		last.Typed.Before(start) {
		t.Errorf("last %+v, want it kept, typed at or after %v", last, start)
	}
}
