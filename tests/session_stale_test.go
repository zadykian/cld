package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A server killed with SIGKILL leaves its socket behind (decision 13.1). list passes over it,
// showing its session ended, beside the others, and detach and kill point at join. join brings the
// session back on a fresh server, by its name, as its entry has no ID yet.
func TestStaleSocket(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	a := startCld(t, s, "tmux", nil, "join", "-s", "a")
	s.WaitProbes(1)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	pid, err := strconv.Atoi(s.MustTmux("cld-a", "list-sessions", "-F", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld a to lose its server", func() bool {
		return !a.Running()
	})
	if _, err := os.Stat(filepath.Join(s.SocketDir(), "cld-a")); err != nil {
		t.Fatalf("the dead server's socket: %v", err)
	}

	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     ended     -            " + s.Work + "\n" +
		"b     attached  now          " + s.Work + "\n"
	result := s.RunCld(nil, "list")
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	for _, test := range []struct{ command, want string }{
		{"kill", "cld: session 'a' has ended; resume it with cld join -s a\n"},
		{"detach", "cld: session 'a' has ended; resume it with cld join -s a\n"},
	} {
		result := s.RunCld(nil, test.command, "-s", "a")
		if result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%s -n a: exit %d, stderr %q, want exit 1, stderr %q",
				test.command, result.Code, result.Stderr, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "a")
	resumed := []string{"--name", "cld-a", "--settings", sessionSettings(s, "cld-a", s.Work),
		"--resume", "cld-a"}
	given := func(p *sandbox.Probe) bool { return slices.Equal(p.Argv, resumed) }
	if probes := s.WaitProbes(3); !slices.ContainsFunc(probes, given) {
		t.Errorf("join -s a started no claude with %q", resumed)
	}
	waitClients(t, s, 2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

// A stale socket costs no tmux: cld connects first, and runs tmux only where a server takes the
// connection (decision 38). list asks only the live servers, and join, detach and kill of a stale
// name ask none. join without -s asks from the highest index down until a server runs. The sockets
// stay. A tmux first on the PATH writes down what it runs.
func TestStaleSocketsRunNoTmux(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	logged, asked := loggedTmux(t, s)
	running, stale := staleSockets(t, s)
	servers := append(slices.Clone(running), "cld-work-11")

	slices.Sort(running)
	want := "NAME     STATE     LAST ACTIVE  DIRECTORY\n"
	for _, name := range running {
		want += fmt.Sprintf("%-9sdetached  now          %s\n",
			strings.TrimPrefix(name, "cld-"), s.Work)
	}
	result := s.RunCld(logged, "list")
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	got := asked()
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(servers))) {
		t.Errorf("list asked %q, want %q", got, servers)
	}
	checkStaleNamesAskNone(t, s, logged, asked)

	startCld(t, s, "tmux", logged, "join", "-n", "work")
	if name := s.WaitProbes(1)[0].Argv[1]; name != "cld-work-12" {
		t.Errorf("claude named %s, want cld-work-12", name)
	}
	// The index's lookup comes first, then the sweep's read of the sessions, at once.
	got = asked()
	if len(got) == 0 || got[0] != "cld-work-11" ||
		!slices.Equal(slices.Sorted(slices.Values(got[1:])), slices.Sorted(slices.Values(servers))) {
		t.Errorf("join asked %q, want cld-work-11, then %q", got, servers)
	}
	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(s.SocketDir(), name)); err != nil {
			t.Errorf("stale socket %s: %v", name, err)
		}
	}
}

// staleSockets starts sessions cld-work-0 to 10, each on its server, and a server cld-work-11 that
// outlives its session. It marks them all as cld marks its own (see TestLeavesAForeignServerAlone).
// It leaves stale sockets from cld-work-12 to 29 and cld-work-99, and returns sessions and sockets.
func staleSockets(t *testing.T, s *sandbox.Sandbox) (running, stale []string) {
	t.Helper()
	for i := range 11 {
		name := "cld-work-" + strconv.Itoa(i)
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
			"new-session", "-d", "-s", name, "-c", s.Work, "sleep", "600")
		running = append(running, name)
	}
	s.MustTmux("cld-work-11", "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
		"new-session", "-d", "-s", "other", "sleep", "600")
	for i := 12; i < 30; i++ {
		stale = append(stale, "cld-work-"+strconv.Itoa(i))
	}
	stale = append(stale, "cld-work-99")
	for _, name := range stale {
		staleSocket(t, s, name)
	}
	return running, stale
}

// checkStaleNamesAskNone checks that join, detach and kill of a stale socket's name ask no server.
// join, which would create the session there, has no terminal here.
func checkStaleNamesAskNone(
	t *testing.T, s *sandbox.Sandbox, logged map[string]string, asked func() []string,
) {
	t.Helper()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "-s", "work-20"}, "cld: join needs a terminal, and its input is not one\n"},
		{[]string{"kill", "-s", "work-99"}, "cld: no session 'work-99' (see cld list)\n"},
		{[]string{"detach", "-s", "work-25"}, "cld: no session 'work-25' (see cld list)\n"},
		{[]string{"join", "-s", "work-50"}, "cld: join needs a terminal, and its input is not one\n"},
	} {
		result := s.RunCld(logged, test.args...)
		if result.Code != 1 || result.Stdout != "" || result.Stderr != test.want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
				strings.Join(test.args, " "), result.Code, result.Stdout, result.Stderr, test.want)
		}
		if got := asked(); len(got) != 0 {
			t.Errorf("%s asked %q, want none", strings.Join(test.args, " "), got)
		}
	}
}
