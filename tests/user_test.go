package tests

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup config user against the sandbox's HOME and CLAUDE_CONFIG_DIR (decision 53).

// preferenceLines are the preferences setup config user writes, as members of the settings.
const preferenceLines = `  "model": "opus",
  "effortLevel": "xhigh",
  "theme": "dark",
  "editorMode": "normal",
  "autoCompactEnabled": true,
  "autoUpdatesChannel": "latest"`

// createdSettings is what setup config user writes where there are no user settings, with
// --permissions set and --notifications channel, "" for none. The set's permission rules are as
// setup config project writes them, and none where they are empty.
func createdSettings(t *testing.T, set, channel string) string {
	t.Helper()
	settings := "{\n  \"$schema\": \"https://json.schemastore.org/claude-code-settings.json\",\n"
	if allow := allowed(t, set); len(allow) > 0 {
		settings += "  \"permissions\": {\n    \"allow\": [\n" + quoted("      ", allow) + "\n    ]"
		if deny := denied(t, set); len(deny) > 0 {
			settings += ",\n    \"deny\": [\n" + quoted("      ", deny) + "\n    ]"
		}
		settings += "\n  },\n"
	}
	settings += preferenceLines
	if channel != "" {
		settings += ",\n  \"preferredNotifChannel\": " + strconv.Quote(channel)
	}
	return settings + "\n}\n"
}

// userSettings is where claude reads its user settings in the sandbox, without CLAUDE_CONFIG_DIR.
func userSettings(s *sandbox.Sandbox) string {
	return filepath.Join(s.Home, ".claude", "settings.json")
}

// checkSetupUser reports a run of setup config user that did not succeed, printing want.
func checkSetupUser(t *testing.T, result sandbox.Result, want string) {
	t.Helper()
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
}

// userFreshCases are TestSetupConfigUser's options, the permission set expected and the channel.
var userFreshCases = []struct {
	args         []string
	set, channel string
}{
	{nil, "read-only", ""},
	{[]string{"--permissions", "cld", "--notifications", "kitty"}, "cld", "kitty"},
	{[]string{"--notifications=terminal_bell", "--permissions=none"}, "none", "terminal_bell"},
	{[]string{"--permissions", "read-only", "--notifications", "auto"}, "read-only", "auto"},
	{[]string{"--notifications", "notifications_disabled"}, "read-only", "notifications_disabled"},
	{[]string{"--permissions", "none", "--permissions", "cld"}, "cld", ""},
}

// Where there are no user settings, setup config user creates them in ~/.claude, as Claude Code
// creates the file and its directory: 0644 and 0755, less the umask. It needs neither tmux nor
// claude, and writes nothing in the project. Run again, it changes nothing.
func TestSetupConfigUser(t *testing.T) {
	t.Parallel()
	for _, test := range userFreshCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			none := map[string]string{"PATH": s.Tools()}
			args := append([]string{"setup", "config", "user"}, test.args...)
			path := userSettings(s)
			checkSetupUser(t, s.RunCld(none, args...), "Created "+path+"\n")
			checkSettings(t, path, createdSettings(t, test.set, test.channel))
			if runtime.GOOS == "linux" {
				mask := umask(t)
				checkMode(t, path, 0o644&^mask)
				checkMode(t, filepath.Dir(path), 0o755&^mask)
			}
			if entries, err := os.ReadDir(s.Work); err != nil || len(entries) != 0 {
				t.Errorf("the work directory holds %v (%v), want nothing", entries, err)
			}
			checkSetupUser(t, s.RunCld(none, args...), "Left "+path+" as it was\n")
			checkSettings(t, path, createdSettings(t, test.set, test.channel))
		})
	}
}

// The settings are $CLAUDE_CONFIG_DIR/settings.json where that is set, as claude reads them.
func TestSetupConfigUserInConfigDir(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, "config", "claude")
	path := filepath.Join(dir, "settings.json")
	result := s.RunCld(map[string]string{"CLAUDE_CONFIG_DIR": dir}, "setup", "config", "user")
	checkSetupUser(t, result, "Created "+path+"\n")
	checkSettings(t, path, createdSettings(t, "read-only", ""))
	if _, err := os.Lstat(filepath.Join(s.Home, ".claude")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("~/.claude: %v, want none", err)
	}
}

// User settings that exist: cld adds what they lack, in the file's order and indentation. It keeps
// the values they have, model's, theme's and the channel's included, and env whole.
func TestSetupConfigUserEditsSettings(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	path := userSettings(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(path, `{
    "env": {"OTEL_METRICS_EXPORTER": "otlp", "A": "<&>"},
    "permissions": {
        "allow": ["Bash(make:*)", "Read"],
        "defaultMode": "acceptEdits"
    },
    "model": "sonnet",
    "theme": "light",
    "preferredNotifChannel": "iterm2",
    "hooks": {"Stop": []}
}
`)
	args := []string{"setup", "config", "user", "--notifications", "kitty"}
	checkSetupUser(t, s.RunCld(nil, args...), "Updated "+path+": $schema, permissions.allow, "+
		"effortLevel, editorMode, autoCompactEnabled, autoUpdatesChannel\n")
	allow := []string{"Bash(make:*)", "Read"}
	allow = append(allow, readOnlyAllow[1:]...)
	want := `{
    "$schema": "https://json.schemastore.org/claude-code-settings.json",
    "env": {"OTEL_METRICS_EXPORTER": "otlp", "A": "<&>"},
    "permissions": {
        "allow": [
` + quoted("            ", allow) + `
        ],
        "defaultMode": "acceptEdits"
    },
    "model": "sonnet",
    "theme": "light",
    "preferredNotifChannel": "iterm2",
    "hooks": {"Stop": []},
    "effortLevel": "xhigh",
    "editorMode": "normal",
    "autoCompactEnabled": true,
    "autoUpdatesChannel": "latest"
}
`
	checkSettings(t, path, want)
	checkSetupUser(t, s.RunCld(nil, args...), "Left "+path+" as it was\n")
	checkSettings(t, path, want)
}

// A settings file that is a symbolic link stays one: the file it leads to changes.
func TestSetupConfigUserSymlink(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	target := filepath.Join(s.Root, "dotfiles", "claude-settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(target, "{}")
	path := userSettings(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	result := s.RunCld(nil, "setup", "config", "user", "--permissions", "none")
	checkSetupUser(t, result, "Updated "+path+": $schema, model, effortLevel, theme, editorMode, "+
		"autoCompactEnabled, autoUpdatesChannel\n")
	if link, err := os.Readlink(path); err != nil || link != target {
		t.Errorf("%s leads to %q (%v), want %s", path, link, err, target)
	}
	checkSettings(t, target, createdSettings(t, "none", ""))
}
