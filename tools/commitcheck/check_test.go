package main

import (
	"slices"
	"strings"
	"testing"
)

// dependabot is the message of Dependabot's bump of golang.org/x/text to 0.43.0, in #189.
const dependabot = `chore(deps): Bump golang.org/x/text from 0.42.0 to 0.43.0

Bumps [golang.org/x/text](https://github.com/golang/text) from 0.42.0 to 0.43.0.
- [Release notes](https://github.com/golang/text/releases)
- [Commits](https://github.com/golang/text/compare/v0.42.0...v0.43.0)

---
updated-dependencies:
- dependency-name: golang.org/x/text
  dependency-version: 0.43.0
  dependency-type: direct:production
  update-type: version-update:semver-minor
...

Signed-off-by: dependabot[bot] <support@github.com>
`

var checkTests = []struct {
	name, msg string
	want      []finding
}{
	{name: "a type alone", msg: "feat: add a command\n"},
	{name: "a scope", msg: "fix(session): keep the pane\n"},
	{name: "a breaking change", msg: "feat(cli)!: drop a flag\n\nBREAKING CHANGE: it is gone\n"},
	{name: "! without a scope", msg: "refactor!: rename\n"},
	{name: "a capital", msg: "chore(deps): Bump github.com/spf13/pflag from 1.0.9 to 1.0.10"},
	{name: "a scope of each kind of character", msg: "ci(go_1-27): bump\n"},
	{
		name: "a body", msg: "docs: say more\n\n- what was wrong\n- what changed\n\n" +
			"Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\n",
	},
	{name: "72 columns", msg: "docs: " + strings.Repeat("x", 66) + "\n\n" + strings.Repeat("y", 72)},
	{name: "72 characters of 3 bytes", msg: "docs: " + strings.Repeat("✳", 66)},
	{
		name: "73 columns", msg: "docs: " + strings.Repeat("x", 67) + "\n\n" + strings.Repeat("y", 73),
		want: []finding{
			{1, "header of 73 columns, over the cap of 72"},
			{3, "line of 73 columns, over the cap of 72"},
		},
	},
	{
		name: "72 columns and a URL",
		msg:  "docs: x\n\n" + strings.Repeat("y", 71) + " https://example.com/" + strings.Repeat("z", 40),
	},
	{
		name: "73 columns and two URLs",
		msg: "docs: x\n\nhttp://example.com/ " + strings.Repeat("y", 36) + " https://example.org/ " +
			strings.Repeat("y", 34),
		want: []finding{{3, "line of 73 columns, over the cap of 72"}},
	},
	{
		name: "a header of a URL", msg: "docs: see https://example.com/" + strings.Repeat("x", 43),
		want: []finding{{1, "header of 73 columns, over the cap of 72"}},
	},
	{name: "Dependabot's", msg: dependabot},
	{name: "an unknown type", msg: "feature: add\n", want: []finding{
		{1, "type \"feature\" is none of build, chore, ci, docs, feat, fix, perf, refactor, " +
			"style, test"},
	}},
	{name: "a capital type", msg: "Fix: it\n", want: []finding{notForm("Fix: it")}},
	{name: "a scope in capitals", msg: "fix(CLI): it\n", want: []finding{notForm("fix(CLI): it")}},
	{name: "an empty scope", msg: "fix(): it\n", want: []finding{notForm("fix(): it")}},
	{name: "no space", msg: "fix:it\n", want: []finding{notForm("fix:it")}},
	{name: "no description", msg: "fix: \n", want: []finding{notForm("fix: ")}},
	{name: "a merge's", msg: "Merge branch 'main'\n", want: []finding{notForm("Merge branch 'main'")}},
	{name: "empty", msg: "", want: []finding{notForm("")}},
	{
		name: "a body without a blank line", msg: "fix: it\n- why\n",
		want: []finding{{2, "no blank line between the header and the body"}},
	},
}

// notForm returns the finding for a header that is not of the form type(scope): description.
func notForm(header string) finding {
	return finding{1, "header \"" + header +
		"\" is not type(scope): description, of a scope of a-z, 0-9, _ and -"}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	for _, tt := range checkTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := check(tt.msg); !slices.Equal(got, tt.want) {
				t.Errorf("check(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestCheckTypes(t *testing.T) {
	t.Parallel()
	for _, typ := range types {
		if got := check(typ + ": x\n"); got != nil {
			t.Errorf("check of type %s = %v, want no finding", typ, got)
		}
	}
}
