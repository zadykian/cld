// Package tests holds cld's tests. They build cld and run it against real tmux servers, one
// private world per test (see internal/sandbox), with a probe in claude's place, and in docker's,
// systemctl's and loginctl's (see probe).
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
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

var terminals = strings.Split(envOr("CLD_TERMINALS", "tmux"), ",")

func TestMain(m *testing.M) {
	// git runs hooks and the commands of rebase --exec with GIT_* variables set, in a linked
	// worktree GIT_DIR among them; git init DIR, as the tests run it, would then initialise that
	// repository again instead of DIR, and take it for a bare one (see
	// docs/design/findings/environment.md). Nothing the tests run inherits any of them.
	for _, variable := range os.Environ() {
		if name, _, _ := strings.Cut(variable, "="); strings.HasPrefix(name, "GIT_") {
			_ = os.Unsetenv(name)
		}
	}
	dir, err := os.MkdirTemp("", "cld-tests.")
	if err == nil {
		err = setup(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func setup(dir string) error {
	// Open to every user, so that a test run as root can run cld as another one.
	if err := os.Chmod(dir, 0o755); err != nil {
		return err
	}
	// cld skips the relative entries of the PATH (see tool.LookPath in internal/tool).
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
	sandbox.Cld = filepath.Join(dir, "cld")
	if err := run("go", "build", "-o", sandbox.Cld, "github.com/zadykian/cld/cmd/cld"); err != nil {
		return err
	}
	sandbox.ProbeBin = filepath.Join(dir, "probe")
	if err := run("go", "build", "-o", filepath.Join(sandbox.ProbeBin, "claude"), "./probe"); err != nil {
		return err
	}
	// The fake docker and the fake systemd are on every sandbox's PATH, before the real ones: no
	// test reaches Docker, or the user's systemd.
	for _, name := range []string{"docker", "systemctl", "loginctl"} {
		if err := os.Symlink("claude", filepath.Join(sandbox.ProbeBin, name)); err != nil {
			return err
		}
	}
	sandbox.FakeTmux = filepath.Join(dir, "fake", "tmux")
	if err := os.Mkdir(filepath.Dir(sandbox.FakeTmux), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(filepath.Join(sandbox.ProbeBin, "claude"), sandbox.FakeTmux); err != nil {
		return err
	}
	if slices.Contains(terminals, "jediterm") {
		libraries := filepath.Join(envOr("CLD_JEDITERM_LIB", filepath.Join("jediterm", "lib")), "*")
		classes := filepath.Join(dir, "jediterm")
		if err := run("javac", "-cp", libraries, "-d", classes, filepath.Join("jediterm", "JediTermDriver.java")); err != nil {
			return err
		}
		terminal.JediTermClasspath = classes + string(os.PathListSeparator) + libraries
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// The tests run nothing with a GIT_* variable they inherit, and gitInit makes the work directory a
// repository even with GIT_DIR set, leaving the repository it names alone: git init DIR would
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
	if after, _ := os.ReadFile(config); string(after) != string(before) {
		t.Errorf("git init changed the config of the repository GIT_DIR names:\n%s\nwas\n%s", after, before)
	}
	if _, err := os.Stat(filepath.Join(s.Work, ".git", "HEAD")); err != nil {
		t.Errorf("git init made no repository of the work directory: %v", err)
	}
}

// forEachTerminal runs body as a subtest per terminal in CLD_TERMINALS.
func forEachTerminal(t *testing.T, body func(t *testing.T, name string)) {
	for _, name := range terminals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body(t, name)
		})
	}
}

// startCld runs cld with args in a new terminal of the given kind; extra variables are added to
// the sandbox environment.
func startCld(t *testing.T, s *sandbox.Sandbox, name string, extra map[string]string, args ...string) terminal.Terminal {
	t.Helper()
	return startCldIn(t, s, name, s.Work, extra, args...)
}

func startCldIn(t *testing.T, s *sandbox.Sandbox, name, dir string, extra map[string]string, args ...string) terminal.Terminal {
	t.Helper()
	term := terminal.New(t, name, s)
	env := map[string]string{}
	maps.Copy(env, s.Env)
	maps.Copy(env, extra)
	term.Start(s.CldArgv(args...), env, dir)
	return term
}
