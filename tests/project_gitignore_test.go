package tests

import (
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The lines setup project adds to .gitignore, and what git ignores after them (decision 28.1).

// gitignoreCase is a .gitignore before setup project and after it, and the lines cld reports
// added, "" where it adds none.
type gitignoreCase struct {
	name, before, after, added string
}

// gitignoreCases are TestSetupProjectGitignore's. A line counts with its leading slash or without,
// and with a carriage return or spaces at its end, but not with a tab.
var gitignoreCases = []gitignoreCase{
	{"empty", "", ignoreLines, addedLines},
	{"no newline at the end", "dist/", "dist/\n" + ignoreLines, addedLines},
	{"carriage returns", "dist/\r\n",
		"dist/\r\n/.claude/settings.local.json\r\n/.claude/plans/\r\n/.claude/worktrees/\r\n",
		addedLines},
	{"there", "dist/\n" + ignoreLines, "", ""},
	{"in another order",
		"/.claude/worktrees/\ndist/\n/.claude/plans/\n/.claude/settings.local.json\n", "", ""},
	{"without slashes",
		".claude/settings.local.json\n.claude/plans/\n.claude/worktrees/\n", "", ""},
	{"with carriage returns",
		"/.claude/settings.local.json\r\n/.claude/plans/\r\n/.claude/worktrees/\r\n", "", ""},
	{"with spaces",
		"/.claude/settings.local.json  \n/.claude/plans/ \n/.claude/worktrees/\n", "", ""},
	{"with a tab", "/.claude/settings.local.json\t\n/.claude/plans/\n/.claude/worktrees/\n",
		"/.claude/settings.local.json\t\n/.claude/plans/\n/.claude/worktrees/\n" +
			"/.claude/settings.local.json\n",
		"/.claude/settings.local.json"},
	{"some", "/.claude/plans/\n",
		"/.claude/plans/\n/.claude/settings.local.json\n/.claude/worktrees/\n",
		"/.claude/settings.local.json, /.claude/worktrees/"},
}

// gitignoreStatus is what git status says after setup project of the files under .claude that
// TestSetupProjectGitignore writes: it ignores the local settings, the plans and the worktrees.
var gitignoreStatus = []string{
	"!! .claude/plans/plan.md",
	"!! .claude/settings.local.json",
	"!! .claude/worktrees/cld-x-0/main.go",
	"?? .claude/CLAUDE.md",
	"?? .claude/commands/review.md",
	"?? .claude/settings.json",
	"?? .claude/skills/deploy/SKILL.md",
	"?? .gitignore",
}

// .gitignore gets cld's lines where git does not read them already, in its line endings, after a
// newline where its last line has none. Each time git adds the rest of .claude.
func TestSetupProjectGitignore(t *testing.T) {
	t.Parallel()
	for _, test := range gitignoreCases {
		t.Run(test.name, test.run)
	}
}

// run runs setup project with the .gitignore before and files under .claude.
func (test gitignoreCase) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	projectWrite(t, s, ".gitignore", test.before)
	for _, name := range []string{
		"CLAUDE.md", "commands/review.md", "skills/deploy/SKILL.md", "plans/plan.md",
		"worktrees/cld-x-0/main.go",
	} {
		projectWrite(t, s, ".claude/"+name, "")
	}
	result := s.RunCld(nil, "setup", "project")
	line, after := "Left .gitignore as it was\n", test.before
	if test.added != "" {
		line, after = "Updated .gitignore: "+test.added+"\n", test.after
	}
	if result.Code != 0 || !strings.HasSuffix(result.Stdout, line) || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout ending in %q",
			result.Code, result.Stdout, result.Stderr, line)
	}
	checkFile(t, s, ".gitignore", after)
	if got := gitStatus(t, s); !slices.Equal(got, gitignoreStatus) {
		t.Errorf("git status %q, want %q", got, gitignoreStatus)
	}
}
