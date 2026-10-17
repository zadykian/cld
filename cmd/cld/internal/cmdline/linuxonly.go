package cmdline

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// LinuxOnly has command, a command of setup typed as typed that takes no argument, run on Linux
// alone, where supported passes. Elsewhere it refuses to run before it looks at anything but -h
// and --help: an option or argument it would refuse too. A --host that pflag read before an error
// lets it run, on the host (decision 62.5).
func LinuxOnly(command *cobra.Command, typed string, supported func() error) {
	command.Args = func(c *cobra.Command, args []string) error {
		if err := supported(); err != nil && !OnHost(c) {
			return err
		}
		return NoArguments(typed)(c, args)
	}
	// The system is refused before the option.
	command.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		err = FlagError(typed)(c, err)
		if unsupported := supported(); unsupported != nil && !errors.Is(err, pflag.ErrHelp) &&
			!OnHost(c) {
			return unsupported
		}
		return err
	})
}

// Host is the option that runs a command on another host (decision 62).
const Host = "host"

// OnHost reports whether command was given --host.
func OnHost(command *cobra.Command) bool { return command.Flags().Changed(Host) }
