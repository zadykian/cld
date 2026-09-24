// Package tests holds cld's tests. They run cld against real tmux servers, one private world per
// test (see internal/sandbox), with a probe in claude's place (see probe).
//
// CLD_TERMINALS lists the terminals the terminal contract runs against (default "tmux"):
// tmux, jediterm (needs a JDK and CLD_JEDITERM_LIB, see jediterm/fetch-deps).
// CLD_BASH runs cld under a specific bash.
package tests

import (
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

var terminals = strings.Split(envOr("CLD_TERMINALS", "tmux"), ",")

func TestMain(m *testing.M) {
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
	sandbox.ProbeBin = filepath.Join(dir, "probe")
	if err := run("go", "build", "-o", filepath.Join(sandbox.ProbeBin, "claude"), "./probe"); err != nil {
		return err
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
	for key, value := range s.Env {
		env[key] = value
	}
	for key, value := range extra {
		env[key] = value
	}
	term.Start(s.CldArgv(args...), env, dir)
	return term
}
