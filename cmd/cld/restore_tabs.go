package main

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/ghostty"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/tabs"
	"github.com/zadykian/cld/internal/tool"
)

// tabsCommand is cld restore tabs, named in its messages as typed, and its options (decision
// 59.11). --watching is the watcher that --watch starts, hidden.
type tabsCommand struct {
	typed                   string
	watch, watching, dryRun bool
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
	flags := command.Flags()
	flags.BoolVar(&t.dryRun, "dry-run", false, "print what the record holds, and do nothing")
	flags.BoolVar(&t.watch, "watch", false, "keep the record: start a process that\n"+
		"reads Ghostty's tabs every 5 s and ends as Ghostty quits")
	flags.BoolVar(&t.watching, "watching", false, "")
	cmdline.Hide(flags, "watching")
	return command
}

// run runs restore tabs. It reopens nothing yet, so it asks for --watch or --dry-run. A usage error
// ends it before any tool runs. Then osascript, which only a Mac has, must be on the PATH.
func (t *tabsCommand) run(*cobra.Command, []string) error {
	if !t.watch && !t.watching && !t.dryRun {
		return fail.Usage(t.typed + ": give --watch or --dry-run (see cld help)")
	}
	if _, err := tool.LookPath("osascript"); err != nil {
		return (&ghostty.Error{Kind: ghostty.NoOsascript}).Fail()
	}
	switch {
	case t.watching:
		return tabs.Watch()
	case t.watch:
		return tabs.Start()
	}
	report, err := tabs.Restore(context.Background(), tabs.Options{DryRun: t.dryRun})
	if err != nil {
		return err
	}
	return printReport(report)
}

// printReport prints report: its lines, then its warnings and notes. A failed step makes the
// status 1, after them (decision 48.8).
func printReport(report *tabs.Report) error {
	if len(report.Lines) > 0 {
		if err := output.Print(strings.Join(report.Lines, "\n") + "\n"); err != nil {
			return err
		}
	}
	for _, warning := range report.Warnings {
		output.Warn(warning)
	}
	for _, note := range report.Notes {
		output.Note(note)
	}
	if report.Failed {
		return fail.Status(1)
	}
	return nil
}
