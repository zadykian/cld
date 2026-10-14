package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// maxColumns is the cap on each line of a message, the header's too, a character a column.
const maxColumns = 72

// types are the types CLAUDE.md's Commits lists.
var types = []string{"build", "chore", "ci", "docs", "feat", "fix", "perf", "refactor", "style",
	"test"}

// headerForm matches a header of the package comment's form, and captures its type.
var headerForm = regexp.MustCompile(`^([a-z]+)(?:\([a-z0-9_-]+\))?!?: \S`)

// A finding is what breaks the style in a line of a message, the header being line 1.
type finding struct {
	line int
	msg  string
}

// check returns the findings in the commit message msg.
func check(msg string) []finding {
	lines := strings.Split(strings.TrimRight(msg, "\n"), "\n")
	var findings []finding
	if m := headerForm.FindStringSubmatch(lines[0]); m == nil {
		findings = append(findings, finding{1, fmt.Sprintf(
			"header %q is not type(scope): description, of a scope of a-z, 0-9, _ and -", lines[0])})
	} else if !slices.Contains(types, m[1]) {
		findings = append(findings, finding{1, fmt.Sprintf("type %q is none of %s", m[1],
			strings.Join(types, ", "))})
	}
	if len(lines) > 1 && lines[1] != "" {
		findings = append(findings, finding{2, "no blank line between the header and the body"})
	}
	for i, line := range lines {
		if n := utf8.RuneCountInString(line); n > maxColumns {
			what := "line"
			if i == 0 {
				what = "header"
			}
			findings = append(findings, finding{i + 1,
				fmt.Sprintf("%s of %d columns, over the cap of %d", what, n, maxColumns)})
		}
	}
	return findings
}
