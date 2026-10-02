package tests

import (
	"bytes"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// On a terminal, cld list shows cld's sessions on the alternate screen, the first one selected
// (decision 14). The arrows move the selection, Enter joins the selected session as cld join does,
// and Esc or Ctrl+C leave, printing the plain table. Elsewhere it prints the table.
func TestListJoin(t *testing.T) {
	t.Parallel()
	t.Run("enter", listJoinEnter)
	t.Run("busy terminal", listJoinBusyTerminal)
	t.Run("terminal that does not answer", listJoinSilentTerminal)
	t.Run("attached", listJoinAttached)
	t.Run("exited", listJoinExited)
	t.Run("exited and attached", listJoinExitedAttached)
	listJoinQuits(t)
	t.Run("keys that do nothing", listJoinIdleKeys)
	t.Run("a frame a key", listJoinFrameAKey)
	t.Run("keys after leaving", listJoinKeysAfterLeaving)
	listJoinSignals(t)
	t.Run("ignored signal", listJoinIgnoredSignal)
	listJoinStops(t)
	t.Run("SIGTSTP without job control", listJoinOrphanedStop)
	t.Run("signal at enter", listJoinSignalAtEnter)
	listJoinLeaveLookup(t)
	t.Run("keys during the lookup", listJoinKeysDuringLookup)
	t.Run("ended", listJoinEnded)
	t.Run("gone", listJoinGone)
	t.Run("gone with the rows above", listJoinGoneAbove)
	t.Run("gone from the last row", listJoinGoneLast)
	t.Run("lingering server", listJoinLingering)
	t.Run("read again fails", listJoinReadAgainFails)
	t.Run("last row", listJoinLastRow)
	t.Run("no sessions", listJoinNoSessions)
	t.Run("narrow", listJoinNarrow)
	t.Run("short", listJoinShort)
	t.Run("resize", listJoinResize)
	t.Run("control characters", listJoinControlCharacters)
	t.Run("application cursor keys", listJoinApplicationKeys)
	t.Run("own pane", listJoinOwnPane)
	t.Run("not a terminal", listJoinNotATerminal)
}

// listJoinEnter checks that Enter joins the selected session, as cld join -s NAME does. The list's
// last output, the main screen, the cursor and the title, comes before a question the terminal
// answers once it has read it (decision 14, Enter's handover). After a detach the terminal is as it
// was before the list.
func listJoinEnter(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work,
		"  b     detached  now          "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		listHints)
	// The selected row is drawn in inverse video, and only that one; the footer is dim.
	styled := cells(term.Styled())
	for i, want := range map[int]string{
		0: "[]  NAME  STATE     LAST ACTIVE  DIRECTORY",
		1: "[inverse=7]> a     detached  now          " + s.Work,
		2: "[]  b     detached  now          " + s.Work,
		5: "[intensity=2]" + listHints,
	} {
		if styled[i] != want {
			t.Errorf("line %d is %q, want %q", i+1, styled[i], want)
		}
	}
	term.Keys("Down", "Enter")
	waitScreen(t, term, "probe --name cld-b")
	if title := term.Title(); title != "✳ cld-b" {
		t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
	}
	waitClients(t, s, 1)
	if clients := s.Clients(); !slices.Equal(clients, []string{"cld-b"}) {
		t.Errorf("clients attached to %q, want one, to cld-b", clients)
	}
	if probes := s.Probes(); len(probes) != 3 {
		t.Errorf("%d claude processes, want 3", len(probes))
	}
	const handOver = "\x1b[?25h\x1b[?1049l" + "\x1b]0;✳ cld-b\a" + "\x1b[c"
	sandbox.WaitFor(t, 10*time.Second,
		"the main screen, the cursor, the title and the question, in that order", func() bool {
			return bytes.Contains(term.Output(), []byte(handOver))
		})
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	list.checkRestored(t, term)
}

// listJoinBusyTerminal checks that a terminal too busy to answer at once, over a slow link say,
// holds the join back until it answers. Its answer does not reach claude as keys. The terminal
// freezes as the lookup ends, and thaws a second and a half later.
func listJoinBusyTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	lookup := holdLookup(t, s, "b")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, pidScript, lookup.env)
	waitScreen(t, term, listHints)
	pid := list.read(t, "pid")
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	term.Keys("Enter")
	lookup.held(t)
	thaw := term.Freeze()
	lookup.release(t)
	time.Sleep(1500 * time.Millisecond)
	if strings.HasPrefix(program(pid), "tmux") {
		t.Error("cld became tmux before the terminal answered")
	}
	thaw()
	waitScreen(t, term, "probe --name cld-b")
	// The terminal answered before it drew claude's screen: a key typed now comes after it.
	term.Keys("z")
	probes["b"].WaitInput(0, "z")
	if answer := attributesAnswer.Find(probes["b"].Input()); answer != nil {
		t.Errorf("claude read the terminal's answer %q as keys", answer)
	}
}

// listJoinSilentTerminal checks that a terminal that does not answer holds the join back five
// seconds, and no longer.
func listJoinSilentTerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	lookup := holdLookup(t, s, "a")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, pidScript, lookup.env)
	waitScreen(t, term, listHints)
	pid := list.read(t, "pid")
	term.Keys("Enter")
	lookup.held(t)
	thaw := term.Freeze()
	released := time.Now()
	lookup.release(t)
	sandbox.WaitFor(t, 20*time.Second, "cld to become tmux", func() bool {
		return strings.HasPrefix(program(pid), "tmux")
	})
	if waited := time.Since(released); waited < 5*time.Second {
		t.Errorf("cld became tmux %v after its lookup, with no answer from the terminal; "+
			"want five seconds", waited)
	}
	thaw()
	waitScreen(t, term, "probe --name cld-a")
}

// attributesAnswer matches a terminal's answer to a question for its primary device attributes.
var attributesAnswer = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)

// listJoinOwnPane checks Enter in a live pane of one of cld's servers, a shell in a window of the
// session, say. It moves the terminal on the pane's session to the session picked, as join does
// there (decision 14.3), and the list exits 0. The session left runs on.
func listJoinOwnPane(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "b")
	term := startCld(t, s, "tmux", nil, "join", "-s", "a")
	waitScreen(t, term, "probe --name cld-a")
	// A window of session a, which the terminal shows; it runs cld list with the TMUX tmux
	// sets for it.
	out := filepath.Join(s.Root, "own")
	s.MustTmux("cld-a", append([]string{"new-window", "-t", "=cld-a:",
		"sh", "-c", `"$@"; echo $? >"$0.code"`, out}, s.CldArgv("list")...)...)
	waitScreen(t, term, listHints)
	term.Keys("Down")
	sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool {
		return selectedRow(term) == "b"
	})
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	if code := waitExitFile(t, "cld list to return", out); code != "0\n" {
		t.Errorf("exit %s, want 0", strings.TrimSpace(code))
	}
	clients, sessions := s.Clients(), s.Sessions()
	if !slices.Equal(clients, []string{"cld-b"}) ||
		!slices.Equal(sessions, []string{"cld-a", "cld-b"}) {
		t.Errorf("clients attached to %q, sessions %q; want the terminal on cld-b, "+
			"and both sessions", clients, sessions)
	}
}

// listJoinNotATerminal checks where list prints the plain table (decision 14.1). Output to a pipe
// gets it, as do input that is not the terminal, a terminal that cannot move the cursor and a job
// in the background.
func listJoinNotATerminal(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b")
	table := []string{
		"NAME  STATE     LAST ACTIVE  DIRECTORY",
		"a     detached  now          " + s.Work,
		"b     detached  now          " + s.Work,
	}
	for _, test := range []struct {
		name, script string
		extra        map[string]string
	}{
		{"a pipe", `{ "$@"; echo $? >"$0.code"; } | cat`, nil},
		{"stdin from /dev/null", `"$@" </dev/null; echo $? >"$0.code"`, nil},
		{"TERM=dumb", `"$@"; echo $? >"$0.code"`, map[string]string{"TERM": "dumb"}},
		{"TERM unset", `env -u TERM "$@"; echo $? >"$0.code"`, nil},
		// It would stop (SIGTTOU) as it set the terminal up. Job control puts it in a process
		// group of its own. It goes off before the job ends, which bash 3.2, macOS's sh, would
		// report (docs/design/findings/environment.md).
		{"a background job", `set -m; "$@" & set +m; wait $!; echo $? >"$0.code"`, nil},
	} {
		term := terminal.New(t, "tmux", s)
		list := startList(t, s, term, test.script, test.extra)
		if code := list.code(t); code != "0" {
			t.Errorf("%s: exit %s, want 0", test.name, code)
		}
		waitLines(t, term, table...)
		if output := term.Output(); bytes.Contains(output, []byte("\x1b[?1049h")) {
			t.Errorf("%s: cld opened the alternate screen: %q", test.name, output)
		}
	}
}
