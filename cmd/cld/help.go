package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/fail"
)

// newHelp is cld help [COMMAND] (decision 12.2), whose RunE showHelp sets. cobra would add
// "[flags]" at the end of its usage line, after COMMAND, where cld reads no options.
func newHelp(typed string) *cobra.Command {
	return &cobra.Command{
		Use:                   "help [flags] [COMMAND]",
		Short:                 "show this help, or the help of COMMAND",
		Args:                  helpArguments(typed),
		ValidArgsFunction:     commandNames,
		DisableFlagsInUseLine: true,
	}
}

// showHelp has help, and -h and --help of every command, completion's too, show cobra's help
// from its default templates (decision 12.1). cobra's help function writes it to the root's
// output, out (see run).
func showHelp(root, help, versionCommand *cobra.Command, typed string) {
	cobraHelp := root.HelpFunc()
	show := func(c *cobra.Command) {
		// Execute has moved the help command after the others as it ran: version goes back
		// after it, where cld lists version.
		root.RemoveCommand(versionCommand)
		root.AddCommand(versionCommand)
		cobraHelp(c, nil)
	}
	root.SetHelpFunc(func(c *cobra.Command, _ []string) { show(c) })
	help.RunE = func(_ *cobra.Command, args []string) error {
		command, err := helpTopic(root, typed, args)
		if err != nil {
			return err
		}
		show(command)
		return nil
	}
}

// helpArguments takes help's COMMAND (see helpTopic), and refuses a "--" before it, as
// cmdline.NoArguments does: the first of these mistakes decides.
func helpArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return cmdline.Unexpected(typed, "--")
		}
		_, err := helpTopic(c.Root(), typed, args)
		return err
	}
}

// helpTopic is the command whose help is shown for args: the root for none, else the one they
// name. They name one of cld's commands, then one of its own, as in help setup telemetry. Errors
// name help as typed.
func helpTopic(root *cobra.Command, typed string, args []string) (*cobra.Command, error) {
	command := root
	for i, name := range args {
		if command != root && !command.HasAvailableSubCommands() {
			return nil, cmdline.Unexpected(typed, name)
		}
		if command = topic(command, name); command == nil {
			return nil, fail.Usage(fmt.Sprintf("%s: unknown command '%s' (see cld help)", typed,
				strings.Join(args[:i+1], " ")))
		}
	}
	return command, nil
}

// topic is the command named name among those the help of parent lists, or nil.
func topic(parent *cobra.Command, name string) *cobra.Command {
	for _, command := range parent.Commands() {
		if command.Name() == name && (command.IsAvailableCommand() || command.Name() == "help") {
			return command
		}
	}
	return nil
}

// commandNames completes help's COMMAND, as cobra's help command does its own: the commands help
// takes (see helpTopic) that start with what was typed, each described by its Short.
func commandNames(c *cobra.Command, args []string, typed string) ([]cobra.Completion,
	cobra.ShellCompDirective) {
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
