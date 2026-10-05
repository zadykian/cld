package tests

import (
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What setup config user refuses: user settings it cannot edit or write, and usage mistakes.

// userRefusals are user settings setup config user cannot edit. Each has a name, what the file
// holds, or "->" and where it leads as a symbolic link, the options, and what cld says. That
// follows the file's path, or takes its place at PATH.
var userRefusals = []struct {
	name, content string
	args          []string
	want          string
}{
	{"not valid", "{", nil, " is not valid JSON: line 1: unexpected end of JSON input"},
	{"empty", "", nil, " is not valid JSON: it is empty"},
	{"no object", "[]", nil, " holds no JSON object"},
	{"permissions no object", `{"permissions": ["Read"]}`, nil,
		"permissions in PATH is not a JSON object"},
	{"allow no array", `{"permissions": {"allow": "Read"}}`, nil,
		"permissions.allow in PATH is not a JSON array"},
	{"deny no array", `{"permissions": {"deny": {}}}`, []string{"--permissions", "cld"},
		"permissions.deny in PATH is not a JSON array"},
	{"a link to no file", "->../dotfiles/settings.json", nil,
		" is a symbolic link to a file that does not exist"},
}

// Settings cld cannot edit end it with status 1, before it writes anything (decision 53.6).
func TestSetupConfigUserRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range userRefusals {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			path := userSettings(s)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if link, found := strings.CutPrefix(test.content, "->"); found {
				if err := os.Symlink(link, path); err != nil {
					t.Fatal(err)
				}
			} else {
				s.WriteFile(path, test.content)
			}
			before := tree(t, s.Home)
			args := append([]string{"setup", "config", "user"}, test.args...)
			want := test.want
			if strings.Contains(want, "PATH") {
				want = strings.Replace(want, "PATH", path, 1)
			} else {
				want = path + want
			}
			checkFailed(t, s.RunCld(nil, args...), 1, "cld: "+want+"\n")
			if after := tree(t, s.Home); !maps.Equal(before, after) {
				t.Errorf("the files changed: %q, were %q", after, before)
			}
		})
	}
}

// A write that fails ends cld with status 1, and changes nothing (decision 53.6). Here the
// settings are a symbolic link to a path of 4095 bytes, so the temporary file beside it has a
// longer one.
func TestSetupConfigUserCannotWrite(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the longest path is Linux's")
	}
	s := sandbox.New(t)
	target := longPath(t, s, "settings.json")
	s.WriteFile(target, "{}\n")
	path := userSettings(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	before := tree(t, s.Home)
	checkFailed(t, s.RunCld(nil, "setup", "config", "user"), 1,
		"cld: cannot write "+path+": file name too long\n")
	if after := tree(t, s.Home); !maps.Equal(before, after) {
		t.Errorf("the files changed: %q, were %q", after, before)
	}
	checkSettings(t, target, "{}\n")
}

const (
	// channelsUsage ends the error of a value --notifications does not take.
	channelsUsage = ": auto, iterm2, terminal_bell, iterm2_with_bell, kitty, ghostty or " +
		"notifications_disabled (see cld help)\n"
	// setupUserUnexpected starts the error of an argument setup config user does not take.
	setupUserUnexpected = "cld: setup config user: unexpected argument "
)

// userUsages are setup config user's options and the usage error cld gives for them.
var userUsages = []struct {
	args []string
	want string
}{
	{[]string{"--notifications", "bell"},
		"cld: invalid channel 'bell' for --notifications" + channelsUsage},
	{[]string{"--notifications", "Kitty"},
		"cld: invalid channel 'Kitty' for --notifications" + channelsUsage},
	{[]string{"--notifications", ""}, "cld: invalid channel '' for --notifications" + channelsUsage},
	{[]string{"--notifications"}, "cld: option '--notifications' needs a value (see cld help)\n"},
	{[]string{"--permissions", "all"},
		"cld: invalid permissions 'all' for --permissions" + permissionsUsage},
	{[]string{"--notifications", "bell", "--permissions", "all"},
		"cld: invalid permissions 'all' for --permissions" + permissionsUsage},
	{[]string{"--mcp", "goland"}, setupUserUnexpected + "'--mcp' (see cld help)\n"},
	{[]string{"x"}, setupUserUnexpected + "'x' (see cld help)\n"},
	{[]string{"--permissions", "cld", "none"}, setupUserUnexpected + "'none' (see cld help)\n"},
	{[]string{"x", "--notifications", "kitty"}, setupUserUnexpected + "'x' (see cld help)\n"},
	{[]string{"--"}, setupUserUnexpected + "'--' (see cld help)\n"},
	{[]string{"-n", "x"}, setupUserUnexpected + "'-n' (see cld help)\n"},
}

// setup config user's mistakes are usage errors, and write nothing. A set --permissions does not
// take comes first, then a channel --notifications does not take, an argument, or an unknown
// option.
func TestSetupConfigUserRejectsArguments(t *testing.T) {
	t.Parallel()
	for _, test := range userUsages {
		args := append([]string{"setup", "config", "user"}, test.args...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(nil, args...), 2, test.want)
			if _, err := os.Lstat(filepath.Join(s.Home, ".claude")); err == nil {
				t.Error("cld wrote ~/.claude")
			}
		})
	}
}
