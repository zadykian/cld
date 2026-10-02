package tests

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// tmux's socket directory, and the servers whose sockets cld finds there.

// Under a long TMUX_TMPDIR, a name within 64 characters can still make the socket path too long
// for sun_path (decision 13.2). There cld ends with tmux's message rather than take that for no
// server, and join stops before it would create the session. And list finds no socket.
func TestSocketPathTooLong(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := filepath.Join(s.Root, strings.Repeat("d", 60))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("n", 64)
	path := filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()), "cld-"+name)
	want := "cld: error connecting to " + path + " (File name too long)\n"
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "-s", name}, want},
		{[]string{"join", "-s", name, "--resume", "SESSION"}, want},
		{[]string{"kill", "-s", name}, want},
		{[]string{"detach", "-s", name}, want},
		{[]string{"list"}, ""},
	} {
		result := s.RunCld(map[string]string{"TMUX_TMPDIR": dir}, test.args...)
		code := min(len(test.want), 1)
		if result.Code != code || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
				test.args[0], result.Code, result.Stdout, result.Stderr, code, test.want)
		}
	}
}

// list fails with the reason where it cannot read tmux's socket directory, here a file, rather
// than show no session. So does join without -s, reading it for the index. With -s, join, detach
// and kill end with tmux's message, alike from 3.3a to 3.7c (decision 38.1). With no TMUX_TMPDIR,
// cld reads /tmp/tmux-UID as tmux does, where the user's own sockets are: no test reaches it.
func TestUnreadableSocketDirectory(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	s.WriteFile(s.SocketDir(), "")
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"list"}, "cld: cannot read " + s.SocketDir() + ": not a directory\n"},
		{[]string{"join"}, "cld: cannot read " + s.SocketDir() + ": not a directory\n"},
		{[]string{"join", "-s", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
		{[]string{"kill", "-s", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
		{[]string{"detach", "-s", "a"}, "cld: " + s.SocketDir() + " is not a directory\n"},
	} {
		result := s.RunCld(nil, test.args...)
		if result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
				test.args[0], result.Code, result.Stdout, result.Stderr, test.want)
		}
	}
}

// tmux refuses a socket directory others can use, and cld ends with its message there, taking no
// socket for no server (decision 38.1). Here tmux-UID holds 12 stale sockets, and list asks no
// more servers once they fail (decision 38.2). A tmux first on the PATH writes down that list
// asked eight at most.
func TestUnsafeSocketDirectory(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for i := range 12 {
		staleSocket(t, s, "cld-"+strconv.Itoa(i))
	}
	if err := os.Chmod(s.SocketDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	logged, asked := loggedTmux(t, s)
	want := "cld: directory " + s.SocketDir() + " has unsafe permissions\n"
	for _, args := range [][]string{
		{"list"}, {"join"}, {"join", "-s", "a"}, {"join", "-s", "0"}, {"detach", "-s", "1"},
		{"kill", "-s", "a"},
	} {
		result := s.RunCld(logged, args...)
		if result.Code != 1 || result.Stdout != "" || result.Stderr != want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
				strings.Join(args, " "), result.Code, result.Stdout, result.Stderr, want)
		}
		if got := asked(); args[0] == "list" && len(got) > 8 {
			t.Errorf("list asked %d servers %q, want 8 at most", len(got), got)
		}
	}
}

// A server can exit while cld asks it, and tmux then says it exited unexpectedly (decision 13.1).
// Then list passes over it, detach and kill find no session there, and join none to attach to: it
// would make session b, but has no terminal here. The fake tmux's server cld-b exits as asked.
func TestServerExitingWhileAsked(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	socket(t, s, "cld-a")
	socket(t, s, "cld-b")
	fake := fakeTmuxEnv(s, map[string]string{
		"CLD_FAKE_TMUX_SESSIONS": fakeSession("a", 0),
		"CLD_FAKE_TMUX_EXITED":   "cld-b",
	})
	for _, test := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"list"}, 0,
			"NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     detached  now          /w\n", ""},
		{[]string{"join", "-s", "b"}, 1, "",
			"cld: join needs a terminal, and its input is not one\n"},
		{[]string{"kill", "-s", "b"}, 1, "", "cld: no session 'b' (see cld list)\n"},
		{[]string{"detach", "-s", "b"}, 1, "", "cld: no session 'b' (see cld list)\n"},
	} {
		result := s.RunCld(fake, test.args...)
		if result.Code != test.code || result.Stdout != test.stdout ||
			result.Stderr != test.stderr {
			t.Errorf("%s: exit %d, stderr %q, stdout\n%s\nwant exit %d, stderr %q, stdout\n%s",
				strings.Join(test.args, " "), result.Code, result.Stderr, result.Stdout,
				test.code, test.stderr, test.stdout)
		}
	}
}
