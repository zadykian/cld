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

// listKillSessionFails checks that a kill-session that fails leaves the session listed, with what
// tmux said in the footer, or its exit status where it said nothing. A tmux first on the PATH
// fails it.
func listKillSessionFails(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	silent := filepath.Join(s.Root, "silent")
	env := wrapTmux(t, s, "case \"$*\" in *kill-session*)\n"+
		"\tif [ -e '"+silent+"' ]; then exit 5; fi\n"+
		"\techo 'tmux: cannot kill' >&2; exit 5 ;;\n"+
		"esac\n")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, env)
	waitScreen(t, term, listHints)
	const header = "  NAME  STATE     LAST ACTIVE  DIRECTORY"
	row := "> a     detached  now          " + s.Work
	term.Keys("C-x", "C-x")
	waitLines(t, term, header, row, "", "tmux: cannot kill")
	s.WriteFile(silent, "")
	// A letter first ends the wait after a kill (see held down).
	term.Keys("k", "C-x", "C-x")
	waitLines(t, term, header, row, "", "tmux kill-session: exit status 5")
	list.checkRaw(t)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q, want [cld-a]", sessions)
	}
}

// listKillReadAgainFails checks that where the sessions cannot be read again after a kill, the
// list says why and keeps its rows, but the one it killed. A tmux first on the PATH fails to read
// them once the test says so.
func listKillReadAgainFails(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	broken := filepath.Join(s.Root, "broken")
	env := wrapTmux(t, s, "case \"$*\" in *'#{?pane_dead,exited'*)\n"+
		"\tif [ -e '"+broken+"' ]; then echo 'lost the server' >&2; exit 1; fi ;;\n"+
		"esac\n")
	term := startCld(t, s, "tmux", env, "list")
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	s.WriteFile(broken, "")
	term.Keys("C-x", "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> c     detached  now          "+s.Work,
		"",
		"lost the server")
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-c"}) {
		t.Errorf("sessions %q, want [cld-a cld-c]", sessions)
	}
}

// listKillServerExiting checks that the list passes over the killed session's server as it exits,
// which a read may meet as one that exited unexpectedly (decision 15.5). A tmux first on the PATH
// keeps that server taking connections, as an exiting one does for a moment. It fails the first
// read of it after the kill, as tmux may then.
func listKillServerExiting(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	exiting := filepath.Join(s.Root, "exiting")
	env := wrapTmux(t, s, "if [ -e '"+exiting+"' ]; then case \"$*\" in\n"+
		"\t*'-L cld-a '*' kill-session -t =cld-a '*) exit 0 ;;\n"+
		"\t*'-L cld-a list-sessions '*'#{?pane_dead,exited'*) rm '"+exiting+"'; "+
		"echo 'server exited unexpectedly' >&2; exit 1 ;;\n"+
		"esac; fi\n")
	term := startCld(t, s, "tmux", env, "list")
	waitScreen(t, term, listHints)
	s.WriteFile(exiting, "")
	term.Keys("C-x", "C-x")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     ended     -            "+s.Work,
		"  b     detached  now          "+s.Work,
		"",
		endedHints)
	if _, err := os.Stat(exiting); err == nil {
		t.Error("the list did not read the killed session's server after the kill")
	}
}

// listKillKeysDuring checks that other keys than those that leave do nothing while the kill runs:
// Down leaves the selection, and Ctrl+X arms nothing, though Down has ended the wait after a kill.
// The two Ctrl+X go in one write, then a key a frame (see waitFrames). A tmux first on the PATH
// holds the kill's lookup.
func listKillKeysDuring(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	lookup := holdLookup(t, s, "a")
	term := startCld(t, s, "tmux", lookup.env, "list")
	waitScreen(t, term, listHints)
	term.Paste("\x18\x18")
	lookup.held(t)
	waitFrames(t, term, 2)
	term.Keys("Down")
	waitFrames(t, term, 3)
	term.Keys("C-x")
	waitFrames(t, term, 4)
	if row := selectedRow(term); row != "a" {
		t.Errorf("row %q selected during the kill, want a", row)
	}
	if strings.Contains(term.Screen(), killArmed) {
		t.Error("Ctrl+X armed the kill while the kill ran")
	}
	lookup.release(t)
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     ended     -            "+s.Work,
		"  b     detached  now          "+s.Work,
		"",
		endedHints)
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool {
		return !probes["a"].Alive()
	})
	if !probes["b"].Alive() {
		t.Error("claude b exited")
	}
}

// listKillLeaveDuring checks that Esc leaves while the kill runs, printing the table: the kill's
// tmux is killed, and the table shows what it did by then. Held in its lookup or kill-session, the
// kill ends nothing; held as it reads the sessions again, it has ended the session.
func listKillLeaveDuring(t *testing.T) {
	t.Helper()
	for _, step := range killSteps {
		t.Run("quit during the kill's "+step.name, func(t *testing.T) {
			t.Parallel()
			checkLeaveDuring(t, step)
		})
	}
}

// killStep is a step of the list's kill, the pattern of the tmux command that runs it, and
// whether the kill has ended the session by then.
type killStep struct {
	name, pattern string
	ends          bool
}

var killSteps = []killStep{
	{"lookup", "*'#{==:#{session_name},cld-a},'*' " +
		"-F #{session_name} #{W:#{P:#{pane_pid} }}\t#{@cld-home}'", false},
	{"kill-session", "*kill-session*", false},
	{"read", "*'#{?pane_dead,exited'*", true},
}

// checkLeaveDuring has a tmux first on the PATH hold the step, once the list has read the sessions
// it opens with, and leaves the list during it.
func checkLeaveDuring(t *testing.T, step killStep) {
	t.Helper()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	hold := holdTmux(t, s, "the kill's "+step.name, step.pattern)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, hold.env)
	waitScreen(t, term, listHints)
	hold.start(t)
	term.Keys("C-x", "C-x")
	held := hold.held(t)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	if !gone(held) {
		t.Error("the kill's tmux outlived cld")
	}
	table := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" + "a     detached  now          " + s.Work +
		"\n" + "b     detached  now          " + s.Work + "\n"
	sessions := []string{"cld-a", "cld-b"}
	if step.ends {
		table = "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
			"b     detached  now          " + s.Work + "\n"
		sessions = []string{"cld-b"}
	}
	afterList(t, term, table)
	list.checkRestored(t, term)
	if got := s.Sessions(); !slices.Equal(got, sessions) {
		t.Errorf("sessions %q, want %q", got, sessions)
	}
	if probes["a"].Alive() == step.ends {
		t.Errorf("claude a alive: %v, want %v", !step.ends, step.ends)
	}
}
