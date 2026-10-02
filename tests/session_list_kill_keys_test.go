package tests

import (
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listKillReadLate checks that a key come by the end of the two seconds counts as typed within
// them, although cld, stopped here, reads it late (decision 15.1). Esc keeps the session and the
// list open, and a second Ctrl+X kills it. cld may take the key or the wait's end first, so Esc is
// typed three times.
func listKillReadLate(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, pidScript, nil)
	pid, err := strconv.Atoi(list.read(t, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	r := &lateReader{term: term, list: list, pid: pid, rows: []string{
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          " + s.Work,
		"  b     detached  now          " + s.Work,
		"",
	}}
	waitLines(t, term, append(r.rows, listHints)...)
	r.escThrice(t)
	sessions := s.Sessions()
	if !slices.Equal(sessions, []string{"cld-a", "cld-b"}) || !probes["a"].Alive() {
		t.Errorf("sessions %q, claude a alive: %v; want both, a alive",
			sessions, probes["a"].Alive())
	}
	for !r.read(t, "C-x") {
	}
	sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool {
		return !probes["a"].Alive()
	})
	waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     ended     -            "+s.Work, "  b     detached  now          "+s.Work, "",
		endedHints)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
		"a     ended     -            "+s.Work+"\n"+"b     detached  now          "+s.Work+"\n")
	if !probes["b"].Alive() {
		t.Error("claude b exited")
	}
}

// lateReader has the list's cld read a key late: stopped as it waits on a kill armed, and going
// on once the two seconds are over.
type lateReader struct {
	term terminal.Terminal
	list listRun
	pid  int
	// rows are the list's lines above its footer
	rows []string
	// late counts the times cld stopped too late to tell
	late int
}

// read arms the kill, stops cld, types key and has cld go on once the two seconds are over and key
// is there to read. Where cld stopped too late to be sure they were not over, it types nothing and
// reports false once the kill has disarmed; the third time, it skips the test.
func (r *lateReader) read(t *testing.T, key string) bool {
	t.Helper()
	pressed := time.Now()
	r.term.Keys("C-x")
	waitLines(t, r.term, append(r.rows, killArmed)...)
	armed := time.Now()
	if err := syscall.Kill(r.pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld to stop", func() bool { return isStopped(r.pid) })
	inTime := time.Since(pressed) < 2*time.Second
	if inTime {
		r.term.Keys(key)
		sandbox.WaitFor(t, 10*time.Second, "the key to reach the terminal", func() bool {
			return r.list.unread(t)
		})
		time.Sleep(time.Until(armed.Add(2200 * time.Millisecond)))
	}
	if err := syscall.Kill(r.pid, syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	if !inTime {
		waitLines(t, r.term, append(r.rows, listHints)...)
		if r.late++; r.late == 3 {
			t.Skip("cld stopped two seconds after Ctrl+X or later three times: too loaded to tell")
		}
	}
	return inTime
}

// escThrice checks, three times, that Esc read late keeps the session and the list open.
func (r *lateReader) escThrice(t *testing.T) {
	t.Helper()
	for kept := 0; kept < 3; {
		if !r.read(t, "Escape") {
			continue
		}
		sandbox.WaitFor(t, 10*time.Second, "the list to take Esc", func() bool {
			return r.list.exited() || strings.Contains(r.term.Screen(), listHints)
		})
		if r.list.exited() {
			t.Fatalf("Esc %d of 3, read late, closed the list", kept+1)
		}
		waitLines(t, r.term, append(r.rows, listHints)...)
		kept++
	}
}

// listKillOtherKeys checks that any other key disarms the kill, and then does its own work: an
// arrow moves the selection, a letter does nothing more, and Ctrl+C leaves. A Ctrl+X right after
// the key, within the two seconds, then arms the kill again rather than kill.
func listKillOtherKeys(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	lines := func(selected, footer string) []string {
		lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
		for _, row := range []string{"a", "b"} {
			marker := " "
			if row == selected {
				marker = ">"
			}
			lines = append(lines, marker+" "+row+"     detached  now          "+s.Work)
		}
		return append(lines, "", footer)
	}
	waitLines(t, term, lines("a", listHints)...)
	term.Keys("C-x")
	waitLines(t, term, lines("a", killArmed)...)
	term.Keys("Down", "C-x")
	waitLines(t, term, lines("b", killArmed)...)
	term.Keys("k", "C-x")
	waitLines(t, term, lines("b", killArmed)...)
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("sessions %q, want [cld-a cld-b]", sessions)
	}
	term.Keys("C-c")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
		"a     detached  now          "+s.Work+"\n"+"b     detached  now          "+s.Work+"\n")
	for name, probe := range probes {
		if !probe.Alive() {
			t.Errorf("claude %s exited", name)
		}
	}
}

// listKillHeldDown checks Ctrl+X held down, which the terminal repeats after a delay and then many
// times a second. After the Ctrl+X that killed, Ctrl+X does nothing until none has come for a
// second (decision 15.1). So its repeats neither arm nor forget the session killed, which stays
// ended on its row; another key ends the wait at once.
func listKillHeldDown(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b", "c")
	term := startCld(t, s, "tmux", nil, "list")
	header := "  NAME  STATE     LAST ACTIVE  DIRECTORY"
	row := func(marker, name string) string { return killRow(s.Work, marker, name) }
	ended := func(marker, name string) string { return endedRow(s.Work, marker, name) }
	waitScreen(t, term, listHints)
	term.Keys("C-x")
	waitScreen(t, term, killArmed)
	holdCtrlX(t, term)
	// The repeats have neither armed nor forgotten a, which has ended, once the kill has. Down,
	// after them, moves the selection from a to b, and ends the wait: a Ctrl+X right after it
	// arms the kill, and a second one kills b.
	waitLines(t, term, header, ended(">", "a"), row(" ", "b"), row(" ", "c"), "", endedHints)
	term.Keys("Down")
	armThen(t, term, func() {
		waitLines(t, term, header, ended(" ", "a"), row(">", "b"), row(" ", "c"), "", killArmed)
		sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool {
			return !probes["a"].Alive()
		})
		if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-b", "cld-c"}) {
			t.Errorf("sessions %q, want [cld-b cld-c]", sessions)
		}
		for _, name := range []string{"b", "c"} {
			if !probes[name].Alive() {
				t.Errorf("claude %s exited", name)
			}
		}
	}, "C-x")
	waitLines(t, term, header, ended(" ", "a"), ended(">", "b"), row(" ", "c"), "", endedHints)
	// A second without Ctrl+X ends the wait too; the test leaves it half a second more.
	time.Sleep(1500 * time.Millisecond)
	term.Keys("C-x")
	waitLines(t, term, header, ended(" ", "a"), ended(">", "b"), row(" ", "c"), "", forgetArmed)
	if !probes["c"].Alive() {
		t.Error("claude c exited")
	}
}

// holdCtrlX types the second Ctrl+X held down: it repeats after half a second, a common delay,
// then 20 times a second for a second, past the second after the kill. Where the terminal took
// longer than its waits, two may have reached cld a second apart, two presses, and the test skips.
func holdCtrlX(t *testing.T, term terminal.Terminal) {
	t.Helper()
	holding := time.Now()
	term.Hold("C-x", 500*time.Millisecond, 50*time.Millisecond, 20)
	over := time.Since(holding) - 500*time.Millisecond - 20*50*time.Millisecond
	if over >= 400*time.Millisecond {
		t.Skipf("typing Ctrl+X held down took %v beyond its waits: "+
			"two may have reached cld a second apart", over.Round(time.Millisecond))
	}
}

// killRow is the list's row of session name, detached in work, after marker.
func killRow(work, marker, name string) string {
	return marker + " " + name + "     detached  now          " + work
}

// endedRow is the list's row of session name, ended in work, after marker.
func endedRow(work, marker, name string) string {
	return marker + " " + name + "     ended     -            " + work
}
