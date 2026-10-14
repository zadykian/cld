package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// What the interactive list's Ctrl+R does where a step of it fails (decision 57.5).

// A rename whose link fails leaves the session whole, and a rename after which the sessions cannot
// be read again shows the new name on the row it has.
func TestListRenameFails(t *testing.T) {
	t.Parallel()
	t.Run("link", testRenameLinkFails)
	t.Run("read", testRenameReadFails)
}

// testRenameLinkFails is TestListRenameFails' rename of b, whose busy mark is a directory, which
// no link takes. The rename removes the entry, environment and last directory it linked as y, and
// b keeps its own.
func testRenameLinkFails(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "b", s.Work, firstID)
	s.WriteFile(companionFile(s, "b", ".env"), "{}")
	s.WriteFile(companionFile(s, "b", ".dir"), s.Work+"\n")
	if err := os.Mkdir(companionFile(s, "b", ".busy"), 0o700); err != nil {
		t.Fatal(err)
	}
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	renameTo(term, "y")
	waitScreen(t, term, "cannot rename session 'b': link "+companionFile(s, "b", ".busy"))
	term.Keys("Escape")
	waitFooter(t, term, endedHints)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	if got, want := readEntry(s, "b"), entry("b", s.Work, firstID); got != want {
		t.Errorf("b's entry %q, want %q", got, want)
	}
	for _, ext := range []string{".env", ".dir"} {
		if !exists(companionFile(s, "b", ext)) {
			t.Errorf("b%s removed", ext)
		}
	}
	for _, ext := range recordFiles {
		if exists(companionFile(s, "y", ext)) {
			t.Errorf("y%s left after the failed rename", ext)
		}
	}
}

// testRenameReadFails is TestListRenameFails' rename of b to y, after which the sessions cannot be
// read again. The list says why, and its row of b shows y, selected. A tmux first on the PATH fails
// to read them once the test says so.
func testRenameReadFails(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	detachedSessions(t, s, "a")
	writeEntry(t, s, "b", s.Work, firstID)
	broken := filepath.Join(s.Root, "broken")
	env := wrapTmux(t, s, "case \"$*\" in *'#{?pane_dead,exited'*)\n"+
		"\tif [ -e '"+broken+"' ]; then echo 'lost the server' >&2; exit 1; fi ;;\n"+
		"esac\n")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, env)
	waitFooter(t, term, listHints)
	term.Keys("Down")
	waitFooter(t, term, endedHints)
	s.WriteFile(broken, "")
	renameTo(term, "y")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> y     ended     -            "+s.Work,
		"",
		"lost the server")
	if got, want := readEntry(s, "y"), entry("y", s.Work, firstID); got != want {
		t.Errorf("y's entry %q, want %q", got, want)
	}
	term.Keys("C-c")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
}
