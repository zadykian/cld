// Package sandbox gives each test an isolated world: its own tmux socket directory, HOME and
// probe records. Tests then run in parallel, and never touch the user's own cld sessions. cld runs
// each session on a server of its own, named like it: session cld-NAME on server cld-NAME.
package sandbox

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"testing"
	"time"
)

var (
	// Cld is the cld binary under test; built by TestMain.
	Cld string
	// ProbeBin is the directory holding the probe installed as "claude", and as "systemctl" and
	// "loginctl", the fake systemd (see SystemdCalls); set by TestMain.
	ProbeBin string
	// FakeTmux is the probe installed as "tmux"; set by TestMain.
	FakeTmux string
	// RealTmux and RealGit are the tmux and the git cld finds on a sandbox's PATH: the first in
	// its absolute entries; set by TestMain.
	RealTmux, RealGit string
)

// Sandbox is the isolated world of one test.
type Sandbox struct {
	t    testing.TB
	Root string
	Home string
	// Work is where cld runs unless a test says otherwise: a directory named "_", of which
	// nothing is left in a session's name. So -s SUFFIX names a session SUFFIX there, and the
	// tests name their sessions exactly.
	Work     string
	ProbeDir string
	// Env is the environment cld runs in; terminals add their own variables on top.
	Env map[string]string
}

// New creates a sandbox that is torn down when the test ends.
func New(tb testing.TB) *Sandbox {
	tb.Helper()
	// The root is TMUX_TMPDIR, short so that socket paths fit in sun_path (decision 13.2).
	root, err := os.MkdirTemp("/tmp", "cld.") //nolint:usetesting // t.TempDir's paths are too long
	if err != nil {
		tb.Fatal(err)
	}
	if root, err = filepath.EvalSymlinks(root); err != nil { // macOS: /tmp -> /private/tmp
		tb.Fatal(err)
	}
	s := &Sandbox{
		t:        tb,
		Root:     root,
		Home:     filepath.Join(root, "home"),
		Work:     filepath.Join(root, "_"),
		ProbeDir: filepath.Join(root, "probe"),
	}
	for _, dir := range []string{s.Home, s.Work, s.ProbeDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
	}
	// cld must not read the user's tmux configuration; this one would show if it did.
	s.WriteFile(filepath.Join(s.Home, ".tmux.conf"),
		"set -g prefix C-a\nset -g status-left POISONED\n")
	s.Env = map[string]string{
		"PATH":          ProbeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME":          s.Home,
		"TMUX_TMPDIR":   root,
		"CLD_PROBE_DIR": s.ProbeDir,
		"TERM":          "xterm-256color",
		"LANG":          locale(),
	}
	tb.Cleanup(func() {
		s.killServers()
		_ = os.RemoveAll(root) //nolint:errcheck // best effort: a straggler may still write there
	})
	return s
}

// locale is a UTF-8 locale the platform has.
func locale() string {
	if runtime.GOOS == "darwin" {
		return "en_US.UTF-8"
	}
	return "C.UTF-8"
}

// Environ turns the sandbox environment plus extra variables into exec.Cmd.Env form.
func (s *Sandbox) Environ(extra map[string]string) []string {
	env := make([]string, 0, len(s.Env)+len(extra))
	for name, value := range s.Env {
		if _, overridden := extra[name]; !overridden {
			env = append(env, name+"="+value)
		}
	}
	for name, value := range extra {
		env = append(env, name+"="+value)
	}
	slices.Sort(env)
	return env
}

// CldArgv is the command line that runs cld with args.
func (s *Sandbox) CldArgv(args ...string) []string {
	return append([]string{Cld}, args...)
}

// Result is the outcome of a command run without a terminal.
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// RunCld runs cld without a terminal, which is enough for everything cld does before tmux but
// the handover itself. join refuses without a terminal, once its other checks pass (see
// RunCldOnTerminal). extra variables are added to the sandbox environment.
func (s *Sandbox) RunCld(extra map[string]string, args ...string) Result {
	s.t.Helper()
	return s.RunCldIn(s.Work, extra, args...)
}

// RunCldIn runs cld as RunCld does, in the directory dir.
func (s *Sandbox) RunCldIn(dir string, extra map[string]string, args ...string) Result {
	s.t.Helper()
	argv := s.CldArgv(args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = s.Environ(extra)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		s.t.Fatalf("run cld: %v", err)
	}
	return Result{
		Code:   cmd.ProcessState.ExitCode(),
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}

// Tools creates a directory holding only the named tools, for a PATH that lacks the others.
// "tmux" is the fake tmux, "claude" the probe, and "systemctl" and "loginctl" the fake systemd;
// any other name links the real tool.
func (s *Sandbox) Tools(names ...string) string {
	s.t.Helper()
	dir, err := os.MkdirTemp(s.Root, "tools.")
	if err != nil {
		s.t.Fatal(err)
	}
	for _, name := range names {
		target := FakeTmux
		switch name {
		case "claude", "systemctl", "loginctl":
			target = filepath.Join(ProbeBin, name)
		case "tmux":
		default:
			if target, err = exec.LookPath(name); err != nil {
				s.t.Fatal(err)
			}
		}
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			s.t.Fatal(err)
		}
	}
	return dir
}

// WriteFile writes a file or fails the test.
func (s *Sandbox) WriteFile(path, content string) {
	s.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// WriteProgram writes a program for cld to run, with the permissions mode, or fails the test. It
// holds syscall.ForkLock as it writes. A child another test forks meanwhile would hold the file
// open for writing until its exec, and running the program would fail with ETXTBSY
// (golang/go#22315).
func (s *Sandbox) WriteProgram(path, content string, mode os.FileMode) {
	s.t.Helper()
	syscall.ForkLock.RLock()
	err := os.WriteFile(path, []byte(content), mode)
	syscall.ForkLock.RUnlock()
	if err != nil {
		s.t.Fatal(err)
	}
}

// WaitFor polls condition until it holds, failing the test after timeout.
func WaitFor(tb testing.TB, timeout time.Duration, what string, condition func() bool) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			tb.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// glob lists the paths that match pattern, or fails the test on a malformed one.
func (s *Sandbox) glob(pattern string) []string {
	s.t.Helper()
	paths, err := filepath.Glob(pattern)
	if err != nil {
		s.t.Fatal(err)
	}
	return paths
}
