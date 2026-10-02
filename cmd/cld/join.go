package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/cmd/cld/internal/idle"
	"github.com/zadykian/cld/cmd/cld/internal/naming"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/session"
)

// joinUse names "[flags]" before ARGS, where cobra would add it at the end, and leaves --new,
// --fork and --detach-others to it, for 80 columns.
const joinUse = "join [-n NAME] [-s SUFFIX] [-w] [--resume SESSION] [flags] [-- ARGS...]"

// joinLong is join's help, which says what join does by the session's state (decision 50.6).
const joinLong = `attach to session NAME-SUFFIX where it runs, beside any terminal attached to
it already: each shows claude, whose window takes the size of the terminal used
last; with --detach-others, those terminals are detached. Where the session has
ended, claude first resumes the conversation it had last, by the ID cld keeps
of it, or else by the name cld-NAME-SUFFIX, in the directory it ran in; with
--new, it starts a new one here instead. Where there is no such session, join
creates it in the current directory. NAME is by default the name of the git
repository the directory is in, or else of the directory itself; without -s,
SUFFIX is INDEX, 0 or, where sessions NAME-INDEX run or have ended within 30
days, one above the highest of their INDEX, and join creates the session. Where
the directory's name leaves nothing, as in /, the session is SUFFIX alone. NAME
and SUFFIX consist of letters, digits, "_" and "-", each starting with a letter
or digit, and make 64 characters at most. Without -n, a running session made in
another repository or directory of the same name is refused. Without -s, join
also ends the sessions idle for longer than $CLD_IDLE_DAYS days, as cld list
does, once it has taken its INDEX.

With --resume, claude resumes SESSION in a session join creates, in the current
directory: whatever claude --resume takes, such as a session ID, a name, or a
search term for claude's picker. The conversation resumed takes the session's
name, cld-NAME-SUFFIX, for good. By name, claude resumes the one conversation
of that name in the directory or any checkout of its git repository; with none
or several, it opens its picker, searching for the name: pick one there, or
give its session ID to --resume; Ctrl+R, once Enter has left the picker's
search box, renames the one selected. With --fork, claude resumes a copy of
SESSION under a new session ID, named after the session, and SESSION keeps its
own name: --fork needs --resume SESSION, other than cld-NAME-SUFFIX.

-w, --new, --resume, --fork and ARGS go to the session join creates or brings
back: where the session runs, they would be lost, and join refuses them. -w
goes only to a new conversation, as claude takes one it resumes back to its
worktree itself: where the session has ended, it needs --new, and it excludes
--resume, as --new does.

In a session - ! cld join in claude, or a shell on the session's tmux server -
join moves the terminal attached to that session, or of several the one used
last, as cld detach finds it: the terminal leaves the session, which runs on,
and runs cld join with the same words in the directory join ran in, with its
own environment, where cld join says what it refuses then. With no terminal
attached, join refuses.

ARGS, after --, go to claude after cld's own arguments: claude's options, such
as --model opus, and a prompt to start with. cld refuses the options it gives
claude itself: -n, --name, -w, --worktree and --settings; those that resume a
conversation, which join does: -r, --resume, -c, --continue and --from-pr; and
those with which claude would leave the session: -p, --print, --bg,
--background, --tmux, --teleport, --init-only, --rewind-files, -h, --help, -v
and --version. A resumed conversation does not keep --mcp-config, --plugin-dir,
--add-dir and --fallback-model, which Claude Code's docs say to give again.`

// joinCommand is cld join, typed as typed with words after it, and its options as cobra reads
// them.
type joinCommand struct {
	typed string
	words []string
	names naming.Options

	worktree, fresh, fork, detachOthers bool
	conversation                        string
	// The command a terminal runs as it moves gives these, not the user (decisions 51.3 and
	// 51.4).
	switchedFrom, moved string
}

// newJoin is cld join (decision 50), typed as typed with words after it.
func newJoin(typed string, words []string) *cobra.Command {
	j := &joinCommand{typed: typed, words: words}
	join := &cobra.Command{
		Use:                   joinUse,
		Short:                 "attach to session NAME-SUFFIX, creating or resuming it first",
		Long:                  joinLong,
		Args:                  cmdline.ClaudeArguments(typed),
		DisableFlagsInUseLine: true,
		RunE:                  j.run,
	}
	j.names = naming.Add(join, "the session's `SUFFIX`, after NAME-: by default the index\n"+
		"above the highest of the sessions NAME-INDEX, or 0")
	flags := join.Flags()
	flags.BoolVarP(&j.worktree, "worktree", "w", false,
		"create the session with claude in git worktree\n"+
			"cld-NAME-SUFFIX, which claude makes from HEAD or\n"+
			"reopens (claude --worktree cld-NAME-SUFFIX)")
	flags.BoolVar(&j.fresh, "new", false, "create a session that has ended anew, with a new\n"+
		"conversation, rather than resume its own")
	flags.StringVar(&j.conversation, "resume", "",
		"create the session with claude resuming `SESSION`,\n"+
			"a session ID, a name or a search term (claude\n--resume SESSION)")
	flags.BoolVar(&j.fork, "fork", false, "with --resume, resume a copy of SESSION under a new\n"+
		"session ID, leaving SESSION as it is (claude\n--fork-session)")
	flags.BoolVar(&j.detachOthers, "detach-others", false,
		"detach any other terminal attached to the session")
	flags.StringVar(&j.switchedFrom, "switched-from", "", "")
	flags.StringVar(&j.moved, "moved", "", "")
	cmdline.Hide(flags, "switched-from", "moved")
	cmdline.CompleteWith(join, "name", naming.Names(true))
	cmdline.CompleteWith(join, "suffix", naming.Suffixes(true))
	return join
}

// run is join's RunE, whose steps go in the order of decision 50.3.
func (j *joinCommand) run(c *cobra.Command, args []string) error {
	if c.Flags().Changed("switched-from") && !session.ValidName(j.switchedFrom) {
		return fail.Usage(fmt.Sprintf("invalid session '%s' for --switched-from (see cld help)",
			j.switchedFrom))
	}
	if c.Flags().Changed("moved") {
		return j.runMoved(c, args)
	}
	if err := j.check(c); err != nil {
		return err
	}
	claudeWords := cmdline.ClaudeWords(c, args)
	joining := session.Joining{DetachOthers: j.detachOthers, Worktree: j.worktree, New: j.fresh,
		Conversation: j.conversation, Fork: j.fork, Args: claudeWords,
		SwitchedFrom: j.switchedFrom, Typed: j.words}
	// Without -s, join creates the session under the next index and sweeps every server; with
	// -s it reads its session's server only (decision 50.3).
	next := !j.names.SuffixGiven()
	var limit time.Duration
	if next {
		var err error
		if limit, err = idle.Limit(); err != nil {
			return err
		}
	}
	var tools []string
	if j.worktree {
		tools = append(tools, "git")
	}
	tmux, err := session.Check(tools...)
	if err != nil {
		return err
	}
	// kept is what an outer tmux keeps from claude (decision 43), and sw the terminal to move
	// where join runs in a pane of cld's servers (decision 51.5).
	kept, sw := tmux.ReadyClient()
	switch {
	case !next:
		return j.joinSuffix(tmux, joining, kept, sw)
	case sw != nil:
		// Moved, the terminal's cld join takes the index, and checks the claude it starts.
		return tmux.SwitchJoin(sw, joining)
	}
	return j.create(tmux, joining, kept, limit)
}

// runMoved runs join as a join in a pane was given it, in that join's directory (see
// session.Moved): the terminal that join moved runs this.
func (j *joinCommand) runMoved(c *cobra.Command, args []string) error {
	flags := c.Flags()
	if len(args) > 0 || flags.NFlag() > 2 || flags.NFlag() == 2 && !flags.Changed("switched-from") {
		return fail.Usage(j.typed + ": --moved goes with --switched-from alone (see cld help)")
	}
	words, err := session.Moved(j.moved)
	if err != nil {
		return err
	}
	if j.switchedFrom != "" {
		words = append([]string{"--switched-from", j.switchedFrom}, words...)
	}
	return run(append([]string{"join"}, words...))
}

// joinSuffix joins the session that -s names, which tmux.Join looks up under the record's lock.
func (j *joinCommand) joinSuffix(tmux *session.Tmux, joining session.Joining, kept []string,
	sw *session.Switch) error {
	suffix, home, err := j.names.Resolve(tmux)
	if err != nil {
		return err
	}
	if err := j.forkName(suffix); err != nil {
		return err
	}
	joining.Home = home
	return tmux.Join(suffix, joining, kept, sw)
}

// create makes the session under the next index, holding the record's lock from the name to tmux
// (see session.Lock), once claude's version passes (decision 6).
func (j *joinCommand) create(tmux *session.Tmux, joining session.Joining, kept []string,
	limit time.Duration) error {
	claude, err := session.CheckClaude()
	if err != nil {
		return err
	}
	unlock := session.Lock()
	defer unlock()
	suffix, _, err := j.names.Resolve(tmux)
	if err != nil {
		return err
	}
	if err := j.forkName(suffix); err != nil {
		return err
	}
	// The sweep comes once the index is taken, so the session made does not take the name of one
	// just ended. A read that fails is only a warning (decisions 46.2 and 50.5).
	if limit > 0 {
		if sessions, err := tmux.Sessions(context.Background()); err != nil {
			output.Warn("cannot end the idle sessions: " + err.Error())
		} else {
			idle.Sweep(tmux, sessions, limit, j.switchedFrom)
		}
	}
	return tmux.Create(claude, suffix, joining, kept)
}
