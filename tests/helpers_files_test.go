package tests

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Git repositories in the sandbox, the files of cld's own repository, and checks of what files
// hold.

// gitInit makes the sandbox's work directory a git repository, whose name, "_", leaves nothing
// of a session's name. git runs in the sandbox's environment, as cld does, so no GIT_DIR, say,
// sends it to another repository (see TestMain).
func gitInit(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	runGit(t, s, s.Root, "init", "-q", s.Work)
}

// runGit runs git with args in dir, in the sandbox's environment (see gitInit), as an author and
// committer of its own, where the sandbox's home has no git configuration.
func runGit(t *testing.T, s *sandbox.Sandbox, dir string, args ...string) {
	t.Helper()
	identity := []string{"-c", "user.name=cld", "-c", "user.email=cld@example.com"}
	cmd := exec.Command("git", append(identity, args...)...)
	cmd.Env = s.Environ(nil)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// gitWorktree makes the linked worktree .claude/worktrees/NAME of the work directory's
// repository, which gitInit made, on a commit of its own, as claude's --worktree NAME makes one,
// and returns its path.
func gitWorktree(t *testing.T, s *sandbox.Sandbox, name string) string {
	t.Helper()
	path := filepath.Join(s.Work, ".claude", "worktrees", name)
	for _, args := range [][]string{
		{
			"-c", "user.name=cld", "-c", "user.email=cld@example.com",
			"commit", "-q", "--allow-empty", "-m", name,
		},
		{"worktree", "add", "-q", "-b", "worktree-" + name, path},
	} {
		cmd := exec.Command("git", append([]string{"-C", s.Work}, args...)...)
		cmd.Env = s.Environ(nil)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return path
}

// repoFile is what the file name of cld's own repository holds.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// tree is what dir holds, but for .git: each directory, file and symbolic link by its path, with
// a file's content and a link's target.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		switch {
		case entry.Name() == ".git":
			return filepath.SkipDir
		case entry.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			entries[name] = "-> " + target
			return err
		case entry.IsDir():
			entries[name] = "/"
		default:
			data, err := os.ReadFile(path)
			entries[name] = string(data)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// checkContent reports a file at path that does not hold want.
func checkContent(t *testing.T, path, want string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Errorf("%s holds (%v)\n%s\nwant\n%s", path, err, data, want)
	}
}

// checkSettings reports a settings file at path that does not hold want.
func checkSettings(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s\n%s\nwant\n%s", path, got, want)
	}
}

// checkMode reports a file at path whose permissions are not want.
func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if info, err := os.Stat(path); err != nil {
		t.Error(err)
	} else if info.Mode().Perm() != want {
		t.Errorf("%s: mode %04o, want %04o", path, info.Mode().Perm(), want)
	}
}

// umask is the file mode creation mask of the tests, and of the cld they run, as Linux has it.
func umask(t *testing.T) os.FileMode {
	t.Helper()
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(status)) {
		if value, found := strings.CutPrefix(line, "Umask:"); found {
			mask, err := strconv.ParseUint(strings.TrimSpace(value), 8, 32)
			if err != nil {
				t.Fatalf("/proc/self/status: %q", line)
			}
			return os.FileMode(mask)
		}
	}
	t.Fatal("/proc/self/status gives no umask")
	return 0
}
