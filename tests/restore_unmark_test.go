package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The run mark's rm in the tmux command that ends a session, held as the session holds its name
// (decision 48.1).

// holdRm puts an rm first on the PATH of the environment it returns, for the servers cld starts
// with it and their run-shell. It holds the removal of the file mark from start until release,
// as holdTmux holds tmux, then runs the tests' rm.
func holdRm(t *testing.T, s *sandbox.Sandbox, what, mark string) heldTmux {
	t.Helper()
	rm, err := exec.LookPath("rm")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(s.Root, "hold.") //nolint:usetesting // cld's servers outlive t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	hold := filepath.Join(dir, "hold")
	t.Cleanup(func() { _ = os.Remove(hold) }) //nolint:errcheck // the test may have released it
	s.WriteProgram(filepath.Join(dir, "rm"), "#!/bin/sh\ncase \"$*\" in *'"+mark+"'*)\n"+
		"\techo $$ >>'"+hold+".begun'\n"+
		"\tif [ -e '"+hold+"' ]; then echo $$ >'"+hold+".held'; fi\n"+
		"\twhile [ -e '"+hold+"' ]; do sleep 0.05; done ;;\n"+
		"esac\nexec '"+rm+"' \"$@\"\n", 0o755)
	path := dir + string(os.PathListSeparator) + s.Env["PATH"]
	return heldTmux{hold: hold, what: what, env: map[string]string{"PATH": path}}
}

// kill and the idle sweep remove the run mark before kill-session, in the command that ends the
// session, so the session holds its name meanwhile. With the rm held, a session made under the
// name is refused, and a terminal that attaches keeps the session, without its mark.
func TestUnmarkBeforeTheKill(t *testing.T) {
	t.Parallel()
	t.Run("kill", testUnmarkBeforeKill)
	t.Run("idle", testUnmarkBeforeSweep)
}

// testUnmarkBeforeKill is TestUnmarkBeforeTheKill for kill.
func testUnmarkBeforeKill(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	rm := holdRm(t, s, "the kill's rm of x's run mark", companionFile(s, "x", ".run"))
	term := startCld(t, s, "tmux", rm.env, "join", "-s", "x")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	rm.start(t)
	kill := startCldAsync(t, s, s.Work, nil, "kill", "-s", "x")
	rm.held(t)
	_, err := s.Tmux("cld-x", "new-session", "-d", "-s", "cld-x")
	if err == nil || !strings.Contains(err.Error(), "duplicate session: cld-x") {
		t.Errorf("new-session of cld-x as the kill removed its run mark: %v, "+
			"want duplicate session", err)
	}
	rm.release(t)
	checkExit(t, "kill", kill(), 0, "", "")
	sandbox.WaitFor(t, 10*time.Second, "cld to return", func() bool { return !term.Running() })
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q after the kill, want none", sessions)
	}
	checkMarks(t, s, "after the kill", map[string]bool{"x": false})
}

// testUnmarkBeforeSweep is TestUnmarkBeforeTheKill for the sweep of idle sessions.
func testUnmarkBeforeSweep(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	rm := holdRm(t, s, "the sweep's rm of x's run mark", companionFile(s, "x", ".run"))
	term := startCld(t, s, "tmux", rm.env, "join", "-s", "x")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	term.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !term.Running() })
	// CLD_IDLE_DAYS 0.00001 is 0.864 s.
	time.Sleep(2 * time.Second)
	rm.start(t)
	list := startCldAsync(t, s, s.Work, map[string]string{"CLD_IDLE_DAYS": "0.00001"}, "list")
	rm.held(t)
	joined := startCld(t, s, "tmux", nil, "join", "-s", "x")
	waitClients(t, s, 1)
	rm.release(t)
	if result := list(); result.Code != 0 || result.Stderr != "" {
		t.Errorf("list: exit %d, stderr %q, want exit 0, no stderr", result.Code, result.Stderr)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-x"}) {
		t.Errorf("sessions %q after the sweep, want [cld-x]", sessions)
	}
	if !joined.Running() {
		t.Errorf("join is not attached:\n%s", joined.Screen())
	}
	checkMarks(t, s, "after the sweep kept x", map[string]bool{"x": false})
}
