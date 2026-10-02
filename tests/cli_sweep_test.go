package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The sweep of the idle sessions: the session cld runs in, and a server it cannot read.

// list keeps the session whose server cld runs on, the socket TMUX names, even long idle
// (decision 46.2): ending it would end cld, and a claude that ran it. It compares sockets as files,
// as TMUX resolves symbolic links. A socket of that name elsewhere is another server's.
func TestSweepKeepsTheSessionCldRunsIn(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		// socket is TMUX's socket, relative to the sandbox's root, whose socket directory is
		// tmux-UID
		socket string
		kept   bool
	}{
		{name: "its socket", socket: "tmux-UID/cld-a", kept: true},
		{name: "its socket through a symbolic link", socket: "link/tmux-UID/cld-a", kept: true},
		{name: "a socket of that name elsewhere", socket: "other/tmux-UID/cld-a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			socket(t, s, "cld-a")
			own := sweptSocket(t, s, test.socket)
			result := s.RunCld(fakeTmuxEnv(s, map[string]string{
				"CLD_FAKE_TMUX_SESSIONS": fakeSession("a", 40*24*time.Hour+time.Hour),
				"TMUX":                   own + ",100,0",
			}), "list")
			if test.kept {
				checkKeptSession(t, s, result)
				return
			}
			want := "cld: ended session 'a', idle for 40 days\n"
			if result.Code != 0 || result.Stdout != "" || result.Stderr != want {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stderr %q",
					result.Code, result.Stdout, result.Stderr, want)
			}
			argv := s.FakeTmuxRecord().Argv
			if len(argv) < 3 || !slices.Equal(argv[:3], []string{"-L", "cld-a", "if"}) {
				t.Errorf("tmux arguments %q, want the kill of session a", argv)
			}
		})
	}
}

// checkKeptSession reports a list that ended session a, or did not show it idle for 40 days.
func checkKeptSession(t *testing.T, s *sandbox.Sandbox, result sandbox.Result) {
	t.Helper()
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     detached  40d          /w\n"
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	if fakeTmuxRan(s) {
		t.Errorf("tmux ran %q", s.FakeTmuxRecord().Argv)
	}
}

// sweptSocket makes a symbolic link to s's root, and a socket cld-a elsewhere. It returns the path
// of socket, relative to the root, with tmux-UID standing for s's socket directory.
func sweptSocket(t *testing.T, s *sandbox.Sandbox, socket string) string {
	t.Helper()
	if err := os.Symlink(s.Root, filepath.Join(s.Root, "link")); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(s.Root, "other", filepath.Base(s.SocketDir()))
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(filepath.Join(other, "cld-a"), "")
	return filepath.Join(s.Root,
		strings.ReplaceAll(socket, "tmux-UID", filepath.Base(s.SocketDir())))
}

// join without -s makes its session although a server does not answer the read for its sweep, and
// says so (decision 50.5). Then list, which needs that read, fails with tmux's message. The fake
// tmux fails on server cld-x, as tmux does where it may not connect; x is no index.
func TestJoinWarnsWhereTheSweepCannotRead(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	socket(t, s, "cld-x")
	fake := fakeTmuxEnv(s, map[string]string{"CLD_FAKE_TMUX_DENIED": "cld-x"})
	denied := "error connecting to /fake/tmux/cld-x (Permission denied)"
	result := s.RunCldOnTerminal(fake, "join")
	title := cldTitle("0")
	want := "cld: warning: cannot end the idle sessions: " + denied + "\n"
	if result.Code != 0 || result.Stdout != title || result.Stderr != want {
		t.Errorf("join: exit %d, stdout %q, stderr %q, want exit 0, stdout %q, stderr %q",
			result.Code, result.Stdout, result.Stderr, title, want)
	}
	argv := s.FakeTmuxRecord().Argv
	if len(argv) < 3 || !slices.Equal(argv[:3], []string{"-u", "-L", "cld-0"}) ||
		!slices.Contains(argv, "new-session") {
		t.Errorf("tmux arguments %q, want new-session on server cld-0", argv)
	}
	result = s.RunCld(fake, "list")
	if want := "cld: " + denied + "\n"; result.Code != 1 || result.Stdout != "" ||
		result.Stderr != want {
		t.Errorf("list: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
}
