package tests

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Joins at once, which the record's lock and the start mark keep apart (decisions 40.6 and 50.4).

// Two joins without -s at once take two indexes, as each holds the record's lock from the name to
// tmux. The first is held in its lookup of cld-5, whose socket takes connections until the lookup
// goes on, so that cld runs tmux there (decision 38.1). Without the lock, the second would be held
// there too, and take the same name.
func TestJoinNextAtOnce(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	if err := os.MkdirAll(s.SocketDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	socket := &net.UnixAddr{Name: filepath.Join(s.SocketDir(), "cld-5"), Net: "unix"}
	listener, err := net.ListenUnix("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() }) //nolint:errcheck // the test closes it unless it fails
	lookup := holdLookup(t, s, "5")
	startCld(t, s, "tmux", lookup.env, "join")
	lookup.held(t)
	startCld(t, s, "tmux", lookup.env, "join")
	time.Sleep(time.Second)
	// Closing the listener removes its socket, as a server that exits leaves none.
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	lookup.release(t)
	s.WaitProbes(2)
	waitClients(t, s, 2)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-0", "cld-1"}) {
		t.Errorf("sessions %q, want [cld-0 cld-1]", sessions)
	}
}

// joinAtOnce is a case of TestJoinOneSessionAtOnce: the first join's arguments, which make
// session cld-suffix, and whether the record has that session's entry, ended, before the joins.
type joinAtOnce struct {
	name   string
	ended  bool
	first  []string
	suffix string
}

// joinsAtOnce are TestJoinOneSessionAtOnce's cases. In "next", the first join makes session 0,
// whose entry the second, join -s 0, would otherwise resume.
var joinsAtOnce = []joinAtOnce{
	{"unknown", false, []string{"join", "-s", "x"}, "x"},
	{"ended", true, []string{"join", "-s", "x"}, "x"},
	{"next", false, []string{"join"}, "0"},
}

// Two joins of one session at once, on two terminals, make it once, and both attach (decision
// 50.4). The lock goes with the first's exec, before its tmux, held here, makes the session. The
// second finds the first's start mark, waits, and attaches once the session is made.
func TestJoinOneSessionAtOnce(t *testing.T) {
	t.Parallel()
	for _, test := range joinsAtOnce {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			joinOneSessionAtOnce(t, test)
		})
	}
}

// joinOneSessionAtOnce runs test's two joins, the first's tmux held, and checks that both attach
// to the session they name.
func joinOneSessionAtOnce(t *testing.T, test joinAtOnce) {
	t.Helper()
	s := sandbox.New(t)
	if test.ended {
		writeEntry(t, s, test.suffix, s.Work, firstID)
	}
	session := "cld-" + test.suffix
	tmux := holdTmux(t, s, "the first join's tmux making "+session,
		"*' new-session -s "+session+" '*")
	tmux.start(t)
	first := startCld(t, s, "tmux", tmux.env, test.first...)
	tmux.held(t)
	second := startCld(t, s, "tmux", tmux.env, "join", "-s", test.suffix)
	time.Sleep(time.Second)
	if begun := tmux.begun(t); begun != 1 {
		t.Errorf("%d tmux commands began to make %s while the first was held, want 1",
			begun, session)
	}
	tmux.release(t)
	waitClients(t, s, 2)
	if clients := s.Clients(); !slices.Equal(clients, []string{session, session}) {
		t.Errorf("clients attached to %q, want both to %s", clients, session)
	}
	if !first.Running() || !second.Running() {
		t.Errorf("first attached: %v, second attached: %v; want both",
			first.Running(), second.Running())
	}
	checkOneClaude(t, s, tmux, test)
	waitScreen(t, second, "probe --name "+session)
}

// checkOneClaude checks that one claude started, as test's first join starts it with the tmux
// held, and that s has its session alone.
func checkOneClaude(t *testing.T, s *sandbox.Sandbox, tmux heldTmux, test joinAtOnce) {
	t.Helper()
	session := "cld-" + test.suffix
	probe := s.WaitProbes(1)[0]
	time.Sleep(500 * time.Millisecond)
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want 1", len(probes))
	}
	var resume []string
	if test.ended {
		resume = []string{"--resume", firstID}
	}
	// The hooks in the settings name the tmux that holds the test's commands.
	held := filepath.Join(strings.Split(tmux.env["PATH"], string(os.PathListSeparator))[0], "tmux")
	want := append([]string{"--name", session, "--settings",
		settings(s, held, sandbox.RealGit, session, s.Work, false)}, resume...)
	if !slices.Equal(probe.Argv, want) {
		t.Errorf("claude arguments %q, want %q", probe.Argv, want)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{session}) {
		t.Errorf("sessions %q, want [%s]", sessions, session)
	}
}

// A join that looks the session up while another cld's tmux makes it attaches once the session
// is there. That holds where its lookup answers after that tmux has removed the start mark too,
// which join reads before the lookup (decision 50.4). The test plays the other cld.
func TestJoinAsTmuxMakesTheSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "x", s.Work, "")
	s.MustTmux("cld-x", "start-server", ";", "set", "-s", "exit-empty", "off",
		";", "set", "-s", "@cld", "1")
	lookup := holdTmuxOutput(t, s, "the join's lookup of cld-x", lookupPattern("x"))
	lookup.start(t)
	mark := companionFile(s, "x", ".start")
	s.WriteFile(mark, strconv.Itoa(os.Getpid())+"\n")
	term := startCld(t, s, "tmux", lookup.env, "join", "-s", "x")
	lookup.held(t)
	s.MustTmux("cld-x", "new-session", "-d", "-s", "cld-x", "sleep", "600")
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	lookup.release(t)
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-x"}) {
		t.Errorf("clients attached to %q, want [cld-x]", clients)
	}
	if !term.Running() {
		t.Errorf("join ended, screen:\n%s", term.Screen())
	}
	if probes := s.Probes(); len(probes) != 0 {
		t.Errorf("%d claudes started, want none", len(probes))
	}
}

// A start mark that names no cld starting the session is none, and join does not wait for it
// (decision 50.4). Such are a mark older than 10 s, and that of a join whose tmux failed for a
// TERM tmux does not know. join brings the one session back, and makes the other, at once.
func TestJoinPassesOverAStaleStartMark(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	unknown := map[string]string{"TERM": "cld-no-such-terminal"}
	if result := s.RunCldOnTerminal(unknown, "join", "-s", "y"); result.Code == 0 {
		t.Errorf("join with TERM unknown to tmux: exit 0, stderr %q, want tmux to fail", result.Stderr)
	}
	if !exists(companionFile(s, "y", ".start")) {
		t.Error("no start mark left by the join whose tmux failed")
	}
	old := companionFile(s, "w", ".start")
	s.WriteFile(old, strconv.Itoa(os.Getpid())+"\n")
	age(t, 11*time.Second, old)
	for i, name := range []string{"y", "w"} {
		began := time.Now()
		startCld(t, s, "tmux", nil, "join", "-s", name)
		waitClients(t, s, i+1)
		if took := time.Since(began); took > 7*time.Second {
			t.Errorf("join -s %s attached %v after it started, "+
				"want it not to wait for the start mark", name, took.Round(time.Millisecond))
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-w", "cld-y"}) {
		t.Errorf("sessions %q, want [cld-w cld-y]", sessions)
	}
	if probes := s.Probes(); len(probes) != 2 {
		t.Errorf("%d claudes started, want 2", len(probes))
	}
}
