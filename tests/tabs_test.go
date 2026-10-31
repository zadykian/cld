package tests

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld restore tabs (#176) against the model Mac of internal/mac, through the fakes that
// tabs_fakes_test.go drives directly. The helpers the tabs_* files share are here.

// tabsEnv is every tabs test's environment beside the sandbox's: the dry-run's times in UTC.
var tabsEnv = map[string]string{"TZ": "UTC"}

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
