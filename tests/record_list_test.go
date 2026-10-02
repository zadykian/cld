package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// A session that has ended in the interactive list, where Enter resumes it and Ctrl+X twice
// forgets it (decision 40.3).

// endedSessions makes sessions a, running, and b, which has ended in the conversation firstID. b
// ran in another directory, whose name leaves nothing of a session's name, which it returns.
func endedSessions(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	detachedSessions(t, s, "a")
	dir := filepath.Join(s.Root, "other", "_")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	term := startCldIn(t, s, "tmux", dir, nil, "join", "-s", "b")
	b := s.WaitProbes(2)[1]
	b.Hook("SessionStart", `{"session_id":"`+firstID+`","source":"startup"}`)
	if result := s.RunCld(nil, "kill", "-s", "b"); result.Code != 0 {
		t.Fatalf("kill: exit %d, stderr %q", result.Code, result.Stderr)
	}
	sandbox.WaitFor(t, 10*time.Second, "cld b to return", func() bool { return !term.Running() })
	return dir
}

// endedRows are the list's header and rows of endedSessions in s, b's in dir, with selected's
// row marked.
func endedRows(s *sandbox.Sandbox, dir, selected string) []string {
	lines := []string{"  NAME  STATE     LAST ACTIVE  DIRECTORY"}
	for _, row := range []string{
		"a     detached  now          " + s.Work,
		"b     ended     -            " + dir,
	} {
		marker := " "
		if row[:1] == selected {
			marker = ">"
		}
		lines = append(lines, marker+" "+row)
	}
	return append(lines, "")
}

// In the interactive list, a session that has ended has hints of its own. Enter resumes it as
// join does, and Ctrl+X twice forgets it, taking its row and entry away. A session whose
// directory has gone is refused, and the list stays open.
func TestListEnded(t *testing.T) {
	t.Parallel()
	t.Run("resume", testListEndedResume)
	t.Run("forget", testListEndedForget)
	t.Run("directory gone", testListEndedGone)
}

// testListEndedResume is TestListEnded's Enter, which resumes b by its conversation's ID, in the
// directory it ran in.
func testListEndedResume(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := endedSessions(t, s)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(endedRows(s, dir, "a"), listHints)...)
	term.Keys("Down")
	waitLines(t, term, append(endedRows(s, dir, "b"), endedHints)...)
	term.Keys("Enter")
	waitScreen(t, term, "probe --name cld-b")
	resumed := s.WaitProbes(3)[2]
	want := []string{"--name", "cld-b", "--settings", sessionSettings(s, "cld-b", dir),
		"--resume", firstID}
	if !slices.Equal(resumed.Argv, want) {
		t.Errorf("claude arguments %q, want %q", resumed.Argv, want)
	}
	if resumed.Cwd != dir {
		t.Errorf("claude runs in %s, want %s", resumed.Cwd, dir)
	}
	if title := term.Title(); title != "✳ cld-b" {
		t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
	}
	term.Keys("C-q", "d")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	list.checkRestored(t, term)
}

// testListEndedForget is TestListEnded's Ctrl+X twice, which forgets b only at the second.
func testListEndedForget(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := endedSessions(t, s)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(endedRows(s, dir, "a"), listHints)...)
	term.Keys("Down")
	armThen(t, term, func() {
		waitLines(t, term, append(endedRows(s, dir, "b"), forgetArmed)...)
		if readEntry(s, "b") == "" {
			t.Error("the first Ctrl+X forgot b")
		}
	}, "C-x")
	waitLines(t, term, "  NAME  STATE     LAST ACTIVE  DIRECTORY",
		"> a     detached  now          "+s.Work, "", listHints)
	if entry := readEntry(s, "b"); entry != "" {
		t.Errorf("b's entry %q after the forget, want none", entry)
	}
	if exists(companionFile(s, "b", ".env")) {
		t.Error("b's environment stays after the forget")
	}
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
		"a     detached  now          "+s.Work+"\n")
}

// testListEndedGone is TestListEnded's Enter on b once its directory has gone.
func testListEndedGone(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := endedSessions(t, s)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitLines(t, term, append(endedRows(s, dir, "a"), listHints)...)
	term.Keys("Down", "Enter")
	waitLines(t, term,
		append(endedRows(s, dir, "b"), "session 'b' ran in "+dir+", which no longer exists")...)
	if list.exited() || len(s.Probes()) != 2 {
		t.Errorf("list exited: %v, %d claude processes; want the list open and the first two",
			list.exited(), len(s.Probes()))
	}
}
