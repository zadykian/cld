package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/remote"
	"github.com/zadykian/cld/internal/session"
)

// hostUsage is --host in the help of each command that takes it.
const hostUsage = "run on `HOST` instead, an ssh destination: ssh\n" +
	"HOST runs cld there (see cld help)"

// onHost is how a command takes --host HOST (decision 62). needs refuses what the host's cld would
// take from its home (decision 62.2). terminal, for join and list, says whether ssh gives the
// host's cld a terminal; nil for the others.
type onHost struct {
	needs    func(*pflag.FlagSet) error
	terminal func() bool
}

// add gives command, typed as typed, --host HOST. Given, once check has passed, it runs command on
// HOST through ssh, which needs no tmux or claude here, rather than its RunE.
func (h onHost) add(command *cobra.Command, typed string) {
	var host string
	command.Flags().StringVar(&host, cmdline.Host, "", hostUsage)
	here := command.RunE
	command.RunE = func(c *cobra.Command, args []string) error {
		if !cmdline.OnHost(c) {
			return here(c, args)
		}
		terminal := h.terminal != nil && h.terminal()
		if err := h.check(c, typed, host, terminal); err != nil {
			return err
		}
		return remote.Exec(host, hostWords(c, args), terminal)
	}
}

// check refuses, with status 2, a HOST --host does not take, the hidden options cld's own
// commands give, and what needs refuses. With status 1, it refuses a terminal to attach in a pane
// of cld's servers, which would show the host's session inside this one (decision 62.2).
func (h onHost) check(c *cobra.Command, typed, host string, terminal bool) error {
	switch {
	case host == "":
		return fail.Usage("option '--host' needs a value (see cld help)")
	case !remote.ValidHost(host):
		return fail.Usage(fmt.Sprintf("invalid host '%s' for --host (see cld help)", host))
	}
	hidden := false
	c.Flags().Visit(func(f *pflag.Flag) { hidden = hidden || f.Hidden })
	if hidden {
		return cmdline.Unexpected(typed, "--host")
	}
	if h.needs != nil {
		if err := h.needs(c.Flags()); err != nil {
			return err
		}
	}
	if terminal && session.Inside() {
		return fail.Runtime(typed + ": --host would show the host's session inside this one: " +
			"run it outside cld's sessions")
	}
	return nil
}

// hostNeeds refuses --host without one of options, which what names, from the command typed as
// typed.
func hostNeeds(typed, what string, options ...string) func(*pflag.FlagSet) error {
	return func(flags *pflag.FlagSet) error {
		if slices.ContainsFunc(options, flags.Changed) {
			return nil
		}
		return fail.Usage(typed + ": --host needs " + what + " (see cld help)")
	}
}

// hostWords is command c as the host's cld takes it: the commands below cld, then the options as
// cobra read them. Each goes as --NAME=VALUE, but --host and --help, and args after "--".
func hostWords(c *cobra.Command, args []string) []string {
	words := strings.Fields(c.CommandPath())[1:]
	c.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == cmdline.Host || f.Name == "help" {
			return
		}
		values := []string{f.Value.String()}
		if slice, ok := f.Value.(pflag.SliceValue); ok {
			values = slice.GetSlice()
		}
		for _, value := range values {
			words = append(words, "--"+f.Name+"="+value)
		}
	})
	if c.ArgsLenAtDash() >= 0 {
		words = append(append(words, "--"), args...)
	}
	return words
}

// terminals reports whether cld's stdin and stdout are terminals, which ssh hands the host's join
// and list as one. There join attaches, and list draws its picker; ssh writes the host's tmux to
// stdout (decision 62.3).
func terminals() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// fromHost runs the words another host's cld sent as __host VALUE, args (decision 62.4).
func fromHost(args []string) error {
	if len(args) != 1 {
		return fail.Usage(remote.Command + " takes one VALUE, which --host sends (see cld help)")
	}
	words, ok := remote.Decode(args[0])
	if !ok {
		return fail.Usage(fmt.Sprintf("invalid value '%s' for %s (see cld help)", args[0],
			remote.Command))
	}
	return run(words)
}
