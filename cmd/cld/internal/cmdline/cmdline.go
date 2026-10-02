// Package cmdline holds what cld's commands share in reading their command line: the checks of
// their arguments, cld's messages for pflag's errors, cld's -h and --help, the words join gives
// claude, and the commands of setup that run on Linux alone. Errors name a command as typed.
package cmdline

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/fail"
)

// NoArguments refuses the first argument left after a command's options, or a "--" among them:
// no command but help takes an argument, and join takes only words for claude.
func NoArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return Unexpected(typed, "--")
		}
		if len(args) > 0 {
			return Unexpected(typed, args[0])
		}
		return nil
	}
}

// Unexpected refuses argument, given to the command typed as typed.
func Unexpected(typed, argument string) error {
	return fail.Usage(fmt.Sprintf("%s: unexpected argument '%s' (see cld help)", typed, argument))
}

// FlagError turns pflag's errors into cld's messages, naming what pflag reports. -h or --help
// before the error shows the help, as before any other argument. cobra looks at -h only once
// every option has been read, and pflag stops at the error.
func FlagError(typed string) func(*cobra.Command, error) error {
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
			return Unexpected(typed, "-"+shorthands)
		}
		return Unexpected(typed, "--"+notExist.GetSpecifiedName())
	case errors.As(err, &invalidValue):
		return Unexpected(typed, "--"+invalidValue.GetFlag().Name+"="+invalidValue.GetValue())
	case errors.As(err, &invalidSyntax):
		return Unexpected(typed, invalidSyntax.GetSpecifiedFlag())
	}
	return fail.Usage(err.Error())
}

// Hide hides options, which cld's own commands give, not the user, from the help and from
// completion (decision 51.2).
func Hide(flags *pflag.FlagSet, names ...string) {
	for _, name := range names {
		if err := flags.MarkHidden(name); err != nil {
			panic(err)
		}
	}
}

// CompleteWith has option of command complete with complete.
func CompleteWith(command *cobra.Command, option string, complete cobra.CompletionFunc) {
	if err := command.RegisterFlagCompletionFunc(option, complete); err != nil {
		panic(err)
	}
}

// AddHelp gives command cld's -h and --help: cobra adds its own only where a command has no
// "help" option.
func AddHelp(command *cobra.Command) {
	option := command.Flags().VarPF(new(helpOption), "help", "h", "help for "+command.Name())
	option.NoOptDefVal = "true"
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
