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
// terminal emulator. join refuses to hand tmux anything else. The terminal's other end, the
// controller, reads what is written there from the start, as a terminal emulator does. A program
// writing there is then not held up; Output returns what was written.
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
func OpenPty(tb testing.TB) *Pty {
	tb.Helper()
	controller, path, err := openPty()
	if err != nil {
		tb.Fatalf("open a pseudo-terminal: %v", err)
	}
	terminal, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		tb.Fatal(errors.Join(err, controller.Close()))
	}
	p := &Pty{t: tb, Path: path, Terminal: terminal, controller: controller,
		read: make(chan struct{})}
	// The read ends once no program has the terminal open: with EIO on Linux.
	go func() {
		defer close(p.read)
		io.Copy(&p.output, controller) //nolint:errcheck // the error that ends it is no failure
	}()
	tb.Cleanup(func() {
		for _, file := range []*os.File{terminal, controller} {
			if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				tb.Error(err)
			}
		}
	})
	return p
}

// OpenController opens a pseudo-terminal's controller and names its terminal, for a terminal
// emulator of the tests' to read and write.
func OpenController() (controller *os.File, path string, err error) {
	return openPty()
}

// Output is what was written to the terminal, once every program that had it open has closed it:
// it closes the test's own Terminal first.
func (p *Pty) Output() string {
	p.t.Helper()
	if err := p.Terminal.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		p.t.Fatal(err)
	}
	select {
	case <-p.read:
	case <-time.After(10 * time.Second):
		p.t.Fatalf("%s still open after 10s", p.Path)
	}
	return p.output.String()
}

// RunCldOnTerminal runs cld as RunCld does, but on a terminal. A pseudo-terminal (see OpenPty) is
// its stdin and stdout, and its controlling terminal in a session of its own, as a shell gives cld
// its terminal. Result's Stdout is what cld wrote there - tmux too, once cld has become tmux.
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
