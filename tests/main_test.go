// Package tests holds cld's tests. They build cld and run it against real tmux servers, one
// private world per test (see internal/sandbox). A probe stands in for claude, systemctl and
// loginctl (see probe).
//
// CLD_TERMINALS lists the terminals the terminal contract runs against (default "tmux"):
// tmux, jediterm (needs a JDK and CLD_JEDITERM_LIB, see jediterm/fetch-deps).
//
// The tests run the same from git hooks and git rebase --exec: TestMain unsets the GIT_*
// variables git sets there, before it runs anything.
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

func TestMain(m *testing.M) {
	// git runs hooks and the commands of rebase --exec with GIT_* variables set, in a linked
	// worktree GIT_DIR among them. git init DIR, as the tests run it, would then initialise that
	// repository again, not DIR, and take it for a bare one (see
	// docs/design/findings/environment.md). Nothing the tests run inherits any of them.
	err := unsetGit()
	dir := ""
	if err == nil {
		dir, err = os.MkdirTemp("", "cld-tests.")
	}
	if err == nil {
		err = setup(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "cleanup:", err)
	}
	os.Exit(code)
}

// unsetGit unsets every GIT_* variable of the tests' environment.
func unsetGit() error {
	for _, variable := range os.Environ() {
		if name, _, _ := strings.Cut(variable, "="); strings.HasPrefix(name, "GIT_") {
			if err := os.Unsetenv(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func setup(dir string) error {
	// Open to every user, so that a test run as root can run cld as another one.
	if err := os.Chmod(dir, 0o755); err != nil {
		return err
	}
	if err := findTools(); err != nil {
		return err
	}
	sandbox.Cld = filepath.Join(dir, "cld")
	if err := run("go", "build", "-o", sandbox.Cld, "github.com/zadykian/cld/cmd/cld"); err != nil {
		return err
	}
	sandbox.ProbeBin = filepath.Join(dir, "probe")
	probe := filepath.Join(sandbox.ProbeBin, "claude")
	if err := run("go", "build", "-o", probe, "./probe"); err != nil {
		return err
	}
	// The fake systemd is on every sandbox's PATH, before the real one: no test reaches the user's
	// systemd.
	for _, name := range []string{"systemctl", "loginctl"} {
		if err := os.Symlink("claude", filepath.Join(sandbox.ProbeBin, name)); err != nil {
			return err
		}
	}
	sandbox.FakeTmux = filepath.Join(dir, "fake", "tmux")
	if err := os.Mkdir(filepath.Dir(sandbox.FakeTmux), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(probe, sandbox.FakeTmux); err != nil {
		return err
	}
	if slices.Contains(terminals, "jediterm") {
		return buildJediTerm(dir)
	}
	return nil
}

// findTools sets sandbox.RealTmux and sandbox.RealGit to the first tmux and git in the PATH's
// absolute entries. cld skips the relative ones (see tool.LookPath in internal/tool).
func findTools() error {
	for _, tool := range []struct {
		name string
		path *string
	}{{"tmux", &sandbox.RealTmux}, {"git", &sandbox.RealGit}} {
		for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
			if path := filepath.Join(entry, tool.name); filepath.IsAbs(entry) && *tool.path == "" {
				if _, err := exec.LookPath(path); err == nil {
					*tool.path = path
				}
			}
		}
		if *tool.path == "" {
			return errors.New(tool.name + " is not installed")
		}
	}
	return nil
}

// buildJediTerm compiles the JediTerm driver into dir, for the terminal contract.
func buildJediTerm(dir string) error {
	libraries := filepath.Join(envOr("CLD_JEDITERM_LIB", filepath.Join("jediterm", "lib")), "*")
	classes := filepath.Join(dir, "jediterm")
	driver := filepath.Join("jediterm", "JediTermDriver.java")
	if err := run("javac", "-cp", libraries, "-d", classes, driver); err != nil {
		return err
	}
	terminal.JediTermClasspath = classes + string(os.PathListSeparator) + libraries
	return nil
}

// The tests run nothing with a GIT_* variable they inherit. gitInit makes the work directory a
// repository even with GIT_DIR set, leaving the repository it names alone. git init DIR would
// initialise that one again instead, and take it for a bare one. Not parallel, for t.Setenv.
func TestGitVariables(t *testing.T) {
	for _, variable := range os.Environ() {
		if strings.HasPrefix(variable, "GIT_") {
			t.Errorf("the tests run with %s", variable)
		}
	}
	s := sandbox.New(t)
	gitInit(t, s)
	// Not named .git, as a worktree's git directory is not: git init guesses that such a
	// repository is bare, and writes core.bare = true into its config.
	gitDir := filepath.Join(s.Root, "rebased")
	if err := os.Rename(filepath.Join(s.Work, ".git"), gitDir); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(gitDir, "config")
	before, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", gitDir)
	gitInit(t, s)
	after, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("git init changed the config of the repository GIT_DIR names:\n%s\nwas\n%s",
			after, before)
	}
	if _, err := os.Stat(filepath.Join(s.Work, ".git", "HEAD")); err != nil {
		t.Errorf("git init made no repository of the work directory: %v", err)
	}
}
