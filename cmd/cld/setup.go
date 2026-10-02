package main

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/completion"
	"github.com/zadykian/cld/internal/fail"
)

// newSetup is cld setup, whose commands are setup project, telemetry, completion and restore. It
// runs nothing itself, so cobra's help shows no usage line of its own: run has made sure that one
// of its commands, or -h or --help, follows it.
func newSetup(typed string) *cobra.Command {
	setup := &cobra.Command{
		Use:   "setup",
		Short: "set up claude in a project, telemetry, shell completion or restore",
	}
	setup.AddCommand(setupProject(typed+" project"), setupTelemetry(typed+" telemetry"),
		setupCompletion(typed+" completion"), setupRestore(typed+" restore"))
	return setup
}

// setupCommand checks the argument after setup before cobra sees it, as run checks the first
// (decision 18.8): one of setup's commands, or -h or --help. After setup completion, it checks
// the shell, as cobra would run zsh's for setup completion --help=false zsh.
func setupCommand(args []string) error {
	const hint = "cld setup project, cld setup telemetry, cld setup completion SHELL or " +
		"cld setup restore (see cld help)"
	switch {
	case len(args) == 0 || args[0] == "":
		return fail.Usage("setup: missing command: " + hint)
	case args[0] == "completion":
		return shellArgument(args[1:])
	case slices.Contains([]string{"project", "telemetry", "restore", "-h", "--help"}, args[0]):
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

// linuxOnly is a command of setup, typed as typed, that runs on Linux alone, where supported
// passes. Elsewhere it refuses to run before it looks at anything but -h and --help: an option
// or argument it would refuse too.
type linuxOnly struct {
	typed     string
	supported func() error
}

// arguments is the command's Args: no argument, once supported passes.
func (l linuxOnly) arguments(c *cobra.Command, args []string) error {
	if err := l.supported(); err != nil {
		return err
	}
	return noArguments(l.typed)(c, args)
}

// flagError is the command's flag error function, which refuses the system first.
func (l linuxOnly) flagError(c *cobra.Command, err error) error {
	err = flagError(l.typed)(c, err)
	if unsupported := l.supported(); unsupported != nil && !errors.Is(err, pflag.ErrHelp) {
		return unsupported
	}
	return err
}
