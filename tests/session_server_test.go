package tests

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Each session runs on a server of its own, named like it, and each claude gets the environment of
// the shell whose cld join made it (decision 13). One server shared by every session gave each
// pane the first session's environment: a resuming claude looked in its CLAUDE_CONFIG_DIR.
func TestServerPerSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	shells := map[string]map[string]string{
		"a": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-a")},
		"b": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-b"), "VIRTUAL_ENV": "/venv/b"},
		"c": {"CLAUDE_CONFIG_DIR": filepath.Join(s.Home, ".claude-c")},
	}
	startCld(t, s, "tmux", shells["a"], "join", "-s", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", shells["b"], "join", "-s", "b")
	s.WaitProbes(2)
	startCld(t, s, "tmux", shells["c"], "join", "-s", "c", "--resume", "cld-c")
	for _, probe := range s.WaitProbes(3) {
		shell := shells[strings.TrimPrefix(probe.Argv[1], "cld-")]
		for _, name := range []string{"CLAUDE_CONFIG_DIR", "VIRTUAL_ENV"} {
			if value, want := probe.Env[name], shell[name]; value != want {
				t.Errorf("claude %s sees %s=%q, want %q", probe.Argv[1], name, value, want)
			}
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c"}) {
		t.Errorf("sessions %q, want [cld-a cld-b cld-c], each on its own server", sessions)
	}
}

// cld's servers read none of the user's tmux configuration, whose status-left the sandbox's
// ~/.tmux.conf poisons, and cld starts no default tmux server.
func TestIgnoresUserTmuxConfig(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join")
	s.WaitProbes(1)
	if left := s.MustTmux("cld-0", "show", "-gv", "status-left"); left == "POISONED" {
		t.Error("~/.tmux.conf was loaded")
	}
	socket := filepath.Join(s.Root, fmt.Sprintf("tmux-%d", os.Getuid()), "default")
	if _, err := os.Stat(socket); err == nil {
		t.Error("cld started the default tmux server")
	}
}

// claude trusts the variables that name a terminal to it over TERM_PROGRAM=tmux, and a server
// keeps the environment of the client that started it. So cld runs tmux without them, and claude
// never sees them, nor what it starts through tmux (decision 33). VS Code's askpass and editor go
// with VSCODE_GIT_ASKPASS_MAIN (see TestVSCodeGit).
func TestClaudeNeverSeesTheTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	const dist = "/home/u/.cursor-server/bin/1/extensions/git/dist/"
	given := map[string]string{
		"TERMINAL_EMULATOR":             "JetBrains-JediTerm",
		"__CFBundleIdentifier":          "com.jetbrains.goland",
		"CURSOR_TRACE_ID":               "0123456789abcdef",
		"VisualStudioVersion":           "17.0",
		"VSCODE_GIT_ASKPASS_MAIN":       dist + "askpass-main.js",
		"VSCODE_GIT_ASKPASS_NODE":       "/home/u/.cursor-server/bin/1/node",
		"VSCODE_GIT_ASKPASS_EXTRA_ARGS": "",
		"VSCODE_GIT_IPC_HANDLE":         "/run/user/1000/vscode-git-1.sock",
		"GIT_ASKPASS":                   dist + "askpass.sh",
		"VSCODE_GIT_EDITOR_MAIN":        dist + "git-editor-main.js",
		"VSCODE_GIT_EDITOR_NODE":        "/home/u/.cursor-server/bin/1/node",
		"VSCODE_GIT_EDITOR_EXTRA_ARGS":  "",
		"GIT_EDITOR":                    `"` + dist + `git-editor.sh"`,
	}
	startCld(t, s, "tmux", given, "join", "-s", "ide")
	probe := s.WaitProbes(1)[0]
	global := strings.Split(s.MustTmux("cld-ide", "show-environment", "-g"), "\n")
	for _, name := range slices.Sorted(maps.Keys(given)) {
		if value, found := probe.Env[name]; found {
			t.Errorf("claude sees %s=%s", name, value)
		}
		if slices.ContainsFunc(global, func(variable string) bool {
			return strings.HasPrefix(variable, name+"=")
		}) {
			t.Errorf("the server's environment has %s:\n%s", name, strings.Join(global, "\n"))
		}
	}
}
