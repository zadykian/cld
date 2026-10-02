package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

// Claude is the claude join starts: the one CheckClaude found and checked.
type Claude struct {
	path string
}

// CheckClaude checks the version of the claude on the PATH, for the commands that start it
// (decision 6). Output with no X.Y.Z at its start passes, so that a new format locks no one out. A
// claude --version that fails is refused with what it printed, as such a claude is unlikely to
// start.
func CheckClaude() (*Claude, error) {
	path, err := tool.LookPath("claude")
	if err != nil {
		return nil, fail.Runtime("claude is not installed")
	}
	// claude --version runs as tmux starts claude: by its path, in the current directory, so that
	// a version manager's shim runs the claude that directory pins (decision 6). A directory that
	// is gone or closed is refused first, as join refuses it.
	dir, err := workingDirectory()
	if err != nil {
		return nil, err
	}
	return checkClaude(path, dir, nil)
}

// checkClaude is CheckClaude's check of the claude at path, run in dir with the environment env,
// or cld's own where env is nil. restore checks the claude a session started with, in its
// directory and its environment (see Tmux.Restore).
func checkClaude(path, dir string, env []string) (*Claude, error) {
	var stdout, stderr bytes.Buffer
	run := func(name string, args ...string) error {
		stdout.Reset()
		stderr.Reset()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		// A process claude leaves in the background, a wrapper's update check say, can hold the
		// pipes open. cld waits for it a second at most, and ends them (decision 6).
		cmd.WaitDelay = time.Second
		if err := cmd.Run(); !errors.Is(err, exec.ErrWaitDelay) {
			return err
		}
		return nil
	}
	err := run(path, "--version")
	// tmux's execvp runs a file the system will not execute with /bin/sh, where os/exec does not.
	// A binary, for another machine or cut short, is no script, and cannot run (decision 6).
	if errors.Is(err, syscall.ENOEXEC) && !binary(path) {
		err = run("/bin/sh", path, "--version")
	}
	required := "claude " + minClaude.String() + " or newer is required"
	output := strings.TrimRight(stdout.String(), "\n")
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		printed := strings.Trim(output+"\n"+strings.TrimRight(stderr.String(), "\n"), "\n")
		return nil, versionFailed(required, exit, printed)
	case err != nil:
		return nil, tool.CannotRun(path, err)
	}
	if v, ok := parseVersion(claudeVersion, output); ok && v.before(minClaude) {
		return nil, fail.Runtime(fmt.Sprintf("%s, found '%s'", required, output))
	}
	return &Claude{path: path}, nil
}

// versionFailed refuses a claude whose --version exited as exit says, with what it printed.
func versionFailed(required string, exit *exec.ExitError, printed string) error {
	how := "status " + strconv.Itoa(exit.ExitCode())
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		how = "signal " + strconv.Itoa(int(status.Signal()))
	}
	message := required + ", but claude --version exited with " + how
	if printed != "" {
		message += ": " + printed
	}
	return fail.Runtime(message)
}

// binary reports whether the file at path is a binary rather than a script, as bash's
// check_binary_file tells them apart. A binary has ELF's magic number, or a NUL in the first line
// (two lines after #!) within 80 bytes. A file cld cannot read passes for a script: /bin/sh then
// says why.
func binary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // a file only read loses nothing
	sample := make([]byte, 80)
	n, _ := io.ReadFull(f, sample) //nolint:errcheck // what was read is the sample
	sample = sample[:n]
	if bytes.HasPrefix(sample, []byte("\x7fELF")) {
		return true
	}
	lines := 1
	if bytes.HasPrefix(sample, []byte("#!")) {
		lines = 2
	}
	for _, c := range sample {
		if c == 0 {
			return true
		}
		if c == '\n' {
			if lines--; lines == 0 {
				break
			}
		}
	}
	return false
}

// workingDirectory is the current directory, where join starts claude. One that has been removed,
// or that cannot be entered, is refused: tmux would start claude in the home directory instead
// (decision 11.10).
func workingDirectory() (string, error) {
	dir, err := os.Getwd()
	if errors.Is(err, fs.ErrNotExist) {
		return "", fail.Runtime("the current directory no longer exists")
	}
	// With PWD set, Getwd first looks at ".", which takes the search permission as well.
	if errors.Is(err, fs.ErrPermission) {
		return "", cannotEnter(err)
	}
	if err != nil {
		return "", fail.Runtime("cannot get the current directory: " + err.Error())
	}
	// tmux, and claude --version, enter the directory by its path.
	if err := unix.Access(dir, unix.X_OK); err != nil {
		return "", cannotEnter(err)
	}
	return dir, nil
}

// cannotEnter refuses a current directory that cannot be entered, with the system's reason.
func cannotEnter(err error) error {
	if errno, ok := errors.AsType[syscall.Errno](err); ok {
		err = errno
	}
	return fail.Runtime("cannot enter the current directory: " + err.Error())
}
