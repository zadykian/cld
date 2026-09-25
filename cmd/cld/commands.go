package main

import (
	"bytes"
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
	// printing the help returned comes back here.
	var printed error
	root := commandLine(typed, &printed)
	root.SetArgs(append([]string{command}, args[1:]...))
	if err := root.Execute(); err != nil {
		return err
	}
	return printed
}

// commandLine is cld's commands, for a command typed as typed: the messages name it that way,
// "-V" for version, say. Printing the help for -h or --help sets printed.
//
// The help is cobra's, from its default templates: each command's Use, and its Long or else its
// Short, then its options with their usages, which name their value in backquotes (`NAME`). Its
// text is here and nowhere else; cobra wraps none of it, so the lines break by hand, within 80
// columns. help, -h and --help print it through fail.Print, and it lists the commands in the
// order they are added here rather than by name.
//
// cobra's defaults give way to cld's command line. main prints the errors, as "cld: MESSAGE".
// help takes one of cld's commands at most, version is a command rather than cobra's --version
// and -v, and there is no completion command. Every command reads its options up to the first
// argument, which pflag would otherwise pass over, and takes no argument but help's COMMAND: the
// first one left, or a "--", which pflag would drop, is refused.
func commandLine(typed string, printed *error) *cobra.Command {
	cobra.EnableCommandSorting = false // The commands in the order they are added.
	root := &cobra.Command{
		Use: "cld",
		Long: `Run Claude Code in named sessions, each on a private tmux server that ignores
~/.tmux.conf. Session NAME is the tmux session "cld-NAME" on the server
"tmux -L cld-NAME", running "claude --name cld-NAME" with Remote Control on.
What claude starts through tmux runs on that server too, and ends with it.

Detach with C-q d; C-q C-q sends C-q to claude. A session whose claude fails
stays, showing why, until cld kill ends it.`,
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.SetFlagErrorFunc(flagError(typed))

	newCommand := &cobra.Command{
		Use:   "new [-n NAME] [-w]",
		Short: "create session NAME in the current directory and attach to it",
	}
	newName := newCommand.Flags().StringP("name", "n", "main", nameUsage)
	worktree := newCommand.Flags().BoolP("worktree", "w", false,
		"run claude in git worktree NAME, which claude creates from\nHEAD or reopens (claude --worktree NAME)")
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
		// new starts claude, so it checks claude's version too: after the checks every command
		// makes, so that cld runs claude only once the tools are found and tmux's version passes,
		// and before any other tmux command. join, kill and list never run claude.
		claude, err := session.CheckClaude()
		if err != nil {
			return err
		}
		return tmux.New(claude, suffix, *worktree)
	}

	join := &cobra.Command{
		Use:   "join [-n NAME]",
		Short: "attach to session NAME, detaching any other terminal from it",
	}
	joinName := join.Flags().StringP("name", "n", "main", nameUsage)
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

	kill := &cobra.Command{
		Use:   "kill [-n NAME]",
		Short: "end session NAME and its tmux server",
		Long: `end session NAME and its tmux server: claude exits as when its terminal closes,
and what claude started through tmux ends too`,
	}
	killName := kill.Flags().StringP("name", "n", "main", nameUsage)
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

	list := &cobra.Command{
		Use:   "list",
		Short: "list the sessions cld started",
		Long: `list the sessions cld started: name, whether a terminal is attached (or claude
exited), and the directory claude is in`,
		RunE: func(*cobra.Command, []string) error {
			tmux, err := session.Check()
			if err != nil {
				return err
			}
			sessions, err := tmux.Sessions()
			if err != nil {
				return err
			}
			return fail.Print(table(sessions))
		},
	}

	// cobra would add "[flags]" at the end of help's usage line, after COMMAND, where cld reads no
	// options. Unlike cobra's help command, cld's completes no command names after help
	// (ValidArgsFunction): cld has no completion yet (#25).
	help := &cobra.Command{
		Use:                   "help [flags] [COMMAND]",
		Short:                 "show this help, or the help of COMMAND",
		Args:                  helpArguments(typed),
		DisableFlagsInUseLine: true,
	}

	// cobra lists commands only: version's Long names its other spellings.
	versionCommand := &cobra.Command{
		Use:   "version",
		Short: "show the version",
		Long:  "show the version; cld -V and cld --version show it too",
		RunE: func(*cobra.Command, []string) error {
			return fail.Print("cld " + version + "\n")
		},
	}

	for _, command := range []*cobra.Command{root, newCommand, join, kill, list, help, versionCommand} {
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
	root.AddCommand(newCommand, join, kill, list, versionCommand)
	root.SetHelpCommand(help)

	// cobra's own help function, which cld's calls, writes the help to stdout and drops the error
	// of a write that fails, which would end cld with status 0: the help goes to a buffer
	// instead, which fail.Print prints.
	cobraHelp := root.HelpFunc()
	printHelp := func(c *cobra.Command) error {
		// Execute has moved the help command after the others as it ran: version goes back after
		// it, where cld lists version.
		root.RemoveCommand(versionCommand)
		root.AddCommand(versionCommand)
		var text bytes.Buffer
		c.SetOut(&text)
		cobraHelp(c, nil)
		c.SetOut(nil)
		return fail.Print(text.String())
	}
	root.SetHelpFunc(func(c *cobra.Command, _ []string) { *printed = printHelp(c) })
	help.RunE = func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return printHelp(root)
		}
		return printHelp(topic(root, args[0]))
	}
	return root
}

// nameUsage is -n and --name in the help of new, join and kill.
const nameUsage = "the session `NAME`: up to 64 letters, digits, \"_\" and \"-\",\nstarting with a letter or digit"

// sessionName is the NAME given with -n, checked once the options have been read: first its
// length, whatever its characters, then its characters.
func sessionName(name string) (string, error) {
	if utf8.RuneCountInString(name) > session.MaxName {
		return "", fail.Usage(fmt.Sprintf("session name '%s' is longer than %d characters (see cld help)", name, session.MaxName))
	}
	if !session.ValidName(name) {
		return "", fail.Usage(fmt.Sprintf("invalid session name '%s' (see cld help)", name))
	}
	return name, nil
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

// helpArguments takes help's COMMAND, one of cld's, and refuses a "--" before it, as noArguments
// does, a COMMAND that is not cld's, and an argument after it: the first of these decides.
func helpArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return unexpected(typed, "--")
		}
		if len(args) > 0 && topic(c.Root(), args[0]) == nil {
			return fail.Usage(fmt.Sprintf("%s: unknown command '%s' (see cld help)", typed, args[0]))
		}
		if len(args) > 1 {
			return unexpected(typed, args[1])
		}
		return nil
	}
}

// topic is the command named name among those the root's help lists, which help shows the help
// of; nil for any other name.
func topic(root *cobra.Command, name string) *cobra.Command {
	for _, command := range root.Commands() {
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
