package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// NAME's default is the name of the directory holding a repository's shared .git, or of a bare
// repository's or a submodule's git directory (decision 24.3). Outside a work tree the directory's
// name, as PWD names it, stands for it, made a NAME (decision 24.4). The fake tmux finds no
// server, so the second join -s fix brings back the session the first made.
func TestNamePrefixes(t *testing.T) {
	t.Parallel()
	for _, test := range namePrefixCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			dir := test.directory(t, s)
			fake := map[string]string{
				"PATH": filepath.Dir(sandbox.FakeTmux) + string(os.PathListSeparator) +
					s.Env["PATH"],
				"CLD_FAKE_TMUX_VERSION": "tmux 3.7c",
			}
			if test.pwd {
				fake["PWD"] = dir
			}
			for _, command := range []struct {
				args []string
				want string
			}{
				{[]string{"join"}, "cld-" + test.prefix + "0"},
				{[]string{"join", "-s", "fix"}, "cld-" + test.prefix + "fix"},
				{[]string{"join", "-s", "fix"}, "cld-" + test.prefix + "fix"},
			} {
				checkPrefixedJoin(t, s, dir, fake, command.args, command.want)
			}
		})
	}
}

// namePrefixCase is a case of TestNamePrefixes.
type namePrefixCase struct {
	name string
	// setup makes what cld runs in and returns the directory: by default git init makes the
	// repository name, and cld runs there
	setup func(t *testing.T, s *sandbox.Sandbox) string
	// pwd is whether cld gets PWD, naming that directory
	pwd bool
	// prefix is what the NAME starts with, before the index or SUFFIX
	prefix string
}

var namePrefixCases = []namePrefixCase{
	{name: "work", prefix: "work-"},
	{name: "work/sub/dir", prefix: "work-"},
	{name: "work/.git", prefix: "git-"},
	{name: "my.site", prefix: "my-site-"},
	{name: ".dotfiles", prefix: "dotfiles-"},
	{name: "a  b", prefix: "a-b-"},
	{name: "café", prefix: "caf-"},
	{name: "__x-", prefix: "x-"},
	{name: "Repo_1", prefix: "Repo_1-"},
	{name: "日本", prefix: ""},
	{name: "a linked worktree", setup: linkedWorktreeDir, prefix: "work-"},
	{name: "a bare repository's worktree", setup: bareWorktreeDir, prefix: "bare-"},
	{name: "a submodule", setup: submoduleDir, prefix: "module-x-"},
	{name: "a directory", setup: inPlainDirectory("plain"), prefix: "plain-"},
	{name: "a directory in a directory", setup: inPlainDirectory("plain/.my site"),
		prefix: "my-site-"},
	{name: "a directory named in another script", setup: inPlainDirectory("日本"), prefix: ""},
	{name: "the root directory", setup: inRootDirectory, prefix: ""},
	{name: "a symbolic link", setup: linkToTarget, pwd: true, prefix: "link-"},
	{name: "a symbolic link, without PWD", setup: linkToTarget, prefix: "target-"},
}

// directory makes what cld runs in, and returns the directory.
func (c namePrefixCase) directory(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	var dir string
	if c.setup != nil {
		dir = c.setup(t, s)
	} else {
		repository, _, _ := strings.Cut(c.name, "/")
		runGit(t, s, s.Root, "init", "-q", repository)
		dir = filepath.Join(s.Root, c.name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// checkPrefixedJoin runs cld with args in dir, against the fake tmux, and checks that cld names
// session want in its title and to tmux.
func checkPrefixedJoin(
	t *testing.T, s *sandbox.Sandbox, dir string, fake map[string]string, args []string,
	want string,
) {
	t.Helper()
	result := s.RunCldOnTerminalIn(dir, fake, args...)
	command := strings.Join(args, " ")
	title := "\x1b]0;✳ " + want + "\x07"
	if result.Code != 0 || result.Stdout != title || result.Stderr != "" {
		t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			command, result.Code, result.Stdout, result.Stderr, title)
		return
	}
	if argv := s.FakeTmuxRecord().Argv; !slices.Equal(argv[:3], []string{"-u", "-L", want}) {
		t.Errorf("%s: tmux arguments start %q, want -u -L %s", command, argv[:3], want)
	}
}

// committedWork makes the repository work with a commit, from which a worktree can branch, and
// returns its directory.
func committedWork(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	dir := repository(t, s, "work")
	runGit(t, s, dir, "commit", "-q", "--allow-empty", "-m", "first")
	return dir
}

// plainDirectory makes the directory name in the sandbox's root, in no repository.
func plainDirectory(t *testing.T, s *sandbox.Sandbox, name string) string {
	t.Helper()
	dir := filepath.Join(s.Root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// inPlainDirectory is a setup that makes the directory name, in no repository.
func inPlainDirectory(name string) func(*testing.T, *sandbox.Sandbox) string {
	return func(t *testing.T, s *sandbox.Sandbox) string {
		t.Helper()
		return plainDirectory(t, s, name)
	}
}

// inRootDirectory is a setup that runs cld in the root directory.
func inRootDirectory(t *testing.T, _ *sandbox.Sandbox) string {
	t.Helper()
	return "/"
}

// linkToTarget makes a symbolic link, link in the sandbox's root, to the directory target.
func linkToTarget(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	path := filepath.Join(s.Root, "link")
	if err := os.Symlink(plainDirectory(t, s, "target"), path); err != nil {
		t.Fatal(err)
	}
	return path
}

// linkedWorktreeDir makes a worktree of work as claude makes one, and returns a directory in it.
func linkedWorktreeDir(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	work := committedWork(t, s)
	runGit(t, s, work, "worktree", "add", "-q", "-b", "worktree-cld-x",
		".claude/worktrees/cld-x")
	return filepath.Join(work, ".claude", "worktrees", "cld-x", "sub")
}

// bareWorktreeDir makes the worktree tree of a bare repository, bare.git, and returns it.
func bareWorktreeDir(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	runGit(t, s, s.Root, "clone", "-q", "--bare", committedWork(t, s), "bare.git")
	runGit(t, s, filepath.Join(s.Root, "bare.git"), "worktree", "add", "-q",
		filepath.Join(s.Root, "tree"))
	return filepath.Join(s.Root, "tree")
}

// submoduleDir makes work the submodule module.x of a repository super, and returns its directory.
func submoduleDir(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	work := committedWork(t, s)
	runGit(t, s, s.Root, "init", "-q", "super")
	runGit(t, s, filepath.Join(s.Root, "super"), "-c", "protocol.file.allow=always",
		"submodule", "add", "-q", work, "module.x")
	return filepath.Join(s.Root, "super", "module.x")
}
