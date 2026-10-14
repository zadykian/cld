package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain unsets the GIT_* variables git sets for hooks and rebase --exec, which would point
// the tests' git at the repository they run in. Every git, run's too, leaves out the user's and
// the system's settings, which may sign commits or set core.abbrev. It looks for no repository
// above the temporary directory, and writes its messages in English.
func TestMain(m *testing.M) {
	for _, variable := range os.Environ() {
		if name, _, _ := strings.Cut(variable, "="); strings.HasPrefix(name, "GIT_") {
			if err := os.Unsetenv(name); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		}
	}
	for name, value := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CEILING_DIRECTORIES": os.TempDir(), "LC_ALL": "C",
	} {
		if err := os.Setenv(name, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	os.Exit(m.Run())
}

// git runs git in dir, and returns its output.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=test",
		"-c", "user.email=test@example.com"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = exit.Stderr
		}
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return strings.TrimSpace(string(out))
}

// commitTo commits nothing in dir with the message msg, and returns its abbreviated name.
func commitTo(t *testing.T, dir, msg string) string {
	t.Helper()
	git(t, dir, "commit", "-q", "--allow-empty", "--allow-empty-message", "-m", msg)
	return git(t, dir, "log", "-1", "--format=%h")
}

// repository makes a git repository whose main, also its origin/main, holds a commit that breaks
// the style, and checks out the branch topic from it.
func repository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	commitTo(t, dir, "Start")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "main")
	git(t, dir, "checkout", "-q", "-b", "topic")
	return dir
}

func TestRun(t *testing.T) {
	t.Parallel()
	dir := repository(t)
	var stdout, stderr strings.Builder
	if status := run(dir, nil, &stdout, &stderr); status != 0 || stdout.Len() != 0 {
		t.Errorf("no commits: status %d, stdout %q, stderr %q; want 0 and nothing", status,
			stdout.String(), stderr.String())
	}

	commitTo(t, dir, "feat(cli): add a command\n\n- what changed")
	bad := commitTo(t, dir, "Fix it\n"+strings.Repeat("x", 73))
	empty := commitTo(t, dir, "")
	// A merge, whose message breaks the style, of a branch whose commit keeps it.
	git(t, dir, "checkout", "-q", "-b", "side", "main")
	commitTo(t, dir, "test: cover it")
	git(t, dir, "checkout", "-q", "topic")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge branch 'side'", "side")
	want := fmt.Sprintf("%[1]s:1: %[2]s\n%[1]s:2: no blank line between the header and the body\n"+
		"%[1]s:2: line of 73 columns, over the cap of 72\n%[3]s:1: %[4]s\n", bad,
		notForm("Fix it").msg, empty, notForm("").msg)
	for _, args := range [][]string{nil, {"main..topic"}} {
		stdout.Reset()
		stderr.Reset()
		status := run(dir, args, &stdout, &stderr)
		if status != 1 || stdout.String() != want {
			t.Errorf("args %q: status %d, stdout %q, stderr %q; want 1 and %q", args, status,
				stdout.String(), stderr.String(), want)
		}
	}
}

var runErrorTests = []struct {
	name         string
	noRepository bool
	args         []string
	status       int
	inStderr     string
}{
	{name: "help", args: []string{"-h"}, status: 0, inStderr: "usage: go run ./tools/commitcheck"},
	{
		name: "an unknown flag", args: []string{"-x"}, status: 2,
		inStderr: "flag provided but not defined: -x\nusage: go run ./tools/commitcheck [RANGE]",
	},
	{
		name: "two ranges", args: []string{"main..topic", "main"}, status: 2,
		inStderr: "usage: go run ./tools/commitcheck [RANGE]",
	},
	{
		name: "an unknown revision", args: []string{"main..nowhere"}, status: 2,
		inStderr: "commitcheck: git log: fatal: bad revision 'main..nowhere'",
	},
	{
		name: "a range git would take as an option", args: []string{"--", "--all"}, status: 2,
		inStderr: "commitcheck: git log: fatal: bad revision '--all'",
	},
	{
		name: "not a repository", noRepository: true, status: 2,
		inStderr: "commitcheck: git log: fatal: not a git repository",
	},
}

func TestRunErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range runErrorTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if !tt.noRepository {
				dir = repository(t)
			}
			var stdout, stderr strings.Builder
			status := run(dir, tt.args, &stdout, &stderr)
			if status != tt.status || !strings.Contains(stderr.String(), tt.inStderr) {
				t.Errorf("status %d, stderr %q; want %d and %q in it", status, stderr.String(),
					tt.status, tt.inStderr)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout %q, want nothing", stdout.String())
			}
		})
	}
}
