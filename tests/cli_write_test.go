package tests

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What cld writes to a stdout that takes no write.

// runToEnd runs cmd, writing to stdout, for its exit status and stderr.
func runToEnd(t *testing.T, cmd *exec.Cmd, stdout io.Writer) (int, string) {
	t.Helper()
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	var exit *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exit) {
		t.Fatalf("run %s: %v", cmd.Path, err)
	}
	return cmd.ProcessState.ExitCode(), stderr.String()
}

// A write to stdout that fails ends cld with status 1, so that output cut short does not pass for
// whole (decisions 11.9, 12.5 and 17.6). Then join hands nothing to tmux. Here stdout is open for
// reading only, for join the same terminal as its stdin. And list and __complete find session x
// through a socket cld-x.
func TestFailedWriteEndsCld(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// sessions is what the fake tmux lists.
		sessions string
		// before is what cobra writes to stderr first.
		before string
	}{
		{[]string{"list"}, fakeSession("x", 0), ""},
		{[]string{"help"}, "", ""},
		{[]string{"help", "join"}, "", ""},
		{[]string{"join", "-h"}, "", ""},
		{[]string{"detach", "-h"}, "", ""},
		{[]string{"kill", "-h", "-x"}, "", ""},
		{[]string{"version"}, "", ""},
		{[]string{"join", "-s", "x"}, "", ""},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, "", ""},
		{[]string{"join", "-s", "x", "--detach-others"}, "cld-x", ""},
		{[]string{"completion", "bash"}, "", ""},
		{[]string{"completion", "zsh", "--help"}, "", ""},
		{[]string{"completion"}, "", ""},
		{[]string{"__complete", "join", "-s", ""}, fakeSession("x", 0), noFileReport},
		{[]string{"help", "setup"}, "", ""},
		{[]string{"setup", "-h", "restore"}, "", ""},
		{[]string{"help", "setup", "config", "project"}, "", ""},
		{[]string{"setup", "config", "project", "--mcp", "goland"}, "", ""},
		{[]string{"setup", "config", "user"}, "", ""},
		{[]string{"help", "setup", "config"}, "", ""},
		{[]string{"help", "setup", "completion"}, "", ""},
		{[]string{"setup", "completion", "fish"}, "", ""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			socket(t, s, "cld-x")
			code, stderr := runFailingWrite(t, s, test.sessions, test.args)
			want := test.before + "cld: write error: bad file descriptor\n"
			if code != 1 || stderr != want {
				t.Errorf("exit %d, stderr %q, want exit 1, stderr %q", code, stderr, want)
			}
			if fakeTmuxRan(s) {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// runFailingWrite runs cld with args, where the fake tmux lists sessions, for its exit status and
// stderr. Its stdout takes no write: the null device, or for join its terminal stdin, each open
// for reading only.
func runFailingWrite(
	t *testing.T, s *sandbox.Sandbox, sessions string, args []string,
) (int, string) {
	t.Helper()
	cmd := exec.Command(sandbox.Cld, args...)
	written := os.DevNull
	if args[0] == "join" {
		pty := sandbox.OpenPty(t)
		cmd.Stdin, written = pty.Terminal, pty.Path
	}
	stdout, err := os.OpenFile(written, os.O_RDONLY|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close() //nolint:errcheck // opened for cld's writes to fail
	cmd.Env = s.Environ(fakeTmuxEnv(s, map[string]string{"CLD_FAKE_TMUX_SESSIONS": sessions}))
	cmd.Dir = s.Work
	return runToEnd(t, cmd, stdout)
}

// With nothing to print, cld writes nothing, as even an empty write to a stdout that takes none
// fails. So kill ends the session and exits 0 with stdout open for reading only, as detach does
// once it has detached the session's terminals.
func TestNothingToPrintWritesNothing(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		// argv is what tmux gets
		argv []string
	}{
		{[]string{"kill", "-s", "a"}, nil},
		{[]string{"detach", "-s", "a"}, []string{"-L", "cld-a",
			"if", "-F", "-t", "=cld-a:", "#{session_attached}", "detach-client -s =cld-a"}},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			stdout, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close() //nolint:errcheck // opened for cld's writes to fail
			cmd := exec.Command(sandbox.Cld, test.args...)
			// The fake tmux finds session a on its server, then records the command.
			cmd.Env = s.Environ(fakeTmuxEnv(s, map[string]string{"CLD_FAKE_TMUX_SESSIONS": "cld-a"}))
			cmd.Dir = s.Work
			if code, stderr := runToEnd(t, cmd, stdout); code != 0 || stderr != "" {
				t.Errorf("exit %d, stderr %q, want exit 0, no stderr", code, stderr)
			}
			want := test.argv
			if want == nil {
				want = []string{"-L", "cld-a", "run-shell", unmarkCommand(s, "a"), ";",
					"kill-session", "-t", "=cld-a", ";", "kill-server"}
			}
			if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv, want) {
				t.Errorf("tmux arguments\n%q\nwant\n%q", argv, want)
			}
		})
	}
}
