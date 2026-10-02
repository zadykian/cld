package tests

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listJoinQuits checks that Esc and Ctrl+C leave, joining nothing: cld exits 0, and the terminal
// is as before, with the plain table printed. So do Alt+Esc, and two Esc and a letter at once: the
// first Esc stands alone unless a sequence follows the second (see keys that do nothing).
func listJoinQuits(t *testing.T) {
	t.Helper()
	for _, quit := range listQuits {
		t.Run("quit with "+quit.name, func(t *testing.T) {
			t.Parallel()
			checkQuit(t, quit.keys)
		})
	}
}

// listQuits are the keys that leave the list, and how the terminal types them.
var listQuits = []struct {
	name string
	keys func(terminal.Terminal)
}{
	{"Escape", func(term terminal.Terminal) { term.Keys("Escape") }},
	{"C-c", func(term terminal.Terminal) { term.Keys("C-c") }},
	{"M-Escape", func(term terminal.Terminal) { term.Keys("M-Escape") }},
	// In one write, so that the letter comes within the wait for a lone Esc.
	{"Escape Escape j", //nolint:dupword // the keys, Esc twice
		func(term terminal.Terminal) { term.Paste("\x1b\x1bj") }},
}

// checkQuit opens the list on sessions a and b, leaves it with keys, and checks that it joined
// nothing and left the terminal as before, with the table.
func checkQuit(t *testing.T, keys func(terminal.Terminal)) {
	t.Helper()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work,
		"  b     detached  now          "+s.Work,
		"",
		listHints)
	if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
		t.Errorf("modes while the list is open %+v, "+
			"want the alternate screen and the cursor hidden", modes)
	}
	keys(term)
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	table := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     detached  now          " + s.Work + "\n" +
		"b     detached  now          " + s.Work + "\n"
	waitLines(t, term, strings.Split(strings.TrimSuffix(table, "\n"), "\n")...)
	afterList(t, term, table)
	list.checkRestored(t, term)
	if clients := s.Clients(); len(clients) != 0 {
		t.Errorf("clients attached to %q, want none", clients)
	}
	for name, probe := range probes {
		if !probe.Alive() {
			t.Errorf("claude %s exited", name)
		}
	}
}

// listJoinIdleKeys checks that the selection stops at the first and the last row, and that other
// keys do nothing (decision 14). Those are letters, Shift+Up, Alt+j and Alt+Up, sent as Esc and
// the key (ESC ESC [ A). Each step ends on a row that a key taken for another would not have left
// selected.
func listJoinIdleKeys(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	selected := func(name string) {
		t.Helper()
		lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
		for _, row := range []string{"a", "b", "c"} {
			marker := " "
			if row == name {
				marker = ">"
			}
			lines = append(lines, marker+" "+row+"     detached  now          "+s.Work)
		}
		waitLines(t, term, append(lines, "", listHints)...)
	}
	selected("a")
	term.Keys("Up", "Down")
	selected("b")
	term.Keys("M-j", "S-Up", "M-Up", "k", "q", "Down")
	selected("c")
	term.Keys("Down", "Up")
	selected("b")
	if list.exited() {
		t.Error("the list closed")
	}
}

// listJoinFrameAKey checks that the list draws once for a key, not once for each of its bytes: an
// arrow comes as three. Keys that come together draw once too, so each arrow is typed once the
// frame for the one before is in the output log (see waitFrames).
func listJoinFrameAKey(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	term := startCld(t, s, "tmux", nil, "list")
	waitScreen(t, term, listHints)
	for arrows, row := range []string{"b", "c"} {
		term.Keys("Down")
		// Each frame starts at the top left; the output log trails the screen.
		var frames int
		sandbox.WaitFor(t, 10*time.Second, "the frame with "+row+" selected in the output log",
			func() bool {
				output := term.Output()
				frames = bytes.Count(output, []byte("\x1b[1;1H"))
				return bytes.Contains(output, []byte("\x1b[7m> "+row+" "))
			})
		if want := arrows + 2; frames != want {
			t.Errorf("%d frames up to the one with %s selected, want %d: "+
				"the first and one an arrow", frames, row, want)
		}
	}
}

// listJoinKeysAfterLeaving checks that the list reads only the keys it takes (decision 14.7).
// What comes with Ctrl+C, in the same write, stays with the terminal for the next program.
func listJoinKeysAfterLeaving(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	term := terminal.New(t, "tmux", s)
	// Out of raw mode, the terminal holds the keys for a line; dd takes them as they are.
	list := startList(t, s, term,
		`"$@"; echo $? >"$0.code"; stty raw; dd bs=64 count=1 of="$0.typed" 2>/dev/null`, nil)
	waitScreen(t, term, listHints)
	term.Paste("\x03typed")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	var typed []byte
	sandbox.WaitFor(t, 10*time.Second, "the keys after Ctrl+C to reach dd", func() bool {
		var err error
		typed, err = os.ReadFile(string(list) + ".typed")
		return err == nil && len(typed) > 0
	})
	if string(typed) != "typed" {
		t.Errorf("dd read %q, want %q", typed, "typed")
	}
}

// listJoinApplicationKeys checks that arrows work after a program left application cursor keys
// on (CSI ?1h), when the terminal sends ESC O A and ESC O B for them.
func listJoinApplicationKeys(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	term := terminal.New(t, "tmux", s)
	term.Start(append([]string{"sh", "-c", `printf '\033[?1h' && exec "$@"`, "sh"},
		s.CldArgv("list")...), s.Env, s.Work)
	waitScreen(t, term, listHints)
	term.Keys("Down", "Enter")
	waitScreen(t, term, "probe --name cld-b")
}

// waitFrames waits until the list has drawn count frames, each from the top left, in the output
// log, which trails the screen. Keys that reach the list together, as typed ones may under load,
// draw one frame. So a test types a key once the frame before has reached the log, or pastes keys.
func waitFrames(t *testing.T, term terminal.Terminal, count int) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, fmt.Sprintf("%d frames in the output log", count),
		func() bool {
			return bytes.Count(term.Output(), []byte("\x1b[1;1H")) >= count
		})
}
