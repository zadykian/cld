package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join -n offers the NAME of each session list shows, what comes before its last "-", described by
// its count of sessions (decision 24.8). join -s offers the SUFFIX of each session of NAME that
// join takes, by its state, an ended one by its directory. In the root directory every name is a
// SUFFIX, and detach -n and -s offer the same of the sessions that run.
func TestCompleteSuffixes(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	work := repository(t, s, "work")
	sub := filepath.Join(work, "sub")
	other := filepath.Join(s.Root, "other")
	for _, dir := range []string{sub, other} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Each on a server marked as cld marks its own (see TestLeavesAForeignServerAlone).
	for _, name := range []string{
		"cld-work-0", "cld-work-fix", "cld-work--x", "cld-work", "cld-other-1", "cld-3", "cld-a-b-c",
	} {
		s.MustTmux(name, "-f", "/dev/null", "set", "-s", "@cld", "1", ";",
			"new-session", "-d", "-s", name, "sleep", "600")
	}
	for _, name := range []string{"work-0", "work-7", "other-2", "4"} {
		writeEntry(t, s, name, other, "")
	}
	for _, test := range completedSuffixes(work, sub, other) {
		for command, want := range map[string]string{"join": test.join, "detach": test.detach} {
			args := append([]string{"__complete", command}, test.args...)
			result := s.RunCldIn(test.dir, nil, args...)
			if result.Code != 0 || result.Stdout != want {
				t.Errorf("%s %q in %s: exit %d, stdout\n%s\nwant\n%s",
					command, test.args, test.dir, result.Code, result.Stdout, want)
			}
		}
	}
}

// suffixCase is a completion of TestCompleteSuffixes, run in dir.
type suffixCase struct {
	dir  string
	args []string
	// join and detach are what join and detach offer
	join, detach string
}

// completedSuffixes are TestCompleteSuffixes' completions in the repository work, its
// subdirectory sub, the directory other, where the entries ended, and the root directory.
func completedSuffixes(work, sub, other string) []suffixCase {
	ended := "\tended in " + other + "\n"
	names := "a-b\t1 session\nother\t2 sessions\nwork-\t1 session\nwork\t3 sessions\n:4\n"
	running := "a-b\t1 session\nother\t1 session\nwork-\t1 session\nwork\t2 sessions\n:4\n"
	return []suffixCase{
		{work, []string{"-s", ""}, "0\tdetached\n7" + ended + "fix\tdetached\n:4\n",
			"0\tdetached\nfix\tdetached\n:4\n"},
		{sub, []string{"-s", ""}, "0\tdetached\n7" + ended + "fix\tdetached\n:4\n",
			"0\tdetached\nfix\tdetached\n:4\n"},
		{work, []string{"-s", "f"}, "fix\tdetached\n:4\n", "fix\tdetached\n:4\n"},
		{work, []string{"-s", "7"}, "7" + ended + ":4\n", ":4\n"},
		{work, []string{"-s", "-"}, ":4\n", ":4\n"},
		{work, []string{"-s", "work"}, ":4\n", ":4\n"},
		{work, []string{"-n", "other", "-s", ""}, "1\tdetached\n2" + ended + ":4\n",
			"1\tdetached\n:4\n"},
		{work, []string{"--name=a", "-s", ""}, "b-c\tdetached\n:4\n", "b-c\tdetached\n:4\n"},
		{work, []string{"-n", "a-b", "--suffix", ""}, "c\tdetached\n:4\n", "c\tdetached\n:4\n"},
		{other, []string{"-s", ""}, "1\tdetached\n2" + ended + ":4\n", "1\tdetached\n:4\n"},
		{"/", []string{"-s", ""},
			"3\tdetached\n4" + ended + "a-b-c\tdetached\nother-1\tdetached\nother-2" + ended +
				"work\tdetached\nwork--x\tdetached\nwork-0\tdetached\nwork-7" + ended +
				"work-fix\tdetached\n:4\n",
			"3\tdetached\na-b-c\tdetached\nother-1\tdetached\nwork\tdetached\nwork--x\tdetached\n" +
				"work-0\tdetached\nwork-fix\tdetached\n:4\n"},
		{work, []string{"-n", ""}, names, running},
		{work, []string{"-n", "w"}, "work-\t1 session\nwork\t3 sessions\n:4\n",
			"work-\t1 session\nwork\t2 sessions\n:4\n"},
		{work, []string{"-s", "0", "-n", ""}, names, running},
	}
}
