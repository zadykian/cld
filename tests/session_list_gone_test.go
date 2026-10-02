package tests

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listJoinGone checks that Enter joins no session gone meanwhile, killed elsewhere and forgotten.
// The footer says so, and the list, still in raw mode, reads the sessions again and selects the
// row that took its place (decision 14.6).
func listJoinGone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	killAndForget(t, s, "b")
	term.Keys("Down", "Enter")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> c     detached  now          "+s.Work,
		"",
		"no session 'b'")
	list.checkRaw(t)
	if list.exited() {
		t.Error("the list closed")
	}
}

// listJoinGoneAbove checks that the selection follows its session's place rather than its row's
// number. With the rows above it gone too, it goes to the next row the list showed, now the first.
func listJoinGoneAbove(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c", "d")
	term := startCld(t, s, "tmux", nil, "list")
	waitScreen(t, term, listHints)
	for _, name := range []string{"a", "b"} {
		killAndForget(t, s, name)
	}
	term.Keys("Down", "Enter")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> c     detached  now          "+s.Work,
		"  d     detached  now          "+s.Work,
		"",
		"no session 'b'")
}

// listJoinGoneLast checks that with no row after it left, the selection goes to the one above,
// although a session made meanwhile now has its row's number.
func listJoinGoneLast(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	term := startCld(t, s, "tmux", nil, "list")
	waitScreen(t, term, listHints)
	killAndForget(t, s, "c")
	detachedSessions(t, s, "z")
	term.Keys("Down", "Down", "Enter")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     detached  now          "+s.Work,
		"  z     detached  now          "+s.Work,
		"",
		"no session 'c'")
}

// listJoinReadAgainFails checks that where the sessions cannot be read again after a failed Enter,
// the list says why as well, and keeps its rows. A tmux first on the PATH fails to read them once
// the test says so.
func listJoinReadAgainFails(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c")
	broken := filepath.Join(s.Root, "broken")
	env := wrapTmux(t, s, "case \"$*\" in *'#{?pane_dead,exited'*)\n"+
		"\tif [ -e '"+broken+"' ]; then echo 'lost the server' >&2; exit 1; fi ;;\n"+
		"esac\n")
	term := startCld(t, s, "tmux", env, "list")
	waitScreen(t, term, listHints)
	killAndForget(t, s, "b")
	s.WriteFile(broken, "")
	term.Keys("Down", "Enter")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> b     detached  now          "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		"no session 'b' · lost the server")
}

// listJoinLastRow checks that with its last row gone, the list shows no sessions under its header,
// and leaving prints nothing (decision 14.4).
func listJoinLastRow(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitScreen(t, term, listHints)
	killAndForget(t, s, "a")
	// cld kill ends the server, which takes a moment to exit. Waiting for it keeps Enter's lookup
	// from reaching it as it goes (see docs/design/findings/tmux-sessions.md).
	sandbox.WaitFor(t, 10*time.Second, "a's server to exit", func() bool {
		_, err := s.Tmux("cld-a", "list-sessions")
		return err != nil && strings.Contains(err.Error(), "no server running")
	})
	term.Keys("Enter")
	const header = "  NAME  STATE     LAST ACTIVE  DIRECTORY"
	waitLines(t, term, header, "no sessions", "", "no session 'a'")
	term.Keys("Down")
	waitLines(t, term, header, "no sessions", "", "esc to quit")
	// Enter has nothing to join.
	term.Keys("Enter")
	waitLines(t, term, header, "no sessions", "", "esc to quit")
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "")
	waitLines(t, term)
}

// listJoinNoSessions checks that with no sessions cld list prints nothing and exits 0 at once
// (decision 14.4).
func listJoinNoSessions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	if output := term.Output(); bytes.Contains(output, []byte("\x1b[?1049h")) {
		t.Errorf("cld opened the alternate screen: %q", output)
	}
	waitLines(t, term)
}

// killAndForget kills session name, as from elsewhere than the list, and forgets its entry.
func killAndForget(t *testing.T, s *sandbox.Sandbox, name string) {
	t.Helper()
	killElsewhere(t, s, name)
	forget(t, s, name)
}
