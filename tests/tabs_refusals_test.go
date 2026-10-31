package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What restore tabs refuses: a Mac it cannot read Ghostty on, a record it cannot read, and
// options that exclude each other.

// automationAdvice is what cld says to do where macOS denies it Automation of Ghostty.
const automationAdvice = ": in System Settings › Privacy & Security › Automation, allow " +
	"Ghostty under the app cld runs in, Ghostty itself in its panes; as a last resort in " +
	"Ghostty's panes, tccutil reset AppleEvents com.mitchellh.ghostty resets every Automation " +
	"grant Ghostty holds"

// Where cld cannot read Ghostty, the dry-run fails with status 1 and cld's message, and leaves the
// record's bytes. So do osascript missing, macOS's Automation denied, AppleScript off in Ghostty's
// config and Ghostty 1.3.1. So do a read that changed twice, and a Ghostty that quit as cld read
// it or ps. So do osascript's own failure, no answer in 15 s, and Ghostty run twice.
func TestTabsRefusesGhostty(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, mode string
		apps       int
		want       string
	}{
		{"no osascript", "", 0, "restore tabs runs on a Mac: osascript is not installed"},
		{"denied", "denied", 0, "macOS denies control of Ghostty" + automationAdvice},
		{"off", "off", 0, "Ghostty says: AppleScript is disabled by the macos-applescript " +
			"configuration; set macos-applescript = true in Ghostty's config"},
		{"1.3.1", "old", 0, "restore tabs needs Ghostty 1.4 or newer, and Ghostty 1.3.1 runs"},
		{"changed", "changed", 0, "Ghostty's windows changed twice while cld read them"},
		{"gone", "gone", 0, "Ghostty quit while cld read it"},
		{"no ps row", "", 0, "Ghostty quit while cld read it"},
		{"garbage", "garbage", 0, "osascript: fake osascript: execution error"},
		{"timeout", "timeout", 0, "Ghostty did not answer in 15 s; if macOS asked whether " +
			"the app cld runs in may control Ghostty, answer, then run cld restore tabs again"},
		{"several", "", 2, "Ghostty runs 2 times, as open -n starts it: quit all but one"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
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

// A record of a newer format is refused and left for the cld that wrote it. One that does not
// parse, or of format 0, is moved aside with a warning, and a new one starts.
func TestTabsRefusesRecord(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, record, want string
		aside              bool
	}{
		{"newer", `{"format": 2, "panes": []}`,
			"cld: tabs.json has format 2, which this cld cannot read: run cld update\n", false},
		{"corrupt", `{"format": 1, "panes": [`, "", true},
		{"format 0", `{"format": 0, "panes": {}}`, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			m := ghosttyApp(t, s)
			m.Windows = []mac.Window{windowOf(tabOf(addPane(m, "~")))}
			saveModel(t, s, m)
			if err := os.MkdirAll(tabsDir(s), 0o700); err != nil {
				t.Fatal(err)
			}
			s.WriteFile(recordPath(s), test.record)
			result := dryRun(t, s)
			if !test.aside {
				checkFailed(t, result, 1, test.want)
				checkContent(t, recordPath(s), test.record)
				return
			}
			checkMovedAside(t, s, result, test.record)
		})
	}
}

// movedAside matches the warning of a record moved aside, and names the file it went to.
var movedAside = regexp.MustCompile(
	`^cld: warning: moved aside a record cld cannot read: (.*/tabs\.json\.bad-\d{8}T\d{6}Z)\n$`)

// checkMovedAside reports result unless it warned, with status 1, of the record moved aside to
// a file that holds record. A new record must have taken its place.
func checkMovedAside(t *testing.T, s *sandbox.Sandbox, result sandbox.Result, record string) {
	t.Helper()
	match := movedAside.FindStringSubmatch(result.Stderr)
	if result.Code != 1 || match == nil || filepath.Dir(match[1]) != tabsDir(s) {
		t.Fatalf("exit %d, stderr %q, want status 1 and the record moved aside in %s",
			result.Code, result.Stderr, tabsDir(s))
	}
	checkContent(t, match[1], record)
	if r := readRecord(t, s); r.Format != 1 || len(r.Panes) != 1 {
		t.Errorf("the new record is %+v, want one of format 1 with a pane", r)
	}
}

// --watch, which only starts the watcher, excludes the restore's options, and --older excludes
// --all. --watching, which --watch starts, takes no other option. Run by hand, --watching refuses
// to run.
func TestTabsRefusesOptions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"--watch", "--dry-run"}, 2, "cld: restore tabs: --watch and --dry-run exclude " +
			"each other: --watch only starts the record's watcher (see cld help)\n"},
		{[]string{"--older", "--watch"}, 2, "cld: restore tabs: --watch and --older exclude " +
			"each other: --watch only starts the record's watcher (see cld help)\n"},
		{[]string{"--watch", "--all"}, 2, "cld: restore tabs: --watch and --all exclude " +
			"each other: --watch only starts the record's watcher (see cld help)\n"},
		{[]string{"--watch", "--closed"}, 2, "cld: restore tabs: --watch and --closed exclude " +
			"each other: --watch only starts the record's watcher (see cld help)\n"},
		{[]string{"--all", "--older"}, 2, "cld: restore tabs: --older and --all exclude each " +
			"other: --all reopens every burst (see cld help)\n"},
		{[]string{"--watching", "--older"}, 2,
			"cld: restore tabs: --watching takes no other option (see cld help)\n"},
		{[]string{"--watching", "--watch"}, 2,
			"cld: restore tabs: --watching takes no other option (see cld help)\n"},
		{[]string{"--watching", "--dry-run"}, 2,
			"cld: restore tabs: --watching takes no other option (see cld help)\n"},
		{[]string{"--watching"}, 1, "cld: restore tabs --watching runs only as --watch starts it\n"},
	} {
		t.Run(test.args[0]+" "+test.args[len(test.args)-1], func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			saveModel(t, s, ghosttyApp(t, s))
			result := s.RunCld(map[string]string{"TZ": "UTC", "TERM_PROGRAM": "ghostty"},
				append([]string{"restore", "tabs"}, test.args...)...)
			checkFailed(t, result, test.code, test.want)
			if calls := s.OsascriptCalls(); len(calls) != 0 {
				t.Errorf("osascript ran: %+v", calls)
			}
		})
	}
}

// --watching run by hand, with fd 3 open on a file other than watch, refuses to run, and leaves
// that file alone. Only the watch file that --watch locks makes a watcher.
func TestTabsRefusesWatching(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	saveModel(t, s, ghosttyApp(t, s))
	if err := os.MkdirAll(tabsDir(s), 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(watchPath(s), "")
	other := filepath.Join(s.Root, "other")
	s.WriteFile(other, "kept\n")
	file, err := os.OpenFile(other, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close() //nolint:errcheck // cld wrote nothing through it
	argv := s.CldArgv("restore", "tabs", "--watching")
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env, cmd.Dir, cmd.ExtraFiles = s.Environ(watchEnv), s.Work, []*os.File{file}
	checkFailed(t, runCommand(t, cmd), 1,
		"cld: restore tabs --watching runs only as --watch starts it\n")
	checkContent(t, other, "kept\n")
	checkContent(t, watchPath(s), "")
}
