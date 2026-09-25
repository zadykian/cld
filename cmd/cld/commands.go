package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/session"
)

// commands maps each command, and each alias of one, to the command it runs.
var commands = map[string]string{
	"new": "new", "join": "join", "kill": "kill", "list": "list",
	"help": "help", "-h": "help", "--help": "help",
	"version": "version", "-V": "version", "--version": "version",
}

// run runs cld with the arguments args. The first is the command, which run checks before cobra
// sees it: cobra would take an unknown one for an argument of cld itself, skip options before
// the command (cld -n x new would run new -n x), and answer its own completion commands,
// __complete and __completeNoDesc.
func run(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fail.Usage("missing command: cld new creates a session, cld join attaches to one (see cld help)")
	}
	typed := args[0]
	command, known := commands[typed]
	if !known {
		// Before cld had commands, "cld NAME" attached to session NAME, creating it first.
		if session.ValidName(typed) {
			return fail.Usage(fmt.Sprintf("unknown command '%s'; for session %[1]s: cld new -n %[1]s, cld join -n %[1]s", typed))
		}
		return fail.Usage(fmt.Sprintf("unknown command '%s' (see cld help)", typed))
	}
	// cobra's help function returns nothing, and cobra ends -h and --help without an error: what
	// printing the usage returned comes back here.
	var printed error
	root := commandLine(typed, &printed)
	root.SetArgs(append([]string{command}, args[1:]...))
	if err := root.Execute(); err != nil {
		return err
	}
	return printed
}

// commandLine is cld's commands, for a command typed as typed: the messages name it that way,
// "-V" for version, say. Printing the usage sets printed.
//
// cobra's defaults give way to cld's command line. main prints the errors, as "cld: MESSAGE",
// and help, -h and --help print one usage for every command. help takes no command, version is
// a command rather than cobra's --version and -v, and there is no completion command. Every
// command reads its options up to the first argument, which pflag would otherwise pass over,
// and takes no argument: the first one left, or a "--", which pflag would drop, is refused.
func commandLine(typed string, printed *error) *cobra.Command {
	root := &cobra.Command{
		Use:               "cld",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.SetHelpFunc(func(*cobra.Command, []string) { *printed = fail.Print(usage) })
	root.SetFlagErrorFunc(flagError(typed))

	newCommand := &cobra.Command{Use: "new"}
	newName := newCommand.Flags().StringP("name", "n", "main", "the session")
	worktree := newCommand.Flags().BoolP("worktree", "w", false, "run claude in git worktree NAME")
	newCommand.RunE = func(*cobra.Command, []string) error {
		suffix, err := sessionName(*newName)
		if err != nil {
			return err
		}
		tools := []string{"claude"}
		if *worktree {
			tools = append(tools, "git")
		}
		tmux, err := session.Check(tools...)
		if err != nil {
			return err
		}
		return tmux.New(suffix, *worktree)
	}

	join := &cobra.Command{Use: "join"}
	joinName := join.Flags().StringP("name", "n", "main", "the session")
	join.RunE = func(*cobra.Command, []string) error {
		suffix, err := sessionName(*joinName)
		if err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		return tmux.Join(suffix)
	}

	kill := &cobra.Command{Use: "kill"}
	killName := kill.Flags().StringP("name", "n", "main", "the session")
	kill.RunE = func(*cobra.Command, []string) error {
		suffix, err := sessionName(*killName)
		if err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		return tmux.Kill(suffix)
	}

	list := &cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error {
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		sessions, err := tmux.Sessions()
		if err != nil {
			return err
		}
		return fail.Print(table(sessions))
	}}

	help := &cobra.Command{Use: "help", RunE: func(c *cobra.Command, _ []string) error {
		c.HelpFunc()(c, nil)
		return nil
	}}

	versionCommand := &cobra.Command{Use: "version", RunE: func(*cobra.Command, []string) error {
		return fail.Print("cld " + version + "\n")
	}}

	for _, command := range []*cobra.Command{newCommand, join, kill, list, help, versionCommand} {
		command.Args = noArguments(typed)
		command.Flags().SetInterspersed(false)
		// cobra adds -h and --help only where a command has no "help" option of its own.
		command.Flags().VarPF(new(helpOption), "help", "h", "show the usage").NoOptDefVal = "true"
	}
	root.AddCommand(newCommand, join, kill, list, versionCommand)
	root.SetHelpCommand(help)
	return root
}

// sessionName is the NAME given with -n, checked once the options have been read.
func sessionName(name string) (string, error) {
	if !session.ValidName(name) {
		return "", fail.Usage(fmt.Sprintf("invalid session name '%s' (see cld help)", name))
	}
	return name, nil
}

// noArguments refuses the first argument left after a command's options, or a "--" among them:
// no command takes an argument.
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

// flagError turns pflag's errors into cld's messages, naming what pflag reports. -h or --help
// before the error shows the help, as it does before any other argument: cobra looks at -h only
// once every option has been read, and pflag stops at the error.
func flagError(typed string) func(*cobra.Command, error) error {
	return func(c *cobra.Command, err error) error {
		if help, _ := c.Flags().GetBool("help"); help {
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

// table lays out sessions for list: NAME, at least four wide, STATE and DIRECTORY, under a
// header; nothing at all without sessions.
func table(sessions []session.Session) string {
	if len(sessions) == 0 {
		return ""
	}
	width := 4
	for _, s := range sessions {
		width = max(width, utf8.RuneCountInString(s.Name))
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%-*s  %-8s  %s\n", width, "NAME", "STATE", "DIRECTORY")
	for _, s := range sessions {
		fmt.Fprintf(&out, "%-*s  %-8s  %s\n", width, s.Name, s.State, s.Directory)
	}
	return out.String()
}
