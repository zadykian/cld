package cmdline

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// LinuxOnly has command, a command of setup typed as typed that takes no argument, run on Linux
// alone, where supported passes. Elsewhere it refuses to run before it looks at anything but -h
// and --help: an option or argument it would refuse too.
func LinuxOnly(command *cobra.Command, typed string, supported func() error) {
	command.Args = func(c *cobra.Command, args []string) error {
		if err := supported(); err != nil {
			return err
		}
		return NoArguments(typed)(c, args)
	}
	// The system is refused before the option.
	command.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		err = FlagError(typed)(c, err)
		if unsupported := supported(); unsupported != nil && !errors.Is(err, pflag.ErrHelp) {
			return unsupported
		}
		return err
	})
}
