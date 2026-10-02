package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/completion"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/picker"
	"github.com/zadykian/cld/internal/project"
	"github.com/zadykian/cld/internal/restore"
	"github.com/zadykian/cld/internal/session"
	"github.com/zadykian/cld/internal/telemetry"
	"github.com/zadykian/cld/internal/update"
)

// commands maps each command, and each alias of one, to the command it runs: cld's, cobra's
// completion, and the hidden commands through which cobra's completion scripts ask cld what to
// offer, on every TAB.
var commands = map[string]string{
	"join": "join", "detach": "detach", "kill": "kill", "list": "list", "restore": "restore", "setup": "setup",
	"update": "update", "completion": "completion",
	"help": "help", "-h": "help", "--help": "help",
	"version": "version", "-V": "version", "--version": "version",
	cobra.ShellCompRequestCmd: cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd: cobra.ShellCompNoDescRequestCmd,
}

// run runs cld with the arguments args. The first is the command, which run checks before cobra
// sees it: cobra would take an unknown one for an argument of cld itself, and skip options before
// the command (cld -n x join would run join -n x). setup has commands of its own, and the argument
// after it is checked the same way (see setupCommand) where setup runs: __complete setup, which
// runs nothing, completes it instead. Any other word is an unknown command, the new and resume of
// cld 0.10.0 and earlier among them, which join has taken over, and a session's name, which "cld
// NAME" took before cld had commands.
func run(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fail.Usage("missing command: cld join attaches to a session, creating or resuming it first (see cld help)")
	}
	typed := args[0]
	command, known := commands[typed]
	if !known {
		return fail.Usage(fmt.Sprintf("unknown command '%s' (see cld help)", typed))
	}
	if command == "setup" {
		if err := setupCommand(args[1:]); err != nil {
			return err
		}
	}
	completing := command == cobra.ShellCompRequestCmd || command == cobra.ShellCompNoDescRequestCmd
	// The completion scripts always pass the word being completed, empty or not; without one,
	// cobra would fail with a message of its own and status 1.
	if completing && len(args) == 1 {
		return fail.Usage(fmt.Sprintf("%s: missing the word to complete (see cld help)", typed))
	}
	// What cobra prints - the help, the completion scripts, the answers to __complete - goes to
	// out, which cld then prints as its own output, so that a write that fails ends cld with
	// status 1 and cld's message. cobra's help function and __complete drop the error, which would
	// end cld with status 0, and the commands that print the scripts return it in Go's words
	// ("write /dev/stdout: ..."). Nothing is written when there is nothing to print, as for kill,
	// since even an empty write to a stdout that cannot take one fails.
	var out bytes.Buffer
	root := commandLine(typed, args[1:], &out)
	root.SetArgs(append([]string{command}, args[1:]...))
	if err := root.Execute(); err != nil {
		return err
	}
	if out.Len() == 0 {
		return nil
	}
	text := out.String()
	if completing {
		text = noFiles(text)
	}
	return output.Print(text)
}

// noFiles turns cobra's answer to __complete, text, into one that offers no file names where
// cobra's lets the shell offer them: cobra answers ShellCompDirectiveDefault, ":0" on the last
// line, where it cannot read the words before the one completed - an unknown command, an option
// the command does not have - whatever the root's default directive. No argument of cld's is a
// file there either.
func noFiles(text string) string {
	if rest, found := strings.CutSuffix(text, ":0\n"); found && (rest == "" || strings.HasSuffix(rest, "\n")) {
		return fmt.Sprintf("%s:%d\n", rest, cobra.ShellCompDirectiveNoFileComp)
	}
	return text
}

// setupCommand checks the argument after setup before cobra sees it, as run checks the first: it
// names one of setup's commands, project, telemetry, completion or restore, or is -h or
// --help, setup's help. cobra would run telemetry for cld setup --local URL telemetry, taking
// --local for an option of setup's. completion has commands of its own, one for each shell, and
// the argument after it is checked the same way: cobra would run zsh's for setup completion
// --help=false zsh.
func setupCommand(args []string) error {
	const hint = "cld setup project, cld setup telemetry, cld setup completion SHELL or cld setup restore (see cld help)"
	switch {
	case len(args) == 0 || args[0] == "":
		return fail.Usage("setup: missing command: " + hint)
	case args[0] == "completion":
		return shellArgument(args[1:])
	case args[0] == "project" || args[0] == "telemetry" || args[0] == "restore" || args[0] == "-h" || args[0] == "--help":
		return nil
	}
	return fail.Usage(fmt.Sprintf("setup: unknown command '%s': %s", args[0], hint))
}

// shellArgument checks the argument after setup completion, as setupCommand checks the one after
// setup: one of the shells, or -h or --help, the help of setup completion.
func shellArgument(args []string) error {
	const hint = "bash, zsh or fish (see cld help)"
	switch {
	case len(args) == 0 || args[0] == "":
		return fail.Usage("setup completion: missing shell: " + hint)
	case slices.Contains(completion.Shells, args[0]) || args[0] == "-h" || args[0] == "--help":
		return nil
	}
	return fail.Usage(fmt.Sprintf("setup completion: unknown shell '%s': %s", args[0], hint))
}

// commandLine is cld's commands, for a command typed as typed, with words after it: the messages
// name it that way, "-V" for version, say. What cobra prints goes to out.
//
// The help is cobra's, from its default templates: each command's Use, and its Long or else its
// Short, then its options with their usages, which name their value in backquotes (`NAME`). Its
// text is here and nowhere else; cobra wraps none of it, so the lines break by hand, within 80
// columns. help, -h and --help print it to out, which run prints through output.Print, and it lists
// the commands in the order they are added here rather than by name.
//
// cobra's defaults give way to cld's command line. main prints the errors, as "cld: MESSAGE". help
// takes one of cld's commands at most - with setup, one of setup's after it, and a shell after
// setup completion - and version is a command rather than cobra's --version and -v. Every command
// reads its options up to the first argument, which pflag would otherwise pass over, and takes no
// argument but help's COMMAND and completion's SHELL, and join's words for claude after a "--"
// (see claudeArguments): the first one left - after COMMAND, the next - or a "--" elsewhere, which
// pflag would drop, is refused.
//
// Completion is cobra's: completion SHELL prints the script, which asks __complete what to offer
// on every TAB - bash's with lines of cld's for ble.sh (see bashScript) - and setup completion
// SHELL writes it where the shell reads it (see setupCompletion). join -n offers the NAME of
// NAME-SUFFIX for the sessions list shows (see sessionNames), and detach -n for those that run,
// their -s the SUFFIX (see sessionSuffixes), help the commands (see commandNames), setup project
// --mcp the MCP servers (see serverNames) and --permissions its sets (see permissionSets), and
// nothing offers file names, as no argument of cld's is a file.
//
// join, detach and kill name their session NAME-SUFFIX with -n NAME and -s SUFFIX (see naming):
// NAME defaults to the repository's or directory's name, and SUFFIX, for join, to the next index;
// kill needs -s, and detach too but in one of cld's servers (see session.Inside). In a pane of one
// of cld's servers, join moves the terminal on that session instead of attaching this one: the
// terminal runs cld join with the same words (see session.Switch).
func commandLine(typed string, words []string, out io.Writer) *cobra.Command {
	cobra.EnableCommandSorting = false // The commands in the order they are added.
	root := &cobra.Command{
		Use: "cld",
		Long: `Run Claude Code in named sessions, each on a private tmux server that ignores
~/.tmux.conf. A session is named NAME-SUFFIX: NAME is by default the name of
the git repository the current directory is in, or else of the directory, and
SUFFIX, for a new session, the next index. Session S is the tmux session
"cld-S" on the server "tmux -L cld-S", running "claude --name cld-S" with
claude's agent view (/bg) off. What claude starts through tmux runs on that
server too, and ends with it.

Detach with C-q d, or ! cld detach in claude where the terminal keeps C-q from
tmux; C-q C-q sends C-q to claude. C-q s shows cld list over the session, where
Enter moves the terminal to the session picked, and Esc closes it; C-q ( and
C-q ) move the terminal to the previous and the next session, C-q L back to
the one it came from, and ! cld join in claude to the session it names. A
session whose claude fails stays, showing why, until cld kill ends it, or it
has been idle for longer than $CLD_IDLE_DAYS days (see cld help list).`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	// Before completion is made: its commands write their scripts to the output the root has then.
	root.SetOut(out)
	root.CompletionOptions.SetDefaultShellCompDirective(cobra.ShellCompDirectiveNoFileComp)
	root.SetFlagErrorFunc(flagError(typed))

	// join's usage line names "[flags]" before ARGS, where cobra would add it at the end, and
	// leaves --new, --fork and --detach-others to it, for 80 columns.
	join := &cobra.Command{
		Use:   "join [-n NAME] [-s SUFFIX] [-w] [--resume SESSION] [flags] [-- ARGS...]",
		Short: "attach to session NAME-SUFFIX, creating or resuming it first",
		Long: `attach to session NAME-SUFFIX where it runs, beside any terminal attached to
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
--add-dir and --fallback-model, which Claude Code's docs say to give again.`,
		Args:                  claudeArguments(typed),
		DisableFlagsInUseLine: true,
	}
	joinNaming := addNaming(join, "the session's `SUFFIX`, after NAME-: by default the index\n"+
		"above the highest of the sessions NAME-INDEX, or 0")
	worktree := join.Flags().BoolP("worktree", "w", false,
		"create the session with claude in git worktree\ncld-NAME-SUFFIX, which claude makes from HEAD or\nreopens (claude --worktree cld-NAME-SUFFIX)")
	fresh := join.Flags().Bool("new", false, "create a session that has ended anew, with a new\nconversation, rather than resume its own")
	conversation := join.Flags().String("resume", "", "create the session with claude resuming `SESSION`,\n"+
		"a session ID, a name or a search term (claude\n--resume SESSION)")
	fork := join.Flags().Bool("fork", false, "with --resume, resume a copy of SESSION under a new\n"+
		"session ID, leaving SESSION as it is (claude\n--fork-session)")
	detachOthers := join.Flags().Bool("detach-others", false, "detach any other terminal attached to the session")
	// The command a terminal runs as it moves gives these, not the user (see session.Switch): the
	// session a switch moved the terminal from, which the session joined records for C-q L, and,
	// where join ran in a pane, the directory it ran in and the words it was given, encoded.
	switchedFrom := join.Flags().String("switched-from", "", "")
	moved := join.Flags().String("moved", "", "")
	for _, hidden := range []string{"switched-from", "moved"} {
		if err := join.Flags().MarkHidden(hidden); err != nil {
			panic(err)
		}
	}
	join.RunE = func(c *cobra.Command, args []string) error {
		if c.Flags().Changed("switched-from") && !session.ValidName(*switchedFrom) {
			return fail.Usage(fmt.Sprintf("invalid session '%s' for --switched-from (see cld help)", *switchedFrom))
		}
		// The terminal that a join in a pane moved runs join as that join was given it, in its
		// directory (see session.Moved): run parses the words anew.
		if c.Flags().Changed("moved") {
			if len(args) > 0 || c.Flags().NFlag() > 2 || c.Flags().NFlag() == 2 && !c.Flags().Changed("switched-from") {
				return fail.Usage(typed + ": --moved goes with --switched-from alone (see cld help)")
			}
			words, err := session.Moved(*moved)
			if err != nil {
				return err
			}
			if *switchedFrom != "" {
				words = append([]string{"--switched-from", *switchedFrom}, words...)
			}
			return run(append([]string{"join"}, words...))
		}
		if err := joinNaming.check(typed, ""); err != nil {
			return err
		}
		resuming := c.Flags().Changed("resume")
		if err := joinOptions(typed, resuming, *conversation, *fork, *fresh, *worktree); err != nil {
			return err
		}
		// A copy of the session's own conversation would take its name too: a resume by that name
		// would find two conversations of it, and open claude's picker. So would a copy of SESSION
		// where SESSION is that name - with -n and -s a mistake on the command line alone, and
		// otherwise one the repository's or directory's name, or the index, makes (see ownName).
		if name, given := joinNaming.givenName(); *fork && given {
			if err := ownName(typed, 2, *conversation, name); err != nil {
				return err
			}
		}
		_, claudeWords := atDash(c, args)
		joining := session.Joining{DetachOthers: *detachOthers, Worktree: *worktree, New: *fresh,
			Conversation: *conversation, Fork: *fork, Args: claudeWords, SwitchedFrom: *switchedFrom, Typed: words}
		// Without -s, join creates the session under the next index (see session.Tmux.Next), and
		// then reads every server to end the idle sessions, as list does (see sweep); with -s it
		// reads its session's server only.
		next := !joinNaming.flags.Changed("suffix")
		var limit time.Duration
		if next {
			var err error
			if limit, err = idleLimit(); err != nil {
				return err
			}
		}
		var tools []string
		if *worktree {
			tools = append(tools, "git")
		}
		tmux, err := session.Check(tools...)
		if err != nil {
			return err
		}
		// join finds a terminal to move where it runs in a pane of cld's own servers before it looks
		// anything up, and otherwise reads what the tmux it runs in keeps from claude (see
		// session.Tmux.ReadyClient).
		kept, sw := tmux.ReadyClient()
		if !next {
			suffix, home, err := joinNaming.resolve(tmux)
			if err != nil {
				return err
			}
			if *fork {
				if err := ownName(typed, 1, *conversation, suffix); err != nil {
					return err
				}
			}
			joining.Home = home
			return tmux.Join(suffix, joining, kept, sw)
		}
		// Moved, the terminal's cld join takes the index, and checks the claude it starts.
		if sw != nil {
			return tmux.SwitchJoin(sw, joining)
		}
		// join starts claude, so it checks claude's version, after the checks every command makes,
		// so that cld runs claude only once the tools are found and tmux's version passes. detach,
		// kill, list and completion never run claude.
		claude, err := session.CheckClaude()
		if err != nil {
			return err
		}
		// From the name to tmux, join holds the lock of cld's record, where it writes the session's
		// entry (see session.Lock).
		unlock := session.Lock()
		defer unlock()
		suffix, _, err := joinNaming.resolve(tmux)
		if err != nil {
			return err
		}
		if *fork {
			if err := ownName(typed, 1, *conversation, suffix); err != nil {
				return err
			}
		}
		// The sweep comes once the index is taken, so that the session made does not take the name
		// of one just ended, by which cld join finds its conversation. It runs under the record's
		// lock, which another join meanwhile waits for. The sweep is not what was asked: where its
		// read of the servers fails, join says so and goes on.
		if limit > 0 {
			if sessions, err := tmux.Sessions(context.Background()); err != nil {
				output.Warn("cannot end the idle sessions: " + err.Error())
			} else {
				sweep(tmux, sessions, limit, *switchedFrom)
			}
		}
		return tmux.Create(claude, suffix, joining, kept)
	}
	if err := join.RegisterFlagCompletionFunc("name", sessionNames(true)); err != nil {
		panic(err)
	}
	if err := join.RegisterFlagCompletionFunc("suffix", sessionSuffixes(true)); err != nil {
		panic(err)
	}

	// detach is C-q d for a terminal that keeps C-q from tmux, where the key never reaches it: with
	// -s it detaches every terminal on the session, and without -n and -s, in one of cld's servers -
	// ! cld detach in claude - the terminal on that session used last (see session.Inside).
	detach := &cobra.Command{
		Use:   "detach [-n NAME] [-s SUFFIX]",
		Short: "detach the terminals attached to session NAME-SUFFIX",
		Long: `detach every terminal attached to session NAME-SUFFIX; claude keeps running.
Without -n and -s, as ! cld detach in claude, or in a shell on the session's
tmux server, detach the terminal used last on that session, as C-q d does, for
a terminal that keeps C-q from tmux: the one it was typed in, unless the mouse
moved over another since, or another took the focus. Elsewhere, -s is needed.
With -s and without -n, a session made in another repository or directory of
the same name is refused.`,
	}
	detachNaming := addNaming(detach, "the session's `SUFFIX`, after NAME-")
	detach.RunE = func(*cobra.Command, []string) error {
		inside := session.Inside() && !detachNaming.given()
		missing := "-s SUFFIX (see cld list)"
		if inside {
			missing = ""
		}
		if err := detachNaming.check(typed, missing); err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		if inside {
			return tmux.DetachTerminal()
		}
		suffix, home, err := detachNaming.resolve(tmux)
		if err != nil {
			return err
		}
		return tmux.Detach(suffix, home)
	}
	if err := detach.RegisterFlagCompletionFunc("name", sessionNames(false)); err != nil {
		panic(err)
	}
	if err := detach.RegisterFlagCompletionFunc("suffix", sessionSuffixes(false)); err != nil {
		panic(err)
	}

	kill := &cobra.Command{
		Use:   "kill [-n NAME] -s SUFFIX",
		Short: "end session NAME-SUFFIX and its tmux server",
		Long: `end session NAME-SUFFIX and its tmux server: claude exits as when its terminal
closes, and what claude started through tmux ends too, also where it keeps the
server running after claude has exited. claude runs its SessionEnd hooks with
the reason "other", and may still run them when cld kill returns. Without -n,
a session made in another repository or directory of the same name is refused.
After a kill, cld list shows the session as ended, and cld join brings its
conversation back; cld restore leaves it ended.`,
	}
	killNaming := addNaming(kill, "the session's `SUFFIX`, after NAME-")
	kill.RunE = func(*cobra.Command, []string) error {
		if err := killNaming.check(typed, "-s SUFFIX (see cld list)"); err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		suffix, home, err := killNaming.resolve(tmux)
		if err != nil {
			return err
		}
		return tmux.Kill(suffix, home)
	}

	// list is interactive on a terminal it can draw on; with no sessions there is nothing to pick.
	// Leaving it prints the table, from the sessions it last read. The session picked is joined as
	// join -n NAME -s SUFFIX joins it, but for one that has gone meanwhile: one that has ended is
	// brought back, its claude checked once the list has handed the terminal over (see
	// session.Tmux.JoinPicked). In a pane of one of cld's servers, where a session attached would
	// show inside one of cld's, the terminal on the pane's session moves to the session picked
	// instead, as join moves it there (see session.Tmux.SwitchTo). It ends the idle sessions first
	// (see sweep). --switch CLIENT and --to are the keys', which end none (see switchList).
	list := &cobra.Command{
		Use:   "list",
		Short: "list cld's sessions; on a terminal, join or kill one",
		Long: `list the sessions cld started: name, whether a terminal is attached (or claude
exited, or the session ended) and whether claude is busy, waiting for an answer
or idle, when it was last active (a terminal attaching, or a key typed in one)
and the directory claude is in or ran in. cld keeps a session that has ended -
by cld kill, claude's /exit, a reboot - for 30 days. A session idle for longer
than $CLD_IDLE_DAYS days, 30 where unset or empty, is ended first, as cld kill
ends it, with a line on stderr; cld join brings its conversation back.
CLD_IDLE_DAYS=0 ends none.

On a terminal, pick one to join or kill: Up and Down select a session, Enter
joins it as cld join does, C-x twice within two seconds kills it as cld kill
does - Esc after the first C-x keeps it - and Esc or C-c leaves, printing the
list. On a session that has ended, Enter resumes it as cld join does, and C-x
twice forgets it. cld list | cat prints the list only.

In a session, C-q s shows the list over it: Enter moves the terminal to the
session picked, as ! cld join does, and Esc closes the list. So does Enter in
cld list run in a shell on the session's tmux server.`,
	}
	// The keys' own options, which the help does not show: C-q s runs list --switch CLIENT in a
	// popup, and C-q (, C-q ) and C-q L list --switch CLIENT --to previous, next or last (see
	// session.switchKeys).
	switchClient := list.Flags().String("switch", "", "")
	to := list.Flags().String("to", "", "")
	for _, hidden := range []string{"switch", "to"} {
		if err := list.Flags().MarkHidden(hidden); err != nil {
			panic(err)
		}
	}
	list.RunE = func(c *cobra.Command, _ []string) error {
		switching := c.Flags().Changed("switch")
		switch {
		case switching && *switchClient == "":
			return fail.Usage("option '--switch' needs a value (see cld help)")
		case c.Flags().Changed("to") && !switching:
			return fail.Usage(typed + ": --to needs --switch CLIENT (see cld help)")
		case c.Flags().Changed("to") && !slices.Contains([]string{"previous", "next", "last"}, *to):
			return fail.Usage(fmt.Sprintf("invalid value '%s' for --to: previous, next or last (see cld help)", *to))
		}
		if switching {
			return switchList(*switchClient, *to)
		}
		limit, err := idleLimit()
		if err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		sessions, err := tmux.Sessions(context.Background())
		if err != nil {
			return err
		}
		sessions = sweep(tmux, sessions, limit, "")
		if len(sessions) > 0 && picker.Available() {
			sw := tmux.Switching()
			picked, last, err := picker.Run(listSource{tmux}, sessions, sw != nil)
			if err != nil {
				return err
			}
			switch {
			case picked.Name != "" && sw != nil:
				return tmux.SwitchTo(sw, picked.Name)
			case picked.Name != "":
				return tmux.JoinPicked(picked.Name)
			}
			sessions = last
		}
		return output.Print(table(sessions))
	}

	// restore checks tmux as every command does, CLD_IDLE_DAYS as list does, and the claude of each
	// session it brings back as join checks its own, in the session's directory (see
	// session.Tmux.Restore). It holds the record's lock for one session at a time, from the lookup
	// to tmux, so that another restore, or a join of the session, at once makes no second one, and
	// a join that creates a session meanwhile waits for one session at most. A session idle for
	// longer than the limit it leaves ended, with a note, as the sweep ends one. A session it
	// cannot bring back is a warning, and the others come back: the status is then 1.
	restoreCommand := &cobra.Command{
		Use:   "restore",
		Short: "bring back the sessions that ran when the machine stopped",
		Long: `bring back the sessions that ran when the machine stopped: a reboot or a crash
ends them, and cld list shows them as ended, beside those that cld kill, C-x in
cld list, the idle sweep or claude's /exit ended, which stay ended. Each comes
back as cld join -n NAME -s SUFFIX brings it back, but without a terminal:
claude resumes its conversation in the directory it ran in, with the
environment the session started with and without the words given after --,
and is told to continue the turn it was in, if any. A session not started or
given a prompt for longer than $CLD_IDLE_DAYS days stays ended, with a note. A
session that cannot come back is a warning, and the status 1. cld setup
restore has your systemd run cld restore as it starts; cld join attaches to a
session.`,
		RunE: func(*cobra.Command, []string) error {
			tmux, err := session.Check()
			if err != nil {
				return err
			}
			limit, err := idleLimit()
			if err != nil {
				return err
			}
			failed := false
			for _, name := range session.Marked() {
				unlock := session.Lock()
				restored, err := tmux.Restore(name, limit)
				unlock()
				switch {
				case err != nil:
					failed = true
					output.Warn(fmt.Sprintf("cannot restore session '%s': %v", name, err))
				case restored != nil && restored.Idle > 0:
					output.Note(fmt.Sprintf("left session '%s' ended, idle for %s", name, idleFor(restored.Idle)))
				case restored != nil:
					line := fmt.Sprintf("Restored session '%s' in %s", name, restored.Directory)
					if restored.Busy {
						line += ", continuing its turn"
					}
					if err := output.Print(line + "\n"); err != nil {
						return err
					}
				}
			}
			if failed {
				return fail.Status(1)
			}
			return nil
		},
	}

	// setup runs nothing itself, so cobra's help shows its commands without a usage line of its
	// own; run has made sure that one of them follows it, or -h or --help.
	setup := &cobra.Command{
		Use:   "setup",
		Short: "set up claude in a project, telemetry, shell completion or restore",
	}
	projectCommand := setupProject(typed + " project")
	telemetryCommand := setupTelemetry(typed + " telemetry")
	shellsCommand := setupCompletion(typed + " completion")
	setupRestoreCommand := setupRestore(typed + " restore")
	setup.AddCommand(projectCommand, telemetryCommand, shellsCommand, setupRestoreCommand)

	// update runs neither tmux nor claude, so it makes none of their checks: it needs the network
	// and the directory cld is in, which internal/update checks as it goes.
	updateCommand := &cobra.Command{
		Use:   "update",
		Short: "update cld to the latest release",
		Long: `update cld to the latest release on GitHub: download the release's cld for
this system, check it against the release's cld.sha256 and that it runs, then
replace the file cld runs from with it - the file a symbolic link leads to.
Where cld is the latest release already, or newer, nothing changes; a cld built
from source, cld dev, is not updated. Once cld is replaced, the scripts that
cld setup completion wrote are written anew where the new cld prints others.`,
		RunE: func(*cobra.Command, []string) error {
			result, err := update.Run(version)
			if err != nil {
				return err
			}
			if err := output.Print(result.Report()); err != nil || result.File == "" {
				return err
			}
			// The new cld prints the completion scripts: its release's cobra may write others. cld
			// is updated by then, so a script not written is a warning, not a failure.
			report, warnings := completion.Refresh(result.File)
			for _, warning := range warnings {
				output.Warn(warning)
			}
			if report == "" {
				return nil
			}
			return output.Print(report)
		},
	}

	// cobra would add "[flags]" at the end of help's usage line, after COMMAND, where cld reads no
	// options. cobra's own help command completes COMMAND, and so does cld's.
	help := &cobra.Command{
		Use:                   "help [flags] [COMMAND]",
		Short:                 "show this help, or the help of COMMAND",
		Args:                  helpArguments(typed),
		ValidArgsFunction:     commandNames,
		DisableFlagsInUseLine: true,
	}

	// cobra lists commands only: version's Long names its other spellings.
	versionCommand := &cobra.Command{
		Use:   "version",
		Short: "show the version",
		Long:  "show the version; cld -V and cld --version show it too",
		RunE: func(*cobra.Command, []string) error {
			return output.Print("cld " + version + "\n")
		},
	}

	all := []*cobra.Command{root, join, detach, kill, list, restoreCommand, setup, projectCommand, telemetryCommand, shellsCommand}
	all = append(append(all, shellsCommand.Commands()...), setupRestoreCommand, updateCommand, help, versionCommand)
	for _, command := range all {
		// cobra adds -h and --help only where a command has no "help" option of its own.
		command.Flags().VarPF(new(helpOption), "help", "h", "help for "+command.Name()).NoOptDefVal = "true"
		if command == root {
			continue // The root only has a help: run always hands cobra one of the commands.
		}
		if command.Args == nil {
			command.Args = noArguments(typed)
		}
		command.Flags().SetInterspersed(false)
	}
	root.AddCommand(join, detach, kill, list, restoreCommand, setup, updateCommand, versionCommand)
	root.SetHelpCommand(help)
	completionCommand(root)

	// cobra's own help function, which cld's calls, writes the help to the command's output, out
	// (see run). The help command is set with SetHelpCommand, so this one serves help as well as
	// -h and --help, those of completion's commands included.
	cobraHelp := root.HelpFunc()
	showHelp := func(c *cobra.Command) {
		// Execute has moved the help command after the others as it ran: version goes back after
		// it, where cld lists version.
		root.RemoveCommand(versionCommand)
		root.AddCommand(versionCommand)
		cobraHelp(c, nil)
	}
	root.SetHelpFunc(func(c *cobra.Command, _ []string) { showHelp(c) })
	help.RunE = func(_ *cobra.Command, args []string) error {
		command, err := helpTopic(root, typed, args)
		if err != nil {
			return err
		}
		showHelp(command)
		return nil
	}
	return root
}

// switchList is list --switch CLIENT, which the keys of a session run on its server with the
// terminal that pressed the key, tmux client CLIENT (see session.Switch): with to, C-q (, C-q ) or
// C-q L, it moves the terminal to the previous, next or last session (see session.Tmux.Step);
// without, C-q s, it is the interactive list in a popup over the terminal, whose Enter moves the
// terminal to the session picked, as join moves it in a pane of the server, leaving its popup to
// close with it; Esc closes the popup, and prints nothing. Neither ends the idle sessions, as list
// does (see sweep): tmux runs the keys with the environment of the session's server - that of the
// cld join that started it, or the one restore recorded - not the terminal's, so a CLD_IDLE_DAYS
// the terminal no longer has, or never had, would decide, and the popup takes the notes of the
// sessions ended away as it closes. The popup takes away what cld writes, and tmux shows over
// claude's pane what a key's run-shell writes, which the keys throw away: so cld says what goes
// wrong on the terminal's message line (see session.Tmux.Tell), as well as on stderr - once it
// knows the terminal, from tmux's checks on, by the tmux found where the checks refuse it.
func switchList(client, to string) error {
	sw, err := session.SwitchClient(client)
	if err != nil {
		return err
	}
	tmux, err := session.Check()
	if err != nil {
		if found, ferr := session.Find(); ferr == nil {
			tell(found, sw, err)
		}
		return err
	}
	err = func() error {
		if to != "" {
			return tmux.Step(context.Background(), sw, to)
		}
		sessions, err := tmux.Sessions(context.Background())
		if err != nil {
			return err
		}
		if len(sessions) == 0 || !picker.Available() {
			return nil
		}
		picked, _, err := picker.Run(listSource{tmux}, sessions, true)
		if err != nil || picked.Name == "" {
			return err
		}
		return tmux.SwitchTo(sw, picked.Name)
	}()
	tell(tmux, sw, err)
	return err
}

// tell shows err on the message line of the terminal of sw (see session.Tmux.Tell), where it is
// cld's own message: an exit status of tmux's follows tmux's message on stderr.
func tell(tmux *session.Tmux, sw *session.Switch, err error) {
	var failure *fail.Error
	switch {
	case errors.As(err, &failure):
		tmux.Tell(sw, failure.Message)
	case err != nil && !errors.As(err, new(fail.Status)):
		tmux.Tell(sw, err.Error())
	}
}

// bleLines go at the end of __start_cld, the function through which cobra's bash script completes
// cld, for bash with ble.sh, which edits the command line in readline's place and runs the script
// itself. Where cld offers no file names, the script turns off -o default with compopt, but only
// where compopt is bash's builtin, and ble.sh has the script call a compopt function of its own:
// -o default stays on, and ble.sh offers file names. It would all the same without -o default, as
// it offers completions of its own - options it reads from the help, file names - wherever a
// function offers nothing, unless the function turns off ble/default. The lines do both where cld
// offers no file names - the directive has ShellCompDirectiveNoFileComp - and compopt is a
// function in a shell ble.sh runs in; elsewhere they do nothing.
const bleLines = `    # ble.sh has the lines above call a compopt function of its own, which they take for no
    # compopt: turn off -o default as they would, and ble.sh's completions where cld offers none.
    if [[ ${BLE_VERSION-} && $(type -t compopt) == function ]] && (((directive & %d) != 0)); then
        compopt +o default +o ble/default
    fi
`

// bashScript writes to w cobra's bash script for root, with descriptions or without, and
// bleLines at the end of the function that completes root: what completion bash prints, and
// setup completion bash writes.
func bashScript(w io.Writer, root *cobra.Command, descriptions bool) error {
	var script bytes.Buffer
	if err := root.GenBashCompletionV2(&script, descriptions); err != nil {
		return err
	}
	end := "\n    __" + root.Name() + "_process_completion_results\n}\n"
	before, after, found := strings.Cut(script.String(), end)
	if !found || strings.Contains(after, end) {
		panic("cobra's bash script has no end of __start_" + root.Name())
	}
	lines := fmt.Sprintf(bleLines, cobra.ShellCompDirectiveNoFileComp)
	_, err := io.WriteString(w, before+strings.TrimSuffix(end, "}\n")+lines+"}\n"+after)
	return err
}

// completionCommand adds cobra's completion command to root: completion SHELL prints the
// completion script for SHELL, bash, zsh, fish or powershell - bash's with lines of cld's (see
// bashScript) - and its help, cobra's Long, says where the script goes and what it needs;
// completion alone shows its help, as with cobra. The short descriptions, which the help of
// completion and of cld list, are cld's. They read their arguments as cld's commands do (see
// commandLine), with cld's -h and --help: an unknown SHELL, an argument after it or an unknown
// option is refused, where cobra would show the help and exit 0, fail with exit status 1, or take
// a later option first.
func completionCommand(root *cobra.Command) {
	root.InitDefaultCompletionCmd()
	completion, _, err := root.Find([]string{"completion"})
	if err != nil || completion == root {
		panic("cobra made no completion command")
	}
	completion.Short = "print the completion script for a shell"
	completion.Long = `print the completion script for a shell, one of the commands below. With it,
-n and -s of cld join complete the sessions cld list shows, and of cld detach
those that run. cld setup completion SHELL writes it where bash, zsh or fish
reads it; the help of each command below says where the script goes by hand,
and what it needs.`
	for _, shell := range completion.Commands() {
		shell.Short = "print the completion script for " + shell.Name()
		if shell.Name() == "bash" {
			shell.RunE = func(c *cobra.Command, _ []string) error {
				noDescriptions, err := c.Flags().GetBool("no-descriptions")
				if err != nil {
					return err
				}
				return bashScript(c.OutOrStdout(), root, !noDescriptions)
			}
		}
	}
	// A command cobra cannot run shows its help before it looks at the arguments: completion runs,
	// to show it once they are read.
	completion.RunE = func(*cobra.Command, []string) error { return pflag.ErrHelp }
	completion.Args = func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return unexpected("completion", "--")
		}
		if len(args) > 0 {
			return fail.Usage(fmt.Sprintf("completion: unknown shell '%s' (see cld help)", args[0]))
		}
		return nil
	}
	for _, command := range append([]*cobra.Command{completion}, completion.Commands()...) {
		name := strings.TrimPrefix(command.CommandPath(), root.Name()+" ")
		command.Flags().VarPF(new(helpOption), "help", "h", "help for "+command.Name()).NoOptDefVal = "true"
		if command != completion {
			command.Args = noArguments(name)
		}
		command.Flags().SetInterspersed(false)
		command.SetFlagErrorFunc(flagError(name))
	}
}

// setupCompletion is cld setup completion, named in its messages as typed, with a command for each
// shell of completion.Shells: each writes the script that cld completion SHELL prints, which
// cobra generates as it does there, where the shell reads it (see internal/completion).
// setupCommand has made sure that a shell follows setup completion, or -h or --help, so it runs
// nothing itself, as setup does not.
func setupCompletion(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "completion",
		Short: "set up cld's completion in bash, zsh or fish",
		Long: `set up cld's completion in a shell, one of the commands below: cld writes the
script that cld completion SHELL prints where the shell reads it. TAB then
completes cld's commands, their options and the names cld list shows. cld
update writes the script anew where the release it installs prints another.`,
		Args: noArguments(typed),
	}
	command.SetFlagErrorFunc(flagError(typed))
	long := map[string]string{
		"bash": `set up cld's completion in bash: write the script cld completion bash prints
to completions/cld in the first directory of $BASH_COMPLETION_USER_DIR, or else
to ~/.local/share/bash-completion/completions/cld ($XDG_DATA_HOME in place of
~/.local/share where set), where bash-completion 2 finds it at the first TAB.
It needs ~/.bashrc to load bash-completion, as Debian's and Ubuntu's do;
bash-completion 1, the one for macOS's bash 3.2, does not read that directory.`,
		"zsh": `set up cld's completion in zsh: write the script cld completion zsh prints to
~/.local/share/cld/zsh/_cld ($XDG_DATA_HOME in place of ~/.local/share where
set), and add the lines that load it to the end of ~/.zshrc ($ZDOTDIR/.zshrc
where ZDOTDIR is set), unless it has them. They run compinit only where
nothing before them has: a second compinit would drop the completions set up
after the first. Where the script is missing, they do nothing.`,
		"fish": `set up cld's completion in fish: write the script cld completion fish prints
to ~/.config/fish/completions/cld.fish ($XDG_CONFIG_HOME in place of ~/.config
where set), where fish finds it at the first TAB.`,
	}
	for _, shell := range completion.Shells {
		name := typed + " " + shell
		shellCommand := &cobra.Command{
			Use:   shell,
			Short: "set up cld's completion in " + shell,
			Long:  long[shell],
			Args:  noArguments(name),
			RunE: func(c *cobra.Command, _ []string) error {
				var script bytes.Buffer
				var err error
				switch shell {
				case "bash":
					err = bashScript(&script, c.Root(), true)
				case "zsh":
					err = c.Root().GenZshCompletion(&script)
				default:
					err = c.Root().GenFishCompletion(&script, true)
				}
				if err != nil {
					return fail.Runtime("cannot make the script: " + err.Error())
				}
				report, err := completion.Setup(shell, script.Bytes())
				if err != nil {
					return err
				}
				return output.Print(report)
			},
		}
		shellCommand.SetFlagErrorFunc(flagError(name))
		command.AddCommand(shellCommand)
	}
	return command
}

// setupProject is cld setup project, named in its messages as typed. --mcp takes the MCP servers
// as a list, given again or separated by commas; each is written once, in project.Servers' order,
// whatever the order given. --permissions takes one of project.PermissionSets, the first where it
// is not given.
func setupProject(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "project [--mcp SERVER] [--permissions SET]",
		Short: "set claude up in the project in the current directory",
		Long: `set claude up in the project in the current directory: .claude/settings.json
holds the settings the project shares through git - what claude may do without
asking, which --permissions sets, and where claude keeps its plans - and
.claude/settings.local.json, holding its $schema alone, is for your own.
.gitignore gets /.claude/settings.local.json, /.claude/plans/ and
/.claude/worktrees/: git ignores those, and adds what the project shares in
.claude, such as its commands, agents and skills. With --mcp, cld adds MCP
servers to .mcp.json, and the settings enable them and allow their tools as
--permissions says.

Where the files exist, cld adds what they lack and keeps everything else, the
values of the settings it would set included, but for a server's entry in
.mcp.json that differs from cld's, which it replaces. Then, in a git work tree,
it checks that git does not ignore the settings, and warns of the other files a
project shares under .claude that git ignores. Review the changes before you
commit them: whoever trusts the project's folder gives claude what they allow.`,
		Args: noArguments(typed),
	}
	command.SetFlagErrorFunc(flagError(typed))
	flags := command.Flags()
	mcp := flags.StringArray("mcp", nil, "an MCP `SERVER` for claude in the project: goland or\n"+
		"rider, the IDE's own server on 127.0.0.1, at the port\n"+
		"in GOLAND_MCP_PORT or RIDER_MCP_PORT where claude\n"+
		"runs, else the IDE's default, 64422 or 64482; or\n"+
		"jbcontext, JetBrains Context's code search. Give --mcp\n"+
		"again, or separate them with commas")
	permissions := flags.String("permissions", "", "what claude may do in the project without asking:\n"+
		"`SET` is read-only, the default, to read files, run\n"+
		"commands that only read, such as git status and ls,\n"+
		"and use the servers' tools that only read; cld, as\n"+
		"cld's own repository has it, to edit files, run git,\n"+
		"go, make, docker and more, use every server tool, but\n"+
		"not merge or push to main; or none, to add nothing")
	command.RunE = func(c *cobra.Command, _ []string) error {
		chosen := map[string]bool{}
		for _, list := range *mcp {
			for name := range strings.SplitSeq(list, ",") {
				if !slices.ContainsFunc(project.Servers, func(s project.Server) bool { return s.Name == name }) {
					return fail.Usage(fmt.Sprintf("invalid MCP server '%s' for --mcp: goland, jbcontext or rider (see cld help)", name))
				}
				chosen[name] = true
			}
		}
		var servers []project.Server
		for _, s := range project.Servers {
			if chosen[s.Name] {
				servers = append(servers, s)
			}
		}
		set := project.PermissionSets[0]
		if c.Flags().Changed("permissions") {
			at := slices.IndexFunc(project.PermissionSets, func(p project.Permissions) bool { return p.Name == *permissions })
			if at < 0 {
				return fail.Usage(fmt.Sprintf("invalid permissions '%s' for --permissions: read-only, cld or none (see cld help)", *permissions))
			}
			set = project.PermissionSets[at]
		}
		return project.Setup(servers, set)
	}
	if err := command.RegisterFlagCompletionFunc("mcp", serverNames); err != nil {
		panic(err)
	}
	if err := command.RegisterFlagCompletionFunc("permissions", permissionSets); err != nil {
		panic(err)
	}
	return command
}

// setupTelemetry is cld setup telemetry, named in its messages as typed. On a system other than
// Linux it refuses to run before it looks at anything but -h and --help: an option or argument
// it would refuse too (see telemetry.Supported).
func setupTelemetry(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "telemetry [--local URL] [--remote URL] [flags]",
		Short: "send claude's telemetry through a local OpenTelemetry collector",
		Long: `send claude's telemetry through a local OpenTelemetry collector: cld runs the
collector in Docker, as the container cld-telemetry listening on 127.0.0.1, and
points claude's user settings at it ($CLAUDE_CONFIG_DIR/settings.json, by
default ~/.claude/settings.json). The collector sends traces, metrics and logs
to --local, such as the JetBrains OpenTelemetry plugin in the IDE, and metrics
only to --remote, such as a team's collector; give one of them, or both.

Running it again replaces the collector and rewrites the settings; the env keys
cld does not manage stay as they are. claude reads its settings as a session
starts: sessions running then keep theirs. Needs Docker; Linux only.`,
		Args: func(c *cobra.Command, args []string) error {
			if err := telemetry.Supported(); err != nil {
				return err
			}
			return noArguments(typed)(c, args)
		},
	}
	command.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		err = flagError(typed)(c, err)
		if unsupported := telemetry.Supported(); unsupported != nil && !errors.Is(err, pflag.ErrHelp) {
			return unsupported
		}
		return err
	})
	flags := command.Flags()
	local := flags.String("local", "", "where traces, metrics and logs go, such as\n"+
		"http://127.0.0.1:4319 (the IDE's plugin on a\n"+
		"fixed port); `URL` is http://HOST:PORT for\n"+
		"plaintext gRPC or https://HOST:PORT for TLS")
	remote := flags.String("remote", "", "where metrics go too (`URL` as for --local), such\n"+
		"as https://otel.example.com:4317")
	port := flags.String("port", "", "the `PORT` the collector listens on, on 127.0.0.1\n"+
		"(default: the one the collector has, or one\n"+
		"the kernel picks)")
	collectorConfig := flags.String("collector-config", "", "`FILE` holds YAML that the collector merges over\n"+
		"cld's config, such as headers or TLS for the\n"+
		"exporters otlp_grpc/local and otlp_grpc/remote")
	command.RunE = func(c *cobra.Command, _ []string) error {
		var options telemetry.Options
		for _, endpoint := range []struct {
			name, url string
			into      **telemetry.Endpoint
		}{{"local", *local, &options.Local}, {"remote", *remote, &options.Remote}} {
			if !c.Flags().Changed(endpoint.name) {
				continue
			}
			parsed, ok := telemetry.ParseEndpoint(endpoint.url)
			if !ok {
				return fail.Usage(fmt.Sprintf("invalid URL '%s' for --%s: http://HOST:PORT or https://HOST:PORT (see cld help)", endpoint.url, endpoint.name))
			}
			*endpoint.into = &parsed
		}
		if c.Flags().Changed("port") {
			var ok bool
			if options.Port, ok = telemetry.ParsePort(*port); !ok {
				return fail.Usage(fmt.Sprintf("invalid port '%s': a number from 1 to 65535 (see cld help)", *port))
			}
		}
		if options.Local == nil && options.Remote == nil {
			return fail.Usage(typed + ": --local URL, --remote URL or both are needed (see cld help)")
		}
		if option := options.Collector(options.Port); options.Port != 0 && option != "" {
			return fail.Usage(fmt.Sprintf("%s is where the collector would listen (--port %d): give the receiver's port, or another --port (see cld help)",
				option, options.Port))
		}
		if c.Flags().Changed("collector-config") {
			options.CollectorConfig = *collectorConfig
			if options.CollectorConfig == "" {
				return fail.Usage("option '--collector-config' needs a value (see cld help)")
			}
		}
		return telemetry.Setup(options)
	}
	return command
}

// setupRestore is cld setup restore, named in its messages as typed. On a system other than
// Linux it refuses to run before it looks at anything but -h and --help, as setup telemetry does
// (see restore.Supported); its other checks, of systemd, are in its RunE, which completion never
// runs.
func setupRestore(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "restore",
		Short: "have your systemd run cld restore at login, or at boot",
		Long: `have your user's systemd run cld restore as it starts, which brings back the
sessions that ran when the machine stopped: cld writes the unit
~/.config/systemd/user/cld-restore.service, which runs this cld with the PATH,
TMUX_TMPDIR, XDG_STATE_HOME and CLD_IDLE_DAYS it has now, and enables it. Your
systemd starts at your first login, and at your last logout ends the sessions
cld restore brought back, unless lingering is on for you (loginctl
enable-linger): then it starts at boot, and keeps them. Run it again after
moving cld, or to change those variables. Needs systemd; Linux only.`,
		Args: func(c *cobra.Command, args []string) error {
			if err := restore.Supported(); err != nil {
				return err
			}
			return noArguments(typed)(c, args)
		},
		RunE: func(*cobra.Command, []string) error {
			// The unit hands CLD_IDLE_DAYS on to cld restore, which would refuse a value that is
			// no number of days at every start.
			if _, err := idleLimit(); err != nil {
				return err
			}
			// The file cld runs from, with its symbolic links resolved, as cld update replaces it.
			cld, err := os.Executable()
			if err != nil {
				return fail.Runtime("cannot find the file cld runs from: " + err.Error())
			}
			report, err := restore.Setup(cld)
			if err != nil {
				return err
			}
			return output.Print(report)
		},
	}
	command.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		err = flagError(typed)(c, err)
		if unsupported := restore.Supported(); unsupported != nil && !errors.Is(err, pflag.ErrHelp) {
			return unsupported
		}
		return err
	})
	return command
}

// sessionNames completes the NAME of -n, join's with all true and detach's with all false: for the
// sessions list shows - with all, those that have ended too, and otherwise those that run - what
// comes before the last "-" of their names, where that and what follows it are both NAMEs - the
// split cld's own messages name a session by (see session.Options) - once each, in list's order,
// and each described by the number of its sessions. With -s then, the command takes each of those
// sessions.
func sessionNames(all bool) cobra.CompletionFunc {
	return func(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
		var names []string
		counts := map[string]int{}
		for _, s := range listed(all) {
			i := strings.LastIndexByte(s.Name, '-')
			if i <= 0 || !session.ValidName(s.Name[:i]) || !session.ValidName(s.Name[i+1:]) || !strings.HasPrefix(s.Name[:i], typed) {
				continue
			}
			if counts[s.Name[:i]]++; counts[s.Name[:i]] == 1 {
				names = append(names, s.Name[:i])
			}
		}
		completions := make([]cobra.Completion, 0, len(names))
		for _, name := range names {
			description := "1 session"
			if counts[name] > 1 {
				description = strconv.Itoa(counts[name]) + " sessions"
			}
			completions = append(completions, cobra.CompletionWithDesc(name, description))
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

// sessionSuffixes completes the SUFFIX of -s, join's with all true and detach's with all false: for
// the sessions list shows - with all, those that have ended too, and otherwise those that run -
// whose names are NAME-SUFFIX with the NAME the command takes - -n's, or else the repository's or
// directory's (see defaultName) - their SUFFIX, where it starts with what was typed and the
// command takes it, in list's order, each described by its state as list shows it, claude's status
// included - "detached, waiting" - or, for a session that has ended, "ended in" the directory it
// ran in. Without -n, join and detach take no session that runs made in another repository or
// directory of the same name (see session.Home.Takes), so none is offered; a session that has
// ended has no home (see session.Session), and each is offered. Where NAME is "", every name is a
// SUFFIX.
func sessionSuffixes(all bool) cobra.CompletionFunc {
	return func(c *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
		var name string
		var home session.Home
		if c.Flags().Changed("name") {
			name, _ = c.Flags().GetString("name") //nolint:errcheck // cannot fail: -n is a string option
		} else {
			name, home = defaultName()
		}
		if name != "" {
			name += "-"
		}
		var suffixes []cobra.Completion
		for _, s := range listed(all) {
			if !home.Takes(s.Home) {
				continue
			}
			if suffix, found := strings.CutPrefix(s.Name, name); found && strings.HasPrefix(suffix, typed) && session.ValidName(suffix) {
				description := s.ShownState()
				if s.State == session.Ended {
					description = session.Ended + " in " + s.Directory
				}
				suffixes = append(suffixes, cobra.CompletionWithDesc(suffix, description))
			}
		}
		return suffixes, cobra.ShellCompDirectiveNoFileComp
	}
}

// listed is the sessions list shows, for completion: with all, every one, and otherwise those that
// run. It never fails: with no server and no record there is nothing to offer, and with no tmux,
// one that fails, or a socket directory it cannot read neither, and cobra.CompErrorln says why on
// stderr, which the completion scripts discard.
func listed(all bool) []session.Session {
	var sessions []session.Session
	tmux, err := session.Find()
	if err == nil {
		sessions, err = tmux.Sessions(context.Background())
	}
	if err != nil {
		cobra.CompErrorln(err.Error())
	}
	if all {
		return sessions
	}
	return slices.DeleteFunc(sessions, func(s session.Session) bool { return s.State == session.Ended })
}

// serverNames completes the SERVER of setup project --mcp: the servers it takes that start with
// what was typed, each described. After a comma it completes the last of the list, offering the
// servers the list does not have yet after the ones it has.
func serverNames(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	before, last := "", typed
	if comma := strings.LastIndexByte(typed, ','); comma >= 0 {
		before, last = typed[:comma+1], typed[comma+1:]
	}
	var names []cobra.Completion
	for _, s := range project.Servers {
		if strings.HasPrefix(s.Name, last) && !slices.Contains(strings.Split(before, ","), s.Name) {
			names = append(names, cobra.CompletionWithDesc(before+s.Name, s.Description))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// permissionSets completes the SET of setup project --permissions: the sets it takes that start
// with what was typed, each described.
func permissionSets(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var names []cobra.Completion
	for _, p := range project.PermissionSets {
		if strings.HasPrefix(p.Name, typed) {
			names = append(names, cobra.CompletionWithDesc(p.Name, p.Description))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// commandNames completes help's COMMAND, as cobra's help command does its own: the commands help
// takes (see helpTopic) that start with what was typed, each described by its Short - cld's, and
// after one with commands of its own, such as setup, those. Nothing follows any other COMMAND.
func commandNames(c *cobra.Command, args []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var names []cobra.Completion
	parent, err := helpTopic(c.Root(), "help", args)
	if err == nil && (parent == c.Root() || parent.HasAvailableSubCommands()) {
		for _, command := range parent.Commands() {
			if topic(parent, command.Name()) == command && strings.HasPrefix(command.Name(), typed) {
				names = append(names, cobra.CompletionWithDesc(command.Name(), command.Short))
			}
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// nameUsage is -n in the help of join, detach and kill: its first line fits within 80 columns
// beside the widest column of theirs, join's, as wide as its --resume SESSION.
const nameUsage = "the session's `NAME`, before -SUFFIX: by default the\ngit repository's name here, or else the directory's"

// naming is the -n and -s of join, detach and kill, which name the session NAME-SUFFIX: NAME is
// -n's, or else the name of the git repository the current directory is in, or of the directory
// (see session.DefaultName) - where that leaves nothing, the session is SUFFIX alone - and SUFFIX
// -s's, or else, for join, the next index (see session.Tmux.Next). kill, and detach but in one of
// cld's servers, need -s; without -n, join, detach and kill take no session that runs made in
// another repository or directory of the same name (see session.Home).
type naming struct {
	flags        *pflag.FlagSet
	name, suffix *string
}

// addNaming gives command -n and -s, whose usage is suffixUsage.
func addNaming(command *cobra.Command, suffixUsage string) naming {
	return naming{
		flags:  command.Flags(),
		name:   command.Flags().StringP("name", "n", "", nameUsage),
		suffix: command.Flags().StringP("suffix", "s", "", suffixUsage),
	}
}

// check checks -n and -s once the options have been read, before anything runs: each that was
// given - its length, whatever its characters, then its characters, as SUFFIX alone names the
// session where the directory's name leaves nothing (see session.DefaultName) - then the name
// both make, which is too long for a session's where they make it longer than one NAME can be
// (see tooLong); and, where missing is not empty, that -s was given, which missing says is
// missing, for the command typed as typed.
func (n naming) check(typed, missing string) error {
	name, suffix := n.flags.Changed("name"), n.flags.Changed("suffix")
	if name {
		if _, err := sessionName(*n.name); err != nil {
			return err
		}
	}
	switch {
	case suffix:
		if utf8.RuneCountInString(*n.suffix) > session.MaxName {
			return fail.Usage(fmt.Sprintf("suffix '%s' is longer than %d characters (see cld help)", *n.suffix, session.MaxName))
		}
		if !session.ValidName(*n.suffix) {
			return fail.Usage(fmt.Sprintf("invalid suffix '%s' (see cld help)", *n.suffix))
		}
		if both := *n.name + "-" + *n.suffix; name && len(both) > session.MaxName {
			return tooLong(2, both)
		}
	case missing != "":
		return fail.Usage(typed + ": missing " + missing)
	}
	return nil
}

// given reports whether -n or -s was given.
func (n naming) given() bool { return n.flags.Changed("name") || n.flags.Changed("suffix") }

// tooLong refuses session name, longer than a session's name can be, with status.
func tooLong(status int, name string) error {
	return &fail.Error{Status: status,
		Message: fmt.Sprintf("session name '%s' is longer than %d characters", name, session.MaxName),
		Advice:  "; give a shorter -n NAME or -s SUFFIX (see cld help)"}
}

// givenName is the session's name, NAME-SUFFIX, where the command line alone makes it: with -n
// and -s.
func (n naming) givenName() (string, bool) {
	if !n.flags.Changed("name") || !n.flags.Changed("suffix") {
		return "", false
	}
	return *n.name + "-" + *n.suffix, true
}

// joinOptions refuses the options of join, typed as typed, that cannot go together, once -n and
// -s are checked and before any tool is looked for: --resume's SESSION, conversation, where
// resuming - given - is empty, or starts with "-", which claude would read as an option; --fork
// without --resume, which has nothing to copy; --new with --resume, as each says which
// conversation claude starts with; and -w, worktree, with --resume, as claude takes a
// conversation back to its worktree itself. A resumed conversation cld cannot tell from one that
// is not: the words for claude after "--" go to either.
func joinOptions(typed string, resuming bool, conversation string, fork, fresh, worktree bool) error {
	switch {
	case resuming && conversation == "":
		return fail.Usage("option '--resume' needs a value (see cld help)")
	case resuming && strings.HasPrefix(conversation, "-"):
		return fail.Usage(fmt.Sprintf("invalid SESSION '%s' for --resume: claude would read it as an option (see cld help)", conversation))
	case fork && !resuming:
		return fail.Usage(typed + ": --fork needs --resume SESSION, the conversation to copy (see cld help)")
	case fresh && resuming:
		return fail.Usage(typed + ": --new and --resume exclude each other: each says which conversation claude starts with (see cld help)")
	case worktree && resuming:
		return fail.Usage(typed + ": -w and --resume exclude each other: claude takes a conversation back to its worktree itself (see cld help)")
	}
	return nil
}

// ownName refuses, with status, join --resume SESSION --fork of conversation, SESSION, into session
// name where SESSION is the name the copy takes, cld-NAME, as claude compares names: lower-cased,
// spaces around them trimmed (see docs/design/findings/claude.md). claude would find the
// conversation of that name and give its copy the same, so that a resume by that name found two. A
// SESSION that names the conversation another way - its session ID, a pick in claude's picker - cld
// cannot tell.
func ownName(typed string, status int, conversation, name string) error {
	if strings.ToLower(strings.TrimSpace(conversation)) != strings.ToLower("cld-"+name) {
		return nil
	}
	return &fail.Error{Status: status,
		Message: typed + ": --fork would give the copy SESSION's own name, cld-" + name,
		Advice:  "; give another -s SUFFIX (see cld help)"}
}

// resolve is the session's name, NAME-SUFFIX, once tmux has been checked: -n's NAME, or else the
// repository's or directory's, and -s's SUFFIX, or else the next index (see session.Tmux.Next).
// One longer than a session's name can be is refused with status 1: the repository's or
// directory's name, or the index, makes it so, not the command line alone (see check). With it
// comes the home that join, detach and kill take the session from (see defaultName).
func (n naming) resolve(tmux *session.Tmux) (string, session.Home, error) {
	name, home := *n.name, session.Home{}
	if !n.flags.Changed("name") {
		name, home = defaultName()
	}
	if name != "" {
		name += "-"
	}
	if n.flags.Changed("suffix") {
		name += *n.suffix
	} else {
		var err error
		if name, err = tmux.Next(context.Background(), name); err != nil {
			return "", session.Home{}, err
		}
	}
	if len(name) > session.MaxName {
		return "", session.Home{}, tooLong(1, name)
	}
	return name, home, nil
}

// defaultName is the NAME of NAME-SUFFIX where -n gives none, and the home that join, detach and
// kill, and the completion of join -s and detach -s, take a session of it from (see
// session.DefaultName): none where NAME is "", as in the root directory, where -s names any
// session whole.
func defaultName() (string, session.Home) {
	name, home := session.DefaultName()
	if name == "" {
		return "", session.Home{}
	}
	return name, home
}

// sessionName is the NAME given with -n, checked once the options have been read: first its
// length, whatever its characters, then its characters. The messages call it a name, as -s's
// call SUFFIX a suffix: the session's name is NAME-SUFFIX (see tooLong).
func sessionName(name string) (string, error) {
	if utf8.RuneCountInString(name) > session.MaxName {
		return "", fail.Usage(fmt.Sprintf("name '%s' is longer than %d characters (see cld help)", name, session.MaxName))
	}
	if !session.ValidName(name) {
		return "", &fail.Error{Status: 2, Message: fmt.Sprintf("invalid name '%s'", name), Advice: " (see cld help)"}
	}
	return name, nil
}

// listSource is what the interactive list reads and acts through: the sessions to list, join's
// checks, which Enter makes while the list is open - the name, then the lookup - and kill's steps,
// or the forget, which the second Ctrl+X takes. The list names a session whole, as -n does, and
// takes it from anywhere.
type listSource struct{ tmux *session.Tmux }

func (l listSource) Sessions(ctx context.Context) ([]session.Session, error) {
	return l.tmux.Sessions(ctx)
}

func (l listSource) Joinable(ctx context.Context, name string) error {
	if _, err := sessionName(name); err != nil {
		return err
	}
	return l.tmux.Joinable(ctx, name)
}

// Forget forgets the session under the record's lock, so that a restore bringing it back at once
// is waited for, and its session found running, as a join's start mark is read under it (see
// session.Tmux.Forget).
func (l listSource) Forget(ctx context.Context, name string) error {
	if _, err := sessionName(name); err != nil {
		return err
	}
	unlock := session.Lock()
	defer unlock()
	return l.tmux.Forget(ctx, name)
}

// Kill is kill's steps - the name, then End - with End's check that the session is still the one
// whose panes' pids the list read. What tmux says when the kill - kill-session, then kill-server,
// or kill-server alone on a server that has outlived the session (see End) - fails becomes the
// error, for the list's footer, rather than going to the terminal the list draws on.
func (l listSource) Kill(ctx context.Context, name string, pids []string) error {
	if _, err := sessionName(name); err != nil {
		return err
	}
	var said bytes.Buffer
	err := l.tmux.End(ctx, name, session.Home{}, pids, &said, &said)
	if status, ok := errors.AsType[fail.Status](err); ok {
		if message := strings.TrimSpace(said.String()); message != "" {
			return fail.Runtime(message)
		}
		return fail.Runtime("tmux kill-session: " + status.Error())
	}
	return err
}

// noArguments refuses the first argument left after a command's options, or a "--" among them:
// no command but help takes an argument.
func noArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return unexpected(typed, "--")
		}
		if len(args) > 0 {
			return unexpected(typed, args[0])
		}
		return nil
	}
}

// helpArguments takes help's COMMAND, one of cld's, then one of its own commands, such as setup's
// after setup and a shell after setup completion, and refuses a "--" before it, as noArguments
// does, a COMMAND that is not cld's, and an argument after it: the first of these decides.
func helpArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return unexpected(typed, "--")
		}
		_, err := helpTopic(c.Root(), typed, args)
		return err
	}
}

// claudeArguments takes the arguments of join: the words after a "--", which go to claude after
// cld's own (see session.Joining), and none before it. It refuses an argument before the "--" -
// an option too, since options come first - and then, among the words for claude, the options
// that claudeOptions refuses.
func claudeArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		args, words := atDash(c, args)
		if len(args) > 0 {
			return unexpected(typed, args[0])
		}
		return refuseOptions(typed, words)
	}
}

// atDash splits args, the arguments of join, at the "--" after which the words go to claude: the
// arguments before it, and the words after it, none without a "--". pflag drops a "--" it reads
// among the options, recording where it was (ArgsLenAtDash), and leaves one after the first
// argument in place, as it reads no option past that argument.
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

// claudeOption is one of claude's options, as a word gives it: short, such as -n, at the start of
// the word, and long, such as --name, or its other spelling alias, alone or with a value after "=".
// claude reads a word -xyz as its option -x with the value yz where -x takes a value, and else as
// -x followed by -yz: so a word that starts with a short option gives that option either way.
type claudeOption struct {
	short, long, alias string
	// why join refuses the option
	why string
}

// claudeOptions are the options of claude's that join refuses among the words for claude: those
// cld gives claude itself, of which claude would keep the one given last - -w and --worktree too,
// which join gives with cld's -w - and those that resume a conversation, which is join's to do;
// and those with which claude would not stay in the session, printing and exiting or leaving the
// pane - the hidden --init-only and --rewind-files too. Only the start of a word counts: the one
// short option of claude 2.1.284's not here, -d, takes the rest of its word as its value, so a
// word holds one of these after its start only as a value.
var claudeOptions = []claudeOption{
	{short: "-n", long: "--name", why: "cld gives claude the session's name, which -n and -s make"},
	{short: "-w", long: "--worktree", why: "cld gives claude --worktree with -w, before --"},
	{long: "--settings", why: "cld gives claude --settings, which this one would replace"},
	{short: "-r", long: "--resume", why: "cld gives claude --resume with --resume SESSION, before --"},
	{short: "-c", long: "--continue", why: "cld join resumes the session's conversation, or --resume SESSION"},
	{long: "--from-pr", why: "cld join resumes the session's conversation, or --resume SESSION"},
	{short: "-p", long: "--print", why: "claude would print its answer and exit, ending the session"},
	{long: "--bg", alias: "--background", why: "claude would start in the background and exit, ending the session"},
	{long: "--tmux", why: "claude would move to a tmux session of its own"},
	{long: "--teleport", why: "claude would resume a session from Claude Code on the web instead"},
	{long: "--init-only", why: "claude would run its startup hooks and exit, ending the session"},
	{long: "--rewind-files", why: "claude would restore files and exit, ending the session"},
	{short: "-h", long: "--help", why: "claude would print its help and exit, ending the session"},
	{short: "-v", long: "--version", why: "claude would print its version and exit, ending the session"},
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
// claudeOptions, saying why. Each word counts, whatever comes before it, a second "--" too, after
// which claude still looks for --tmux, --bg and --background: a value that one of claude's
// options takes after it, spelled as one of these, goes after "=" (--append-system-prompt=-p...).
// claude reports the other words it does not take, and a claude that fails at startup stays on
// screen with what it said.
func refuseOptions(typed string, words []string) error {
	for _, word := range words {
		for _, option := range claudeOptions {
			if option.given(word) {
				return fail.Usage(fmt.Sprintf("%s: '%s' after --: %s (see cld help)", typed, word, option.why))
			}
		}
	}
	return nil
}

// helpTopic is the command whose help help shows, given args: the root for none, else the
// command they name, one of cld's, then one of its own commands if it has them, as in help setup
// telemetry. help is named as typed in the errors.
func helpTopic(root *cobra.Command, typed string, args []string) (*cobra.Command, error) {
	command := root
	for i, name := range args {
		if command != root && !command.HasAvailableSubCommands() {
			return nil, unexpected(typed, name)
		}
		if command = topic(command, name); command == nil {
			return nil, fail.Usage(fmt.Sprintf("%s: unknown command '%s' (see cld help)", typed, strings.Join(args[:i+1], " ")))
		}
	}
	return command, nil
}

// topic is the command named name among those the help of parent lists, which help shows the
// help of; nil for any other name.
func topic(parent *cobra.Command, name string) *cobra.Command {
	for _, command := range parent.Commands() {
		if command.Name() == name && (command.IsAvailableCommand() || command.Name() == "help") {
			return command
		}
	}
	return nil
}

// flagError turns pflag's errors into cld's messages, naming what pflag reports. -h or --help
// before the error shows the help, as it does before any other argument: cobra looks at -h only
// once every option has been read, and pflag stops at the error.
func flagError(typed string) func(*cobra.Command, error) error {
	return func(c *cobra.Command, err error) error {
		if help, _ := c.Flags().GetBool("help"); help { //nolint:errcheck // no -h option: no help asked
			return pflag.ErrHelp
		}
		var (
			valueRequired *pflag.ValueRequiredError
			notExist      *pflag.NotExistError
			invalidValue  *pflag.InvalidValueError
			invalidSyntax *pflag.InvalidSyntaxError
		)
		switch {
		case errors.As(err, &valueRequired):
			option := "--" + valueRequired.GetSpecifiedName()
			if valueRequired.GetSpecifiedShortnames() != "" {
				option = "-" + valueRequired.GetSpecifiedName()
			}
			return fail.Usage(fmt.Sprintf("option '%s' needs a value (see cld help)", option))
		case errors.As(err, &notExist):
			// The shorthands from the unknown one to the end of its group: all of -xw, the x of -wx.
			if shorthands := notExist.GetSpecifiedShortnames(); shorthands != "" {
				return unexpected(typed, "-"+shorthands)
			}
			return unexpected(typed, "--"+notExist.GetSpecifiedName())
		case errors.As(err, &invalidValue):
			return unexpected(typed, "--"+invalidValue.GetFlag().Name+"="+invalidValue.GetValue())
		case errors.As(err, &invalidSyntax):
			return unexpected(typed, invalidSyntax.GetSpecifiedFlag())
		}
		return fail.Usage(err.Error())
	}
}

// helpOption is -h and --help: a boolean, as cobra needs, that keeps its value when given one
// that is not a boolean. pflag's own stores false before it fails, so in -h --help=x the error
// would win over the -h before it.
type helpOption bool

func (h *helpOption) Set(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	*h = helpOption(parsed)
	return nil
}

func (h *helpOption) String() string { return strconv.FormatBool(bool(*h)) }

func (h *helpOption) Type() string { return "bool" }

// unexpected refuses argument, given to the command typed as typed.
func unexpected(typed, argument string) error {
	return fail.Usage(fmt.Sprintf("%s: unexpected argument '%s' (see cld help)", typed, argument))
}

// table lays out sessions for list: NAME, at least four wide, STATE, with claude's status (see
// session.Session's ShownState) and at least eight wide, each as wide as its longest, LAST ACTIVE
// and DIRECTORY, under a header; nothing at all without sessions.
func table(sessions []session.Session) string {
	if len(sessions) == 0 {
		return ""
	}
	width, stateWidth := 4, 8
	for _, s := range sessions {
		width = max(width, utf8.RuneCountInString(s.Name))
		stateWidth = max(stateWidth, utf8.RuneCountInString(s.ShownState()))
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%-*s  %-*s  %-11s  %s\n", width, "NAME", stateWidth, "STATE", "LAST ACTIVE", "DIRECTORY")
	for _, s := range sessions {
		fmt.Fprintf(&out, "%-*s  %-*s  %-11s  %s\n", width, s.Name, stateWidth, s.ShownState(), s.LastActive(), s.Directory)
	}
	return out.String()
}

// days matches a number of days as CLD_IDLE_DAYS takes it: decimal digits, with a fraction or
// without.
var days = regexp.MustCompile(`^([0-9]+\.?[0-9]*|\.[0-9]+)$`)

// idleLimit is how long a session may stay idle before list, and join without -s, end it (see
// sweep): CLD_IDLE_DAYS days - a fraction of a day too, as the tests take - and 30 where it is
// unset or empty; 0 for no limit, where nothing is ended. Anything else is refused rather than
// taken for another limit, which could end the sessions sooner than meant: a negative number, an
// exponent, a unit. A limit longer than a time.Duration holds, some 292 years, is none.
func idleLimit() (time.Duration, error) {
	value := os.Getenv("CLD_IDLE_DAYS")
	if value == "" {
		return 30 * 24 * time.Hour, nil
	}
	if !days.MatchString(value) {
		return 0, fail.Runtime(fmt.Sprintf("CLD_IDLE_DAYS is not a number of days: '%s'", value))
	}
	// days took value, so ParseFloat fails only on a number too large for a float64, giving +Inf.
	count, _ := strconv.ParseFloat(value, 64) //nolint:errcheck // explained above
	limit := math.Ceil(count * float64(24*time.Hour))
	if limit >= math.MaxInt64 {
		return 0, nil
	}
	return time.Duration(limit), nil
}

// sweep ends each of sessions that has been idle for longer than limit - none where limit is 0 - as
// kill ends it, with a note on stderr for each, and returns the sessions as list then shows them:
// those it ended as ended, where cld's record keeps them (see session.EndedSession), as after cld
// kill. It is list's first step - but for the keys' list --switch (see switchList) - and join's
// without -s once it has its index. The kill checks again that the session is idle (see
// session.Tmux.EndIdle): one that a terminal has attached to since, say, stays. A kill that fails
// is a warning, and the session stays too: the sweep is not what was asked. The session whose
// server cld runs on, as when its claude runs cld, stays however long it has been idle (see
// session.OwnServer), and so does session left, the one a join that a move runs has just moved the
// terminal from (join --switched-from), whose claude may have run the join that moved it, from its
// Bash tool, with no key on the session; one that has ended is idle for no time (see
// session.Session's Idle).
func sweep(tmux *session.Tmux, sessions []session.Session, limit time.Duration, left string) []session.Session {
	if limit == 0 {
		return sessions
	}
	own, inside := session.OwnServer()
	var kept []session.Session
	for _, s := range sessions {
		if s.Idle > limit && !(inside && s.Name == own) && s.Name != left {
			ended, err := tmux.EndIdle(context.Background(), s.Name, limit)
			if err != nil {
				output.Warn(fmt.Sprintf("cannot end session '%s', idle for %s: %s", s.Name, idleFor(s.Idle), err))
			}
			if ended {
				output.Note(fmt.Sprintf("ended session '%s', idle for %s", s.Name, idleFor(s.Idle)))
				if row, recorded := session.EndedSession(s.Name); recorded {
					kept = append(kept, row)
				}
				continue
			}
		}
		kept = append(kept, s)
	}
	return kept
}

// idleFor is how long a session has been idle, for the sweep's notes: in whole days, hours,
// minutes or seconds, the largest that fits - "31 days", "1 hour".
func idleFor(idle time.Duration) string {
	unit, name := time.Second, "second"
	for _, larger := range []struct {
		size time.Duration
		name string
	}{{24 * time.Hour, "day"}, {time.Hour, "hour"}, {time.Minute, "minute"}} {
		if idle >= larger.size {
			unit, name = larger.size, larger.name
			break
		}
	}
	count := int64(idle / unit)
	if count != 1 {
		name += "s"
	}
	return strconv.FormatInt(count, 10) + " " + name
}
