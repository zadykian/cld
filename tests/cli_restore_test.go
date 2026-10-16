package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The word after restore.

// restoreCommands ends the message for a missing or unknown word after restore.
const restoreCommands = "cld restore sessions or cld restore tabs (see cld help)\n"

// restore runs nothing itself: sessions or tabs follows it, checked before cobra sees it, as
// setup's commands are (decisions 59.1 and 60.1). cobra would run tabs for restore --help=false
// tabs. restore tabs reopens nothing yet, and asks for --watch or --dry-run before it looks for
// osascript.
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
		{[]string{"restore", "Tabs"}, "cld: restore: unknown command 'Tabs': " + restoreCommands},
		{[]string{"restore", "session"},
			"cld: restore: unknown command 'session': " + restoreCommands},
		{[]string{"restore", "x", "tabs"}, "cld: restore: unknown command 'x': " + restoreCommands},
		{[]string{"restore", "-x"}, unexpected("restore", "-x")},
		{[]string{"restore", "--help=false", "tabs"}, unexpected("restore", "--help=false")},
		{[]string{"restore", "--help=false", "sessions"}, unexpected("restore", "--help=false")},
		{[]string{"restore", "--"}, unexpected("restore", "--")},
		{[]string{"restore", "--", "sessions"}, unexpected("restore", "--")},
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
