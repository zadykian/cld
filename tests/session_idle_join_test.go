package tests

import (
	"bytes"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// endedNote matches the note of session 0 ended as idle, on a terminal.
var endedNote = regexp.MustCompile(`cld: ended session '0', idle for [0-9]+ seconds\r?\n`)

// join without -s ends the idle sessions as list does, once it has taken the index (decision
// 46.2). So 0 ends, and join makes 1 rather than take 0's name. join runs with the TMUX of x's
// claude, and keeps x, as idle as 0: ending it would end the claude that ran cld.
func TestJoinEndsIdleSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "0", "x")
	time.Sleep(10 * time.Second)
	term := startCld(t, s, "tmux",
		map[string]string{"CLD_IDLE_DAYS": "0.0001", "TMUX": probes["x"].Env["TMUX"]}, "join")
	sandbox.WaitFor(t, 10*time.Second, "claude 0 to exit", func() bool {
		return !probes["0"].Alive()
	})
	var fresh *sandbox.Probe
	for _, probe := range s.WaitProbes(3) {
		if probe.PID != probes["0"].PID && probe.PID != probes["x"].PID {
			fresh = probe
		}
	}
	if !slices.Equal(fresh.Argv[:2], []string{"--name", "cld-1"}) {
		t.Errorf("join started claude with %q, want --name cld-1", fresh.Argv)
	}
	waitClients(t, s, 1)
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-1", "cld-x"}) || !probes["x"].Alive() {
		t.Errorf("sessions %q, claude x alive: %v; want [cld-1 cld-x], x alive",
			sessions, probes["x"].Alive())
	}
	if !endedNote.Match(term.Output()) {
		t.Errorf("the terminal got no note of the session ended: %q", term.Output())
	}
}

// The kill of an idle session checks again that the session is idle (decision 46.4). A terminal
// that attaches between list's read and the kill keeps it, and list says nothing of it. Its table
// shows the session as it read it.
func TestKeepsAnIdleSessionJoinedMeanwhile(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probe := detachedSessions(t, s, "a")["a"]
	time.Sleep(10 * time.Second)
	kill := holdTmux(t, s, "the kill of idle session a", "*' if -F -t =cld-a: '*")
	kill.start(t)
	env := maps.Clone(kill.env)
	env["CLD_IDLE_DAYS"] = "0.0001"
	list := exec.Command(sandbox.Cld, "list")
	list.Env, list.Dir = s.Environ(env), s.Work
	var stdout, stderr bytes.Buffer
	list.Stdout, list.Stderr = &stdout, &stderr
	if err := list.Start(); err != nil {
		t.Fatal(err)
	}
	kill.held(t)
	term := startCld(t, s, "tmux", nil, "join", "-s", "a")
	waitClients(t, s, 1)
	kill.release(t)
	if err := list.Wait(); err != nil {
		t.Errorf("list: %v, stderr %q", err, stderr.String())
	}
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     detached  now          " + s.Work + "\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Errorf("stderr %q, stdout\n%s\nwant no stderr, stdout\n%s",
			stderr.String(), stdout.String(), want)
	}
	if !probe.Alive() || !term.Running() || !slices.Equal(s.Sessions(), []string{"cld-a"}) {
		t.Errorf("claude alive: %v, terminal attached: %v, sessions %q; "+
			"want session a as it was", probe.Alive(), term.Running(), s.Sessions())
	}
}

// A join that a move runs, without -s, ends the idle sessions as join does, but for the session the
// terminal has just left (decision 51.5). That one, detached by the move, runs on. The terminal's
// variables, CLD_IDLE_DAYS among them, are the join's.
func TestJoinMoveKeepsTheSessionItLeft(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "0")
	term := startCld(t, s, "tmux", map[string]string{"CLD_IDLE_DAYS": "0.0001"},
		"join", "-s", "a")
	waitScreen(t, term, "probe --name cld-a")
	time.Sleep(10 * time.Second)
	result := s.RunCld(paneOf(t, s, "a"), "join")
	if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
		t.Fatalf("join in a's pane: exit %d, stdout %q, stderr %q, want exit 0 and no output",
			result.Code, result.Stdout, result.Stderr)
	}
	waitScreen(t, term, "probe --name cld-1")
	sandbox.WaitFor(t, 10*time.Second, "claude 0 to exit", func() bool {
		return !probes["0"].Alive()
	})
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-1", "cld-a"}) || !claudeOf(t, s, "a").Alive() {
		t.Errorf("sessions %q, want [cld-1 cld-a], claude a running", sessions)
	}
	if !endedNote.Match(term.Output()) {
		t.Errorf("the terminal got no note of the session ended: %q", term.Output())
	}
}

// The list C-q s shows over a session ends no idle session (decision 51.2). tmux runs it with the
// server's environment: here the CLD_IDLE_DAYS of 8.64 s that the terminal's join started it with,
// past which session 0 has been idle.
func TestPopupEndsNoSession(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "0")
	term := startCld(t, s, "tmux", map[string]string{"CLD_IDLE_DAYS": "0.0001"},
		"join", "-s", "a")
	waitScreen(t, term, "probe --name cld-a")
	time.Sleep(10 * time.Second)
	term.Keys("C-q", "s")
	waitScreen(t, term, listHints)
	time.Sleep(500 * time.Millisecond)
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-0", "cld-a"}) || !probes["0"].Alive() {
		t.Errorf("sessions %q with the list over a, want [cld-0 cld-a], claude 0 running",
			sessions)
	}
	if strings.Contains(term.Screen(), "ended") {
		t.Errorf("the list over a shows a session ended:\n%s", term.Screen())
	}
	term.Keys("Escape")
	sandbox.WaitFor(t, 10*time.Second, "the list to close", func() bool {
		return !strings.Contains(term.Screen(), listHints)
	})
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-a"}) {
		t.Errorf("clients attached to %q, want [cld-a]", clients)
	}
}
