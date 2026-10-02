package tests

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The hooks join gives claude keep its status in @cld-status on its session, for the tab's title
// (decision 25). That is busy from a prompt on, waiting while claude asks, and idle once the turn
// is done, failed or interrupted, or claude says so. They run with the TMUX of claude's pane, which
// they do not need (see TestHooksOutsideThePane).
func TestStatusHooks(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "x", "--resume", "cld-x")
	probe := s.WaitProbes(1)[0]
	status := func() string { return s.Format("cld-x", "#{@cld-status}") }
	if got := status(); got != "" {
		t.Fatalf("@cld-status is %q before any hook, want none", got)
	}
	for _, step := range []struct{ event, input, want string }{
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"PostToolUse", `{"tool_name":"Bash"}`, "busy"},
		{"PermissionRequest", `{"tool_name":"Bash"}`, "waiting"},
		{"PostToolUse", `{"tool_name":"Bash"}`, "busy"},
		{"Elicitation", `{"mcp_server_name":"m"}`, "waiting"},
		{"ElicitationResult", `{"mcp_server_name":"m"}`, "busy"},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt":false}`, "busy"},
		{"Notification", `{"notification_type":"permission_prompt"}`, "busy"},
		{"Stop", `{}`, "idle"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"PostToolUseFailure", `{"tool_name":"Bash","is_interrupt":true}`, "idle"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"StopFailure", `{"error":"rate_limit"}`, "idle"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "busy"},
		{"Notification", `{"notification_type":"idle_prompt"}`, "idle"},
		{"SessionStart", `{"source":"clear"}`, "idle"},
	} {
		probe.Hook(step.event, step.input)
		if got := status(); got != step.want {
			t.Errorf("@cld-status is %q after %s %s, want %q",
				got, step.event, step.input, step.want)
		}
	}
	if global := s.MustTmux("cld-x", "show", "-gqv", "@cld-status"); global != "" {
		t.Errorf("global @cld-status %q, want none: it goes to claude's session", global)
	}
}

// The same hooks keep @cld-worktree on claude's session for the title (decision 26): 1 while
// claude's directory is in a linked git worktree, and 0 elsewhere. They set it as claude starts,
// and each time its directory changes. claude runs the hooks in its directory.
func TestWorktreeHooks(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		// start is where cld runs, in the work directory's repository, which has the linked
		// worktree .claude/worktrees/x
		start string
		want  string
	}{
		{"in the main worktree", ".", "0"},
		{"in a linked worktree", ".claude/worktrees/x", "1"},
		{"in a linked worktree's subdirectory", ".claude/worktrees/x/sub", "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			worktree := gitWorktree(t, s, "x")
			if err := os.Mkdir(filepath.Join(worktree, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			startCldIn(t, s, "tmux", filepath.Join(s.Work, test.start), nil,
				"join", "-s", "x")
			probe := s.WaitProbes(1)[0]
			status := func() string { return s.Format("cld-x", "#{@cld-worktree}") }
			probe.Hook("SessionStart", `{"source":"startup"}`)
			if got := status(); got != test.want {
				t.Errorf("@cld-worktree is %q as claude starts, want %q", got, test.want)
			}
			for _, step := range []struct{ dir, want string }{
				{worktree, "1"},
				{s.Work, "0"},
				{filepath.Join(worktree, "sub"), "1"},
				{s.Root, "0"},
				{filepath.Join(s.Work, ".git"), "0"},
				{worktree, "1"},
			} {
				probe.Send("cd " + step.dir)
				probe.Hook("CwdChanged", `{"new_cwd":"`+step.dir+`"}`)
				if got := status(); got != step.want {
					t.Errorf("@cld-worktree is %q once claude is in %s, want %q",
						got, step.dir, step.want)
				}
			}
		})
	}
}

// Where cld finds no git, claude gets no hooks that run it, and the title never says [w]. The
// hooks that keep the session's entry in cld's record run no git, and agent view is off all the
// same.
func TestWorktreeHooksWithoutGit(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCldOnTerminal(map[string]string{
		"PATH": s.Tools("tmux", "claude"), "CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
	}, "join", "-s", "x")
	if result.Code != 0 {
		t.Fatalf("exit %d, stderr %q, want exit 0", result.Code, result.Stderr)
	}
	argv := s.FakeTmuxRecord().Argv
	i := slices.Index(argv, "--settings")
	if i < 0 {
		t.Fatalf("no --settings in tmux's arguments %q", argv)
	}
	var given struct {
		DisableAgentView bool           `json:"disableAgentView"`
		Hooks            map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(argv[i+1]), &given); err != nil {
		t.Fatal(err)
	}
	if !given.DisableAgentView {
		t.Errorf("settings without agent view off: %s", argv[i+1])
	}
	events := slices.Sorted(maps.Keys(given.Hooks))
	want := []string{"Elicitation", "ElicitationResult", "Notification", "PermissionRequest",
		"PostToolUse", "PostToolUseFailure", "SessionEnd", "SessionStart", "Stop", "StopFailure",
		"UserPromptSubmit"}
	if !slices.Equal(events, want) {
		t.Errorf("hooks for %q, want %q", events, want)
	}
	start, err := json.Marshal(given.Hooks["SessionStart"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(start), "rev-parse") {
		t.Errorf("SessionStart hooks run git: %s", start)
	}
}

// claude may run the hooks in a background worker of its daemon, without TMUX and TMUX_PANE
// (claude 2.1.284). The hooks name the server and the session themselves, so they set claude's
// session all the same (decision 25). They set no session claude made, nor one of the default
// server, where a bare tmux goes without TMUX.
func TestHooksOutsideThePane(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "x")
	probe := s.WaitProbes(1)[0]
	s.MustTmux("cld-x", "new-session", "-d", "-s", "made")
	s.MustTmux("default", "new-session", "-d", "-s", "other")
	probe.Send("unsetenv TMUX TMUX_PANE")
	for _, step := range []struct{ event, input, option, want string }{
		{"SessionStart", `{"source":"resume"}`, "@cld-worktree", "0"},
		{"UserPromptSubmit", `{"prompt":"go"}`, "@cld-status", "busy"},
		{"PermissionRequest", `{"tool_name":"Bash"}`, "@cld-status", "waiting"},
		{"PostToolUse", `{"tool_name":"Bash"}`, "@cld-status", "busy"},
		{"Stop", `{}`, "@cld-status", "idle"},
	} {
		probe.Hook(step.event, step.input)
		if got := s.Format("cld-x", "#{"+step.option+"}"); got != step.want {
			t.Errorf("%s is %q after %s %s, want %q",
				step.option, got, step.event, step.input, step.want)
		}
	}
	for _, other := range []struct{ server, session string }{
		{"cld-x", "made"}, {"default", "other"},
	} {
		got := s.MustTmux(other.server, "list-panes", "-s", "-t", "="+other.session,
			"-F", "#{@cld-status}#{@cld-worktree}")
		if got != "" {
			t.Errorf("session %s on server %s has %q of the hooks' options, want none",
				other.session, other.server, got)
		}
	}
}

// The hooks name the server's socket by an absolute path where TMUX_TMPDIR is relative too. tmux
// takes that from the directory cld runs in, and the hooks run in claude's, which claude changes.
func TestHooksUnderRelativeTmuxTmpdir(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCldOnTerminal(map[string]string{
		"PATH": filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) +
			s.Env["PATH"],
		"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
		"TMUX_TMPDIR":           "..",
	}, "join", "-s", "x")
	if result.Code != 0 {
		t.Fatalf("exit %d, stderr %q, want exit 0", result.Code, result.Stderr)
	}
	argv := s.FakeTmuxRecord().Argv
	i := slices.Index(argv, "--settings")
	if i < 0 {
		t.Fatalf("no --settings in tmux's arguments %q", argv)
	}
	// The work directory's parent is the sandbox's TMUX_TMPDIR.
	want := settings(s, sandbox.FakeTmux, sandbox.RealGit, "cld-x", s.Work, false)
	if argv[i+1] != want {
		t.Errorf("settings\n%s\nwant\n%s", argv[i+1], want)
	}
}
