package tests

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join's -C, which runs join as if in DIR (decision 61), up to the fake tmux.

// enterCase is a join with -C, run in from, with PWD naming it where pwd is set. It makes session
// cld-NAME in dir, its home in home. Every path is under the sandbox's root, as ROOT is in args.
type enterCase struct {
	args            []string
	from            string
	pwd             bool
	name, dir, home string
}

// enterCases are TestJoinDirectory's, in the directories enterDirectories makes.
var enterCases = []enterCase{
	{[]string{"join", "-C", "ROOT/repo"}, "_", false, "repo-0", "repo", "repo"},
	{[]string{"join", "-C", "../repo/sub"}, "_", false, "repo-0", "repo/sub", "repo"},
	{[]string{"join", "-w", "-s", "x", "-C", "../repo"}, "_", false, "repo-x", "repo", "repo"},
	{[]string{"join", "-C", "../plain/api", "-s", "x"}, "_", false, "api-x", "plain/api",
		"plain/api"},
	{[]string{"join", "--resume", "SESSION", "-C", "../plain/api"}, "_", false, "api-0",
		"plain/api", "plain/api"},
	{[]string{"join", "-C", ".."}, "link/x", true, "link-0", "link", "link"},
	{[]string{"join", "-C", "../x", "-s", "y"}, "link/x", true, "x-y", "link/x", "link/x"},
	{[]string{"join", "-C", ".."}, "link/x", false, "real-0", "real", "real"},
	{[]string{"join", "-C", "ROOT/link/x/.."}, "_", false, "link-0", "link", "link"},
}

// enterDirectories makes, in s's root, the repository repo with sub in it, plain/api in none, and
// real/x, with link a symbolic link to real.
func enterDirectories(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	runGit(t, s, s.Root, "init", "-q", "repo")
	makeDirs(t, filepath.Join(s.Root, "repo", "sub"), filepath.Join(s.Root, "plain"),
		filepath.Join(s.Root, "plain", "api"), filepath.Join(s.Root, "real"),
		filepath.Join(s.Root, "real", "x"))
	if err := os.Symlink("real", filepath.Join(s.Root, "link")); err != nil {
		t.Fatal(err)
	}
}

// join -C DIR makes its session in DIR, the home and NAME's default coming from DIR, and -w's
// repository too. A relative DIR goes from the directory PWD names, ".." taken from the name, as
// cd takes it; PWD then names DIR for tmux. Without PWD, ".." leaves link/x for real.
func TestJoinDirectory(t *testing.T) {
	t.Parallel()
	for _, test := range enterCases {
		name := strings.Join(test.args, " ") + " in " + test.from
		if test.pwd {
			name += ", PWD set"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			enterDirectories(t, s)
			from, dir := filepath.Join(s.Root, test.from), filepath.Join(s.Root, test.dir)
			env := fakeTmuxEnv(s, nil)
			if test.pwd {
				env["PWD"] = from
			}
			args := withRoot(s, test.args)
			checkTitled(t, s.RunCldOnTerminalIn(from, env, args...), test.name)
			checkTmuxServer(t, s, test.name)
			record, home := s.FakeTmuxRecord(), filepath.Join(s.Root, test.home)
			if record.Cwd != dir || record.Env["PWD"] != dir ||
				!optionFollowedBy(record.Argv, "-c", dir) ||
				!optionFollowedBy(record.Argv, "@cld-home", home) {
				t.Errorf("tmux in %s, PWD %q, arguments %q; want it in %s, -c there, @cld-home %s",
					record.Cwd, record.Env["PWD"], record.Argv, dir, home)
			}
			if got, want := readEntry(s, test.name), entry(test.name, dir, ""); got != want {
				t.Errorf("%s's entry %q, want %q", test.name, got, want)
			}
		})
	}
}

// join refuses a DIR it cannot enter with status 1, naming it as given, once the options are
// checked (decision 61.2). It makes no session, leaves no socket and changes nothing in the
// record, whether it would have made a session, resumed e or taken an index. ~ is a name.
func TestJoinCannotEnterDirectory(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"join", "-C", "missing"}, "missing: no such file or directory"},
		{[]string{"join", "-s", "x", "-C", "missing/"}, "missing/: no such file or directory"},
		{[]string{"join", "--resume", "SESSION", "-C", "ROOT/missing"},
			"ROOT/missing: no such file or directory"},
		{[]string{"join", "-w", "-C", "file"}, "file: not a directory"},
		{[]string{"join", "-C", "file/sub", "-s", "e"}, "file/sub: not a directory"},
		{[]string{"join", "-C", "~"}, "~: no such file or directory"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s)
			s.WriteFile(filepath.Join(s.Work, "file"), "")
			writeEntry(t, s, "e", s.Work, firstID)
			home, work := tree(t, s.Home), tree(t, s.Work)
			result := s.RunCldOnTerminal(fakeTmuxEnv(s, nil), withRoot(s, test.args)...)
			want := "cld: cannot enter " + strings.ReplaceAll(test.want, "ROOT", s.Root) + "\n"
			checkFailed(t, result, 1, want)
			if fakeTmuxRan(s) {
				t.Error("cld handed over to tmux")
			}
			if servers := s.Servers(); len(servers) > 0 {
				t.Errorf("sockets %q, want none", servers)
			}
			if after := tree(t, s.Home); !maps.Equal(after, home) {
				t.Errorf("the home directory holds %q, was %q", after, home)
			}
			if after := tree(t, s.Work); !maps.Equal(after, work) {
				t.Errorf("the work directory holds %q, was %q", after, work)
			}
		})
	}
}
