package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join -w hands the worktree to claude: --worktree cld-NAME-SUFFIX, and settings that branch it
// from HEAD (decision 4). claude starts where cld runs, and makes or reopens the worktree itself.
// The words after "--" come after --worktree's value. The repository is named work.
func TestJoinWorktree(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// session is claude's --name and --worktree
		session string
		// after is what claude gets after --worktree's value
		after []string
	}{
		{[]string{"join", "-w"}, "cld-work-0", nil},
		{[]string{"join", "-n", "feat", "--worktree"}, "cld-feat-0", nil},
		{[]string{"join", "-w", "--name=feat"}, "cld-feat-0", nil},
		{[]string{"join", "-wn", "feat"}, "cld-feat-0", nil},
		{[]string{"join", "-s", "feat", "-w"}, "cld-work-feat", nil},
		{[]string{"join", "-ws", "x", "-n", "feat"}, "cld-feat-x", nil},
		{[]string{"join", "-s", "feat", "-w", "--new"}, "cld-work-feat", nil},
		{[]string{"join", "-w", "--", "--effort", "high", "start"}, "cld-work-0",
			[]string{"--effort", "high", "start"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			sub := filepath.Join(repository(t, s, "work"), "sub")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			startCldIn(t, s, "tmux", sub, nil, test.args...)
			probe := s.WaitProbes(1)[0]
			fromHead := settings(s, sandbox.RealTmux, sandbox.RealGit, test.session, sub, true)
			want := append([]string{"--name", test.session, "--settings", fromHead,
				"--worktree", test.session}, test.after...)
			if !slices.Equal(probe.Argv, want) {
				t.Errorf("claude arguments %q, want %q", probe.Argv, want)
			}
			if probe.Cwd != sub {
				t.Errorf("claude starts in %s, want %s", probe.Cwd, sub)
			}
		})
	}
}

// Outside a git work tree join -w fails before starting anything: claude would say so in a
// session left to kill.
func TestJoinWorktreeRequiresRepository(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	want := "cld: --worktree needs a git repository, and " + s.Work + " is not in one\n"
	result := s.RunCld(nil, "join", "-s", "feat", "-w")
	if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}
