package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/fail"
)

// check refuses, with status 2, what the command line alone refuses (decision 50.2), claudeArgs
// telling whether words for claude follow "--".
func (j *joinCommand) check(c *cobra.Command, claudeArgs bool) error {
	if err := j.names.Check(j.typed, ""); err != nil {
		return err
	}
	resuming := c.Flags().Changed("resume")
	if err := j.onlyExisting(resuming, claudeArgs); err != nil {
		return err
	}
	if err := j.together(resuming); err != nil {
		return err
	}
	// -n and -s may give the copy SESSION's own name (decision 45.1).
	if name, given := j.names.GivenName(); j.fork && given {
		return ownName(j.typed, 2, j.conversation, name)
	}
	return nil
}

// together refuses --resume's SESSION where resuming, given, is empty or starts with "-", which
// claude would read as an option, and options that cannot go together (decision 50.2). The words
// after "--" go to any conversation, resumed or not.
func (j *joinCommand) together(resuming bool) error {
	switch {
	case resuming && j.conversation == "":
		return fail.Usage("option '--resume' needs a value (see cld help)")
	case resuming && strings.HasPrefix(j.conversation, "-"):
		return fail.Usage(fmt.Sprintf("invalid SESSION '%s' for --resume: claude would read it "+
			"as an option (see cld help)", j.conversation))
	case j.fork && !resuming:
		return fail.Usage(j.typed + ": --fork needs --resume SESSION, the conversation to copy " +
			"(see cld help)")
	case j.fresh && resuming:
		return fail.Usage(j.typed + ": --new and --resume exclude each other: each says which " +
			"conversation claude starts with (see cld help)")
	case j.worktree && resuming:
		return fail.Usage(j.typed + ": -w and --resume exclude each other: claude takes a " +
			"conversation back to its worktree itself (see cld help)")
	}
	return nil
}

// onlyExisting refuses --existing without -s, as join would then create a session under the next
// index (decision 58.2). It refuses the options that go only to a session join creates or makes
// anew, and words for claude. resuming tells whether --resume was given, and claudeArgs whether
// words follow "--".
func (j *joinCommand) onlyExisting(resuming, claudeArgs bool) error {
	var option string
	switch {
	case !j.existing:
		return nil
	case !j.names.SuffixGiven():
		return fail.Usage(j.typed + ": --existing needs -s SUFFIX, the session to join " +
			"(see cld help)")
	case j.fresh:
		option = "--new"
	case resuming:
		option = "--resume"
	case j.fork:
		option = "--fork"
	case j.worktree:
		option = "-w"
	case claudeArgs:
		option = "the words after --"
	default:
		return nil
	}
	return fail.Usage(j.typed + ": --existing and " + option + " exclude each other: " +
		"--existing creates no session, and resumes one as it was (see cld help)")
}

// forkName refuses, with status 1, a copy given SESSION's own name by the name join resolved
// (decision 45.1). The repository's or directory's name, or the index, made that name.
func (j *joinCommand) forkName(name string) error {
	if !j.fork {
		return nil
	}
	return ownName(j.typed, 1, j.conversation, name)
}

// ownName refuses, with status, a --fork of conversation, --resume's SESSION, into session name
// where SESSION is the copy's own name. That name is cld-NAME, compared as claude compares names
// (decision 45.1).
func ownName(typed string, status int, conversation, name string) error {
	// claude compares names lower-cased and trimmed (docs/design/findings/claude.md).
	//nolint:staticcheck // as claude does: EqualFold would also take ſ for s
	if strings.ToLower(strings.TrimSpace(conversation)) != strings.ToLower("cld-"+name) {
		return nil
	}
	return &fail.Error{Status: status,
		Message: typed + ": --fork would give the copy SESSION's own name, cld-" + name,
		Advice:  "; give another -s SUFFIX (see cld help)"}
}
