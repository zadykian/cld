// Package tests holds cld's tests. They run cld against real tmux servers, one private world per
// test (see internal/sandbox), with a probe in claude's place (see probe).
//
// CLD_BASH runs cld under a specific bash.
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

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
	return os.Symlink(filepath.Join(sandbox.ProbeBin, "claude"), sandbox.FakeTmux)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
	}
	return nil
}
