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

// The interactive list's Ctrl+R, which renames a session that has ended: its entry and the files
// beside it, under the record's lock (decision 57).

// renameHints are the editor's hints, after "rename to ", the name typed and the cursor's cell.
const renameHints = " · enter to rename · esc to cancel"

// modTimes are the times of session name's files in s with the extensions exts, by extension.
func modTimes(t *testing.T, s *sandbox.Sandbox, name string, exts ...string) map[string]time.Time {
	t.Helper()
	times := map[string]time.Time{}
	for _, ext := range exts {
		info, err := os.Stat(companionFile(s, name, ext))
		if err != nil {
			t.Fatal(err)
		}
		times[ext] = info.ModTime()
	}
	return times
}

// checkRenamed checks that session from's files in s are to's. to's entry is want, and the files
// of times have those times; none of from's are left.
func checkRenamed(
	t *testing.T, s *sandbox.Sandbox, from, to, want string, times map[string]time.Time,
) {
	t.Helper()
	if got := readEntry(s, to); got != want {
		t.Errorf("%s's entry %q, want %q", to, got, want)
	}
	for ext, then := range times {
		if got := modTimes(t, s, to, ext)[ext]; !got.Equal(then) {
			t.Errorf("%s%s's time %v, want %s%s's, %v", to, ext, got, from, ext, then)
		}
	}
	for _, ext := range recordFiles {
		if exists(companionFile(s, from, ext)) {
			t.Errorf("%s%s stays after the rename", from, ext)
		}
	}
}

// In the interactive list, Ctrl+R on a session that has ended edits its name in the footer. Enter
// renames its row, selected, and its files, which keep their times. Enter on the row then resumes
// its conversation under the new name, where its claude was last.
func TestListRename(t *testing.T) {
	t.Parallel()
	t.Run("rename", testListRename)
	t.Run("footer", testListRenameFooter)
	t.Run("editing", testListRenameEditing)
}

// testListRename is TestListRename's rename to y of b, whose claude moved before the kill. The row
// of y shows where b's claude was, as y.dir has it, and Enter resumes y there (decision 56.4).
func testListRename(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	moved := filepath.Join(s.Root, "moved")
	makeDirs(t, moved)
	dir := endedSessions(t, s, moved)
	files := []string{".json", ".env", ".dir"}
	for _, ext := range files {
		age(t, time.Hour, companionFile(s, "b", ext))
	}
	times := modTimes(t, s, "b", files...)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(endedRows(s, moved, "a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(endedRows(s, moved, "b"), endedHints)...)
	term.Keys("C-r")
	waitFooter(t, term, "rename to b "+renameHints)
	renameTo(term, "y")
	waitLines(t, term,
		"  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"  a     detached  now          "+s.Work,
		"> y     ended     -            "+moved,
		"", endedHints)
	checkRenamed(t, s, "b", "y", entry("y", dir, firstID), times)
	checkLastDirectory(t, s, "y", "the rename", moved)
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-y")
	resumed := s.WaitProbes(3)[2]
	want := []string{"--name", "cld-y", "--settings", sessionSettings(s, "cld-y", dir),
		"--resume", firstID}
	if !slices.Equal(resumed.Argv, want) {
		t.Errorf("claude arguments %q, want %q", resumed.Argv, want)
	}
	if resumed.Cwd != moved || resumed.Env["PWD"] != moved {
		t.Errorf("claude runs in %s, PWD %q, want %s", resumed.Cwd, resumed.Env["PWD"], moved)
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
}

// testListRenameFooter is TestListRename's footers. In 80 columns an ended row's hints lose the
// arrows'; in 30 the editor's is cut from the left. Its cursor is a cell in inverse video.
func testListRenameFooter(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "b", s.Work, firstID)
	term := terminal.New(t, "tmux", s)
	term.Resize(80, 24)
	list := startList(t, s, term, listScript, nil)
	hints := strings.TrimPrefix(endedHints, "↑/↓ to navigate · ")
	waitFooter(t, term, hints)
	term.Keys("C-r")
	waitFooter(t, term, "rename to b "+renameHints)
	styled := cells(term.Styled())
	want := "[]rename to b[inverse=7] [intensity=2]" + renameHints
	if got := styled[len(styled)-1]; got != want {
		t.Errorf("the editor's footer %q, want %q", got, want)
	}
	// A name too long is refused without itself, which fills the footer.
	term.Keys("C-u")
	term.Paste(strings.Repeat("y", 65) + "\r")
	waitFooter(t, term, "the name is longer than 64 characters")
	term.Resize(30, 24)
	term.Keys("C-u", "b")
	waitFooter(t, term, "rename to b")
	term.Paste(strings.Repeat("y", 25))
	typed := "rename to b" + strings.Repeat("y", 25)
	waitFooter(t, term, typed[len(typed)-29:])
	term.Keys("Escape")
	waitFooter(t, term, strings.TrimRight(cutTo(hints, 30), " "))
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
}

// testListRenameEditing is TestListRename's editor. Enter on the same name closes it, and the
// arrows, Ctrl+X and Ctrl+R do nothing in it. A name it refuses keeps it open with the message, a
// control character too. Backspace and Ctrl+H take a character off whole. A pasted CR is Enter.
func testListRenameEditing(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "b", s.Work, firstID)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	term.Keys("C-r")
	waitFooter(t, term, "rename to b "+renameHints)
	term.Keys("Enter")
	waitFooter(t, term, endedHints)
	term.Keys("C-r", "Up", "C-x", "C-x", "C-r", "/", "Enter")
	waitFooter(t, term, "rename to b/  · invalid name 'b/'")
	term.Paste("é")
	waitFooter(t, term, "rename to b/é "+renameHints)
	term.Keys("BSpace")
	waitFooter(t, term, "rename to b/ "+renameHints)
	term.Keys("C-h", "C-a", "Enter")
	waitFooter(t, term, "rename to b?  · invalid name 'b?'")
	term.Keys("Escape")
	waitFooter(t, term, endedHints)
	if list.exited() {
		t.Error("Esc in the editor left the list")
	}
	term.Keys("C-r", "C-u")
	term.Paste("y\r")
	waitFooter(t, term, endedHints)
	waitScreen(t, term, "> y     ended")
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	if got, want := readEntry(s, "y"), entry("y", s.Work, firstID); got != want {
		t.Errorf("y's entry %q, want %q", got, want)
	}
}

// A session a reboot ended keeps its run mark, busy mark, environment and claude's last directory,
// with their times, under the name the list gives it. restore brings it back by that name, where
// its claude was last, continuing its turn.
func TestListRenameRestore(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	moved := filepath.Join(s.Root, "moved")
	makeDirs(t, moved)
	writeRestorable(t, s, "x", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"),
		s.Environ(nil))
	s.WriteFile(companionFile(s, "x", ".busy"), "")
	s.WriteFile(companionFile(s, "x", ".dir"), moved+"\n")
	files := []string{".json", ".env", ".dir", ".run", ".busy"}
	for _, ext := range files {
		age(t, time.Hour, companionFile(s, "x", ext))
	}
	times := modTimes(t, s, "x", files...)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	renameTo(term, "y")
	waitScreen(t, term, "> y     ended")
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
	checkRenamed(t, s, "x", "y", entry("y", s.Work, firstID), times)
	checkExit(t, "restore", s.RunCld(nil, "restore"), 0,
		"Restored session 'y' in "+moved+", continuing its turn\n", "")
	restored := s.WaitProbes(1)[0]
	want := []string{"--name", "cld-y", "--settings", sessionSettings(s, "cld-y", s.Work),
		"--resume", firstID, continuePrompt}
	if !slices.Equal(restored.Argv, want) || restored.Cwd != moved {
		t.Errorf("claude arguments %q in %s, want %q in %s", restored.Argv, restored.Cwd, want, moved)
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-y"}) {
		t.Errorf("sessions %q, want [cld-y]", sessions)
	}
}

// The rename gives the old name's index again: the moved entry no longer counts it, and the index
// that join gave at its start expired. So join -n api does not give api-3 again. An expired entry
// of the new name goes first, with the file beside it.
func TestListRenameIndex(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "api-3", s.Work, firstID)
	writeEntry(t, s, "auth", s.Work, secondID)
	age(t, 31*24*time.Hour, entryFile(s, "auth"))
	s.WriteFile(companionFile(s, "auth", ".env"), "{}")
	then := time.Now().UTC().AddDate(0, 0, -31).Format(time.RFC3339)
	s.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(entryFile(s, "api-3"))), "indexes.json"),
		`{"api-":{"index":3,"time":"`+then+`"}}`+"\n")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	renameTo(term, "auth")
	waitScreen(t, term, "> auth  ended")
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("list: exit %s, want 0", code)
	}
	if got, want := readEntry(s, "auth"), entry("auth", s.Work, firstID); got != want {
		t.Errorf("auth's entry %q, want %q", got, want)
	}
	if exists(companionFile(s, "auth", ".env")) {
		t.Error("the expired auth.env moved under the new name")
	}
	joinNamed(t, s, "api-4", "-n", "api")
}
