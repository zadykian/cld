package tests

import (
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// What the interactive list's Ctrl+R refuses to rename, leaving the session unchanged (decision
// 57.4).

// The list refuses to rename a session that runs, one whose server still runs, and one without a
// conversation's ID. Nor does it give a session a name that is taken.
func TestListRenameRefused(t *testing.T) {
	t.Parallel()
	t.Run("taken", testRenameTaken)
	t.Run("running", testRenameRunning)
	t.Run("server runs", testRenameServerRuns)
	t.Run("server of the name runs", testRenameNameServerRuns)
	t.Run("no conversation", testRenameNoConversation)
}

// testRenameTaken is TestListRenameRefused's rename of b to a, which runs, then to c, which has
// ended. y has a file of the record that names another session, and another cld is starting w.
// The editor stays open with each refusal, and Esc closes it.
func testRenameTaken(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	dir := endedSessions(t, s, "")
	writeEntry(t, s, "c", s.Work, secondID)
	s.WriteFile(entryFile(s, "y"), entry("z", s.Work, secondID))
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, listHints)
	term.Keys("Down")
	waitFooter(t, term, endedHints)
	renameTo(term, "a")
	waitFooter(t, term, "rename to a  · session 'a' runs, and keeps its name")
	term.Keys("C-u", "c", "Enter")
	waitFooter(t, term, "rename to c  · session 'c' has ended, and keeps its name until forgotten")
	term.Keys("C-u", "y", "Enter")
	waitFooter(t, term, "rename to y  · session name 'y' is taken: cld's record has y.json")
	s.WriteFile(companionFile(s, "w", ".start"), strconv.Itoa(os.Getpid()))
	term.Keys("C-u", "w", "Enter")
	waitFooter(t, term, "rename to w  · session 'w' is starting")
	term.Keys("Escape")
	waitFooter(t, term, endedHints)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	for name, want := range map[string]string{
		"b": entry("b", dir, firstID), "c": entry("c", s.Work, secondID),
		"y": entry("z", s.Work, secondID), "w": "",
	} {
		if got := readEntry(s, name); got != want {
			t.Errorf("%s's entry %q, want %q", name, got, want)
		}
	}
	if sessions := s.Sessions(); !slices.Equal(sessions, []string{"cld-a"}) {
		t.Errorf("sessions %q, want [cld-a]", sessions)
	}
}

// testRenameRunning is TestListRenameRefused's Ctrl+R on a row that runs, which says to end it
// first. It disarms the kill as any key does: the next Ctrl+X arms it again.
func testRenameRunning(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	probes := detachedSessions(t, s, "a")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, listHints)
	term.Keys("C-x")
	waitFooter(t, term, killArmed)
	term.Keys("C-r")
	waitFooter(t, term, "end session 'a' with ctrl+x twice first, to rename it")
	term.Keys("C-x")
	waitFooter(t, term, killArmed)
	if !probes["a"].Alive() {
		t.Error("Ctrl+X after Ctrl+R killed a")
	}
	term.Keys("Escape")
	waitFooter(t, term, listHints)
	term.Keys("Escape")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
}

// outlive makes session name in s, in the conversation of that ID, whose server outlives it with a
// session its claude made there.
func outlive(t *testing.T, s *sandbox.Sandbox, name, conversation string) {
	t.Helper()
	probe := detachedSessions(t, s, name)[name]
	probe.Hook("SessionStart", `{"session_id":"`+conversation+`","source":"startup"}`)
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-"+name+"/side")
	})
	probe.Send("exit")
	sandbox.WaitFor(t, 10*time.Second, name+"'s session to end", func() bool {
		return !slices.Contains(s.Sessions(), "cld-"+name)
	})
}

// testRenameServerRuns is TestListRenameRefused's rename of b, whose server outlived it, as join
// refuses it. The footer points at cld kill, which ends that server.
func testRenameServerRuns(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	outlive(t, s, "b", firstID)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	renameTo(term, "y")
	waitFooter(t, term, "rename to y  · tmux server cld-b still runs; end it with cld kill -s b")
	if got, want := readEntry(s, "b"), entry("b", s.Work, firstID); got != want {
		t.Errorf("b's entry %q, want %q", got, want)
	}
	if exists(entryFile(s, "y")) {
		t.Error("y's entry made")
	}
	term.Keys("C-c")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
}

// testRenameNameServerRuns is TestListRenameRefused's rename of b to y, whose server outlived its
// session.
func testRenameNameServerRuns(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "b", s.Work, firstID)
	outlive(t, s, "y", secondID)
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	renameTo(term, "y")
	waitFooter(t, term, "rename to y  · tmux server cld-y still runs; end it with cld kill -s y")
	term.Keys("C-c")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	for name, want := range map[string]string{
		"b": entry("b", s.Work, firstID), "y": entry("y", s.Work, secondID),
	} {
		if got := readEntry(s, name); got != want {
			t.Errorf("%s's entry %q, want %q", name, got, want)
		}
	}
}

// testRenameNoConversation is TestListRenameRefused's rename of x, whose entry has no
// conversation's ID: claude would look for it by the new name. Ctrl+C then leaves from the
// editor, printing the table.
func testRenameNoConversation(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeEntry(t, s, "x", s.Work, "")
	term := terminal.New(t, "tmux", s)
	list := startList(t, s, term, listScript, nil)
	waitFooter(t, term, endedHints)
	renameTo(term, "y")
	waitFooter(t, term, "rename to y  · session 'x' has no conversation's ID to resume")
	term.Keys("C-c")
	if code := list.code(t); code != "0" {
		t.Errorf("exit %s, want 0", code)
	}
	afterList(t, term, "NAME  STATE     LAST ACTIVE  DIRECTORY\n"+
		"x     ended     -            "+s.Work+"\n")
	if got, want := readEntry(s, "x"), entry("x", s.Work, ""); got != want {
		t.Errorf("x's entry %q, want %q", got, want)
	}
	if exists(entryFile(s, "y")) {
		t.Error("y's entry made")
	}
}
