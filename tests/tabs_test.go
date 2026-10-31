package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/internal/session"
	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld restore tabs (#176) against the model Mac of internal/mac, through the fakes that
// tabs_fakes_test.go drives directly. A dry-run merges as a watcher's tick does, so the record's
// tests need no watcher. The helpers the tabs_* files share are here.

// tabsEnv is every tabs test's environment beside the sandbox's: the dry-run's times in UTC.
var tabsEnv = map[string]string{"TZ": "UTC"}

// tabsDir is the directory of cld's record of Ghostty's tabs in s.
func tabsDir(s *sandbox.Sandbox) string {
	return filepath.Join(s.Home, ".local", "state", "cld", "tabs")
}

// recordPath is the record of Ghostty's tabs in s.
func recordPath(s *sandbox.Sandbox) string {
	return filepath.Join(tabsDir(s), "tabs.json")
}

// readRecord is the record of Ghostty's tabs in s.
func readRecord(t *testing.T, s *sandbox.Sandbox) *tabs.Record {
	t.Helper()
	data, err := os.ReadFile(recordPath(s))
	if err != nil {
		t.Fatal(err)
	}
	var r tabs.Record
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("tabs.json: %v\n%s", err, data)
	}
	return &r
}

// writeRecord writes r as the record of Ghostty's tabs in s, as cld would: for times beyond a
// test's span.
func writeRecord(t *testing.T, s *sandbox.Sandbox, r *tabs.Record) {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tabsDir(s), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath(s), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// dryRun runs cld restore tabs --dry-run in s.
func dryRun(t *testing.T, s *sandbox.Sandbox) sandbox.Result {
	t.Helper()
	return s.RunCld(tabsEnv, "restore", "tabs", "--dry-run")
}

// tick saves m as s's Mac, and merges what it shows into the record by a dry-run, which must
// succeed. It returns the record then.
func tick(t *testing.T, s *sandbox.Sandbox, m *mac.Model) *tabs.Record {
	t.Helper()
	saveModel(t, s, m)
	if result := dryRun(t, s); result.Code != 0 || result.Stderr != "" {
		t.Fatalf("restore tabs --dry-run: exit %d, stderr %q", result.Code, result.Stderr)
	}
	return readRecord(t, s)
}

// sinceTimes matches each pane's since in tabs.json, the tick's time, which
// testdata/tabs/record.json gives as T.
var sinceTimes = regexp.MustCompile(`"since": "[^"]*"`)

// A first dry-run writes the record, readable by the user alone, of every pane by its terminal's
// id. It prints Ghostty, that no watcher runs, and the notes on Ghostty's config.
func TestTabsDryRunRecords(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.WriteProbeFile(mac.ShowConfig, "title = x\nmacos-titlebar-style = hidden\n")
	m := ghosttyAt(812, ghosttyStart)
	ssh := addPane(m, "◐ cld-app-3", []string{"ssh", "ws"})
	shell := addPane(m, "~")
	vim := addPane(m, "vim", []string{"vim", "notes"})
	m.Terminals[shell].Cwd = "/Users/me/src"
	m.Windows = []mac.Window{windowOf(tabOf(ssh), tabOf(shell, vim))}
	saveModel(t, s, m)
	before := time.Now().UTC().Truncate(time.Second)
	result := dryRun(t, s)
	checkExit(t, "restore tabs --dry-run", result, 0, dryRunReport, "")
	want := readFile(t, filepath.Join("testdata", "tabs", "record.json"))
	got := sinceTimes.ReplaceAllString(readFile(t, recordPath(s)), `"since": "T"`)
	if got != want {
		t.Errorf("tabs.json:\n%s\nwant testdata/tabs/record.json:\n%s", got, want)
	}
	for id, p := range readRecord(t, s).Panes {
		if p.Since.Before(before) || p.Since.After(time.Now()) {
			t.Errorf("pane %s since %v, not the dry-run's time", id, p.Since)
		}
	}
	checkMode(t, recordPath(s), 0o600)
	checkMode(t, tabsDir(s), 0o700)
}

// dryRunReport is what the dry-run prints of TestTabsDryRunRecords's Ghostty.
const dryRunReport = `Ghostty 1.4.0, pid 812, started 2025-01-02 09:12
The watcher: none runs; cld restore tabs --watch in a Ghostty pane starts one
Notes:
  window-save-state is default: whether Ghostty brings back its windows after Cmd+Q rests on ` +
	`macOS's own setting; set it to always
  title is set in Ghostty's config: it hides cld's title, and with it each pane's session
  shell-integration-features lacks ssh-terminfo: cld types TERM=xterm-256color for hosts ` +
	`Ghostty has no terminfo cached for
  macos-titlebar-style is hidden: rebuilt tabs open as windows
`

// Under window-save-state = never, Ghostty brings back no window, whatever macOS's setting.
func TestTabsDryRunNever(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	s.WriteProbeFile(mac.ShowConfig, "window-save-state = never\n"+
		"shell-integration-features = ssh-terminfo\n")
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
	saveModel(t, s, m)
	checkReport(t, s, "Notes:\n  window-save-state is never: Ghostty brings back no window "+
		"after Cmd+Q\n")
}

// restore tabs runs neither tmux nor claude: a PATH of the tools it runs is enough.
func TestTabsNeedsNoTmux(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "✳ cld-x", []string{"ssh", "ws"})))}
	saveModel(t, s, m)
	tools := s.Tools("osascript", "ps", "ghostty")
	result := s.RunCld(map[string]string{"TZ": "UTC", "PATH": tools}, "restore", "tabs", "--dry-run")
	if result.Code != 0 || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, want exit 0 and no stderr", result.Code, result.Stderr)
	}
	if r := readRecord(t, s); len(r.Panes) != 1 {
		t.Errorf("recorded %d panes, want 1", len(r.Panes))
	}
}

// A dry-run that finds the record as Ghostty shows it writes nothing: the file keeps its bytes,
// its inode and its time. claude's busy markers, which turn every second, are its idle ✳ there.
func TestTabsWritesOnlyChanges(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyApp(t, s)
	ssh := addPane(m, "✳ cld-app", []string{"ssh", "ws"})
	m.Windows = []mac.Window{windowOf(tabOf(ssh, addPane(m, "~")))}
	tick(t, s, m)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(recordPath(s), old, old); err != nil {
		t.Fatal(err)
	}
	first, err := os.Stat(recordPath(s))
	if err != nil {
		t.Fatal(err)
	}
	data := readFile(t, recordPath(s))
	for _, marker := range []string{"✳", session.BusyEven, session.BusyOdd} {
		title := marker + " cld-app"
		m.Terminals[ssh].Name = title
		tick(t, s, m)
		info, err := os.Stat(recordPath(s))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(first, info) || !info.ModTime().Equal(old) ||
			readFile(t, recordPath(s)) != data {
			t.Errorf("titled %q: tabs.json written again", title)
		}
	}
}

// recordOf is a record of the run of m's Ghostty, of panes and windows, for writeRecord.
func recordOf(m *mac.Model, panes map[string]*tabs.Pane, windows ...tabs.Window) *tabs.Record {
	return &tabs.Record{Format: tabs.Format, Run: tabs.Run{ID: runOf(m), Version: m.Version},
		Panes: panes, Windows: windows, Pending: []*tabs.Loss{}, Losses: []*tabs.Loss{}}
}

// leaf is a record's leaf of pane id.
func leaf(id string) *tabs.Node { return &tabs.Node{Pane: id} }

// split is a record's split of first and second, right or down.
func split(direction string, first, second *tabs.Node) *tabs.Node {
	return &tabs.Node{Split: direction, First: first, Second: second}
}

// named names the panes ids a, b, c and on, for drawRecord.
func named(ids ...string) map[string]string {
	names := map[string]string{}
	for i, id := range ids {
		names[id] = string(rune('a' + i))
	}
	return names
}

// drawWindows draws a record's windows as shape draws the model's: " | " between windows and " "
// between tabs, * before a selected tab, and each tab's tree, then / and its shape. A pane is
// drawn by its name in names, and a split as right(A,B) or down(A,B).
func drawWindows(windows []tabs.Window, names map[string]string) string {
	drawn := make([]string, 0, len(windows))
	for _, w := range windows {
		tabList := make([]string, 0, len(w.Tabs))
		for _, tab := range w.Tabs {
			mark := ""
			if tab.Selected {
				mark = "*"
			}
			tabList = append(tabList, mark+drawNode(tab.Tree, names)+"/"+tab.Shape)
		}
		drawn = append(drawn, strings.Join(tabList, " "))
	}
	return strings.Join(drawn, " | ")
}

// drawNode draws a record's tree for drawWindows.
func drawNode(n *tabs.Node, names map[string]string) string {
	switch {
	case n == nil:
		return "nil"
	case n.Pane != "":
		return names[n.Pane]
	}
	return n.Split + "(" + drawNode(n.First, names) + "," + drawNode(n.Second, names) + ")"
}

// drawLosses draws losses, a line each: the cause, where a tab or split goes back, and its
// windows as drawWindows draws them. A tab goes back as "tab 2 beside a", the second in a's
// window, and a split as "pane right of a".
func drawLosses(losses []*tabs.Loss, names map[string]string) string {
	var lines []string
	for _, l := range losses {
		line := l.Cause
		switch a := l.Anchor; {
		case a == nil:
		case a.Split != "":
			line += " " + a.Split + " of " + names[a.Pane]
		default:
			line += fmt.Sprintf(" %d beside %s", a.Index, names[a.Pane])
		}
		lines = append(lines, line+": "+drawWindows(l.Windows, names))
	}
	return strings.Join(lines, "\n")
}

// clockAt is time at as the dry-run writes it: in UTC, as 15:04 today, else with its date.
func clockAt(at time.Time) string {
	now, at := time.Now().UTC(), at.UTC()
	if now.Format(time.DateOnly) == at.Format(time.DateOnly) {
		return at.Format("15:04")
	}
	return at.Format("2006-01-02 15:04")
}
