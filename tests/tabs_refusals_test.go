package tests

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What restore tabs refuses: a Mac it cannot read Ghostty on.

// automationAdvice is what cld says to do where macOS denies it Automation of Ghostty.
const automationAdvice = ": in System Settings › Privacy & Security › Automation, allow " +
	"Ghostty under the app cld runs in, Ghostty itself in its panes; as a last resort in " +
	"Ghostty's panes, tccutil reset AppleEvents com.mitchellh.ghostty resets every Automation " +
	"grant Ghostty holds"

// Where cld cannot read Ghostty, the dry-run fails with status 1 and cld's message. So do
// osascript missing, macOS's Automation denied, AppleScript off in Ghostty's config and Ghostty
// 1.3.1. So do a read that changed twice, and a Ghostty that quit as cld read it or ps. So do
// osascript's own failure, no answer in 15 s, a Ghostty that quit as cld read it, and Ghostty run
// twice.
func TestTabsRefusesGhostty(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, mode, want string
		apps             int
	}{
		{"no osascript", "", "restore tabs runs on a Mac: osascript is not installed", 0},
		{"denied", "denied", "macOS denies control of Ghostty" + automationAdvice, 0},
		{"off", "off", "Ghostty says: AppleScript is disabled by the macos-applescript " +
			"configuration; set macos-applescript = true in Ghostty's config", 0},
		{"1.3.1", "old", "restore tabs needs Ghostty 1.4 or newer, and Ghostty 1.3.1 runs", 0},
		{"changed", "changed", "Ghostty's windows changed twice while cld read them", 0},
		{"gone", "gone", "Ghostty quit while cld read it", 0},
		{"no ps row", "", "Ghostty quit while cld read it", 0},
		{"garbage", "garbage", "osascript: fake osascript: execution error", 0},
		{"timeout", "timeout", "Ghostty did not answer in 15 s; if macOS asked whether " +
			"the app cld runs in may control Ghostty, answer, then run cld restore tabs again", 0},
		{"several", "", "Ghostty runs 2 times, as open -n starts it: quit all but one", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyAt(ghosttyPID, ghosttyStart)
			m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "✳ cld-x", ssh("ws"))))}
			tick(t, s, m)
			before := readFile(t, recordPath(s))
			m.Apps = test.apps
			if test.name == "no ps row" {
				m.Processes = m.Processes[1:]
			}
			saveModel(t, s, m)
			env := map[string]string{"TZ": "UTC", "CLD_FAKE_OSASCRIPT_FAIL": test.mode}
			if test.name == "no osascript" {
				env["PATH"] = s.Tools("ps", "ghostty")
			}
			result := s.RunCld(env, "restore", "tabs", "--dry-run")
			checkFailed(t, result, 1, "cld: "+test.want+"\n")
			if readFile(t, recordPath(s)) != before {
				t.Error("tabs.json changed")
			}
		})
	}
}
