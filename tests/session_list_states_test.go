package tests

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listJoinAttached checks that Enter on an attached row joins beside the other terminal, as cld
// join does (decision 14.5). The footer is the same.
func listJoinAttached(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	other := startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 1)
	term := startCld(t, s, "tmux", nil, "list")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work,
		"  b     attached  now          "+s.Work,
		"",
		listHints)
	term.Keys("Down")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     attached  now          "+s.Work,
		"",
		listHints)
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	waitClients(t, s, 2)
	clients := s.Clients()
	if !slices.Equal(clients, []string{"cld-b", "cld-b"}) || !other.Running() {
		t.Errorf("clients attached to %q, want two, to cld-b", clients)
	}
}

// listJoinExited checks that Enter on an exited row joins, and the terminal shows claude's last
// words and the hint, as cld join does.
func listJoinExited(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	probes["b"].Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool {
		return s.Format("cld-b", "#{pane_dead}") == "1"
	})
	term := startCld(t, s, "tmux", nil, "list")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work,
		"  b     exited    now          "+s.Work,
		"",
		listHints)
	term.Keys("Down", "Enter")
	hint := "claude exited with status 1: cld kill -s b ends the session, " +
		"C-q d or cld detach -s b detaches"
	waitScreen(t, term, hint)
	if screen := term.Screen(); !strings.HasSuffix(strings.TrimRight(screen, " \n"), "\n"+hint) {
		t.Errorf("the hint is not on the message line:\n%s", screen)
	}
}

// listJoinExitedAttached checks that a row reads exited once claude has, a terminal attached or
// not, and that Enter joins beside that terminal all the same.
func listJoinExitedAttached(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	other := startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 1)
	for _, probe := range s.Probes() {
		if probe.Argv[1] == "cld-b" {
			probe.Send("exit 1")
		}
	}
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool {
		return s.Format("cld-b", "#{pane_dead}") == "1"
	})
	term := startCld(t, s, "tmux", nil, "list")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work,
		"  b     exited    now          "+s.Work,
		"",
		listHints)
	term.Keys("Down")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     exited    now          "+s.Work,
		"",
		listHints)
	term.Keys("Enter")
	waitScreen(t, term, "claude exited with status 1")
	waitClients(t, s, 2)
	clients := s.Clients()
	if !slices.Equal(clients, []string{"cld-b", "cld-b"}) || !other.Running() {
		t.Errorf("clients attached to %q, want two, to cld-b", clients)
	}
}

// listJoinEnded checks that Enter on a session ended meanwhile, killed elsewhere, brings it back as
// cld join does (decision 50.8). claude resumes its conversation by the session's name, as its
// entry has no ID.
func listJoinEnded(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	term.Keys("Down", "Enter")
	waitScreen(t, term, "probe --name cld-b")
	waitClients(t, s, 1)
	resumed := []string{"--name", "cld-b", "--settings", sessionSettings(s, "cld-b", s.Work),
		"--resume", "cld-b"}
	given := func(p *sandbox.Probe) bool { return slices.Equal(p.Argv, resumed) }
	if !slices.ContainsFunc(s.Probes(), given) {
		t.Errorf("no claude started with %q", resumed)
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
}

// listJoinLingering checks that Enter leaves a session whose server outlived it unjoined, as cld
// join refuses it, and shows it as ended.
func listJoinLingering(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b", "c")
	probes["b"].Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-b/side")
	})
	term := startCld(t, s, "tmux", nil, "list")
	waitScreen(t, term, listHints)
	probes["b"].Send("exit")
	sandbox.WaitFor(t, 10*time.Second, "b's session to end", func() bool {
		return !slices.Contains(s.Sessions(), "cld-b")
	})
	term.Keys("Down", "Enter")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     ended     -            "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		"session 'b' has ended, but its tmux server still runs")
	if clients := s.Clients(); len(clients) != 0 {
		t.Errorf("clients attached to %q, want none", clients)
	}
}

// A session cld join --resume made is like any other in the list (decision 16.9). Enter joins it,
// and Ctrl+X twice kills it with its server, leaving it ended. After such a kill, by mistake say,
// cld join -n NAME -s SUFFIX brings its conversation back in a new session.
func TestListResumedSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	resumed := startCld(t, s, "tmux", nil, "join", "-s", "b", "--resume", "cld-b")
	sandbox.WaitFor(t, 10*time.Second, "a terminal attached to cld-b", func() bool {
		return slices.Contains(s.Clients(), "cld-b")
	})
	resumed.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !resumed.Running() })
	argv := []string{"--name", "cld-b", "--settings", sessionSettings(s, "cld-b", s.Work),
		"--resume", "cld-b"}
	var b *sandbox.Probe
	sandbox.WaitFor(t, 10*time.Second, "claude b to start", func() bool {
		b = resumedClaude(s, argv, nil)
		return b != nil
	})
	joinResumedFromList(t, s)
	killResumedFromList(t, s, b)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	sandbox.WaitFor(t, 10*time.Second, "claude b to resume again", func() bool {
		return resumedClaude(s, argv, b) != nil
	})
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
}

// resumedClaude is the claude started with argv but other, or nil where there is none.
func resumedClaude(s *sandbox.Sandbox, argv []string, other *sandbox.Probe) *sandbox.Probe {
	for _, probe := range s.Probes() {
		if slices.Equal(probe.Argv, argv) && (other == nil || probe.PID != other.PID) {
			return probe
		}
	}
	return nil
}

// resumedRows are the list's header and its rows a and b, detached in work, the session selected.
func resumedRows(work, selected string) []string {
	lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
	for _, name := range []string{"a", "b"} {
		mark := " "
		if name == selected {
			mark = ">"
		}
		lines = append(lines, mark+" "+name+"     detached  now          "+work)
	}
	return append(lines, "")
}

// joinResumedFromList joins session b from the list, and detaches.
func joinResumedFromList(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(resumedRows(s.Work, "a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(resumedRows(s.Work, "b"), listHints)...)
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("clients attached to %q, want one, to cld-b", clients)
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("join: exit %s, want 0", code)
	}
}

// killResumedFromList kills session b, whose claude is b, from the list, and leaves the list.
func killResumedFromList(t *testing.T, s *sandbox.Sandbox, b *sandbox.Probe) {
	t.Helper()
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(resumedRows(s.Work, "a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(resumedRows(s.Work, "b"), listHints)...)
	armThen(t, term, func() {
		waitLines(t, term, append(resumedRows(s.Work, "b"), killArmed)...)
	}, "C-x")
	waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work, "> b     ended     -            "+s.Work, "",
		endedHints)
	sandbox.WaitFor(t, 10*time.Second, "claude b to exit", func() bool { return !b.Alive() })
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q after the kill, want [cld-a]", sessions)
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("kill: exit %s, want 0", code)
	}
}
