package tests

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Where the socket directory ignores case, names that differ only in case share a socket (decision
// 13.6). join, detach and kill refuse A, naming session a, with a's claude running and once a's
// server outlives it; list shows a once. Elsewhere a symbolic link cld-A to a's socket plays such a
// directory, as a casefold tmpfs would (docs/design/findings/tmux-sessions.md).
func TestNamesDifferingInCase(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "a")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	socket := filepath.Join(s.SocketDir(), "cld-A")
	if _, err := os.Lstat(socket); errors.Is(err, os.ErrNotExist) {
		if err := os.Symlink("cld-a", socket); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	checkCaseClash(t, s, "a running")
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     attached  now          " + s.Work + "\n"
	result := s.RunCld(nil, "list")
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	if !probe.Alive() || len(s.Probes()) != 1 {
		t.Fatalf("claude a alive: %v, %d claude processes; want it alive and alone",
			probe.Alive(), len(s.Probes()))
	}

	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-a/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	checkCaseClash(t, s, "a lingering")
	sessions := s.MustTmux("cld-a", "list-sessions", "-F", "#{session_name}")
	if sessions != "side" {
		t.Errorf("sessions on a's server %q, want side", sessions)
	}
}

// checkCaseClash checks that join, whatever its options, detach and kill refuse session A as
// clashing with a, when names the case in a failure.
func checkCaseClash(t *testing.T, s *sandbox.Sandbox, when string) {
	t.Helper()
	const clash = "cld: session name 'A' clashes with session 'a': " +
		"tmux's socket directory ignores case here, so both names reach server cld-a " +
		"(see tmux -L cld-a ls)\n"
	for _, args := range [][]string{
		{"join", "-s", "A"}, {"join", "-s", "A", "--new"}, {"join", "-s", "A", "--resume", "x"},
		{"detach", "-s", "A"}, {"kill", "-s", "A"},
	} {
		result := s.RunCld(nil, args...)
		if result.Code != 1 || result.Stdout != "" || result.Stderr != clash {
			t.Errorf("%s: %s: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
				when, strings.Join(args, " "), result.Code, result.Stdout, result.Stderr, clash)
		}
	}
}
