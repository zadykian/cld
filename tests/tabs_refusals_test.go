package tests

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What restore tabs refuses: a Mac it cannot read Ghostty on.

// Where cld cannot read Ghostty, the dry-run fails with status 1 and cld's message. So do
// osascript's own failure, and no answer in 15 s.
func TestTabsRefusesGhostty(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, mode, want string
	}{
		{"garbage", "garbage", "osascript: fake osascript: execution error"},
		{"timeout", "timeout", "Ghostty did not answer in 15 s; if macOS asked whether " +
			"the app cld runs in may control Ghostty, answer, then run cld restore tabs again"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			saveModel(t, s, ghosttyAt(ghosttyPID, ghosttyStart))
			env := map[string]string{"TZ": "UTC", "CLD_FAKE_OSASCRIPT_FAIL": test.mode}
			checkFailed(t, s.RunCld(env, "restore", "tabs", "--dry-run"), 1, "cld: "+test.want+"\n")
		})
	}
}
