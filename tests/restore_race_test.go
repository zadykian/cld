package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// Two restores, and a restore and a join, at once: the record's lock and the start mark make one
// session of them (decisions 48.6 and 50.4).

// secondCld is a cld that runs as restore brings session x back: its arguments, and the status
// and output it ends with.
type secondCld struct {
	name           string
	args           []string
	code           int
	stdout, stderr string
}

// restore holds the record's lock for each session it brings back, from the lookup to tmux. A
// second restore, or a join without a terminal, waits while the first restore's tmux is held, then
// finds the session running. TestJoinRacingRestore has a join on a terminal.
func TestRestoreAtOnce(t *testing.T) {
	t.Parallel()
	for _, second := range []secondCld{
		{"restore", []string{"restore"}, 0, "", ""},
		{"join", []string{"join", "-s", "x"}, 1, "",
			"cld: join needs a terminal, and its input is not one\n"},
	} {
		t.Run(second.name, func(t *testing.T) {
			t.Parallel()
			restoreAtOnce(t, second)
		})
	}
}

// restoreAtOnce runs restore, its tmux held, and second, which waits for the lock, and checks
// that they make session x once.
func restoreAtOnce(t *testing.T, second secondCld) {
	t.Helper()
	s := sandbox.New(t)
	writeRestorable(t, s, "x", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"),
		s.Environ(nil))
	tmux := holdTmux(t, s, "restore's tmux making cld-x", "*' new-session -d -s cld-x '*")
	tmux.start(t)
	first := startCldAsync(t, s, s.Work, tmux.env, "restore")
	tmux.held(t)
	other := startCldAsync(t, s, s.Work, tmux.env, second.args...)
	waitForLock(t, s, "the second "+second.name+" to wait for the record's lock")
	tmux.release(t)
	checkExit(t, "first restore", first(), 0, "Restored session 'x' in "+s.Work+"\n", "")
	checkExit(t, second.name, other(), second.code, second.stdout, second.stderr)
	checkMadeOnce(t, s, tmux)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
		t.Errorf("sessions %q, want [cld-x]", sessions)
	}
}

// checkMadeOnce checks that one tmux command made session x, and one claude started, which it
// returns.
func checkMadeOnce(t *testing.T, s *sandbox.Sandbox, tmux heldTmux) *sandbox.Probe {
	t.Helper()
	if begun := tmux.begun(t); begun != 1 {
		t.Errorf("%d tmux commands made cld-x, want 1", begun)
	}
	probe := s.WaitProbes(1)[0]
	time.Sleep(500 * time.Millisecond)
	if probes := s.Probes(); len(probes) != 1 {
		t.Errorf("%d claudes started, want 1", len(probes))
	}
	return probe
}

// A join on a terminal racing restore of an ended session makes one session, which the join ends
// attached to, whichever goes first. restore first holds the lock, and join first leaves a start
// mark, by which restore leaves the session to it, saying nothing.
func TestJoinRacingRestore(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		// held is what the tmux that makes cld-x runs, which the test holds
		held string
		race func(t *testing.T, s *sandbox.Sandbox, tmux heldTmux) terminal.Terminal
	}{
		{"restore first", "*' new-session -d -s cld-x '*", restoreBeforeJoin},
		{"join first", "*' new-session -s cld-x '*", joinBeforeRestore},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			writeRestorable(t, s, "x", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"),
				s.Environ(nil))
			tmux := holdTmux(t, s, test.name+": the tmux making cld-x", test.held)
			tmux.start(t)
			checkJoinedRace(t, s, tmux, test.race(t, s, tmux))
		})
	}
}

// restoreBeforeJoin runs restore, its tmux held until a join on a terminal waits for the lock,
// and returns the join's terminal.
func restoreBeforeJoin(t *testing.T, s *sandbox.Sandbox, tmux heldTmux) terminal.Terminal {
	t.Helper()
	restore := startCldAsync(t, s, s.Work, tmux.env, "restore")
	tmux.held(t)
	term := startCld(t, s, "tmux", tmux.env, "join", "-s", "x")
	waitForLock(t, s, "join to wait for the record's lock")
	tmux.release(t)
	checkExit(t, "restore", restore(), 0, "Restored session 'x' in "+s.Work+"\n", "")
	return term
}

// joinBeforeRestore runs a join on a terminal, its tmux held until restore has returned, and
// returns the join's terminal.
func joinBeforeRestore(t *testing.T, s *sandbox.Sandbox, tmux heldTmux) terminal.Terminal {
	t.Helper()
	term := startCld(t, s, "tmux", tmux.env, "join", "-s", "x")
	tmux.held(t)
	if !exists(companionFile(s, "x", ".start")) {
		t.Error("join left no start mark as it became tmux")
	}
	result := startCldAsync(t, s, s.Work, tmux.env, "restore")()
	tmux.release(t)
	checkExit(t, "restore", result, 0, "", "")
	return term
}

// checkJoinedRace checks that the race made session x once, with the join on term attached, its
// claude resuming x's conversation, and the start mark gone.
func checkJoinedRace(t *testing.T, s *sandbox.Sandbox, tmux heldTmux, term terminal.Terminal) {
	t.Helper()
	waitClients(t, s, 1)
	if !term.Running() {
		t.Errorf("join is not attached:\n%s", term.Screen())
	}
	probe := checkMadeOnce(t, s, tmux)
	// The hooks in the settings name the tmux that holds the test's commands.
	held := filepath.Join(strings.Split(tmux.env["PATH"], string(os.PathListSeparator))[0], "tmux")
	want := []string{"--name", "cld-x", "--settings",
		settings(s, held, sandbox.RealGit, "cld-x", s.Work, false), "--resume", firstID}
	if !slices.Equal(probe.Argv, want) {
		t.Errorf("claude arguments %q, want %q", probe.Argv, want)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
		t.Errorf("sessions %q, want [cld-x]", sessions)
	}
	// The run-shell that makes the run mark has removed the start mark before.
	checkMarks(t, s, "after the race", map[string]bool{"x": true})
	if exists(companionFile(s, "x", ".start")) {
		t.Error("the start mark is left once tmux has made the session")
	}
}
