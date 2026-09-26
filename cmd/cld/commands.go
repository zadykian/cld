package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/picker"
	"github.com/zadykian/cld/internal/session"
)

// commands maps each command, and each alias of one, to the command it runs: cld's, cobra's
// completion, and the hidden commands through which cobra's completion scripts ask cld what to
// offer, on every TAB.
var commands = map[string]string{
	"new": "new", "resume": "resume", "join": "join", "kill": "kill", "list": "list", "completion": "completion",
	"help": "help", "-h": "help", "--help": "help",
	"version": "version", "-V": "version", "--version": "version",
	cobra.ShellCompRequestCmd: cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd: cobra.ShellCompNoDescRequestCmd,
}

// run runs cld with the arguments args. The first is the command, which run checks before cobra
// sees it: cobra would take an unknown one for an argument of cld itself, and skip options before
// the command (cld -n x new would run new -n x).
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
	completing := command == cobra.ShellCompRequestCmd || command == cobra.ShellCompNoDescRequestCmd
	// The completion scripts always pass the word being completed, empty or not; without one,
	// cobra would fail with a message of its own and status 1.
	if completing && len(args) == 1 {
		return fail.Usage(fmt.Sprintf("%s: missing the word to complete (see cld help)", typed))
	}
	// What cobra prints - the help, the completion scripts, the answers to __complete - goes to
	// out, which cld then prints as its own output, so that a write that fails ends cld with
	// status 1 and cld's message. cobra's help function and __complete drop the error, which would
	// end cld with status 0, and the commands that print the scripts return it in Go's words
	// ("write /dev/stdout: ..."). Nothing is written when there is nothing to print, as for kill,
	// since even an empty write to a stdout that cannot take one fails.
	var out bytes.Buffer
	root := commandLine(typed, &out)
	root.SetArgs(append([]string{command}, args[1:]...))
	if err := root.Execute(); err != nil {
		return err
	}
	if out.Len() == 0 {
		return nil
	}
	text := out.String()
	if completing {
		text = noFiles(text)
	}
	return fail.Print(text)
}

// noFiles turns cobra's answer to __complete, text, into one that offers no file names where
// cobra's lets the shell offer them: cobra answers ShellCompDirectiveDefault, ":0" on the last
// line, where it cannot read the words before the one completed - an unknown command, an option
// the command does not have - whatever the root's default directive. No argument of cld's is a
// file there either.
func noFiles(text string) string {
	if rest, found := strings.CutSuffix(text, ":0\n"); found && (rest == "" || strings.HasSuffix(rest, "\n")) {
		return fmt.Sprintf("%s:%d\n", rest, cobra.ShellCompDirectiveNoFileComp)
	}
	return text
}

// commandLine is cld's commands, for a command typed as typed: the messages name it that way,
// "-V" for version, say. What cobra prints goes to out.
//
// The help is cobra's, from its default templates: each command's Use, and its Long or else its
// Short, then its options with their usages, which name their value in backquotes (`NAME`). Its
// text is here and nowhere else; cobra wraps none of it, so the lines break by hand, within 80
// columns. help, -h and --help print it to out, which run prints through fail.Print, and it lists
// the commands in the order they are added here rather than by name.
//
// cobra's defaults give way to cld's command line. main prints the errors, as "cld: MESSAGE".
// help takes one of cld's commands at most, and version is a command rather than cobra's
// --version and -v. Every command reads its options up to the first argument, which pflag would
// otherwise pass over, and takes no argument but help's COMMAND, resume's SESSION and
// completion's SHELL: the first one left - after COMMAND or SESSION, the next - or a "--", which
// pflag would drop, is refused.
//
// Completion is cobra's: completion SHELL prints the script, which asks __complete what to offer
// on every TAB. join -n offers the sessions list shows (see sessionNames), help the commands, and
// nothing offers file names, as no argument of cld's is a file.
func commandLine(typed string, out io.Writer) *cobra.Command {
	cobra.EnableCommandSorting = false // The commands in the order they are added.
	root := &cobra.Command{
		Use: "cld",
		Long: `Run Claude Code in named sessions, each on a private tmux server that ignores
~/.tmux.conf. Session NAME is the tmux session "cld-NAME" on the server
"tmux -L cld-NAME", running "claude --name cld-NAME" with Remote Control on.
What claude starts through tmux runs on that server too, and ends with it.

Detach with C-q d; C-q C-q sends C-q to claude. A session whose claude fails
stays, showing why, until cld kill ends it.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	// Before completion is made: its commands write their scripts to the output the root has then.
	root.SetOut(out)
	root.CompletionOptions.SetDefaultShellCompDirective(cobra.ShellCompDirectiveNoFileComp)
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
		// new starts claude, so it checks claude's version too, as resume does: after the checks
		// every command makes, so that cld runs claude only once the tools are found and tmux's
		// version passes, and before any other tmux command. join, kill, list and completion never
		// run claude.
		claude, err := session.CheckClaude()
		if err != nil {
			return err
		}
		return tmux.New(claude, suffix, *worktree)
	}

	// resume makes its session the way new does, and has claude resume a conversation in it. Its
	// usage line names its options before SESSION, as help's does before COMMAND.
	resume := &cobra.Command{
		Use:   "resume [-n NAME] [flags] [SESSION]",
		Short: "create session NAME with claude resuming its conversation",
		Long: `create session NAME in the current directory and attach to it, as new does,
with claude resuming the conversation named cld-NAME, or SESSION: whatever
claude --resume takes, such as a session ID, a name, or a search term for
claude's picker. SESSION comes after the options and does not start with "-".`,
		Args:                  conversationArgument(typed),
		DisableFlagsInUseLine: true,
	}
	resumeName := resume.Flags().StringP("name", "n", "main", nameUsage)
	resume.RunE = func(_ *cobra.Command, args []string) error {
		suffix, err := sessionName(*resumeName)
		if err != nil {
			return err
		}
		tmux, err := session.Check("claude")
		if err != nil {
			return err
		}
		// resume starts claude as new does, so it checks claude's version where new does.
		claude, err := session.CheckClaude()
		if err != nil {
			return err
		}
		conversation := "" // the conversation named cld-NAME
		if len(args) > 0 {
			conversation = args[0]
		}
		return tmux.Resume(claude, suffix, conversation)
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
	if err := join.RegisterFlagCompletionFunc("name", sessionNames); err != nil {
		panic(err)
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

	// list is interactive on a terminal it can draw on, other than a pane of one of cld's servers,
	// where join would refuse the session picked; with no sessions there is nothing to pick.
	// Leaving it prints the table, from the sessions it last read.
	list := &cobra.Command{
		Use:   "list",
		Short: "list the sessions cld started; on a terminal, join or kill one",
		Long: `list the sessions cld started: name, whether a terminal is attached (or claude
exited), and the directory claude is in.

On a terminal, pick one to join or kill: Up and Down select a session, Enter
joins it as cld join does, C-x twice within two seconds kills it as cld kill
does - Esc after the first C-x keeps it - and Esc or C-c leaves, printing the
list. cld list | cat prints the list only.`,
		RunE: func(*cobra.Command, []string) error {
			tmux, err := session.Check()
			if err != nil {
				return err
			}
			sessions, err := tmux.Sessions(context.Background())
			if err != nil {
				return err
			}
			if len(sessions) > 0 && picker.Available() {
				if _, own := tmux.OwnPane(); !own {
					picked, last, err := picker.Run(listSource{tmux}, sessions)
					if err != nil {
						return err
					}
					if picked != "" {
						return tmux.Attach(picked)
					}
					sessions = last
				}
			}
			return fail.Print(table(sessions))
		},
	}

	// cobra would add "[flags]" at the end of help's usage line, after COMMAND, where cld reads no
	// options. cobra's own help command completes COMMAND, and so does cld's.
	help := &cobra.Command{
		Use:                   "help [flags] [COMMAND]",
		Short:                 "show this help, or the help of COMMAND",
		Args:                  helpArguments(typed),
		ValidArgsFunction:     commandNames,
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

	for _, command := range []*cobra.Command{root, newCommand, resume, join, kill, list, help, versionCommand} {
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
	root.AddCommand(newCommand, resume, join, kill, list, versionCommand)
	root.SetHelpCommand(help)
	completionCommand(root)

	// cobra's own help function, which cld's calls, writes the help to the command's output, out
	// (see run). The help command is set with SetHelpCommand, so this one serves help as well as
	// -h and --help, those of completion's commands included.
	cobraHelp := root.HelpFunc()
	showHelp := func(c *cobra.Command) {
		// Execute has moved the help command after the others as it ran: version goes back after
		// it, where cld lists version.
		root.RemoveCommand(versionCommand)
		root.AddCommand(versionCommand)
		cobraHelp(c, nil)
	}
	root.SetHelpFunc(func(c *cobra.Command, _ []string) { showHelp(c) })
	help.RunE = func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			showHelp(root)
		} else {
			showHelp(topic(root, args[0]))
		}
		return nil
	}
	return root
}

// completionCommand adds cobra's completion command to root: completion SHELL prints the
// completion script for SHELL, bash, zsh, fish or powershell, and its help, cobra's Long, says
// where the script goes and what it needs; completion alone shows its help, as with cobra. The
// short descriptions, which the help of completion and of cld list, are cld's. They read their
// arguments as cld's commands do (see commandLine), with cld's -h and --help: an unknown SHELL,
// an argument after it or an unknown option is refused, where cobra would show the help and exit
// 0, fail with exit status 1, or take a later option first.
func completionCommand(root *cobra.Command) {
	root.InitDefaultCompletionCmd()
	completion, _, err := root.Find([]string{"completion"})
	if err != nil || completion == root {
		panic("cobra made no completion command")
	}
	completion.Short = "print the completion script for a shell"
	completion.Long = `print the completion script for a shell, one of the commands below. With it,
cld join -n completes the names cld list shows. The help of each command says
where its script goes and what it needs.`
	for _, shell := range completion.Commands() {
		shell.Short = "print the completion script for " + shell.Name()
	}
	// A command cobra cannot run shows its help before it looks at the arguments: completion runs,
	// to show it once they are read.
	completion.RunE = func(*cobra.Command, []string) error { return pflag.ErrHelp }
	completion.Args = func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return unexpected("completion", "--")
		}
		if len(args) > 0 {
			return fail.Usage(fmt.Sprintf("completion: unknown shell '%s' (see cld help)", args[0]))
		}
		return nil
	}
	for _, command := range append([]*cobra.Command{completion}, completion.Commands()...) {
		name := strings.TrimPrefix(command.CommandPath(), root.Name()+" ")
		command.Flags().VarPF(new(helpOption), "help", "h", "help for "+command.Name()).NoOptDefVal = "true"
		if command != completion {
			command.Args = noArguments(name)
		}
		command.Flags().SetInterspersed(false)
		command.SetFlagErrorFunc(flagError(name))
	}
}

// sessionNames completes the NAME of join -n: the names of the sessions list shows that start with
// what was typed, in list's order, each described by its state. Every one is a name join takes:
// list reads only the socket of a valid NAME, and only session cld-NAME on it (see
// session.Tmux.Sessions). It offers no file names, and it never fails: with no server there is
// nothing to offer, and with no tmux, one that fails, or a socket directory it cannot read
// neither, and cobra.CompErrorln says why on stderr, which the completion scripts discard.
func sessionNames(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var sessions []session.Session
	tmux, err := session.Find()
	if err == nil {
		sessions, err = tmux.Sessions(context.Background())
	}
	if err != nil {
		cobra.CompErrorln(err.Error())
	}
	var names []cobra.Completion
	for _, s := range sessions {
		if strings.HasPrefix(s.Name, typed) {
			names = append(names, cobra.CompletionWithDesc(s.Name, s.State))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// commandNames completes help's COMMAND, as cobra's help command does its own: the commands help
// takes (see topic) that start with what was typed, each described by its Short. Nothing follows
// COMMAND.
func commandNames(c *cobra.Command, args []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var names []cobra.Completion
	if len(args) == 0 {
		for _, command := range c.Root().Commands() {
			if topic(c.Root(), command.Name()) == command && strings.HasPrefix(command.Name(), typed) {
				names = append(names, cobra.CompletionWithDesc(command.Name(), command.Short))
			}
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// nameUsage is -n and --name in the help of new, resume, join and kill.
const nameUsage = "the session `NAME`: up to 64 letters, digits, \"_\" and \"-\",\nstarting with a letter or digit"

// sessionName is the NAME given with -n, checked once the options have been read: first its
// length, whatever its characters, then its characters.
func sessionName(name string) (string, error) {
	if utf8.RuneCountInString(name) > session.MaxName {
		return "", fail.Usage(fmt.Sprintf("session name '%s' is longer than %d characters (see cld help)", name, session.MaxName))
	}
	if !session.ValidName(name) {
		return "", &fail.Error{Status: 2, Message: fmt.Sprintf("invalid session name '%s'", name), Advice: " (see cld help)"}
	}
	return name, nil
}

// listSource is what the interactive list reads and acts through: the sessions to list, join's
// checks, which Enter makes while the list is open - the name, then the lookup - and kill's steps,
// which the second Ctrl+X takes.
type listSource struct{ tmux *session.Tmux }

func (l listSource) Sessions(ctx context.Context) ([]session.Session, error) {
	return l.tmux.Sessions(ctx)
}

func (l listSource) Joinable(ctx context.Context, name string) error {
	if _, err := sessionName(name); err != nil {
		return err
	}
	return l.tmux.Joinable(ctx, name)
}

// Kill is kill's steps - the name, then End - with End's check that the session is still the one
// whose panes' pids the list read. What tmux says when the kill - kill-session, then kill-server -
// fails becomes the error, for the list's footer, rather than going to the terminal the list draws
// on.
func (l listSource) Kill(ctx context.Context, name string, pids []string) error {
	if _, err := sessionName(name); err != nil {
		return err
	}
	var said bytes.Buffer
	err := l.tmux.End(ctx, name, pids, &said, &said)
	var status fail.Status
	if errors.As(err, &status) {
		if message := strings.TrimSpace(said.String()); message != "" {
			return fail.Runtime(message)
		}
		return fail.Runtime("tmux kill-session: " + status.Error())
	}
	return err
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

// conversationArgument takes resume's one argument, SESSION, the conversation claude resumes. It
// refuses an empty one and one starting with "-", which claude would read as an option;
// anything after SESSION, an option too, since options come first; and a "--", which pflag
// would drop, handing claude what follows it as SESSION (resume -n x -- -p).
func conversationArgument(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return unexpected(typed, "--")
		}
		if len(args) > 0 && (args[0] == "" || strings.HasPrefix(args[0], "-")) {
			return unexpected(typed, args[0])
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
