// Package tool finds the programs cld runs on the PATH: tmux, claude, git and tty for the
// sessions, git for setup config project, systemctl and loginctl for setup restore. It says how
// cld ends when the system cannot run one, as a shell would.
package tool

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zadykian/cld/internal/fail"
)

// LookPath finds name in the absolute entries of the PATH, as bash does (decision 11.5). It returns
// the first executable file of that name, or else the first file of that name, which then fails
// to run (see CannotRun). exec.LookPath would stop at a match in a relative entry.
func LookPath(name string) (string, error) {
	lastResort := ""
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		path := filepath.Join(dir, name)
		if _, err := exec.LookPath(path); err == nil {
			return path, nil
		}
		if info, err := os.Stat(path); lastResort == "" && err == nil && !info.IsDir() {
			lastResort = path
		}
	}
	if lastResort != "" {
		return lastResort, nil
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

// Command is the program name, found by LookPath, to run with args, cld's stdin and stderr.
func Command(name string, args ...string) (*exec.Cmd, error) {
	path, err := LookPath(name)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	cmd.Args[0] = name
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	return cmd, nil
}

// CannotRun ends cld when the system cannot run the program at path at all - a #! naming no
// interpreter, a binary for another machine, a file without the execute permission - with the
// status a shell gives: 127 when the system reports no such file, 126 otherwise.
func CannotRun(path string, err error) error {
	status := 126
	if errors.Is(err, fs.ErrNotExist) {
		status = 127
	}
	if pathError, ok := errors.AsType[*fs.PathError](err); ok {
		err = pathError.Err
	}
	return &fail.Error{Status: status, Message: fmt.Sprintf("cannot run %s: %v", path, err)}
}
