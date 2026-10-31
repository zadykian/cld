package main

import (
	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/ghostty"
	"github.com/zadykian/cld/internal/tool"
)

// tabsCommand is cld restore tabs, named in its messages as typed, and its options (decision
// 59.11).
type tabsCommand struct {
	typed  string
	dryRun bool
}

// restoreTabs is cld restore tabs, which keeps a record of Ghostty's tabs on a Mac (decision 59).
// cld hides it until it does all that decision 59 says.
func restoreTabs(typed string) *cobra.Command {
	t := &tabsCommand{typed: typed}
	command := &cobra.Command{
		Use:    "tabs",
		Short:  "keep a record of a Mac's Ghostty windows, tabs and splits",
		Hidden: true,
		Args:   cmdline.NoArguments(typed),
		RunE:   t.run,
	}
	command.SetFlagErrorFunc(cmdline.FlagError(typed))
	command.Flags().BoolVar(&t.dryRun, "dry-run", false, "print what the record holds, and do nothing")
	return command
}

// run runs restore tabs. It reopens nothing yet, so it asks for --dry-run, which prints nothing
// either as yet. A usage error ends it before any tool runs. Then osascript, which only a Mac has,
// must be on the PATH.
func (t *tabsCommand) run(*cobra.Command, []string) error {
	if !t.dryRun {
		return fail.Usage(t.typed + ": give --dry-run (see cld help)")
	}
	if _, err := tool.LookPath("osascript"); err != nil {
		return (&ghostty.Error{Kind: ghostty.NoOsascript}).Fail()
	}
	return nil
}
