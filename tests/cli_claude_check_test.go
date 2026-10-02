package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Which commands run claude --version, and where.

// The messages for a claude older than 2.1.232, the fake one's version here, and a tmux older than
// 3.5a.
const (
	claudeTooOld = "cld: claude 2.1.232 or newer is required, found '2.1.231 (Claude Code)'\n"
	tmuxTooOld   = "cld: tmux 3.5a or newer is required, found 'tmux 3.4'\n"
)

// noDocker is setup telemetry's message without docker, or its refusal elsewhere than Linux.
var noDocker = func() string {
	if runtime.GOOS != "linux" {
		return "cld: setup telemetry works on Linux only\n"
	}
	return "cld: docker is not installed\n"
}()

// onlyJoinCase is a command line run with a claude too old for join, and how it ends.
type onlyJoinCase struct {
	args        []string
	tmuxVersion string
	sessions    string
	code        int
	stdout      string
	stderr      string
	// ran is whether claude --version runs.
	ran bool
}

// onlyJoinCases are the cases of TestOnlyJoinRunsClaude, made as it runs, since a session the
// fake tmux lists is active as of then.
func onlyJoinCases() []onlyJoinCase {
	return []onlyJoinCase{
		{[]string{"join"}, "tmux 3.7c", "", 1, "", claudeTooOld, true},
		{[]string{"join", "-s", "main"}, "tmux 3.7c", "", 1, "", claudeTooOld, true},
		{[]string{"join", "-s", "main"}, "tmux 3.7c", "cld-main", 1, "",
			"cld: join needs a terminal, and its input is not one\n", false},
		{[]string{"join", "-s", "main", "--resume", "SESSION"}, "tmux 3.7c", "cld-main", 1, "",
			"cld: session 'main' exists, and --resume would be lost: its claude has started; " +
				"attach to it with cld join -s main, or give another -s SUFFIX\n", false},
		{[]string{"join", "-w"}, "tmux 3.7c", "", 1, "", "cld: git is not installed\n", false},
		{[]string{"join"}, "tmux 3.4", "", 1, "", tmuxTooOld, false},
		{[]string{"join", "--resume", "SESSION"}, "tmux 3.7c", "", 1, "", claudeTooOld, true},
		{[]string{"join", "-s", "x"}, "tmux 3.4", "", 1, "", tmuxTooOld, false},
		{[]string{"kill", "-s", "main"}, "tmux 3.7c", "", 1, "",
			"cld: no session 'main' (see cld list)\n", false},
		{[]string{"detach", "-s", "main"}, "tmux 3.7c", "", 1, "",
			"cld: no session 'main' (see cld list)\n", false},
		{[]string{"list"}, "tmux 3.7c", "", 0, "", "", false},
		{[]string{"restore"}, "tmux 3.7c", "", 0, "", "", false},
		{[]string{"restore"}, "tmux 3.4", "", 1, "", tmuxTooOld, false},
		{[]string{"__complete", "join", "-s", ""}, "tmux 3.4", "", 0, ":4\n", noFileReport, false},
		{[]string{"__complete", "join", "--resume", "x", "-n", ""}, "tmux 3.4", "", 0, ":4\n",
			noFileReport, false},
		{[]string{"__completeNoDesc", "join", "-s", ""}, "tmux 3.4", fakeSession("main", 0), 0,
			"main\n:4\n", noFileReport, false},
		{[]string{"setup", "telemetry", "--remote", "https://otel.example.com:4317"}, "tmux 3.4", "",
			1, "", noDocker, false},
		{[]string{"__complete", "setup", "telemetry", "--local", ""}, "tmux 3.4", "", 0, ":4\n",
			noFileReport, false},
		{[]string{"setup", "project"}, "tmux 3.4", "", 0,
			"Created .claude/settings.json\nCreated .claude/settings.local.json\nCreated .gitignore\n",
			"", false},
		{[]string{"__complete", "setup", "project", "--mcp", "r"}, "tmux 3.4", "", 0,
			"rider\tRider's MCP server, port $RIDER_MCP_PORT or 64482\n:4\n", noFileReport, false},
	}
}

// Of these commands only join runs claude --version, where it creates or brings back the session,
// not where it attaches (decision 6). Here restore has no session to bring back. The check follows
// those of the tools and tmux, and the lookup (decision 50.3). A claude too old for join records
// that it ran. TestJoinInItsOwnPaneWithNoTerminal pins that it follows the check of cld's own pane.
func TestOnlyJoinRunsClaude(t *testing.T) {
	t.Parallel()
	for _, test := range onlyJoinCases() {
		name := strings.Join(test.args, " ") + ", " + test.tmuxVersion
		if test.sessions != "" {
			name += ", session main"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if test.sessions != "" {
				socket(t, s, "cld-main")
			}
			tools := s.Tools("tmux")
			ran := filepath.Join(s.Root, "claude ran")
			script := "#!/bin/sh\necho \"$*\" >'" + ran + "'\necho '2.1.231 (Claude Code)'\n"
			s.WriteProgram(filepath.Join(tools, "claude"), script, 0o755)
			result := s.RunCld(map[string]string{
				"PATH":                   tools,
				"CLD_FAKE_TMUX_VERSION":  test.tmuxVersion,
				"CLD_FAKE_TMUX_SESSIONS": test.sessions,
			}, test.args...)
			if result.Code != test.code || result.Stdout != test.stdout ||
				result.Stderr != test.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
					result.Code, result.Stdout, result.Stderr, test.code, test.stdout, test.stderr)
			}
			checkClaudeRan(t, ran, test.ran)
			if fakeTmuxRan(s) {
				t.Error("tmux ran a command other than -V and list-sessions")
			}
		})
	}
}

// checkClaudeRan reports a claude that ran other than as want says, with --version alone, as the
// fake claude writes down in the file ran.
func checkClaudeRan(t *testing.T, ran string, want bool) {
	t.Helper()
	args, err := os.ReadFile(ran)
	if want && string(args) != "--version\n" {
		t.Errorf("claude ran with %q, want --version", args)
	}
	if !want && err == nil {
		t.Errorf("claude ran with %q", args)
	}
}

// join runs claude --version in the directory claude starts in (decision 6), where a version
// manager's shim, mise's say, runs the claude that directory pins. The fake claude reports the
// version in .claude-version where the directory has one, and global otherwise, as a shim would.
func TestChecksClaudeWhereItStarts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		global, pinned string
		accepted       bool
	}{
		{"2.1.282 (Claude Code)", "2.1.100 (Claude Code)", false},
		{"2.1.100 (Claude Code)", "2.1.282 (Claude Code)", true},
	} {
		t.Run("pinned "+test.pinned, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			tools := s.Tools("tmux")
			script := "#!/bin/sh\nif [ -f .claude-version ]; then read -r v <.claude-version; " +
				"echo \"$v\"; else echo '" + test.global + "'; fi\n"
			s.WriteProgram(filepath.Join(tools, "claude"), script, 0o755)
			pin := []byte(test.pinned + "\n")
			if err := os.WriteFile(filepath.Join(s.Work, ".claude-version"), pin, 0o644); err != nil {
				t.Fatal(err)
			}
			env := map[string]string{"PATH": tools, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}
			result := s.RunCldOnTerminal(env, "join")
			if !test.accepted {
				want := "cld: claude 2.1.232 or newer is required, found '" + test.pinned + "'\n"
				checkFailed(t, result, 1, want)
				if fakeTmuxRan(s) {
					t.Error("tmux started")
				}
				return
			}
			checkTitled(t, result, "0")
			if argv := s.FakeTmuxRecord().Argv; !slices.Contains(argv, s.Work) {
				t.Errorf("tmux arguments %q name no %s", argv, s.Work)
			}
		})
	}
}
