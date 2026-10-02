package tests

import (
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// list first ends each session idle past CLD_IDLE_DAYS days, as kill ends it, with a note on
// stderr, and shows it ended (decision 46). 0.0001 days is 8.64 s. A terminal attached keeps a
// session, and so does a key: c, detached by tmux's detach-client, which moves neither time, stays
// for the key alone. With 0 list ends none, nor do completion and join -s.
func TestEndsIdleSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "c")
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(3)
	joined := startCld(t, s, "tmux", nil, "join", "-s", "c")
	waitClients(t, s, 2)
	idle := map[string]string{"CLD_IDLE_DAYS": "0.0001"}
	time.Sleep(10 * time.Second)

	checkIdleDaysRefused(t, s)
	checkEndsNone(t, s, idle)

	mark := probes["c"].Mark()
	joined.Keys("z")
	probes["c"].WaitInput(mark, "z")
	s.MustTmux("cld-c", "detach-client", "-s", "=cld-c")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !joined.Running() })
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     ended     -            " + s.Work + "\n" +
		"b     attached  now          " + s.Work + "\n" +
		"c     detached  now          " + s.Work + "\n" +
		"d     attached  now          " + s.Work + "\n"
	checkEndedIdle(t, s.RunCld(idle, "list"), want)
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool {
		return !probes["a"].Alive()
	})
	if _, err := s.Tmux("cld-a", "list-sessions"); err == nil {
		t.Error("session a's server survived")
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b", "cld-c", "cld-d"}) {
		t.Errorf("sessions %q, want [cld-b cld-c cld-d]", sessions)
	}
	// The sweep ends a as kill would: restore leaves it ended, and brings back the others.
	checkMarks(t, s, "after the sweep",
		map[string]bool{"a": false, "b": true, "c": true, "d": true})
}

// checkIdleDaysRefused checks that list and join without -s refuse a CLD_IDLE_DAYS that is no
// number of days, before anything runs.
func checkIdleDaysRefused(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	for _, value := range []string{"x", "-1", "1e3", "30d", " 30", "0x1p3", "inf"} {
		want := "cld: CLD_IDLE_DAYS is not a number of days: '" + value + "'\n"
		for _, command := range []string{"list", "join"} {
			result := s.RunCld(map[string]string{"CLD_IDLE_DAYS": value}, command)
			if result.Code != 1 || result.Stdout != "" || result.Stderr != want {
				t.Errorf("%s, CLD_IDLE_DAYS=%q: exit %d, stdout %q, stderr %q, "+
					"want exit 1, stderr %q",
					command, value, result.Code, result.Stdout, result.Stderr, want)
			}
		}
	}
}

// checkEndsNone checks that list with CLD_IDLE_DAYS 0, and completion and join -s d with idle,
// end none of the sessions a to c.
func checkEndsNone(t *testing.T, s *sandbox.Sandbox, idle map[string]string) {
	t.Helper()
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     detached  now          " + s.Work + "\n" +
		"b     attached  now          " + s.Work + "\n" +
		"c     attached  now          " + s.Work + "\n"
	result := s.RunCld(map[string]string{"CLD_IDLE_DAYS": "0"}, "list")
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("CLD_IDLE_DAYS=0: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	completions := "a\tdetached\nb\tattached\nc\tattached\n:4\n"
	result = s.RunCld(idle, "__complete", "join", "-s", "")
	if result.Code != 0 || result.Stdout != completions {
		t.Errorf("__complete join -s: exit %d, stdout %q, want %q",
			result.Code, result.Stdout, completions)
	}
	startCld(t, s, "tmux", idle, "join", "-s", "d")
	s.WaitProbes(4)
	waitClients(t, s, 3)
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-a", "cld-b", "cld-c", "cld-d"}) {
		t.Fatalf("sessions %q, want [cld-a cld-b cld-c cld-d]", sessions)
	}
}

// checkEndedIdle checks that list printed want, and the note of session a ended, idle for 9 s or
// more.
func checkEndedIdle(t *testing.T, result sandbox.Result, want string) {
	t.Helper()
	ended := regexp.MustCompile(`^cld: ended session 'a', idle for ([0-9]+) seconds\n$`).
		FindStringSubmatch(result.Stderr)
	if result.Code != 0 || result.Stdout != want || ended == nil {
		t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, "+
			"stderr \"cld: ended session 'a', idle for N seconds\", stdout\n%s",
			result.Code, result.Stderr, result.Stdout, want)
		return
	}
	if seconds, err := strconv.Atoi(ended[1]); err != nil || seconds < 9 {
		t.Errorf("session a idle for %d seconds, want 9 or more", seconds)
	}
}
