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
	older, all, closed      bool
}

// restoreTabs is cld restore tabs, which keeps a record of Ghostty's tabs on a Mac (decision 59).
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
	flags.BoolVar(&t.all, "all", false, "reopen every window lost in this run of Ghostty and the\n"+
		"one before")
	flags.BoolVar(&t.closed, "closed", false, "also reopen the tabs and splits closed one at a time")
	flags.BoolVar(&t.dryRun, "dry-run", false, "print what would reopen and what would be typed, "+
		"and do\nnothing")
	flags.BoolVar(&t.older, "older", false, "reopen the windows lost before those reopened last, "+
		"one\nburst at a time")
	flags.BoolVar(&t.watch, "watch", false, "keep the record: start, unless one runs, a process "+
		"that\nreads Ghostty's tabs every 5 s and ends as Ghostty quits")
	flags.BoolVar(&t.watching, "watching", false, "")
	cmdline.Hide(flags, "watching")
	return command
}

// run runs restore tabs: the watcher, its start, or the restore. A usage error ends it before any
// tool runs. Then osascript, which only a Mac has, must be on the PATH.
func (t *tabsCommand) run(*cobra.Command, []string) error {
	if err := t.together(); err != nil {
		return err
	}
	if _, err := tool.LookPath("osascript"); err != nil {
		return (&ghostty.Error{Kind: ghostty.NoOsascript}).Fail()
	}
	switch {
	case t.watching:
		return tabs.Watch()
	case t.watch:
		_, err := tabs.Start()
		return err
	}
	report, err := tabs.Restore(context.Background(), tabs.Options{DryRun: t.dryRun,
		Older: t.older, All: t.all, Closed: t.closed})
	if err != nil {
		return err
	}
	if err := printReport(report); err != nil {
		return err
	}
	if own := report.Own; own != nil {
		// A failed step's status is lost to the exec, its warnings out first (decision 59.9).
		if err := output.Print(own.Joining() + "\n"); err != nil {
			return err
		}
		return own.Exec()
	}
	if report.Failed {
		return fail.Status(1)
	}
	return nil
}

// together refuses the options that exclude each other: --watching takes none, --watch only
// starts the watcher, and --all reopens what --older would.
func (t *tabsCommand) together() error {
	restoring := []struct {
		set  bool
		name string
	}{{t.dryRun, "--dry-run"}, {t.older, "--older"}, {t.all, "--all"}, {t.closed, "--closed"}}
	for _, option := range restoring {
		switch {
		case !option.set:
		case t.watching:
			return fail.Usage(t.typed + ": --watching takes no other option (see cld help)")
		case t.watch:
			return fail.Usage(t.typed + ": --watch and " + option.name + " exclude each other: " +
				"--watch only starts the record's watcher (see cld help)")
		}
	}
	switch {
	case t.watching && t.watch:
		return fail.Usage(t.typed + ": --watching takes no other option (see cld help)")
	case t.older && t.all:
		return fail.Usage(t.typed + ": --older and --all exclude each other: --all reopens every " +
			"burst (see cld help)")
	}
	return nil
}

// printReport prints report: its lines, then its warnings and notes. Its error is only that of a
// write that failed.
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
	return nil
}
