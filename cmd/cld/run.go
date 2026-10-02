package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
)

// commands maps each command, and each alias of one, to the command it runs. They are cld's,
// cobra's completion, and the hidden commands through which the completion scripts ask what to
// offer.
var commands = map[string]string{
	"join": "join", "detach": "detach", "kill": "kill", "list": "list", "restore": "restore",
	"setup": "setup", "update": "update", "completion": "completion",
	"help": "help", "-h": "help", "--help": "help",
	"version": "version", "-V": "version", "--version": "version",
	cobra.ShellCompRequestCmd:       cobra.ShellCompRequestCmd,
	cobra.ShellCompNoDescRequestCmd: cobra.ShellCompNoDescRequestCmd,
}

// run runs cld with args, checking the command before cobra sees it, and where setup runs, the
// one after setup (see docs/design/overview.md, "The command line on cobra"). Any other word is
// an unknown command, new and resume among them (decision 50.6).
func run(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fail.Usage("missing command: cld join attaches to a session, creating or " +
			"resuming it first (see cld help)")
	}
	typed := args[0]
	command, known := commands[typed]
	if !known {
		return fail.Usage(fmt.Sprintf("unknown command '%s' (see cld help)", typed))
	}
	if command == "setup" {
		if err := setupCommand(args[1:]); err != nil {
			return err
		}
	}
	completing := command == cobra.ShellCompRequestCmd || command == cobra.ShellCompNoDescRequestCmd
	// The scripts always pass the word being completed; without one, cobra would fail with a
	// message of its own and status 1.
	if completing && len(args) == 1 {
		return fail.Usage(fmt.Sprintf("%s: missing the word to complete (see cld help)", typed))
	}
	// cld prints what cobra writes, whose failed write cobra drops or reports in Go's words, so
	// that it ends cld with status 1 (decisions 12.5 and 17.6). Nothing is printed where there is
	// nothing: even an empty write fails on a stdout that cannot take one.
	var out bytes.Buffer
	root := commandLine(typed, args[1:], &out)
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

// noFiles turns cobra's answer to __complete, text, into one that offers no file names. cobra
// answers ":0", on which shells offer them, where it cannot read the line (see
// docs/design/findings/environment.md, "Completion"). No argument of cld's is a file.
func noFiles(text string) string {
	rest, found := strings.CutSuffix(text, ":0\n")
	if found && (rest == "" || strings.HasSuffix(rest, "\n")) {
		return fmt.Sprintf("%s:%d\n", rest, cobra.ShellCompDirectiveNoFileComp)
	}
	return text
}
