package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The word after restore.

// restoreCommands ends the message for a missing or unknown word after restore.
const restoreCommands = "cld restore sessions (see cld help)\n"

// restore runs nothing itself: sessions follows it, checked before cobra sees it, as setup's
// commands are (decision 60.1). An option before sessions is no option of restore's: cobra would
// run sessions for restore --help=false sessions.
func TestRestoreRequiresCommand(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"restore"}, "cld: restore: missing command: " + restoreCommands},
		{[]string{"restore", ""}, "cld: restore: missing command: " + restoreCommands},
		{[]string{"restore", "", "sessions"}, "cld: restore: missing command: " + restoreCommands},
		{[]string{"restore", "x"}, "cld: restore: unknown command 'x': " + restoreCommands},
		{[]string{"restore", "Sessions"},
			"cld: restore: unknown command 'Sessions': " + restoreCommands},
		{[]string{"restore", "session"},
			"cld: restore: unknown command 'session': " + restoreCommands},
		{[]string{"restore", "x", "sessions"},
			"cld: restore: unknown command 'x': " + restoreCommands},
		{[]string{"restore", "-x"}, unexpected("restore", "-x")},
		{[]string{"restore", "--help=false", "sessions"}, unexpected("restore", "--help=false")},
		{[]string{"restore", "--"}, unexpected("restore", "--")},
		{[]string{"restore", "--", "sessions"}, unexpected("restore", "--")},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools()}, test.args...)
			checkFailed(t, result, 2, test.want)
		})
	}
}
