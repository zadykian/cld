package main

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/session"
)

// sessionNames completes the NAME of -n, join's with all and detach's without, from the sessions
// list shows: with all, those that have ended too (decisions 24.8 and 50.7). Each NAME, split at
// the last "-" as cld's messages split a name, comes once, in list's order, described by the
// number of its sessions.
func sessionNames(all bool) cobra.CompletionFunc {
	return func(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion,
		cobra.ShellCompDirective) {
		var names []string
		counts := map[string]int{}
		for _, s := range listed(all) {
			name, ok := splitName(s.Name)
			if !ok || !strings.HasPrefix(name, typed) {
				continue
			}
			if counts[name]++; counts[name] == 1 {
				names = append(names, name)
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
}

// splitName is the NAME of a session's whole name, where its last "-" splits it into a NAME
// and a SUFFIX (see session.Options).
func splitName(whole string) (string, bool) {
	i := strings.LastIndexByte(whole, '-')
	if i <= 0 || !session.ValidName(whole[:i]) || !session.ValidName(whole[i+1:]) {
		return "", false
	}
	return whole[:i], true
}

// sessionSuffixes completes the SUFFIX of -s, join's with all and detach's without, from the
// sessions list shows under the NAME the command takes, each described by its state as list shows
// it, or "ended in" its directory. Without -n, a running session made elsewhere is not offered
// (decision 37.5).
func sessionSuffixes(all bool) cobra.CompletionFunc {
	return func(c *cobra.Command, _ []string, typed string) ([]cobra.Completion,
		cobra.ShellCompDirective) {
		var name string
		var home session.Home
		if c.Flags().Changed("name") {
			name, _ = c.Flags().GetString("name") //nolint:errcheck // -n is a string option
		} else {
			name, home = defaultName()
		}
		if name != "" {
			name += "-"
		}
		var suffixes []cobra.Completion
		for _, s := range listed(all) {
			suffix, found := strings.CutPrefix(s.Name, name)
			if !home.Takes(s.Home) || !found || !strings.HasPrefix(suffix, typed) ||
				!session.ValidName(suffix) {
				continue
			}
			description := s.ShownState()
			if s.State == session.Ended {
				description = session.Ended + " in " + s.Directory
			}
			suffixes = append(suffixes, cobra.CompletionWithDesc(suffix, description))
		}
		return suffixes, cobra.ShellCompDirectiveNoFileComp
	}
}

// listed is the sessions list shows, for completion: with all, every one, and else those that
// run. It never fails: where tmux cannot read them, cobra.CompErrorln says why on stderr, which
// the completion scripts discard, and nothing is offered (decision 17.4).
func listed(all bool) []session.Session {
	var sessions []session.Session
	tmux, err := session.Find()
	if err == nil {
		sessions, err = tmux.Sessions(context.Background())
	}
	if err != nil {
		cobra.CompErrorln(err.Error())
	}
	if all {
		return sessions
	}
	ended := func(s session.Session) bool { return s.State == session.Ended }
	return slices.DeleteFunc(sessions, ended)
}
