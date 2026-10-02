package main

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
)

// rootLong is the root's help, which says what a session is (decision 12.6).
//
//nolint:dupword // C-q C-q is a key typed twice
const rootLong = `Run Claude Code in named sessions, each on a private tmux server that ignores
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
has been idle for longer than $CLD_IDLE_DAYS days (see cld help list).`

// commandLine is cld's commands, for a command typed as typed with words after it: messages name
// it as typed, "-V" for version, say. What cobra prints goes to out. cobra's defaults give way to
// cld's command line (see docs/design/overview.md, "The command line on cobra").
func commandLine(typed string, words []string, out io.Writer) *cobra.Command {
	cobra.EnableCommandSorting = false // The commands in the order they are added.
	root := &cobra.Command{Use: "cld", Long: rootLong, SilenceErrors: true, SilenceUsage: true}
	// Before completion is made: its commands write their scripts to the output the root has then.
	root.SetOut(out)
	root.CompletionOptions.SetDefaultShellCompDirective(cobra.ShellCompDirectiveNoFileComp)
	root.SetFlagErrorFunc(cmdline.FlagError(typed))

	help, versionCommand := newHelp(typed), newVersion()
	root.AddCommand(newJoin(typed, words), newDetach(typed), newKill(typed), newList(typed),
		newRestore(), newSetup(typed), newUpdate(), versionCommand)
	readOptions(root, typed, append(tree(root), help))
	root.SetHelpCommand(help)
	completionCommand(root)
	showHelp(root, help, versionCommand, typed)
	return root
}

// tree is command and every command under it.
func tree(command *cobra.Command) []*cobra.Command {
	all := []*cobra.Command{command}
	for _, child := range command.Commands() {
		all = append(all, tree(child)...)
	}
	return all
}

// readOptions gives every command in all cld's -h and --help (see cmdline.AddHelp). All but the
// root read options up to the first argument, where pflag would read past it, and take no
// argument where they set no Args. The root needs no more: run always hands cobra a command.
func readOptions(root *cobra.Command, typed string, all []*cobra.Command) {
	for _, command := range all {
		cmdline.AddHelp(command)
		if command == root {
			continue
		}
		if command.Args == nil {
			command.Args = cmdline.NoArguments(typed)
		}
		command.Flags().SetInterspersed(false)
	}
}
