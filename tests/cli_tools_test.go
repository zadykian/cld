package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The tools cld looks for on the PATH: tmux, claude and git.

// join needs tmux, claude where it starts claude, and git with -w; the other commands need only
// tmux. The fake tmux finds no session, so detach and kill get as far as saying so, and join as
// far as the claude it would start.
func TestRequiresTools(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		present []string
		want    string
	}{
		{[]string{"join"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"join"}, []string{"tmux"}, "cld: claude is not installed\n"},
		{[]string{"join", "-w"}, []string{"tmux", "claude"}, "cld: git is not installed\n"},
		{[]string{"join", "-s", "x", "-w"}, []string{"tmux"}, "cld: git is not installed\n"},
		{[]string{"join", "--resume", "SESSION"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, []string{"tmux"},
			"cld: claude is not installed\n"},
		{[]string{"join", "-s", "main"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"join", "-s", "main"}, []string{"tmux"}, "cld: claude is not installed\n"},
		{[]string{"kill", "-s", "x"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"kill", "-s", "x"}, []string{"tmux"}, "cld: no session 'x' (see cld list)\n"},
		{[]string{"detach", "-s", "x"}, []string{"claude"}, "cld: tmux is not installed\n"},
		{[]string{"detach", "-s", "x"}, []string{"tmux"}, "cld: no session 'x' (see cld list)\n"},
		{[]string{"list"}, []string{"claude"}, "cld: tmux is not installed\n"},
	} {
		t.Run(strings.Join(test.args, " ")+" "+strings.Join(test.present, ","), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools(test.present...)}, test.args...)
			if result.Code != 1 || result.Stderr != test.want {
				t.Errorf("exit %d, stderr %q, want exit 1, stderr %q",
					result.Code, result.Stderr, test.want)
			}
		})
	}
}

// cld runs no program from a relative PATH entry, "." or an empty one (decision 11.5). The work
// directory has a tmux, a claude and a git that fail, saying so, if run. join hands tmux the
// absolute entry's claude by its path (see TestStartsTheClaudeItChecks).
func TestIgnoresRelativePathEntries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		present []string
		code    int
		want    string
	}{
		{[]string{"join", "-s", "main"}, nil, 1, "cld: tmux is not installed\n"},
		{[]string{"join", "-s", "main"}, []string{"tmux"}, 1, "cld: claude is not installed\n"},
		{[]string{"join"}, []string{"tmux"}, 1, "cld: claude is not installed\n"},
		{[]string{"join", "-w"}, []string{"tmux", "claude"}, 1, "cld: git is not installed\n"},
		// tmux, claude --version and git, run from the absolute entry, find no session, a version
		// that passes and the repository.
		{[]string{"join", "-w"}, []string{"tmux", "claude", "git"}, 0, ""},
	} {
		for _, relative := range []string{".", ""} {
			name := strings.Join(test.args, " ") + " " + strings.Join(test.present, ",") +
				" after '" + relative + "'"
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				gitInit(t, s)
				writeRelativeTools(t, s.Work)
				tools := s.Tools(test.present...)
				path := relative + string(os.PathListSeparator) + tools
				result := s.RunCldOnTerminal(map[string]string{"PATH": path}, test.args...)
				if result.Code != test.code || result.Stderr != test.want {
					t.Errorf("PATH %s: exit %d, stderr %q, want exit %d, stderr %q",
						path, result.Code, result.Stderr, test.code, test.want)
				}
				if test.code != 0 {
					return
				}
				argv, claude := s.FakeTmuxRecord().Argv, filepath.Join(tools, "claude")
				if !slices.Contains(argv, claude) || slices.Contains(argv, "claude") {
					t.Errorf("tmux arguments %q name claude otherwise than as %s", argv, claude)
				}
			})
		}
	}
}

// writeRelativeTools writes a tmux, a claude and a git in dir, which fail, saying so, if run.
func writeRelativeTools(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"tmux", "claude", "git"} {
		script := "#!/bin/sh\necho \"relative " + name + " ran\" >&2\nexit 99\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// A claude or git without the execute permission, where the PATH has no executable one, is found
// as bash found it (decision 11.5). So join cannot run that claude for its version, ending with
// 126, and that git cannot say the directory is in a repository. An executable one later on the
// PATH still counts.
func TestToolsWithoutExecutePermission(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// denied are the tools without the execute permission, in a PATH entry before present.
		denied, present []string
		code            int
		stderr          string
	}{
		{[]string{"join", "-s", "x"}, []string{"claude"}, []string{"tmux"}, 126,
			"cld: cannot run DENIED/claude: permission denied\n"},
		{[]string{"join", "-s", "x", "-w"}, []string{"git"}, []string{"tmux", "claude"}, 1,
			"cld: --worktree needs a git repository, and WORK is not in one\n"},
		{[]string{"join", "-s", "x", "-w"}, []string{"git"}, []string{"tmux", "claude", "git"}, 0, ""},
		{[]string{"join", "-s", "x"}, []string{"tmux", "claude"}, []string{"tmux", "claude"}, 0, ""},
	} {
		name := strings.Join(test.args, " ") + ", " + strings.Join(test.denied, ",") + " denied, " +
			strings.Join(test.present, ",") + " present"
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			denied := deniedTools(t, s, test.denied)
			result := s.RunCldOnTerminal(map[string]string{
				"PATH":                  denied + string(os.PathListSeparator) + s.Tools(test.present...),
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			}, test.args...)
			stdout := ""
			stderr := strings.NewReplacer("WORK", s.Work, "DENIED", denied).Replace(test.stderr)
			if test.code == 0 {
				stdout = cldTitle("x")
			}
			if result.Code != test.code || result.Stdout != stdout || result.Stderr != stderr {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
					result.Code, result.Stdout, result.Stderr, test.code, stdout, stderr)
			}
			if handedOver := fakeTmuxRan(s); handedOver != (test.code == 0) {
				t.Errorf("cld handed over to tmux: %v, want %v", handedOver, test.code == 0)
			}
		})
	}
}

// deniedTools writes the tools names, which fail, saying so, if run, without the execute
// permission in a directory of their own, and returns that directory.
func deniedTools(t *testing.T, s *sandbox.Sandbox, names []string) string {
	t.Helper()
	denied := filepath.Join(s.Root, "denied")
	if err := os.Mkdir(denied, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		script := "#!/bin/sh\necho \"" + name + " without the execute permission ran\" >&2\nexit 99\n"
		if err := os.WriteFile(filepath.Join(denied, name), []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return denied
}
