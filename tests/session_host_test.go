package tests

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Sessions on a host through --host alone (decision 62). The host is the sandbox itself, which the
// fake ssh reaches.

// hostProfile makes s the host of --host: its login shell finds the cld under test and real tmux.
func hostProfile(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	writeProfile(t, s, filepath.Dir(sandbox.Cld)+":"+s.RealTools("tmux"))
}

// hostTable is what list prints of session app-1 on the host, in state, made in the home where
// the fake ssh starts.
func hostTable(s *sandbox.Sandbox, state string) string {
	return "NAME   STATE     LAST ACTIVE  DIRECTORY\napp-1  " + state + "  now          " + s.Home +
		"\n"
}

// join makes the session on the host, in the home ssh starts in, and its claude takes the words
// after "--", which go as __host VALUE. list prints the host's table, detach detaches the
// terminal, and kill ends the session. The host cld's refusal comes back, with its status.
func TestHostSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	hostProfile(t, s)
	term := startCld(t, s, "tmux", nil, "join", "--host", "ws", "-n", "app", "-s", "1", "--",
		"two words")
	claude := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if !slices.Contains(claude.Argv, "two words") {
		t.Errorf("claude's arguments %q, want \"two words\" among them", claude.Argv)
	}
	checkSSH(t, s, hostSSH("ws", true, hostValue("join", "--name=app", "--suffix=1", "--",
		"two words")...))
	checkExit(t, "list", s.RunCld(nil, "list", "--host", "ws"), 0, hostTable(s, "attached"), "")
	checkExit(t, "detach", s.RunCld(nil, "detach", "--host", "ws", "-n", "app", "-s", "1"), 0,
		"", "")
	sandbox.WaitFor(t, 10*time.Second, "join to end", func() bool { return !term.Running() })
	checkExit(t, "list", s.RunCld(nil, "list", "--host", "ws"), 0, hostTable(s, "detached"), "")
	checkExit(t, "kill", s.RunCld(nil, "kill", "--host", "ws", "-n", "app", "-s", "1"), 0, "", "")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool { return !claude.Alive() })
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after kill, want none", sessions)
	}
	checkExit(t, "kill", s.RunCld(nil, "kill", "--host", "ws", "-n", "app", "-s", "1"), 1, "",
		"cld: session 'app-1' has ended; resume it with cld join -n app -s 1\n")
}

// join --resume reaches the host's claude with SESSION as given, though no shell reads it
// unchanged: as __host VALUE. -C names the directory on the host that join makes the session in.
func TestHostResume(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	hostProfile(t, s)
	startCld(t, s, "tmux", nil, "join", "--host", "ws", "-C", s.Work, "--resume", "it's a b")
	claude := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	if !optionFollowedBy(claude.Argv, "--resume", "it's a b") || claude.Cwd != s.Work {
		t.Errorf("claude's arguments %q in %s, want --resume \"it's a b\" in %s", claude.Argv,
			claude.Cwd, s.Work)
	}
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-0"}) {
		t.Errorf("clients on %q, want one on cld-0", clients)
	}
	checkSSH(t, s, hostSSH("ws", true, hostValue("join", "--directory="+s.Work,
		"--resume=it's a b")...))
}
