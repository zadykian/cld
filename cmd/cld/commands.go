package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/completion"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/picker"
	"github.com/zadykian/cld/internal/project"
	"github.com/zadykian/cld/internal/session"
	"github.com/zadykian/cld/internal/telemetry"
	"github.com/zadykian/cld/internal/update"
)

// commands maps each command, and each alias of one, to the command it runs: cld's, cobra's
// completion, and the hidden commands through which cobra's completion scripts ask cld what to
// offer, on every TAB.
var commands = map[string]string{
	"new": "new", "resume": "resume", "join": "join", "kill": "kill", "list": "list", "setup": "setup",
	"update": "update", "completion": "completion",
	"help": "help", "-h": "help", "--help": "help",
	"version": "version", "-V": "version", "--version": "version",
	cobra.ShellCompRequestCmd: cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd: cobra.ShellCompNoDescRequestCmd,
}

// run runs cld with the arguments args. The first is the command, which run checks before cobra
// sees it: cobra would take an unknown one for an argument of cld itself, and skip options before
// the command (cld -n x new would run new -n x). setup has commands of its own, and the argument
// after it is checked the same way (see setupCommand) where setup runs: __complete setup, which
// runs nothing, completes it instead.
func run(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fail.Usage("missing command: cld new creates a session, cld join attaches to one (see cld help)")
	}
	typed := args[0]
	command, known := commands[typed]
	if !known {
		// Before cld had commands, "cld NAME" attached to session NAME, creating it first.
		if session.ValidName(typed) {
			return fail.Usage(fmt.Sprintf("unknown command '%s'; for a session %[1]s-SUFFIX: cld new -n %[1]s, cld join -n %[1]s -s SUFFIX", typed))
		}
		return fail.Usage(fmt.Sprintf("unknown command '%s' (see cld help)", typed))
	}
	if command == "setup" {
		if err := setupCommand(args[1:]); err != nil {
			return err
		}
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
	return output.Print(text)
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

// setupCommand checks the argument after setup before cobra sees it, as run checks the first: it
// names one of setup's commands, project, telemetry or completion, or is -h or --help, setup's
// help. cobra would run telemetry for cld setup --local URL telemetry, taking --local for an
// option of setup's. completion has commands of its own, one for each shell, and the argument
// after it is checked the same way: cobra would run zsh's for setup completion --help=false zsh.
func setupCommand(args []string) error {
	const hint = "cld setup project, cld setup telemetry or cld setup completion SHELL (see cld help)"
	switch {
	case len(args) == 0 || args[0] == "":
		return fail.Usage("setup: missing command: " + hint)
	case args[0] == "completion":
		return shellArgument(args[1:])
	case args[0] == "project" || args[0] == "telemetry" || args[0] == "-h" || args[0] == "--help":
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

// commandLine is cld's commands, for a command typed as typed: the messages name it that way,
// "-V" for version, say. What cobra prints goes to out.
//
// The help is cobra's, from its default templates: each command's Use, and its Long or else its
// Short, then its options with their usages, which name their value in backquotes (`NAME`). Its
// text is here and nowhere else; cobra wraps none of it, so the lines break by hand, within 80
// columns. help, -h and --help print it to out, which run prints through output.Print, and it lists
// the commands in the order they are added here rather than by name.
//
// cobra's defaults give way to cld's command line. main prints the errors, as "cld: MESSAGE". help
// takes one of cld's commands at most - with setup, one of setup's after it, and a shell after
// setup completion - and version is a command rather than cobra's --version and -v. Every command
// reads its options up to the first argument, which pflag would otherwise pass over, and takes no
// argument but help's COMMAND, resume's SESSION and completion's SHELL: the first one left - after
// COMMAND or SESSION, the next - or a "--", which pflag would drop, is refused.
//
// Completion is cobra's: completion SHELL prints the script, which asks __complete what to offer
// on every TAB, and setup completion SHELL writes it where the shell reads it (see
// setupCompletion). join -n offers the NAME of NAME-SUFFIX for the sessions list shows (see
// sessionNames), join -s their SUFFIX (see sessionSuffixes), help the commands (see
// commandNames), setup project --mcp the MCP servers (see serverNames), and nothing offers file
// names, as no argument of cld's is a file.
//
// new, resume, join and kill name their session NAME-SUFFIX with -n NAME and -s SUFFIX (see
// naming): NAME defaults to the repository's or directory's name, and SUFFIX, for new and for
// resume with SESSION, to the next index; join and kill need -s, and resume -s or SESSION.
func commandLine(typed string, out io.Writer) *cobra.Command {
	cobra.EnableCommandSorting = false // The commands in the order they are added.
	root := &cobra.Command{
		Use: "cld",
		Long: `Run Claude Code in named sessions, each on a private tmux server that ignores
~/.tmux.conf. A session is named NAME-SUFFIX: NAME is by default the name of
the git repository the current directory is in, or else of the directory, and
SUFFIX, for a new session, the next index. Session S is the tmux session
"cld-S" on the server "tmux -L cld-S", running "claude --name cld-S" with
Remote Control on. What claude starts through tmux runs on that server too,
and ends with it.

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
		Use:   "new [-n NAME] [-s SUFFIX] [-w]",
		Short: "create session NAME-SUFFIX in this directory and attach to it",
		Long: `create session NAME-SUFFIX in the current directory and attach to it. NAME is
by default the name of the git repository the directory is in, or else of the
directory itself; SUFFIX is by default INDEX, 0 or, where sessions NAME-INDEX
run, one above the highest of their INDEX. Where the directory's name leaves
nothing, as in /, the session is SUFFIX alone. NAME and SUFFIX consist of
letters, digits, "_" and "-", each starting with a letter or digit, and make
64 characters at most.`,
	}
	newNaming := addNaming(newCommand, "the session's `SUFFIX`, after NAME-: by default the index\n"+
		"above the highest of the sessions NAME-INDEX, or 0")
	worktree := newCommand.Flags().BoolP("worktree", "w", false,
		"run claude in git worktree cld-NAME-SUFFIX, which claude\nmakes from HEAD or reopens (claude --worktree\ncld-NAME-SUFFIX)")
	newCommand.RunE = func(*cobra.Command, []string) error {
		if err := newNaming.check(typed, ""); err != nil {
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
		suffix, err := newNaming.resolve(tmux)
		if err != nil {
			return err
		}
		return tmux.New(claude, suffix, *worktree)
	}

	// resume makes its session the way new does, and has claude resume a conversation in it. Its
	// usage line names its options before SESSION, as help's does before COMMAND.
	resume := &cobra.Command{
		Use:   "resume [-n NAME] [-s SUFFIX] [flags] [SESSION]",
		Short: "create session NAME-SUFFIX with claude resuming its conversation",
		Long: `create session NAME-SUFFIX in the current directory and attach to it, as new
does, with claude resuming the conversation named cld-NAME-SUFFIX, or SESSION:
whatever claude --resume takes, such as a session ID, a name, or a search term
for claude's picker. SESSION comes after the options and does not start with
"-". Without SESSION, -s is needed.`,
		Args:                  conversationArgument(typed),
		DisableFlagsInUseLine: true,
	}
	resumeNaming := addNaming(resume, "the session's `SUFFIX`, after NAME-: with SESSION, by\n"+
		"default the index new would give")
	resume.RunE = func(_ *cobra.Command, args []string) error {
		missing := "-s SUFFIX or SESSION (see cld help)"
		if len(args) > 0 {
			missing = ""
		}
		if err := resumeNaming.check(typed, missing); err != nil {
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
		suffix, err := resumeNaming.resolve(tmux)
		if err != nil {
			return err
		}
		conversation := "" // the conversation named cld-NAME
		if len(args) > 0 {
			conversation = args[0]
		}
		return tmux.Resume(claude, suffix, conversation)
	}

	// join attaches beside the terminals on the session, which --detach-others detaches.
	join := &cobra.Command{
		Use:   "join [-n NAME] -s SUFFIX [--detach-others]",
		Short: "attach to session NAME-SUFFIX",
		Long: `attach to session NAME-SUFFIX, beside any terminal attached to it already:
each shows claude, whose window takes the size of the terminal used last. With
--detach-others, those terminals are detached.`,
	}
	joinNaming := addNaming(join, "the session's `SUFFIX`, after NAME-")
	detachOthers := join.Flags().Bool("detach-others", false, "detach any other terminal attached to the session")
	join.RunE = func(*cobra.Command, []string) error {
		if err := joinNaming.check(typed, "-s SUFFIX (see cld list)"); err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		suffix, err := joinNaming.resolve(tmux)
		if err != nil {
			return err
		}
		return tmux.Join(suffix, *detachOthers)
	}
	if err := join.RegisterFlagCompletionFunc("name", sessionNames); err != nil {
		panic(err)
	}
	if err := join.RegisterFlagCompletionFunc("suffix", sessionSuffixes); err != nil {
		panic(err)
	}

	kill := &cobra.Command{
		Use:   "kill [-n NAME] -s SUFFIX",
		Short: "end session NAME-SUFFIX and its tmux server",
		Long: `end session NAME-SUFFIX and its tmux server: claude exits as when its terminal
closes, and what claude started through tmux ends too`,
	}
	killNaming := addNaming(kill, "the session's `SUFFIX`, after NAME-")
	kill.RunE = func(*cobra.Command, []string) error {
		if err := killNaming.check(typed, "-s SUFFIX (see cld list)"); err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		suffix, err := killNaming.resolve(tmux)
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
						return tmux.Attach(picked, false)
					}
					sessions = last
				}
			}
			return output.Print(table(sessions))
		},
	}

	// setup runs nothing itself, so cobra's help shows its commands without a usage line of its
	// own; run has made sure that one of them follows it, or -h or --help.
	setup := &cobra.Command{
		Use:   "setup",
		Short: "set up claude in a project, its telemetry, or shell completion",
	}
	projectCommand := setupProject(typed + " project")
	telemetryCommand := setupTelemetry(typed + " telemetry")
	shellsCommand := setupCompletion(typed + " completion")
	setup.AddCommand(projectCommand, telemetryCommand, shellsCommand)

	// update runs neither tmux nor claude, so it makes none of their checks: it needs the network
	// and the directory cld is in, which internal/update checks as it goes.
	updateCommand := &cobra.Command{
		Use:   "update",
		Short: "update cld to the latest release",
		Long: `update cld to the latest release on GitHub: download the release's cld for
this system, check it against the release's cld.sha256 and that it runs, then
replace the file cld runs from with it - the file a symbolic link leads to.
Where cld is the latest release already, or newer, nothing changes; a cld built
from source, cld dev, is not updated. Once cld is replaced, the scripts that
cld setup completion wrote are written anew where the new cld prints others.`,
		RunE: func(*cobra.Command, []string) error {
			result, err := update.Run(version)
			if err != nil {
				return err
			}
			if err := output.Print(result.Report()); err != nil || result.File == "" {
				return err
			}
			// The new cld prints the completion scripts: its release's cobra may write others. cld
			// is updated by then, so a script not written is a warning, not a failure.
			report, warnings := completion.Refresh(result.File)
			for _, warning := range warnings {
				output.Warn(warning)
			}
			if report == "" {
				return nil
			}
			return output.Print(report)
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
			return output.Print("cld " + version + "\n")
		},
	}

	all := []*cobra.Command{root, newCommand, resume, join, kill, list, setup, projectCommand, telemetryCommand, shellsCommand}
	all = append(append(all, shellsCommand.Commands()...), updateCommand, help, versionCommand)
	for _, command := range all {
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
	root.AddCommand(newCommand, resume, join, kill, list, setup, updateCommand, versionCommand)
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
		command, err := helpTopic(root, typed, args)
		if err != nil {
			return err
		}
		showHelp(command)
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
cld join -n and -s complete the sessions cld list shows. cld setup completion
SHELL writes it where bash, zsh or fish reads it; the help of each command
below says where the script goes by hand, and what it needs.`
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

// setupCompletion is cld setup completion, named in its messages as typed, with a command for each
// shell of completion.Shells: each writes the script that cld completion SHELL prints, which
// cobra generates as it does there, where the shell reads it (see internal/completion).
// setupCommand has made sure that a shell follows setup completion, or -h or --help, so it runs
// nothing itself, as setup does not.
func setupCompletion(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "completion",
		Short: "set up cld's completion in bash, zsh or fish",
		Long: `set up cld's completion in a shell, one of the commands below: cld writes the
script that cld completion SHELL prints where the shell reads it. TAB then
completes cld's commands, their options and the names cld list shows. cld
update writes the script anew where the release it installs prints another.`,
		Args: noArguments(typed),
	}
	command.SetFlagErrorFunc(flagError(typed))
	long := map[string]string{
		"bash": `set up cld's completion in bash: write the script cld completion bash prints
to completions/cld in the first directory of $BASH_COMPLETION_USER_DIR, or else
to ~/.local/share/bash-completion/completions/cld ($XDG_DATA_HOME in place of
~/.local/share where set), where bash-completion 2 finds it at the first TAB.
It needs ~/.bashrc to load bash-completion, as Debian's and Ubuntu's do;
bash-completion 1, the one for macOS's bash 3.2, does not read that directory.`,
		"zsh": `set up cld's completion in zsh: write the script cld completion zsh prints to
~/.local/share/cld/zsh/_cld ($XDG_DATA_HOME in place of ~/.local/share where
set), and add the lines that load it to the end of ~/.zshrc ($ZDOTDIR/.zshrc
where ZDOTDIR is set), unless it has them. They run compinit only where
nothing before them has: a second compinit would drop the completions set up
after the first. Where the script is missing, they do nothing.`,
		"fish": `set up cld's completion in fish: write the script cld completion fish prints
to ~/.config/fish/completions/cld.fish ($XDG_CONFIG_HOME in place of ~/.config
where set), where fish finds it at the first TAB.`,
	}
	for _, shell := range completion.Shells {
		name := typed + " " + shell
		shellCommand := &cobra.Command{
			Use:   shell,
			Short: "set up cld's completion in " + shell,
			Long:  long[shell],
			Args:  noArguments(name),
			RunE: func(c *cobra.Command, _ []string) error {
				var script bytes.Buffer
				var err error
				switch shell {
				case "bash":
					err = c.Root().GenBashCompletionV2(&script, true)
				case "zsh":
					err = c.Root().GenZshCompletion(&script)
				default:
					err = c.Root().GenFishCompletion(&script, true)
				}
				if err != nil {
					return fail.Runtime("cannot make the script: " + err.Error())
				}
				report, err := completion.Setup(shell, script.Bytes())
				if err != nil {
					return err
				}
				return output.Print(report)
			},
		}
		shellCommand.SetFlagErrorFunc(flagError(name))
		command.AddCommand(shellCommand)
	}
	return command
}

// setupProject is cld setup project, named in its messages as typed. --mcp takes the MCP servers
// as a list, given again or separated by commas; each is written once, in project.Servers' order,
// whatever the order given.
func setupProject(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "project [--mcp SERVER]",
		Short: "set claude up in the project in the current directory",
		Long: `set claude up in the project in the current directory, as cld's own repository
has it: .claude/settings.json holds the settings the project shares through
git - what claude may do without asking, and a few settings more - and
.claude/settings.local.json, holding its $schema alone, is for your own.
.gitignore gets /.claude/* and !/.claude/settings.json: git ignores what claude
keeps in .claude, but for the shared settings. With --mcp, cld adds MCP servers
to .mcp.json, and the settings enable them and let claude use them.

Where the files exist, cld adds what they lack: it sets its settings, adds its
permissions and servers, and keeps everything else - settings.local.json whole.
Then, in a git work tree, it checks that git does not ignore the settings.`,
		Args: noArguments(typed),
	}
	command.SetFlagErrorFunc(flagError(typed))
	mcp := command.Flags().StringArray("mcp", nil, "an MCP `SERVER` for claude in the project: goland or rider,\n"+
		"the IDE's own server on 127.0.0.1, at the port in\n"+
		"GOLAND_MCP_PORT or RIDER_MCP_PORT where claude runs, else\n"+
		"the IDE's default, 64422 or 64482; or jbcontext, JetBrains\n"+
		"Context's code search. Give --mcp again, or separate them\n"+
		"with commas")
	command.RunE = func(*cobra.Command, []string) error {
		chosen := map[string]bool{}
		for _, list := range *mcp {
			for _, name := range strings.Split(list, ",") {
				if !slices.ContainsFunc(project.Servers, func(s project.Server) bool { return s.Name == name }) {
					return fail.Usage(fmt.Sprintf("invalid MCP server '%s' for --mcp: goland, jbcontext or rider (see cld help)", name))
				}
				chosen[name] = true
			}
		}
		var servers []project.Server
		for _, s := range project.Servers {
			if chosen[s.Name] {
				servers = append(servers, s)
			}
		}
		return project.Setup(servers)
	}
	if err := command.RegisterFlagCompletionFunc("mcp", serverNames); err != nil {
		panic(err)
	}
	return command
}

// setupTelemetry is cld setup telemetry, named in its messages as typed. On a system other than
// Linux it refuses to run before it looks at anything but -h and --help: an option or argument
// it would refuse too (see telemetry.Supported).
func setupTelemetry(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "telemetry [--local URL] [--remote URL] [flags]",
		Short: "send claude's telemetry through a local OpenTelemetry collector",
		Long: `send claude's telemetry through a local OpenTelemetry collector: cld runs the
collector in Docker, as the container cld-telemetry listening on 127.0.0.1, and
points claude's user settings at it ($CLAUDE_CONFIG_DIR/settings.json, by
default ~/.claude/settings.json). The collector sends traces, metrics and logs
to --local, such as the JetBrains OpenTelemetry plugin in the IDE, and metrics
only to --remote, such as a team's collector; give one of them, or both.

Running it again replaces the collector and rewrites the settings; the env keys
cld does not manage stay as they are. claude reads its settings as a session
starts: sessions running then keep theirs. Needs Docker; Linux only.`,
		Args: func(c *cobra.Command, args []string) error {
			if err := telemetry.Supported(); err != nil {
				return err
			}
			return noArguments(typed)(c, args)
		},
	}
	command.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		err = flagError(typed)(c, err)
		if unsupported := telemetry.Supported(); unsupported != nil && !errors.Is(err, pflag.ErrHelp) {
			return unsupported
		}
		return err
	})
	flags := command.Flags()
	local := flags.String("local", "", "where traces, metrics and logs go, such as\n"+
		"http://127.0.0.1:4319 (the IDE's plugin on a\n"+
		"fixed port); `URL` is http://HOST:PORT for\n"+
		"plaintext gRPC or https://HOST:PORT for TLS")
	remote := flags.String("remote", "", "where metrics go too (`URL` as for --local), such\n"+
		"as https://otel.example.com:4317")
	port := flags.String("port", "", "the `PORT` the collector listens on, on 127.0.0.1\n"+
		"(default: the one the collector has, or one\n"+
		"the kernel picks)")
	collectorConfig := flags.String("collector-config", "", "`FILE` holds YAML that the collector merges over\n"+
		"cld's config, such as headers or TLS for the\n"+
		"exporters otlp_grpc/local and otlp_grpc/remote")
	command.RunE = func(c *cobra.Command, _ []string) error {
		var options telemetry.Options
		for _, endpoint := range []struct {
			name, url string
			into      **telemetry.Endpoint
		}{{"local", *local, &options.Local}, {"remote", *remote, &options.Remote}} {
			if !c.Flags().Changed(endpoint.name) {
				continue
			}
			parsed, ok := telemetry.ParseEndpoint(endpoint.url)
			if !ok {
				return fail.Usage(fmt.Sprintf("invalid URL '%s' for --%s: http://HOST:PORT or https://HOST:PORT (see cld help)", endpoint.url, endpoint.name))
			}
			*endpoint.into = &parsed
		}
		if c.Flags().Changed("port") {
			var ok bool
			if options.Port, ok = telemetry.ParsePort(*port); !ok {
				return fail.Usage(fmt.Sprintf("invalid port '%s': a number from 1 to 65535 (see cld help)", *port))
			}
		}
		if options.Local == nil && options.Remote == nil {
			return fail.Usage(typed + ": --local URL, --remote URL or both are needed (see cld help)")
		}
		if option := options.Collector(options.Port); options.Port != 0 && option != "" {
			return fail.Usage(fmt.Sprintf("%s is where the collector would listen (--port %d): give the receiver's port, or another --port (see cld help)",
				option, options.Port))
		}
		if c.Flags().Changed("collector-config") {
			options.CollectorConfig = *collectorConfig
			if options.CollectorConfig == "" {
				return fail.Usage("option '--collector-config' needs a value (see cld help)")
			}
		}
		return telemetry.Setup(options)
	}
	return command
}

// sessionNames completes the NAME of join -n: for the sessions list shows, what comes before the
// last "-" of their names, where that and what follows it are both NAMEs - the split cld's own
// messages name a session by (see session.Options) - once each, in list's order, and each
// described by the number of its sessions. With -s then, join takes each of those sessions.
func sessionNames(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var names []string
	counts := map[string]int{}
	for _, s := range listed() {
		i := strings.LastIndexByte(s.Name, '-')
		if i <= 0 || !session.ValidName(s.Name[:i]) || !session.ValidName(s.Name[i+1:]) || !strings.HasPrefix(s.Name[:i], typed) {
			continue
		}
		if counts[s.Name[:i]]++; counts[s.Name[:i]] == 1 {
			names = append(names, s.Name[:i])
		}
	}
	completions := make([]cobra.Completion, 0, len(names))
	for _, name := range names {
		description := "1 session"
		if counts[name] > 1 {
			description = strconv.Itoa(counts[name]) + " sessions"
		}
		completions = append(completions, cobra.CompletionWithDesc(name, description))
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// sessionSuffixes completes the SUFFIX of join -s: for the sessions list shows whose names are
// NAME-SUFFIX with the NAME join takes - -n's, or else the repository's or directory's (see
// session.DefaultName) - their SUFFIX, where it starts with what was typed and join takes it, in
// list's order, each described by its state. Where NAME is "", every name is a SUFFIX.
func sessionSuffixes(c *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	name := session.DefaultName()
	if c.Flags().Changed("name") {
		name, _ = c.Flags().GetString("name")
	}
	if name != "" {
		name += "-"
	}
	var suffixes []cobra.Completion
	for _, s := range listed() {
		if suffix, found := strings.CutPrefix(s.Name, name); found && strings.HasPrefix(suffix, typed) && session.ValidName(suffix) {
			suffixes = append(suffixes, cobra.CompletionWithDesc(suffix, s.State))
		}
	}
	return suffixes, cobra.ShellCompDirectiveNoFileComp
}

// listed is the sessions list shows, for completion. It never fails: with no server there is
// nothing to offer, and with no tmux, one that fails, or a socket directory it cannot read
// neither, and cobra.CompErrorln says why on stderr, which the completion scripts discard.
func listed() []session.Session {
	var sessions []session.Session
	tmux, err := session.Find()
	if err == nil {
		sessions, err = tmux.Sessions(context.Background())
	}
	if err != nil {
		cobra.CompErrorln(err.Error())
	}
	return sessions
}

// serverNames completes the SERVER of setup project --mcp: the servers it takes that start with
// what was typed, each described. After a comma it completes the last of the list, offering the
// servers the list does not have yet after the ones it has.
func serverNames(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
	before, last := "", typed
	if comma := strings.LastIndexByte(typed, ','); comma >= 0 {
		before, last = typed[:comma+1], typed[comma+1:]
	}
	var names []cobra.Completion
	for _, s := range project.Servers {
		if strings.HasPrefix(s.Name, last) && !slices.Contains(strings.Split(before, ","), s.Name) {
			names = append(names, cobra.CompletionWithDesc(before+s.Name, s.Description))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// commandNames completes help's COMMAND, as cobra's help command does its own: the commands help
// takes (see helpTopic) that start with what was typed, each described by its Short - cld's, and
// after one with commands of its own, such as setup, those. Nothing follows any other COMMAND.
func commandNames(c *cobra.Command, args []string, typed string) ([]cobra.Completion, cobra.ShellCompDirective) {
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

// nameUsage is -n in the help of new, resume, join and kill: its first line fits within 80
// columns beside -s's own column, as wide as join's --detach-others.
const nameUsage = "the session's `NAME`, before -SUFFIX: by default the\ngit repository's name here, or else the directory's"

// naming is the -n and -s of new, resume, join and kill, which name the session NAME-SUFFIX: NAME
// is -n's, or else the name of the git repository the current directory is in, or of the
// directory (see session.DefaultName) - where that leaves nothing, the session is SUFFIX alone -
// and SUFFIX -s's, or else, for new and for resume with SESSION, the next index (see
// session.Tmux.Next). join and kill, and resume without SESSION, need -s.
type naming struct {
	flags        *pflag.FlagSet
	name, suffix *string
}

// addNaming gives command -n and -s, whose usage is suffixUsage.
func addNaming(command *cobra.Command, suffixUsage string) naming {
	return naming{
		flags:  command.Flags(),
		name:   command.Flags().StringP("name", "n", "", nameUsage),
		suffix: command.Flags().StringP("suffix", "s", "", suffixUsage),
	}
}

// check checks -n and -s once the options have been read, before anything runs: each that was
// given - its length, whatever its characters, then its characters, as SUFFIX alone names the
// session where the directory's name leaves nothing (see session.DefaultName) - then the name
// both make, which is too long for a session's where they make it longer than one NAME can be
// (see tooLong); and, where missing is not empty, that -s was given, which missing says is
// missing, for the command typed as typed.
func (n naming) check(typed, missing string) error {
	name, suffix := n.flags.Changed("name"), n.flags.Changed("suffix")
	if name {
		if _, err := sessionName(*n.name); err != nil {
			return err
		}
	}
	switch {
	case suffix:
		if utf8.RuneCountInString(*n.suffix) > session.MaxName {
			return fail.Usage(fmt.Sprintf("suffix '%s' is longer than %d characters (see cld help)", *n.suffix, session.MaxName))
		}
		if !session.ValidName(*n.suffix) {
			return fail.Usage(fmt.Sprintf("invalid suffix '%s' (see cld help)", *n.suffix))
		}
		if both := *n.name + "-" + *n.suffix; name && len(both) > session.MaxName {
			return tooLong(2, both)
		}
	case missing != "":
		return fail.Usage(typed + ": missing " + missing)
	}
	return nil
}

// tooLong refuses session name, longer than a session's name can be, with status.
func tooLong(status int, name string) error {
	return &fail.Error{Status: status,
		Message: fmt.Sprintf("session name '%s' is longer than %d characters", name, session.MaxName),
		Advice:  "; give a shorter -n NAME or -s SUFFIX (see cld help)"}
}

// resolve is the session's name, NAME-SUFFIX, once tmux has been checked: -n's NAME, or else the
// repository's or directory's, and -s's SUFFIX, or else the next index (see session.Tmux.Next).
// One longer than a session's name can be is refused with status 1: the repository's or
// directory's name, or the index, makes it so, not the command line alone (see check).
func (n naming) resolve(tmux *session.Tmux) (string, error) {
	name := *n.name
	if !n.flags.Changed("name") {
		name = session.DefaultName()
	}
	if name != "" {
		name += "-"
	}
	if n.flags.Changed("suffix") {
		name += *n.suffix
	} else {
		var err error
		if name, err = tmux.Next(context.Background(), name); err != nil {
			return "", err
		}
	}
	if len(name) > session.MaxName {
		return "", tooLong(1, name)
	}
	return name, nil
}

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

// helpArguments takes help's COMMAND, one of cld's, then one of its own commands, such as setup's
// after setup and a shell after setup completion, and refuses a "--" before it, as noArguments
// does, a COMMAND that is not cld's, and an argument after it: the first of these decides.
func helpArguments(typed string) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return unexpected(typed, "--")
		}
		_, err := helpTopic(c.Root(), typed, args)
		return err
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

// helpTopic is the command whose help help shows, given args: the root for none, else the
// command they name, one of cld's, then one of its own commands if it has them, as in help setup
// telemetry. help is named as typed in the errors.
func helpTopic(root *cobra.Command, typed string, args []string) (*cobra.Command, error) {
	command := root
	for i, name := range args {
		if command != root && !command.HasAvailableSubCommands() {
			return nil, unexpected(typed, name)
		}
		if command = topic(command, name); command == nil {
			return nil, fail.Usage(fmt.Sprintf("%s: unknown command '%s' (see cld help)", typed, strings.Join(args[:i+1], " ")))
		}
	}
	return command, nil
}

// topic is the command named name among those the help of parent lists, which help shows the
// help of; nil for any other name.
func topic(parent *cobra.Command, name string) *cobra.Command {
	for _, command := range parent.Commands() {
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
