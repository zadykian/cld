package main

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/completion"
	"github.com/zadykian/cld/internal/fail"
)

// newSetup is cld setup, whose commands are setup config, completion and restore. It runs nothing
// itself, so cobra's help shows no usage line of its own: run has made sure that one of its
// commands, or -h or --help, follows it.
func newSetup(typed string) *cobra.Command {
	setup := &cobra.Command{
		Use:   "setup",
		Short: "set up claude's settings, shell completion or restore",
	}
	setup.AddCommand(setupConfig(typed+" config"), setupCompletion(typed+" completion"),
		setupRestore(typed+" restore"))
	return setup
}

// setupCommand checks the argument after setup before cobra sees it, as run checks the first
// (decision 52.3): one of setup's commands, or -h or --help. After setup config it checks the
// command, and after setup completion the shell, as cobra would run zsh's for setup completion
// --help=false zsh.
func setupCommand(args []string) error {
	const hint = "cld setup config COMMAND, cld setup completion SHELL or cld setup restore " +
		"(see cld help)"
	switch {
	case len(args) == 0 || args[0] == "":
		return fail.Usage("setup: missing command: " + hint)
	case args[0] == "config":
		return configCommand(args[1:])
	case args[0] == "completion":
		return shellArgument(args[1:])
	case slices.Contains([]string{"restore", "-h", "--help"}, args[0]):
		return nil
	}
	return fail.Usage(fmt.Sprintf("setup: unknown command '%s': %s", args[0], hint))
}

// setupConfig is cld setup config, whose commands write claude's settings (decision 53). It runs
// nothing itself: setupCommand has made sure that one of its commands, or -h or --help, follows it.
func setupConfig(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "set up claude's settings, in a project or your own",
		Long: `set up claude's settings, one of the commands below: those a project shares
through git, or your own, which claude reads in every project. Each adds what
the settings lack, and keeps the values they have.`,
		Args: cmdline.NoArguments(typed),
	}
	command.SetFlagErrorFunc(cmdline.FlagError(typed))
	command.AddCommand(setupProject(typed+" project"), setupUser(typed+" user"))
	return command
}

// configCommand checks the argument after setup config, as setupCommand checks the one after
// setup: one of its commands, or -h or --help, the help of setup config.
func configCommand(args []string) error {
	const hint = "project or user (see cld help)"
	switch {
	case len(args) == 0 || args[0] == "":
		return fail.Usage("setup config: missing command: " + hint)
	case slices.Contains([]string{"project", "user", "-h", "--help"}, args[0]):
		return nil
	}
	return fail.Usage(fmt.Sprintf("setup config: unknown command '%s': %s", args[0], hint))
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
