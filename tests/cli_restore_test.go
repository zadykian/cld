package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The word after restore.

// restoreCommands ends the message for an unknown word after restore.
const restoreCommands = "cld restore or cld restore tabs (see cld help)\n"

// restore takes no word but its command tabs, checked before cobra sees it, as setup's are
// (decision 59.1). An option before tabs is no option of restore's: cobra would run tabs for
// restore --help=false tabs. restore tabs reopens nothing yet, and asks for --watch or --dry-run
// before it looks for osascript.
func TestRestoreRequiresTabs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"restore", "x"}, "cld: restore: unknown command 'x': " + restoreCommands},
		{[]string{"restore", "Tabs"}, "cld: restore: unknown command 'Tabs': " + restoreCommands},
		{[]string{"restore", "x", "tabs"}, "cld: restore: unknown command 'x': " + restoreCommands},
		{[]string{"restore", "-x"}, unexpected("restore", "-x")},
		{[]string{"restore", "--help=false", "tabs"}, unexpected("restore", "--help=false")},
		{[]string{"restore", "--"}, unexpected("restore", "--")},
		{[]string{"restore", "--", "tabs"}, unexpected("restore", "--")},
		{[]string{"restore", ""}, unexpected("restore", "")},
		{[]string{"restore", "tabs"},
			"cld: restore tabs: give --watch or --dry-run (see cld help)\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools()}, test.args...)
			checkFailed(t, result, 2, test.want)
		})
	}
}
