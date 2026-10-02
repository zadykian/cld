package tests

import (
	"maps"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The command-line tests check what cld does before it hands over to tmux, with no terminal
// emulator. A test that hands over runs cld on a pty of its own, mostly to the fake tmux. These
// are their helpers: the fake tmux, the sessions it lists, its sockets and what cld ends with.

// fakeTmuxEnv is the environment, with vars, in which cld finds the fake tmux first on the PATH,
// passing the version check (see probe).
func fakeTmuxEnv(s *sandbox.Sandbox, vars map[string]string) map[string]string {
	path := filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) + s.Env["PATH"]
	env := map[string]string{"PATH": path, "CLD_FAKE_TMUX_VERSION": "tmux 3.7c"}
	maps.Copy(env, vars)
	return env
}

// cldTitle is the title join prints for session cld-NAME as it hands over (see decision 31.4).
func cldTitle(name string) string {
	return "\x1b]0;\u2733 cld-" + name + "\x07"
}

// checkTitled ends the test where join did not end as it does on handing over to the fake tmux:
// with status 0, and the title of session cld-NAME alone.
func checkTitled(t *testing.T, result sandbox.Result, name string) {
	t.Helper()
	if title := cldTitle(name); result.Code != 0 || result.Stdout != title || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			result.Code, result.Stdout, result.Stderr, title)
	}
}

// fakeTmuxRan is whether the fake tmux ran a command other than -V and list-sessions, which it
// records rather than runs: join's handover among them.
func fakeTmuxRan(s *sandbox.Sandbox) bool {
	_, err := os.Stat(filepath.Join(s.ProbeDir, "tmux.json"))
	return err == nil
}

// checkFailed reports a result other than exit status code, stderr want and nothing on stdout.
func checkFailed(t *testing.T, result sandbox.Result, code int, want string) {
	t.Helper()
	if result.Code != code || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stderr %q",
			result.Code, result.Stdout, result.Stderr, code, want)
	}
}

// fakeSession is the line the fake tmux prints for list-sessions as Sessions asks it: session
// cld-NAME, detached, claude's pid 100, the directory /w. Its activity and its last attach are
// idle before now.
func fakeSession(name string, idle time.Duration) string {
	return fakeSessionAt(name, fakeTime(idle), fakeTime(idle))
}

// fakeSessionAt is fakeSession's line with the session's activity and its last attach as tmux
// gives them, in seconds since the epoch. The last attach is empty where no terminal attached.
func fakeSessionAt(name, activity, attached string) string {
	return fakeSessionIn(name, "detached ", activity, attached)
}

// fakeSessionIn is fakeSessionAt's line with state, the field of the session's state and claude's
// status (see session.Tmux.Sessions). Such are "detached " where no hook set the status,
// "attached busy" and "exited". A session attached has a terminal attached.
func fakeSessionIn(name, state, activity, attached string) string {
	clients := "0"
	if strings.HasPrefix(state, "attached") {
		clients = "1"
	}
	return "cld-" + name + "\t" + state + "\t" + clients + "\t100\t" + activity + " " + attached +
		"\t0\t/w"
}

// fakeTime is the time ago before now in whole seconds since the epoch, as tmux gives a session's
// times. It rounds up, so cld reads up to a second less than ago; rounded down, 59 s could read
// as a minute. So a case stays over a second above the unit it shows, and seconds below the next.
func fakeTime(ago time.Duration) string {
	since := time.Now().Add(-ago)
	seconds := since.Unix()
	if since.Nanosecond() > 0 {
		seconds++
	}
	return strconv.FormatInt(seconds, 10)
}

// socket makes the sandbox's socket of server take connections until the test ends, as a running
// server's does: cld connects before it runs tmux there (decision 38.1). The fake tmux answers its
// lookups, whatever the socket; a stale one is staleSocket's.
func socket(t *testing.T, s *sandbox.Sandbox, server string) {
	t.Helper()
	if err := os.MkdirAll(s.SocketDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	address := &net.UnixAddr{Name: filepath.Join(s.SocketDir(), server), Net: "unix"}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() }) //nolint:errcheck // nothing to do at cleanup
}
