package cmdline

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/fail"
)

// ClaudeArguments takes the arguments of join: the words after a "--", which go to claude after
// cld's own (decision 41.1), and none before it. Among the words, it refuses the options that
// claudeOptions lists.
func ClaudeArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		args, words := atDash(c, args)
		if len(args) > 0 {
			return Unexpected(typed, args[0])
		}
		return refuseOptions(typed, words)
	}
}

// ClaudeWords is the words for claude among args, the arguments of join: those after its "--".
func ClaudeWords(c *cobra.Command, args []string) []string {
	_, words := atDash(c, args)
	return words
}

// atDash splits args, the arguments of join, at the "--" after which the words go to claude.
// pflag drops a "--" among the options, keeping its place (ArgsLenAtDash). It leaves one after
// the first argument, past which it reads no option.
func atDash(c *cobra.Command, args []string) (before, words []string) {
	dash := c.ArgsLenAtDash()
	if dash >= 0 {
		return args[:dash], args[dash:]
	}
	if dash = slices.Index(args, "--"); dash >= 0 {
		return args[:dash], args[dash+1:]
	}
	return args, nil
}

// claudeOption is one of claude's options as a word gives it (decision 41.3). The short one
// counts at the start of the word, as claude reads -xyz. The long one, or its other spelling
// alias, counts alone or before "=".
type claudeOption struct {
	short, long, alias string
	// why join refuses the option
	why string
}

// claudeOptions are the options of claude's that join refuses among the words for claude
// (decision 41.2). They are those cld gives claude itself, those that resume a conversation, and
// those with which claude would leave the session.
var claudeOptions = []claudeOption{
	{short: "-n", long: "--name",
		why: "cld gives claude the session's name, which -n and -s make"},
	{short: "-w", long: "--worktree", why: "cld gives claude --worktree with -w, before --"},
	{long: "--settings", why: "cld gives claude --settings, which this one would replace"},
	{short: "-r", long: "--resume",
		why: "cld gives claude --resume with --resume SESSION, before --"},
	{short: "-c", long: "--continue",
		why: "cld join resumes the session's conversation, or --resume SESSION"},
	{long: "--from-pr", why: "cld join resumes the session's conversation, or --resume SESSION"},
	{short: "-p", long: "--print",
		why: "claude would print its answer and exit, ending the session"},
	{long: "--bg", alias: "--background",
		why: "claude would start in the background and exit, ending the session"},
	{long: "--tmux", why: "claude would move to a tmux session of its own"},
	{long: "--teleport", why: "claude would resume a session from Claude Code on the web instead"},
	{long: "--init-only", why: "claude would run its startup hooks and exit, ending the session"},
	{long: "--rewind-files", why: "claude would restore files and exit, ending the session"},
	{short: "-h", long: "--help", why: "claude would print its help and exit, ending the session"},
	{short: "-v", long: "--version",
		why: "claude would print its version and exit, ending the session"},
}

// given reports whether word gives option o.
func (o claudeOption) given(word string) bool {
	if o.short != "" && strings.HasPrefix(word, o.short) && !strings.HasPrefix(word, "--") {
		return true
	}
	for _, long := range []string{o.long, o.alias} {
		if long != "" && (word == long || strings.HasPrefix(word, long+"=")) {
			return true
		}
	}
	return false
}

// refuseOptions refuses the first of words, the words for claude of join, that gives one of
// claudeOptions, saying why. Each word counts, after a second "--" too (decision 41.3).
func refuseOptions(typed string, words []string) error {
	for _, word := range words {
		for _, option := range claudeOptions {
			if option.given(word) {
				return fail.Usage(fmt.Sprintf("%s: '%s' after --: %s (see cld help)", typed, word,
					option.why))
			}
		}
	}
	return nil
}
