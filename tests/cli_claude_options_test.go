package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// claude's options that join refuses after "--".

// Why join refuses each of claude's options.
const (
	whyNaming     = "cld gives claude the session's name, which -n and -s make"
	whyWorktree   = "cld gives claude --worktree with -w, before --"
	whyResumes    = "cld gives claude --resume with --resume SESSION, before --"
	whyResumed    = "cld join resumes the session's conversation, or --resume SESSION"
	whySettings   = "cld gives claude --settings, which this one would replace"
	whyAnswer     = "claude would print its answer and exit, ending the session"
	whyBackground = "claude would start in the background and exit, ending the session"
	whyTmux       = "claude would move to a tmux session of its own"
	whyTeleport   = "claude would resume a session from Claude Code on the web instead"
	whyInitOnly   = "claude would run its startup hooks and exit, ending the session"
	whyRewind     = "claude would restore files and exit, ending the session"
	whyHelp       = "claude would print its help and exit, ending the session"
	whyVersion    = "claude would print its version and exit, ending the session"
)

// claudeOptionCases are join's command lines with an option of claude's that join refuses.
var claudeOptionCases = []struct {
	args []string
	// word is the word refused, and why the reason
	word, why string
}{
	{[]string{"join", "--", "-n", "x"}, "-n", whyNaming},
	{[]string{"join", "--", "-nx"}, "-nx", whyNaming},
	{[]string{"join", "--", "--name", "x"}, "--name", whyNaming},
	{[]string{"join", "--", "--name=x"}, "--name=x", whyNaming},
	{[]string{"join", "-s", "x", "--", "-n", "x"}, "-n", whyNaming},
	{[]string{"join", "--", "-w"}, "-w", whyWorktree},
	{[]string{"join", "-w", "--", "--worktree=y"}, "--worktree=y", whyWorktree},
	{[]string{"join", "-s", "x", "--", "--worktree"}, "--worktree", whyWorktree},
	{[]string{"join", "--resume", "a", "--", "-wy"}, "-wy", whyWorktree},
	{[]string{"join", "--", "-r", "x"}, "-r", whyResumes},
	{[]string{"join", "--", "--resume=x"}, "--resume=x", whyResumes},
	{[]string{"join", "--", "-c"}, "-c", whyResumed},
	{[]string{"join", "--new", "--", "--continue"}, "--continue", whyResumed},
	{[]string{"join", "-s", "x", "--", "-r", "y"}, "-r", whyResumes},
	{[]string{"join", "--resume", "a", "--", "--resume"}, "--resume", whyResumes},
	{[]string{"join", "-s", "x", "--", "-c"}, "-c", whyResumed},
	{[]string{"join", "--detach-others", "--", "--continue"}, "--continue", whyResumed},
	{[]string{"join", "--", "--from-pr", "12"}, "--from-pr", whyResumed},
	{[]string{"join", "-s", "x", "--", "--from-pr=12"}, "--from-pr=12", whyResumed},
	{[]string{"join", "--", "--settings", "{}"}, "--settings", whySettings},
	{[]string{"join", "--resume", "a", "--", "--settings=s.json"}, "--settings=s.json", whySettings},
	{[]string{"join", "--", "-p", "hi"}, "-p", whyAnswer},
	{[]string{"join", "--", "--print"}, "--print", whyAnswer},
	{[]string{"join", "--", "--model", "opus", "-pc"}, "-pc", whyAnswer},
	{[]string{"join", "-n", "x", "--", "-p"}, "-p", whyAnswer},
	{[]string{"join", "--", "--bg"}, "--bg", whyBackground},
	{[]string{"join", "--", "--background"}, "--background", whyBackground},
	{[]string{"join", "--resume", "a", "--", "--bg=1"}, "--bg=1", whyBackground},
	{[]string{"join", "-w", "--", "--tmux"}, "--tmux", whyTmux},
	{[]string{"join", "--", "--tmux=classic"}, "--tmux=classic", whyTmux},
	{[]string{"join", "--", "--teleport"}, "--teleport", whyTeleport},
	{[]string{"join", "-s", "x", "--", "--teleport=id"}, "--teleport=id", whyTeleport},
	{[]string{"join", "--", "--init-only"}, "--init-only", whyInitOnly},
	{[]string{"join", "--resume", "a", "--", "--rewind-files", "id"}, "--rewind-files", whyRewind},
	{[]string{"join", "--", "--rewind-files=id"}, "--rewind-files=id", whyRewind},
	{[]string{"join", "--", "-h"}, "-h", whyHelp},
	{[]string{"join", "--", "--help"}, "--help", whyHelp},
	{[]string{"join", "--resume", "a", "--", "-v"}, "-v", whyVersion},
	{[]string{"join", "--", "--version"}, "--version", whyVersion},
	// The first word refused decides, wherever it comes.
	{[]string{"join", "--", "--append-system-prompt", "-p", "-n", "x"}, "-p", whyAnswer},
	{[]string{"join", "--", "prompt", "--", "--tmux"}, "--tmux", whyTmux},
	{[]string{"join", "--resume", "a", "--", "--", "--bg"}, "--bg", whyBackground},
}

// join refuses claude's options after "--" that undo what cld gives claude, resume a conversation
// or leave the session, naming each and why (decisions 41.2 and 41.3). It refuses them
// whatever join would do with the session, with status 2 and before looking for any tool: the
// PATH has none here.
func TestRefusesClaudeOptions(t *testing.T) {
	t.Parallel()
	for _, test := range claudeOptionCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			want := "cld: " + test.args[0] + ": '" + test.word + "' after --: " + test.why +
				" (see cld help)\n"
			checkFailed(t, s.RunCld(map[string]string{"PATH": s.Tools()}, test.args...), 2, want)
		})
	}
}
