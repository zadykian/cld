package tests

import (
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld sees only its own session on each server (decision 13): a session claude's tmux makes there
// has another name, even one like cld's. list leaves it out, detach and kill find none, and join
// makes that name a session of its own. cld finds cld-a beside cld-a-x by its whole name, and kill
// ends the others with the server.
func TestSeesOnlyItsOwnSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "a")
	makeSessionsInside(t, s)

	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     attached  now          " + s.Work + "\n"
	result := s.RunCld(nil, "list")
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	lost := lostRefusal("a", "--resume", "-s a")
	result = s.RunCld(nil, "join", "-s", "a", "--resume", "cld-a")
	if result.Code != 1 || result.Stdout != "" || result.Stderr != lost {
		t.Errorf("join -s a --resume cld-a: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, lost)
	}
	second := startCld(t, s, "tmux", nil, "join", "-s", "a")
	waitClients(t, s, 2)
	waitScreen(t, second, "probe --name cld-a")
	checkNamesInside(t, s)

	result = s.RunCld(nil, "kill", "-s", "a")
	if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Errorf("kill -n a: exit %d, stdout %q, stderr %q, want exit 0 and no output",
			result.Code, result.Stdout, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "the clds attached to a to return", func() bool {
		return !first.Running() && !second.Running()
	})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a-x", "cld-inside"}) {
		t.Errorf("sessions %q after kill -n a, want [cld-a-x cld-inside]", sessions)
	}
}

// makeSessionsInside has session a's claude make sessions cld-inside and cld-a-x on its server, as
// a bare tmux would, and a session whose program fails.
func makeSessionsInside(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	for _, session := range []string{"cld-inside", "cld-a-x"} {
		probe.Send("tmux new-session -d -s " + session + " sleep 600")
		sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make "+session, func() bool {
			return slices.Contains(s.Sessions(), "cld-a/"+session)
		})
	}
	// As claude's tmux would make it, but made once this returns. What cld sets for a failed
	// claude stays on claude's pane, so this session closes, as tmux would close it.
	s.MustTmux("cld-a", "new-session", "-d", "-s", "cld-failing", "false")
	sandbox.WaitFor(t, 10*time.Second, "the failing session to close", func() bool {
		return !slices.Contains(s.Sessions(), "cld-a/cld-failing")
	})
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-a", "cld-a/cld-a-x", "cld-a/cld-inside"}) {
		t.Fatalf("sessions %q, want [cld-a cld-a/cld-a-x cld-a/cld-inside]", sessions)
	}
}

// checkNamesInside checks that kill and detach find no session inside, and that join makes
// sessions inside and a-x, --resume too, each on a server of its own.
func checkNamesInside(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	for _, test := range []struct{ command, want string }{
		{"kill", "cld: no session 'inside' (see cld list)\n"},
		{"detach", "cld: no session 'inside' (see cld list)\n"},
	} {
		result := s.RunCld(nil, test.command, "-s", "inside")
		if result.Code != 1 || result.Stderr != test.want {
			t.Errorf("%s -n inside: exit %d, stderr %q, want exit 1, stderr %q",
				test.command, result.Code, result.Stderr, test.want)
		}
	}
	startCld(t, s, "tmux", nil, "join", "-s", "inside")
	named := func(p *sandbox.Probe) bool { return p.Argv[1] == "cld-inside" }
	if probes := s.WaitProbes(2); !slices.ContainsFunc(probes, named) {
		t.Errorf("join -s inside started no claude named cld-inside")
	}
	startCld(t, s, "tmux", nil, "join", "-s", "a-x", "--resume", "cld-a-x")
	resumed := []string{"--name", "cld-a-x", "--settings", sessionSettings(s, "cld-a-x", s.Work),
		"--resume", "cld-a-x"}
	given := func(p *sandbox.Probe) bool { return slices.Equal(p.Argv, resumed) }
	if probes := s.WaitProbes(3); !slices.ContainsFunc(probes, given) {
		t.Errorf("join -s a-x --resume cld-a-x started no claude with %q", resumed)
	}
	sessions := s.Sessions()
	want := []string{"cld-a", "cld-a-x", "cld-a/cld-a-x", "cld-a/cld-inside", "cld-inside"}
	if !slices.Equal(sessions, want) {
		t.Errorf("sessions %q, want [cld-a cld-a-x cld-a/cld-a-x cld-a/cld-inside cld-inside]",
			sessions)
	}
}
