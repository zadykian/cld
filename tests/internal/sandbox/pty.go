package sandbox

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// Pty is a pseudo-terminal of the test's, for cld to run on where a test needs a terminal but no
// terminal emulator: join refuses to hand tmux anything else. What is written to the terminal is
// read at its other end, the controller, from the start, as a terminal emulator reads it, so that a
// program writing there is not held up; Output returns it.
type Pty struct {
	t testing.TB
	// Path names the terminal: /dev/pts/N on Linux, /dev/ttysN on macOS.
	Path string
	// Terminal is the terminal, open for reading and writing, and never the test's controlling
	// terminal.
	Terminal   *os.File
	controller *os.File
	output     bytes.Buffer
	read       chan struct{}
}

// OpenPty opens a pseudo-terminal, closed when the test ends.
func OpenPty(t testing.TB) *Pty {
	t.Helper()
	controller, path, err := openPty()
	if err != nil {
		t.Fatalf("open a pseudo-terminal: %v", err)
	}
	terminal, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		controller.Close()
		t.Fatal(err)
	}
	p := &Pty{t: t, Path: path, Terminal: terminal, controller: controller, read: make(chan struct{})}
	// The read ends once no program has the terminal open: with EIO on Linux.
	go func() {
		defer close(p.read)
		_, _ = io.Copy(&p.output, controller)
	}()
	t.Cleanup(func() {
		terminal.Close()
		controller.Close()
	})
	return p
}

// Output is what was written to the terminal, once every program that had it open has closed it:
// it closes the test's own Terminal first.
func (p *Pty) Output() string {
	p.t.Helper()
	p.Terminal.Close()
	select {
	case <-p.read:
	case <-time.After(10 * time.Second):
		p.t.Fatalf("%s still open after 10s", p.Path)
	}
	return p.output.String()
}

// RunCldOnTerminal runs cld as RunCld does, but on a terminal: a pseudo-terminal (see OpenPty) is
// its stdin and stdout, and its controlling terminal, in a session of its own, as a shell gives
// cld its terminal. Result's Stdout is what cld wrote there - tmux too, once cld has become tmux.
// stderr stays a pipe.
func (s *Sandbox) RunCldOnTerminal(extra map[string]string, args ...string) Result {
	s.t.Helper()
	return s.RunCldOnTerminalIn(s.Work, extra, args...)
}

// RunCldOnTerminalIn runs cld as RunCldOnTerminal does, in the directory dir.
func (s *Sandbox) RunCldOnTerminalIn(dir string, extra map[string]string, args ...string) Result {
	s.t.Helper()
	pty := OpenPty(s.t)
	argv := s.CldArgv(args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = s.Environ(extra)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pty.Terminal, pty.Terminal, &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		s.t.Fatalf("run cld: %v", err)
	}
	return Result{Code: cmd.ProcessState.ExitCode(), Stdout: pty.Output(), Stderr: stderr.String()}
}
