package tests

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What join --host does with signals, ssh killed, the terminal's input and a missing ssh
// (decision 68).

// syncBuffer is a buffer a test reads while a program writes it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startJoin starts cld join --host ws -n app -s 1 on a terminal of its own, with the fake ssh and
// env. It returns the command, the terminal and cld's stderr as it grows. A channel gets how cld
// ends.
func startJoin(t *testing.T, s *sandbox.Sandbox, env map[string]string) (
	*exec.Cmd, *sandbox.Pty, *syncBuffer, chan error) {
	t.Helper()
	p := sandbox.OpenPty(t)
	argv := s.CldArgv("join", "--host", "ws", "-n", "app", "-s", "1")
	cmd := exec.Command(argv[0], argv[1:]...)
	env["PATH"] = s.Tools("ssh")
	cmd.Env = s.Environ(env)
	stderr := &syncBuffer{}
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = s.Work, p.Terminal, p.Terminal, stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return cmd, p, stderr, done
}

// endsWith waits for cld to end, and checks its status.
func endsWith(t *testing.T, cmd *exec.Cmd, done chan error, stderr *syncBuffer, want int) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal(errors.Join(errors.New("cld runs on 10 s after the signal"), cmd.Process.Kill()))
	}
	if code := cmd.ProcessState.ExitCode(); code != want {
		t.Errorf("exit %d, stderr %q, want exit %d", code, stderr.String(), want)
	}
}

var signals = []syscall.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}

// Ctrl-C in the wait before a try, a hangup as the pane closes, or SIGTERM, ends join with 128
// plus the signal's number, and no try follows. The signal comes once cld has said it waits.
func TestHostJoinSignal(t *testing.T) {
	t.Parallel()
	for _, signal := range signals {
		t.Run(signal.String(), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			cmd, _, stderr, done := startJoin(t, s, map[string]string{"CLD_FAKE_SSH_EXIT": "255"})
			sandbox.WaitFor(t, 10*time.Second, "the note of the wait", func() bool {
				return strings.Contains(stderr.String(), dropNote)
			})
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			endsWith(t, cmd, done, stderr, 128+int(signal))
			checkSSH(t, s, joinSSH("ws", "join", "--name=app", "--suffix=1"))
		})
	}
}

// A signal while ssh runs goes on to ssh, and ends join with 128 plus its number, though ssh
// ends with 255: no drop, no try. The fake ssh holds until a signal.
func TestHostJoinSignalWhileRunning(t *testing.T) {
	t.Parallel()
	for _, signal := range signals {
		t.Run(signal.String(), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			cmd, _, stderr, done := startJoin(t, s, map[string]string{"CLD_FAKE_SSH_HOLD": "1"})
			sandbox.WaitFor(t, 10*time.Second, "ssh's call", func() bool {
				return len(s.SSHCalls()) == 1
			})
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			endsWith(t, cmd, done, stderr, 128+int(signal))
			if strings.Contains(stderr.String(), "trying again") {
				t.Errorf("cld tried again after the signal: %q", stderr.String())
			}
			checkSSH(t, s, joinSSH("ws", "join", "--name=app", "--suffix=1"))
		})
	}
}

// ssh killed by a signal, not cld's, ends join with 128 plus its number, and no try follows.
func TestHostJoinSSHKilled(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCldOnTerminal(map[string]string{"PATH": s.Tools("ssh"),
		"CLD_FAKE_SSH_KILL": "9"}, "join", "--host", "ws", "-n", "app", "-s", "1")
	if result.Code != 128+9 || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, want exit 137, no stderr", result.Code, result.Stderr)
	}
	checkSSH(t, s, joinSSH("ws", "join", "--name=app", "--suffix=1"))
}

// What the user types while cld waits is flushed before the next try: ssh would send it to the
// host, where claude takes it as a prompt. The fake ssh logs what the terminal holds on each try.
func TestHostJoinFlushesInput(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	held := filepath.Join(t.TempDir(), "held")
	cmd, p, stderr, done := startJoin(t, s, map[string]string{"CLD_FAKE_SSH_EXIT": "255,0",
		"CLD_FAKE_SSH_STDIN": held})
	sandbox.WaitFor(t, 10*time.Second, "the note of the wait", func() bool {
		return strings.Contains(stderr.String(), dropNote)
	})
	p.Write("git status\n")
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal(errors.Join(errors.New("cld runs on"), cmd.Process.Kill()))
	}
	got, err := os.ReadFile(held)
	if err != nil {
		t.Fatal(err)
	}
	if want := "\"\"\n\"\"\n"; string(got) != want {
		t.Errorf("the terminals held %q on the tries, want %q", got, want)
	}
}

// Without ssh, or with one the system cannot run, join ends as a join without a terminal does.
func TestHostJoinNoSSH(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCldOnTerminal(map[string]string{"PATH": s.Tools()}, "join", "--host", "ws",
		"-n", "app", "-s", "1")
	if result.Code != 1 || !strings.Contains(result.Stderr, "ssh is not installed") {
		t.Errorf("exit %d, stderr %q, want exit 1, ssh not installed", result.Code, result.Stderr)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("not a program\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result = s.RunCldOnTerminal(map[string]string{"PATH": dir}, "join", "--host", "ws", "-n", "app",
		"-s", "1")
	if result.Code != 126 || !strings.Contains(result.Stderr, "cannot run") {
		t.Errorf("exit %d, stderr %q, want exit 126, cannot run", result.Code, result.Stderr)
	}
}
