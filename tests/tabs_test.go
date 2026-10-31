package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

// A first dry-run writes the record, readable by the user alone, with Ghostty's run: its pid and
// its ps row's start, and its version.
func TestTabsDryRunRecords(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	saveModel(t, s, ghosttyAt(812, ghosttyStart))
	checkExit(t, "restore tabs --dry-run", dryRun(t, s), 0,
		"Ghostty 1.4.0, pid 812, started 2025-01-02 09:12\n", "")
	want := readFile(t, filepath.Join("testdata", "tabs", "record.json"))
	if got := readFile(t, recordPath(s)); got != want {
		t.Errorf("tabs.json:\n%s\nwant testdata/tabs/record.json:\n%s", got, want)
	}
	checkMode(t, recordPath(s), 0o600)
	checkMode(t, tabsDir(s), 0o700)
}
