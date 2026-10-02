package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/completion"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
)

// bleLines go at the end of __start_cld, the function of cobra's bash script that completes cld,
// for bash with ble.sh (decision 27); %d is cobra.ShellCompDirectiveNoFileComp.
const bleLines = "" +
	`    # ble.sh has the lines above call a compopt function of its own, which they take for no
    # compopt: turn off -o default as they would, and ble.sh's completions where cld offers none.
    if [[ ${BLE_VERSION-} && $(type -t compopt) == function ]] && (((directive & %d) != 0)); then
        compopt +o default +o ble/default
    fi
`

// completionLong is the help of completion.
const completionLong = `print the completion script for a shell, one of the commands below. With it,
-n and -s of cld join complete the sessions cld list shows, and of cld detach
those that run. cld setup completion SHELL writes it where bash, zsh or fish
reads it; the help of each command below says where the script goes by hand,
and what it needs.`

// shellLongs is the help of setup completion's command for each shell.
var shellLongs = map[string]string{
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

// bashScript writes to w cobra's bash script for root, with descriptions or without, and
// bleLines at the end of the function that completes root (decision 27.2).
func bashScript(w io.Writer, root *cobra.Command, descriptions bool) error {
	var script bytes.Buffer
	if err := root.GenBashCompletionV2(&script, descriptions); err != nil {
		return err
	}
	end := "\n    __" + root.Name() + "_process_completion_results\n}\n"
	before, after, found := strings.Cut(script.String(), end)
	if !found || strings.Contains(after, end) {
		panic("cobra's bash script has no end of __start_" + root.Name())
	}
	lines := fmt.Sprintf(bleLines, cobra.ShellCompDirectiveNoFileComp)
	_, err := io.WriteString(w, before+strings.TrimSuffix(end, "}\n")+lines+"}\n"+after)
	return err
}

// completionCommand adds cobra's completion command to root, with cld's help texts and bash's
// script (see bashScript). Its commands read their arguments as cld's do (decision 17.5), and
// completion alone shows its help, as with cobra.
func completionCommand(root *cobra.Command) {
	root.InitDefaultCompletionCmd()
	parent, _, err := root.Find([]string{"completion"})
	if err != nil || parent == root {
		panic("cobra made no completion command")
	}
	parent.Short = "print the completion script for a shell"
	parent.Long = completionLong
	for _, shell := range parent.Commands() {
		shell.Short = "print the completion script for " + shell.Name()
		if shell.Name() == "bash" {
			shell.RunE = func(c *cobra.Command, _ []string) error {
				noDescriptions, err := c.Flags().GetBool("no-descriptions")
				if err != nil {
					return err
				}
				return bashScript(c.OutOrStdout(), root, !noDescriptions)
			}
		}
	}
	// A command cobra cannot run shows its help before it looks at the arguments: completion
	// runs, to show it once they are read.
	parent.RunE = func(*cobra.Command, []string) error { return pflag.ErrHelp }
	parent.Args = func(c *cobra.Command, args []string) error {
		if c.ArgsLenAtDash() >= 0 {
			return cmdline.Unexpected("completion", "--")
		}
		if len(args) > 0 {
			return fail.Usage(fmt.Sprintf("completion: unknown shell '%s' (see cld help)", args[0]))
		}
		return nil
	}
	for _, command := range append([]*cobra.Command{parent}, parent.Commands()...) {
		name := strings.TrimPrefix(command.CommandPath(), root.Name()+" ")
		cmdline.AddHelp(command)
		if command != parent {
			command.Args = cmdline.NoArguments(name)
		}
		command.Flags().SetInterspersed(false)
		command.SetFlagErrorFunc(cmdline.FlagError(name))
	}
}

// setupCompletion is cld setup completion, named in its messages as typed, with a command for
// each of completion.Shells (decision 22). It runs nothing itself: setupCommand has made sure
// that a shell, or -h or --help, follows it.
func setupCompletion(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "completion",
		Short: "set up cld's completion in bash, zsh or fish",
		Long: `set up cld's completion in a shell, one of the commands below: cld writes the
script that cld completion SHELL prints where the shell reads it. TAB then
completes cld's commands, their options and the names cld list shows. cld
update writes the script anew where the release it installs prints another.`,
		Args: cmdline.NoArguments(typed),
	}
	command.SetFlagErrorFunc(cmdline.FlagError(typed))
	for _, shell := range completion.Shells {
		name := typed + " " + shell
		shellCommand := &cobra.Command{
			Use:   shell,
			Short: "set up cld's completion in " + shell,
			Long:  shellLongs[shell],
			Args:  cmdline.NoArguments(name),
			RunE: func(c *cobra.Command, _ []string) error {
				return writeScript(c.Root(), shell)
			},
		}
		shellCommand.SetFlagErrorFunc(cmdline.FlagError(name))
		command.AddCommand(shellCommand)
	}
	return command
}

// writeScript writes the script that cld completion shell prints, which cobra makes for root,
// where the shell reads it (see internal/completion).
func writeScript(root *cobra.Command, shell string) error {
	var script bytes.Buffer
	var err error
	switch shell {
	case "bash":
		err = bashScript(&script, root, true)
	case "zsh":
		err = root.GenZshCompletion(&script)
	default:
		err = root.GenFishCompletion(&script, true)
	}
	if err != nil {
		return fail.Runtime("cannot make the script: " + err.Error())
	}
	report, err := completion.Setup(shell, script.Bytes())
	if err != nil {
		return err
	}
	return output.Print(report)
}
