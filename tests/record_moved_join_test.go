package tests

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// Where a session that has ended shows and comes back, by the directory its claude was last in
// beside its entry (decisions 56.4 and 56.5).

// backCase is a session of TestComesBackIn: its entry names dir, and claude's last directory is
// last, none where "". It shows and comes back in back, with its home in home.
type backCase struct {
	name, dir, last, back, home string
}

// backCases are the sessions of TestComesBackIn in s, made in made, by their names' order.
func backCases(t *testing.T, s *sandbox.Sandbox, made string) []backCase {
	t.Helper()
	moved, gone := filepath.Join(s.Root, "moved"), filepath.Join(s.Root, "gone")
	worktree, file := filepath.Join(made, ".claude", "worktrees", "w"), filepath.Join(s.Root, "file")
	for _, dir := range []string{made, moved, worktree} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.WriteFile(file, "")
	// tmux names a directory by its target, which link's entry keeps by the link's name.
	link := filepath.Join(s.Root, "link")
	if err := os.Symlink(made, link); err != nil {
		t.Fatal(err)
	}
	return []backCase{
		{"file", made, file, made, made},
		{"gone", made, gone, made, made},
		{"link", link, made, link, link},
		{"made-gone", gone, moved, moved, moved},
		{"moved", made, moved, moved, made},
		{"older", made, "", made, made},
		{"relative", made, "moved", made, made},
		{"worktree", made, worktree, made, made},
	}
}

// An ended session shows, and comes back in, the directory its claude was last in. That one can
// be entered, is in no worktree claude made, and is not its entry's by another name. Else it has
// its entry's, which the entry keeps, and its home while it can be entered. An older cld wrote
// none. Where neither can be entered, join refuses, naming the entry's. The fake tmux records it.
func TestComesBackIn(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	made, elsewhere := filepath.Join(s.Root, "made"), filepath.Join(s.Root, "elsewhere", "_")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := backCases(t, s, made)
	writeEntry(t, s, "both-gone", filepath.Join(s.Root, "gone"), firstID)
	s.WriteFile(companionFile(s, "both-gone", ".dir"), filepath.Join(s.Root, "gone2")+"\n")
	want := "NAME       STATE     LAST ACTIVE  DIRECTORY\n" +
		"both-gone  ended     -            " + filepath.Join(s.Root, "gone") + "\n"
	for _, c := range cases {
		writeEntry(t, s, c.name, c.dir, firstID)
		if c.last != "" {
			s.WriteFile(companionFile(s, c.name, ".dir"), c.last+"\n")
		}
		want += c.name + "           "[len(c.name):] + "ended     -            " + c.back + "\n"
	}
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
	for _, c := range cases {
		result := s.RunCldOnTerminalIn(elsewhere, fakeTmux(s), "join", "-s", c.name)
		record := s.FakeTmuxRecord()
		if result.Code != 0 || result.Stderr != "" || record.Cwd != c.back ||
			record.Env["PWD"] != c.back || !optionFollowedBy(record.Argv, "-c", c.back) ||
			!optionFollowedBy(record.Argv, "@cld-home", c.home) {
			t.Errorf("join -s %s: exit %d, stderr %q, tmux in %s, PWD %q, arguments %q; "+
				"want it in %s, -c there, @cld-home %s", c.name, result.Code, result.Stderr,
				record.Cwd, record.Env["PWD"], record.Argv, c.back, c.home)
		}
		if got, want := readEntry(s, c.name), entry(c.name, c.dir, firstID); got != want {
			t.Errorf("%s's entry %q, want %q", c.name, got, want)
		}
		checkLastDirectory(t, s, c.name, "join -s "+c.name, c.back)
	}
	checkJoinRefused(t, "join of both-gone", s.RunCldIn(elsewhere, nil, "join", "-s", "both-gone"),
		"cld: session 'both-gone' ran in "+filepath.Join(s.Root, "gone")+", which no longer "+
			"exists; resume it from here with cld join -n both -s gone --resume "+firstID+"\n")
}

// restore brings a session back where its claude was last, PWD naming it, and says so. Its entry
// keeps the directory join made it in.
func TestRestoreLastDirectory(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	moved := filepath.Join(s.Root, "moved")
	makeDirs(t, moved)
	writeRestorable(t, s, "a", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"),
		s.Environ(nil))
	s.WriteFile(companionFile(s, "a", ".dir"), moved+"\n")
	checkExit(t, "restore", s.RunCld(nil, "restore"), 0, "Restored session 'a' in "+moved+"\n", "")
	claude := s.WaitProbes(1)[0]
	want := []string{"--name", "cld-a", "--settings", sessionSettings(s, "cld-a", s.Work),
		"--resume", firstID}
	if !slices.Equal(claude.Argv, want) || claude.Cwd != moved || claude.Env["PWD"] != moved {
		t.Errorf("claude %q in %s, PWD %q, want %q in %s",
			claude.Argv, claude.Cwd, claude.Env["PWD"], want, moved)
	}
	if got, want := readEntry(s, "a"), entry("a", s.Work, firstID); got != want {
		t.Errorf("entry %q, want %q", got, want)
	}
}
