package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// claude's temp directories of its conversations, in the sandbox's temp root, and their removal
// as a session ends (decision 67.2).

// claudeTemp is claude's temp directory of the conversation id in project, in s's temp root,
// made with a file in it.
func claudeTemp(t *testing.T, s *sandbox.Sandbox, project, id string) string {
	t.Helper()
	dir := filepath.Join(s.ClaudeTemp(), "claude-"+strconv.Itoa(os.Getuid()), project, id)
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(filepath.Join(dir, "tasks", "a.output"), "output\n")
	return dir
}

// Each end of a session removes claude's temp directory of its conversation, in each project, and
// no other. The ends are cld's kill, claude's exit with status 0, and the sweep of idle sessions.
func TestEndRemovesClaudeTemp(t *testing.T) {
	t.Parallel()
	for name, end := range map[string]func(*testing.T, *sandbox.Sandbox, terminal.Terminal,
		*sandbox.Probe){
		"kill": func(t *testing.T, s *sandbox.Sandbox, _ terminal.Terminal, _ *sandbox.Probe) {
			t.Helper()
			checkExit(t, "kill", s.RunCld(nil, "kill", "-s", "x"), 0, "", "")
		},
		"exit": func(_ *testing.T, _ *sandbox.Sandbox, _ terminal.Terminal, probe *sandbox.Probe) {
			probe.Send("exit")
		},
		"idle": func(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal, _ *sandbox.Probe) {
			t.Helper()
			term.Keys("C-q", "d")
			sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool {
				return !term.Running()
			})
			// CLD_IDLE_DAYS 0.00001 is 0.864 s.
			time.Sleep(2 * time.Second)
			s.RunCld(map[string]string{"CLD_IDLE_DAYS": "0.00001"}, "list")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			term := startCld(t, s, "tmux", nil, "join", "-s", "x")
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			probe.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
			mine := []string{claudeTemp(t, s, "-a", firstID), claudeTemp(t, s, "-b", firstID)}
			other := claudeTemp(t, s, "-a", secondID)
			end(t, s, term, probe)
			sandbox.WaitFor(t, 10*time.Second, "session x to end", func() bool {
				return len(s.Sessions()) == 0
			})
			for _, dir := range mine {
				if exists(dir) {
					t.Errorf("%s: claude's temp directory %s is left", name, dir)
				}
			}
			if !exists(filepath.Join(other, "tasks", "a.output")) {
				t.Errorf("%s: another conversation's temp directory %s is gone", name, other)
			}
		})
	}
}

// kill ends a session whose server outlived it, and removes claude's temp directory of its
// conversation, which claude may have made since its pane went (decision 67.2). claude's pane goes
// by kill-pane, as the hook for exit status 0 ends it, so the end's own removal is the one to run.
func TestOutlivedKillRemovesClaudeTemp(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "x")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-x/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	// The directory is made once the pane's own removal has run.
	dir := claudeTemp(t, s, "-a", firstID)
	checkExit(t, "kill", s.RunCld(nil, "kill", "-s", "x"), 0, "", "")
	if exists(dir) {
		t.Errorf("claude's temp directory %s is left by the kill of the server that outlived x", dir)
	}
}

// The temp root is CLAUDE_CODE_TMPDIR, else TMPDIR (decision 67.1), where an empty variable counts
// as unset. A session that has no conversation yet, with no SessionStart, removes nothing at its
// end, whatever directories are there (decision 67.2).
func TestEndTempRoot(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		env   map[string]string
		start bool
		gone  bool
	}{
		{"empty CLAUDE_CODE_TMPDIR", map[string]string{"CLAUDE_CODE_TMPDIR": ""}, true, true},
		{"no conversation", nil, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			startCld(t, s, "tmux", test.env, "join", "-s", "x")
			probe := s.WaitProbes(1)[0]
			waitClients(t, s, 1)
			if test.start {
				probe.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
			}
			dirs := []string{claudeTemp(t, s, "-a", firstID), claudeTemp(t, s, "-a", secondID)}
			checkExit(t, "kill", s.RunCld(test.env, "kill", "-s", "x"), 0, "", "")
			sandbox.WaitFor(t, 10*time.Second, "session x to end", func() bool {
				return len(s.Sessions()) == 0
			})
			if exists(dirs[0]) == test.gone {
				t.Errorf("claude's temp directory %s removed: %v, want %v", dirs[0], !exists(dirs[0]),
					test.gone)
			}
			if !exists(dirs[1]) {
				t.Errorf("another conversation's temp directory %s is gone", dirs[1])
			}
		})
	}
}
