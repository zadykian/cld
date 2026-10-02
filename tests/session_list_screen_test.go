package tests

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// listJoinNarrow checks that every line is cut at the terminal's width in cells, so that none wraps
// (decision 14.2). A wide character (日) where a row reaches the edge is left out whole, and é, the
// arrows and the dots take a cell each. Leaving prints the whole directory.
func listJoinNarrow(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// The row shows 29 cells of the directory, so it lives outside the sandbox, where its é
	// shows: /tmp/éN, or /private/tmp/éN on macOS. Its first 日 takes the row's cells 60 and 61.
	const prefix = "> a     attached  now          "
	base := shortDirectory(t, "/tmp/é")
	shown := base + "/" + strings.Repeat("_", 59-len(prefix)-utf8.RuneCountInString(base+"/"))
	dir := shown + "日本日本"
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	startCldIn(t, s, "tmux", dir, nil, "join", "-s", "a")
	s.WaitProbes(1)
	waitClients(t, s, 1)
	detachedSessions(t, s, "b")
	term := terminal.New(t, "tmux", s)
	term.Resize(60, 40)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		prefix+shown,
		cutTo("  b     detached  now          "+s.Work, 60),
		"",
		footerIn(60))
	term.Keys("Down")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     attached  now          "+shown,
		cutTo("> b     detached  now          "+s.Work, 60),
		"",
		footerIn(60))
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	table := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     attached  now          " + dir + "\n" +
		"b     detached  now          " + s.Work + "\n"
	afterList(t, term, table)
}

// listJoinShort checks that a terminal too short for every row keeps the selected one in view,
// with the header and the footer.
func listJoinShort(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a", "b", "c", "d")
	term := terminal.New(t, "tmux", s)
	term.Resize(120, 6)
	term.Start(s.CldArgv("list"), s.Env, s.Work)
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work,
		"  b     detached  now          "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		listHints)
	term.Keys("Down", "Down", "Down")
	scrolled := []string{
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  b     detached  now          " + s.Work,
		"  c     detached  now          " + s.Work,
		"> d     detached  now          " + s.Work,
		"",
		listHints,
	}
	waitLines(t, term, scrolled...)
	// Back up, the rows scroll back with the selection.
	term.Keys("Up", "Up", "Up")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work,
		"  b     detached  now          "+s.Work,
		"  c     detached  now          "+s.Work,
		"",
		listHints)
	// A taller terminal shows the rows scrolled out above, now that they fit.
	term.Keys("Down", "Down", "Down")
	waitLines(t, term, scrolled...)
	term.Resize(120, 10)
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"  b     detached  now          "+s.Work,
		"  c     detached  now          "+s.Work,
		"> d     detached  now          "+s.Work,
		"",
		listHints)
}

// listJoinResize checks that a resize redraws the list at the new size. It takes lines away, so
// that a list drawing for the old size would push its footer off. A line too wide does not show
// as such, since the next line drawn clears what it wrapped onto.
func listJoinResize(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a", "b")
	long := filepath.Join(s.Work, "a-directory-longer-than-thirty-cells")
	if err := os.Mkdir(long, 0o755); err != nil {
		t.Fatal(err)
	}
	probes["a"].Send("cd " + long)
	sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
		return s.Format("cld-a", "#{pane_current_path}") == long
	})
	term := startCld(t, s, "tmux", nil, "list")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+long,
		"  b     detached  now          "+s.Work,
		"",
		listHints)
	// Cut at 30 cells, the screen shows no spaces at the end of a line.
	cutTo30 := func(line string) string { return strings.TrimRight(cutTo(line, 30), " ") }
	term.Resize(30, 5)
	waitLines(t, term,
		cutTo30("  NAME  STATE     LAST ACTIVE  DIRECTORY"),
		cutTo30("> a     detached  now          "+long),
		cutTo30("  b     detached  now          "+s.Work),
		"",
		footerIn(30))
	term.Keys("Down")
	waitLines(t, term,
		cutTo30("  NAME  STATE     LAST ACTIVE  DIRECTORY"),
		cutTo30("  a     detached  now          "+long),
		cutTo30("> b     detached  now          "+s.Work),
		"",
		footerIn(30))
}

// listJoinControlCharacters checks that a control character in a directory shows as "?", so that
// none moves the cursor or changes the terminal (decision 14.2): here an ESC would clear the
// screen. tmux 3.7c passes them on to a UTF-8 client, cld's -u or s.Format's LANG.
func listJoinControlCharacters(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a")
	dir := filepath.Join(s.Work, "e\x01f\x1b[2Jg")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	probes["a"].Send("cd " + dir)
	var reported string
	sandbox.WaitFor(t, 10*time.Second, "claude to move", func() bool {
		reported = s.Format("cld-a", "#{pane_current_path}")
		return reported != s.Work
	})
	term := startCld(t, s, "tmux", nil, "list")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+
			strings.NewReplacer("\x01", "?", "\x1b", "?").Replace(reported),
		"",
		listHints)
}

// shortDirectory creates a directory named prefix and a number, with the symbolic links in its
// path resolved, and removes it when the test ends.
func shortDirectory(t *testing.T, prefix string) string {
	t.Helper()
	for {
		dir := prefix + strconv.Itoa(rand.IntN(1000))
		err := os.Mkdir(dir, 0o755)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) }) //nolint:errcheck // a test's leftover in /tmp
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
}

// cutTo is text cut to columns characters, for text whose characters take one cell each.
func cutTo(text string, columns int) string {
	runes := []rune(text)
	return string(runes[:min(columns, len(runes))])
}

// footerIn is the list's hints as they show in columns cells: without the arrows' hint where the
// hints do not all fit, then cut, without the spaces the screen does not show at the end.
func footerIn(columns int) string {
	hints := listHints
	if utf8.RuneCountInString(hints) > columns {
		hints = strings.TrimPrefix(hints, "↑/↓ to navigate · ")
	}
	return strings.TrimRight(cutTo(hints, columns), " ")
}
