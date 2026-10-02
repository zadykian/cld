package tests

import (
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

// join -s completes the sessions list shows that start with what was typed, each described by its
// state, and offers no file names (decision 17.2). It starts nothing, and leaves out the sessions
// cld did not start; detach -s offers those that run. Here a name is its SUFFIX, and none has a
// NAME for -n (see TestCompleteSuffixes). Nothing else completes (decision 17.3).
func TestCompleteNames(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCld(nil, "__complete", "join", "-s", "")
	if result.Code != 0 || result.Stdout != ":4\n" {
		t.Errorf("without a server: exit %d, stdout %q, want exit 0, stdout %q",
			result.Code, result.Stdout, ":4\n")
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after completing without a server, want none", sessions)
	}
	startCompletedSessions(t, s)
	probes := len(s.Probes())

	listed := []string{"bad", "cafe", "gone", "rev", "review"}
	result = s.RunCld(nil, "list")
	var shown []string
	for _, line := range strings.Split(strings.TrimSuffix(result.Stdout, "\n"), "\n")[1:] {
		shown = append(shown, strings.Fields(line)[0])
	}
	if result.Code != 0 || !slices.Equal(shown, listed) {
		t.Errorf("list: exit %d, names %q, want %q:\n%s", result.Code, shown, listed, result.Stdout)
	}

	for _, test := range append(completedNames(s.Work), completeNothing...) {
		result := s.RunCld(test.env, test.args...)
		if result.Code != 0 || result.Stdout != test.want {
			t.Errorf("cld %q, %v: exit %d, stdout\n%s\nwant\n%s",
				test.args, test.env, result.Code, result.Stdout, test.want)
		}
	}
	if count := len(s.Probes()); count != probes {
		t.Errorf("%d claude processes after completing, want %d", count, probes)
	}
}

// startCompletedSessions starts the sessions TestCompleteNames completes: rev attached, review
// detached, bad's claude exited, cafe renamed by hand and gone's server killed, which both end. It
// adds four that are none of cld's. claude makes one, and one is made by hand beside review; one
// is on a server cld did not start, and one on cld 0.3.0's shared server.
func startCompletedSessions(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	startCld(t, s, "tmux", nil, "join", "-s", "rev")
	rev := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for i, name := range []string{"review", "cafe", "bad", "gone"} {
		term := startCld(t, s, "tmux", nil, "join", "-s", name)
		s.WaitProbes(2 + i)
		waitClients(t, s, 2)
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld "+name+" to detach", func() bool {
			return !term.Running()
		})
	}
	for _, probe := range s.Probes() {
		if probe.Argv[1] == "cld-bad" {
			probe.Send("exit 1")
		}
	}
	sandbox.WaitFor(t, 10*time.Second, "claude bad to exit", func() bool {
		return s.Format("cld-bad", "#{pane_dead}") == "1"
	})
	// Renamed by hand, session cafe is no longer cld-cafe, which its server runs without.
	s.MustTmux("cld-cafe", "rename-session", "-t", "=cld-cafe", "cld-café")
	sigkillServer(t, s, "cld-gone")
	rev.Send("tmux new-session -d -s cld-inside sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-rev/cld-inside")
	})
	s.MustTmux("cld-review", "new-session", "-d", "-s", "cld-by-hand", "sleep", "600")
	s.MustTmux("cld-own", "-f", "/dev/null", "new-session", "-d", "-s", "cld-own", "sleep", "600")
	s.MustTmux("cld", "-f", "/dev/null", "new-session", "-d", "-s", "cld-old", "sleep", "600")
	want := []string{"cld-bad", "cld-cafe/cld-café", "cld-own", "cld-rev", "cld-rev/cld-inside",
		"cld-review", "cld-review/cld-by-hand"}
	if sessions := s.Sessions(); !slices.Equal(sessions, want) {
		t.Fatalf("sessions %q, want %q", sessions, want)
	}
}

// sigkillServer kills the server with SIGKILL, which leaves its socket behind.
func sigkillServer(t *testing.T, s *sandbox.Sandbox, server string) {
	t.Helper()
	pid, err := strconv.Atoi(s.MustTmux(server, "list-sessions", "-F", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 10*time.Second, "server "+server+" to die", func() bool {
		_, err := s.Tmux(server, "list-sessions")
		return err != nil
	})
	if _, err := os.Stat(filepath.Join(s.SocketDir(), server)); err != nil {
		t.Fatalf("the dead server's socket: %v", err)
	}
}

// completeCase is a completion of TestCompleteNames, with the variables env, and what it offers.
type completeCase struct {
	args []string
	env  map[string]string
	want string
}

// completedNames are the completions of the sessions' names, which ended in work.
func completedNames(work string) []completeCase {
	cafe, gone := "cafe\tended in "+work, "gone\tended in "+work
	names := []string{"bad\texited", cafe, gone, "rev\tattached", "review\tdetached"}
	running := []string{"bad\texited", "rev\tattached", "review\tdetached"}
	all, revs := offered(names...), offered("rev\tattached", "review\tdetached")
	notUTF8 := map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "C"}
	return []completeCase{
		{[]string{"__complete", "join", "-s", ""}, nil, all},
		{[]string{"__complete", "join", "--suffix", ""}, nil, all},
		{[]string{"__complete", "join", "--suffix="}, nil, all},
		{[]string{"__complete", "join", "-s", "re"}, nil, revs},
		{[]string{"__complete", "join", "-s", "revi"}, nil, offered("review\tdetached")},
		{[]string{"__complete", "join", "-s", "x"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "caf"}, nil, offered(cafe)},
		{[]string{"__complete", "join", "-s", "g"}, nil, offered(gone)},
		{[]string{"__complete", "join", "-s", "in"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "b"}, nil, offered("bad\texited")},
		{[]string{"__complete", "join", "-s", "o"}, nil, offered()},
		{[]string{"__complete", "join", "-s", "ow"}, nil, offered()},
		// pflag's -s=SUFFIX; in -sSUFFIX cobra takes the word for options, and finds none.
		{[]string{"__complete", "join", "-s=re"}, nil, revs},
		{[]string{"__complete", "join", "-sre"}, nil, offered()},
		{[]string{"__completeNoDesc", "join", "-s", ""}, nil, offered(bareNames(names)...)},
		{[]string{"__complete", "join", "-s", ""},
			map[string]string{"CLD_COMPLETION_DESCRIPTIONS": "0"}, offered(bareNames(names)...)},
		{[]string{"__complete", "join", "-s", ""}, notUTF8, all},
		{[]string{"__complete", "join", "--resume", "x", "-s", "g"}, nil, offered(gone)},
		{[]string{"__complete", "detach", "-s", ""}, nil, offered(running...)},
		{[]string{"__complete", "detach", "-s", "g"}, nil, offered()},
		{[]string{"__complete", "detach", "--suffix=re"}, nil, revs},
		{[]string{"__completeNoDesc", "detach", "-s", ""}, nil, offered(bareNames(running)...)},
	}
}

// completeNothing are the completions that offer nothing: no name has a NAME of NAME-SUFFIX for -n,
// and no other argument completes.
var completeNothing = []completeCase{
	{[]string{"__complete", "join", "-n", ""}, nil, offered()},
	{[]string{"__complete", "detach", "-n", ""}, nil, offered()},
	{[]string{"__complete", "join", "--resume", ""}, nil, offered()},
	{[]string{"__complete", "join", "--resume", "g"}, nil, offered()},
	{[]string{"__complete", "join", "-s", "rev", ""}, nil, offered()},
	{[]string{"__complete", "join", "--", ""}, nil, offered()},
	{[]string{"__complete", "join", "--", "--"}, nil, offered()},
	{[]string{"__complete", "join", "-s", "rev", "--", "-"}, nil, offered()},
	{[]string{"__complete", "join", "--", "-s", ""}, nil, offered()},
	{[]string{"__complete", "kill", "-s", ""}, nil, offered()},
	{[]string{"__complete", "join", ""}, nil, offered()},
	{[]string{"__complete", "detach", ""}, nil, offered()},
	{[]string{"__complete", "list", ""}, nil, offered()},
	// cobra answers these with ShellCompDirectiveDefault, and the shell would offer files.
	{[]string{"__complete", "joni", "-n", ""}, nil, offered()},
	{[]string{"__complete", "join", "-x", "-n", ""}, nil, offered()},
	{[]string{"__complete", "list", "-n", ""}, nil, offered()},
}

// offered is what __complete prints for names, each one line, and then the directive.
func offered(names ...string) string {
	return strings.Join(append(names, ":4"), "\n") + "\n"
}

// bareNames are names without their descriptions.
func bareNames(names []string) []string {
	var bare []string
	for _, name := range names {
		bare = append(bare, strings.Split(name, "\t")[0])
	}
	return bare
}
