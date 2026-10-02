package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/fail"
)

// noArguments refuses the first argument left after a command's options, or a "--" among them:
// no command but help takes an argument, and join takes only words for claude.
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

// unexpected refuses argument, given to the command typed as typed.
func unexpected(typed, argument string) error {
	return fail.Usage(fmt.Sprintf("%s: unexpected argument '%s' (see cld help)", typed, argument))
}

// flagError turns pflag's errors into cld's messages, naming what pflag reports. -h or --help
// before the error shows the help, as before any other argument. cobra looks at -h only once
// every option has been read, and pflag stops at the error.
func flagError(typed string) func(*cobra.Command, error) error {
	return func(c *cobra.Command, err error) error {
		if help, _ := c.Flags().GetBool("help"); help { //nolint:errcheck // no -h: no help asked
			return pflag.ErrHelp
		}
		return optionError(typed, err)
	}
}

// optionError is cld's message for pflag's error err, for the command typed as typed.
func optionError(typed string, err error) error {
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
		// The shorthands from the unknown one to the end of its group: -xw whole, the x of -wx.
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

// hide hides options, which cld's own commands give, not the user, from the help and from
// completion (decision 51.2).
func hide(flags *pflag.FlagSet, names ...string) {
	for _, name := range names {
		if err := flags.MarkHidden(name); err != nil {
			panic(err)
		}
	}
}

// completeWith has option of command complete with complete.
func completeWith(command *cobra.Command, option string, complete cobra.CompletionFunc) {
	if err := command.RegisterFlagCompletionFunc(option, complete); err != nil {
		panic(err)
	}
}
