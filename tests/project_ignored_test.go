package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What setup project says of the files a project shares under .claude that git ignores all the
// same (decisions 19.7 and 28.2).

const (
	// ignoredCreated and ignoredUpdated are what setup project reports of the settings and of
	// .gitignore.
	ignoredCreated = "Created .claude/settings.json\nCreated .claude/settings.local.json\n"
	ignoredUpdated = "Updated .gitignore: " + addedLines + "\n"
	// ignoredOthers are the paths a project shares under .claude, but for the settings.
	ignoredOthers = ".claude/commands/, .claude/agents/, .claude/skills/, .claude/rules/, " +
		".claude/hooks/ and .claude/CLAUDE.md"
	ignoredShared = ", which a project shares through git, by the pattern "
)

// ignoredCase is a .gitignore and a .git/info/exclude, whether the work directory is a git work
// tree, with git on the PATH, and what setup project does there.
type ignoredCase struct {
	name, gitignore, exclude string
	git                      bool
	code                     int
	stdout, stderr           string
}

// ignoredCases are TestSetupProjectIgnoredAllTheSame's. cld's lines before are the /.claude/* and
// !/.claude/settings.json it wrote until decision 28.1 (decision 19.5).
var ignoredCases = []ignoredCase{
	{"the directory", ".claude/\n", "", true, 1, ignoredCreated + ignoredUpdated,
		"cld: warning: git ignores " + ignoredOthers + ignoredShared + ".claude/ (.gitignore, line 1)\n" +
			"cld: git ignores .claude/settings.json all the same, by the pattern .claude/ " +
			"(.gitignore, line 1)\n"},
	{"cld's lines before", "/.claude/*\n!/.claude/settings.json\n", "", true, 0,
		ignoredCreated + ignoredUpdated,
		"cld: warning: git ignores " + ignoredOthers + ignoredShared +
			"/.claude/* (.gitignore, line 1)\n"},
	{"cld's lines before, with exceptions",
		"/.claude/*\n!/.claude/settings.json\n!/.claude/*/\n!/.claude/CLAUDE.md\n", "", true, 0,
		ignoredCreated + ignoredUpdated, ""},
	{"some of them", "skills/\n*.md\n.claude/rules\n", "", true, 0, ignoredCreated + ignoredUpdated,
		"cld: warning: git ignores .claude/skills/" + ignoredShared + "skills/ (.gitignore, line 1)\n" +
			"cld: warning: git ignores .claude/rules/" + ignoredShared +
			".claude/rules (.gitignore, line 3)\n" +
			"cld: warning: git ignores .claude/CLAUDE.md" + ignoredShared + "*.md (.gitignore, line 2)\n"},
	{"after cld's lines", ignoreLines + "*.json\n", "", true, 1,
		ignoredCreated + "Left .gitignore as it was\n",
		"cld: git ignores .claude/settings.json all the same, by the pattern *.json " +
			"(.gitignore, line 4)\n"},
	{"git's excludes", "", ".claude\n", true, 1, ignoredCreated + "Created .gitignore\n",
		"cld: warning: git ignores " + ignoredOthers + ignoredShared +
			".claude (.git/info/exclude, line 1)\n" +
			"cld: git ignores .claude/settings.json all the same, by the pattern .claude " +
			"(.git/info/exclude, line 1)\n"},
	{"no work tree", ".claude/\n", "", false, 0, ignoredCreated + ignoredUpdated, ""},
}

// Once the files are written, cld warns of each pattern by which git ignores a path a project
// shares, and fails where git ignores .claude/settings.json. It asks git nothing outside a work
// tree, or without git. A path goes as git sees it: a symbolic link as a file, a submodule whole.
func TestSetupProjectIgnoredAllTheSame(t *testing.T) {
	t.Parallel()
	for _, test := range ignoredCases {
		t.Run(test.name, test.run)
	}
	t.Run("no git", ignoredNoGit)
	t.Run("a symbolic link", ignoredSymbolicLink)
	t.Run("files added all the same", ignoredFilesAdded)
	t.Run("submodules", ignoredSubmodules)
}

// run runs setup project where git looks no further up than the sandbox for a work tree.
func (test ignoredCase) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	extra := map[string]string{"GIT_CEILING_DIRECTORIES": s.Root}
	if test.git {
		gitInit(t, s)
	}
	if test.gitignore != "" {
		projectWrite(t, s, ".gitignore", test.gitignore)
	}
	if test.exclude != "" {
		projectWrite(t, s, ".git/info/exclude", test.exclude)
	}
	result := s.RunCld(extra, "setup", "project")
	checkIgnored(t, result, test.code, test.stdout, test.stderr)
	checkFile(t, s, ".claude/settings.json", projectSettings(t, "read-only"))
}

// Without git on the PATH, cld asks nothing.
func ignoredNoGit(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	projectWrite(t, s, ".gitignore", ".claude/\n")
	result := s.RunCld(map[string]string{"PATH": s.Tools()}, "setup", "project")
	want := ignoredCreated + ignoredUpdated
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
}

// A symbolic link to a directory of skills, which git keeps as a link, goes as a file.
func ignoredSymbolicLink(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	projectWrite(t, s, ".gitignore", "/.claude/*\n!/.claude/settings.json\n*.json\n")
	skills := filepath.Join(s.Root, "skills")
	for _, dir := range []string{filepath.Join(skills, "deploy"), filepath.Join(s.Work, ".claude")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(skills, filepath.Join(s.Work, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	result := s.RunCld(map[string]string{"GIT_CEILING_DIRECTORIES": s.Root}, "setup", "project")
	want := "cld: warning: git ignores .claude/commands/, .claude/agents/, .claude/skills, " +
		".claude/rules/, .claude/hooks/ and .claude/CLAUDE.md" + ignoredShared +
		"/.claude/* (.gitignore, line 1)\n" +
		"cld: git ignores .claude/settings.json all the same, by the pattern *.json " +
		"(.gitignore, line 3)\n"
	checkIgnored(t, result, 1, ignoredCreated+ignoredUpdated, want)
}

// A directory with files the project added all the same (git add -f) is ignored all the same:
// git ignores a new file there. A file git tracks is shared, whatever pattern matches it.
func ignoredFilesAdded(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	projectWrite(t, s, ".gitignore", ".claude/\n")
	projectWrite(t, s, ".claude/settings.json", projectSettings(t, "read-only"))
	projectWrite(t, s, ".claude/CLAUDE.md", "")
	projectWrite(t, s, ".claude/skills/deploy/SKILL.md", "")
	runGit(t, s, s.Work, "add", "-f",
		".claude/settings.json", ".claude/CLAUDE.md", ".claude/skills/deploy/SKILL.md")
	result := s.RunCld(map[string]string{"GIT_CEILING_DIRECTORIES": s.Root}, "setup", "project")
	stdout := "Left .claude/settings.json as it was\nCreated .claude/settings.local.json\n" +
		ignoredUpdated
	want := "cld: warning: git ignores .claude/commands/, .claude/agents/, .claude/skills/, " +
		".claude/rules/ and .claude/hooks/" + ignoredShared + ".claude/ (.gitignore, line 1)\n"
	checkIgnored(t, result, 0, stdout, want)
}

// A submodule, checked out (the skills) or not (the agents, an empty directory), is shared
// whatever pattern matches it: its files are its own repository's. A repository in .claude that
// is no submodule (the commands) is ignored.
func ignoredSubmodules(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	projectWrite(t, s, ".gitignore", "/.claude/*\n!/.claude/settings.json\n")
	for _, name := range []string{"skills", "agents", "commands"} {
		dir := filepath.Join(s.Work, ".claude", name)
		runGit(t, s, s.Root, "init", "-q", dir)
		runGit(t, s, dir, "commit", "-q", "--allow-empty", "-m", name)
	}
	runGit(t, s, s.Work, "add", "-f", ".claude/skills", ".claude/agents")
	if err := os.RemoveAll(filepath.Join(s.Work, ".claude", "agents", ".git")); err != nil {
		t.Fatal(err)
	}
	result := s.RunCld(map[string]string{"GIT_CEILING_DIRECTORIES": s.Root}, "setup", "project")
	want := "cld: warning: git ignores .claude/commands/, .claude/rules/, .claude/hooks/ and " +
		".claude/CLAUDE.md" + ignoredShared + "/.claude/* (.gitignore, line 1)\n"
	checkIgnored(t, result, 0, ignoredCreated+ignoredUpdated, want)
}

// checkIgnored reports a setup project that did not end with code, stdout and stderr.
func checkIgnored(t *testing.T, result sandbox.Result, code int, stdout, stderr string) {
	t.Helper()
	if result.Code != code || result.Stdout != stdout || result.Stderr != stderr {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q",
			result.Code, result.Stdout, result.Stderr, code, stdout, stderr)
	}
}
