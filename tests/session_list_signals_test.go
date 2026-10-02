package tests

import (
	"bytes"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listJoinSignals checks that SIGTERM, SIGHUP, SIGINT and SIGQUIT end the list as they end cld,
// with 128 and the signal's number, once the terminal is as before (decision 14, Signals).
func listJoinSignals(t *testing.T) {
	t.Helper()
	for _, sig := range []syscall.Signal{
		syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT,
	} {
		t.Run("signal "+strconv.Itoa(int(sig)), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a")
			term := terminal.New(t, "tmux", s)
			list := startList(t, s, term, pidScript, nil)
			waitScreen(t, term, listHints)
			signalList(t, list, sig)
			if code, want := list.code(t), strconv.Itoa(128+int(sig)); code != want {
				t.Errorf("exit %s, want %s", code, want)
			}
			list.checkRestored(t, term)
		})
	}
}

// signalList sends sig to the cld that list runs, by the pid that pidScript writes down.
func signalList(t *testing.T, list listRun, sig syscall.Signal) {
	t.Helper()
	pid, err := strconv.Atoi(list.read(t, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, sig); err != nil {
		t.Fatal(err)
	}
}

// listJoinIgnoredSignal checks that a signal cld was started with ignored stays ignored, as under
// nohup: SIGHUP leaves the list open.
func listJoinIgnoredSignal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, "trap '' HUP; "+pidScript, nil)
	waitScreen(t, term, listHints)
	signalList(t, list, syscall.SIGHUP)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	list.checkRestored(t, term)
}

// listJoinStops checks SIGTSTP, from outside as Ctrl+Z is a key in raw mode, which stops cld with
// the terminal put back. SIGSTOP leaves the list on the screen, and the terminal in raw mode. Once
// the shell has cld go on (fg), the list takes the terminal again and draws it all.
func listJoinStops(t *testing.T) {
	t.Helper()
	for _, sig := range []syscall.Signal{syscall.SIGTSTP, syscall.SIGSTOP} {
		t.Run("stopped with "+strconv.Itoa(int(sig)), func(t *testing.T) {
			t.Parallel()
			checkStopped(t, sig)
		})
	}
}

// checkStopped stops the list, run as a job of a shell with job control, with sig, and has the
// shell put it back in the foreground.
func checkStopped(t *testing.T, sig syscall.Signal) {
	t.Helper()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startJob(t, s, term)
	waitLines(t, term, stoppedRows(s.Work, "a")...)
	signalList(t, list, sig)
	// cld stops itself with SIGSTOP for SIGTSTP (see pause in internal/picker).
	status, want := list.read(t, "stopped"), strconv.Itoa(128+int(syscall.SIGSTOP))
	if status != want {
		t.Errorf("the shell reports cld stopped with status %s, want %s", status, want)
	}
	if sig == syscall.SIGTSTP {
		checkPutBack(t, term, list)
	}
	s.WriteFile(string(list)+".go", "")
	waitLines(t, term, stoppedRows(s.Work, "a")...)
	if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
		t.Errorf("modes once cld goes on %+v, want the alternate screen and the cursor hidden",
			modes)
	}
	list.checkRaw(t)
	term.Keys("Down")
	waitLines(t, term, stoppedRows(s.Work, "b")...)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
		"a     detached  now          "+s.Work+"\n"+"b     detached  now          "+s.Work+"\n")
	list.checkRestored(t, term)
}

// stoppedRows are the list of sessions a and b, detached in work, with the session selected.
func stoppedRows(work, selected string) []string {
	lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
	for _, row := range []string{"a", "b"} {
		marker := " "
		if row == selected {
			marker = ">"
		}
		lines = append(lines, marker+" "+row+"     detached  now          "+work)
	}
	return append(lines, "", listHints)
}

// checkPutBack checks that cld, stopped, has put the terminal back: its mode, the main screen and
// the cursor, with the shell's line.
func checkPutBack(t *testing.T, term terminal.Terminal, list listRun) {
	t.Helper()
	if before, during := list.read(t, "before"), list.read(t, "during"); before != during {
		t.Errorf("stty -g while cld is stopped\n%s\nwant as before\n%s", during, before)
	}
	waitScreen(t, term, "the shell's line")
	sandbox.WaitFor(t, 10*time.Second, "the main screen and the cursor while cld is stopped",
		func() bool {
			modes := term.Modes()
			return !modes.AltScreen && modes.Cursor
		})
}

// listJoinOrphanedStop checks that SIGTSTP stops nothing in an orphaned process group, where no
// shell would have cld go on. The list puts the terminal back, takes it again and goes on.
func listJoinOrphanedStop(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, pidScript, nil)
	waitScreen(t, term, listHints)
	signalList(t, list, syscall.SIGTSTP)
	const backAndAgain = "\x1b[?25h\x1b[?1049l" + "\x1b[?1049h\x1b[?25l"
	sandbox.WaitFor(t, 10*time.Second, "the terminal put back and taken again", func() bool {
		return bytes.Contains(term.Output(), []byte(backAndAgain))
	})
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	list.checkRestored(t, term)
}

// listJoinSignalAtEnter checks that a signal while Enter looks the session up ends cld at once,
// joining nothing, on a server that hangs too. The lookup's tmux is killed, and the tab keeps its
// title. A tmux first on the PATH holds the lookup.
func listJoinSignalAtEnter(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	lookup := holdLookup(t, s, "a")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, pidScript, lookup.env)
	waitScreen(t, term, listHints)
	pid, err := strconv.Atoi(list.read(t, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	term.Keys("Enter")
	held := lookup.held(t)
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := list.code(t); code != "143" {
		t.Errorf("exit %s, want 143", code)
	}
	if !gone(held) {
		t.Error("the lookup's tmux outlived cld")
	}
	list.checkRestored(t, term)
	if clients := s.Clients(); len(clients) != 0 {
		t.Errorf("clients attached to %q, want none", clients)
	}
	if title := term.Title(); title == "✳ cld-a" {
		t.Errorf("terminal title %q, for a session not joined", title)
	}
}
