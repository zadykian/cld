package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

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

// dryRun runs cld restore tabs --dry-run in s.
func dryRun(t *testing.T, s *sandbox.Sandbox) sandbox.Result {
	t.Helper()
	return s.RunCld(tabsEnv, "restore", "tabs", "--dry-run")
}

// The dry-run says whether Ghostty runs, and its version, pid and start. A Ghostty that does not
// run is no failure. NSRunningApplication gives the start, or else ps's row does.
func TestTabsDryRunRuns(t *testing.T) {
	t.Parallel()
	const started = "Ghostty 1.4.0, pid 812, started 2025-01-02 09:12\n"
	for _, test := range []struct {
		name string
		edit func(*mac.Model)
		want string
	}{
		{"running", func(*mac.Model) {}, started},
		{"no launch date", func(m *mac.Model) { m.Launched = 0 }, started},
		{"not running", func(m *mac.Model) { m.Running = false }, "Ghostty is not running\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyAt(ghosttyPID, ghosttyStart)
			test.edit(m)
			saveModel(t, s, m)
			checkExit(t, "restore tabs --dry-run", dryRun(t, s), 0, test.want, "")
		})
	}
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

// A first dry-run writes the record, readable by the user alone. It holds Ghostty's run, and every
// pane by its terminal's id.
func TestTabsDryRunRecords(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyAt(812, ghosttyStart)
	first, second := addPane(m, "~"), addPane(m, "vim")
	m.Terminals[second].Cwd = "/Users/me/src"
	m.Windows = []mac.Window{windowOf(tabOf(first), tabOf(second))}
	saveModel(t, s, m)
	before := time.Now().UTC().Truncate(time.Second)
	checkExit(t, "restore tabs --dry-run", dryRun(t, s), 0,
		"Ghostty 1.4.0, pid 812, started 2025-01-02 09:12\n", "")
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

// A dry-run that finds the record as Ghostty shows it writes nothing: the file keeps its bytes,
// its inode and its time.
func TestTabsWritesOnlyChanges(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	m := ghosttyAt(812, ghosttyStart)
	m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
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
	tick(t, s, m)
	info, err := os.Stat(recordPath(s))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(first, info) || !info.ModTime().Equal(old) || readFile(t, recordPath(s)) != data {
		t.Error("tabs.json written again")
	}
}
