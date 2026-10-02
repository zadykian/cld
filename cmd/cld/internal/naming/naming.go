// Package naming is the -n and -s of join, detach and kill, which name a session NAME-SUFFIX
// (decision 24): their checks, the name they resolve to once tmux has been checked, and their
// completion from the sessions list shows.
package naming

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/session"
)

// nameUsage is -n in the help of join, detach and kill: its first line fits within 80 columns
// beside the widest column of theirs, join's, as wide as its --resume SESSION.
const nameUsage = "the session's `NAME`, before -SUFFIX: by default the\n" +
	"git repository's name here, or else the directory's"

// Options is the -n and -s of a command, which name the session NAME-SUFFIX: -n's NAME, or else
// the repository's or directory's name, and -s's SUFFIX, or else, for join, the next index.
type Options struct {
	flags        *pflag.FlagSet
	name, suffix *string
}

// Add gives command -n and -s, whose usage is suffixUsage.
func Add(command *cobra.Command, suffixUsage string) Options {
	return Options{
		flags:  command.Flags(),
		name:   command.Flags().StringP("name", "n", "", nameUsage),
		suffix: command.Flags().StringP("suffix", "s", "", suffixUsage),
	}
}

// Check checks -n and -s, where given, once the options have been read, and the name both make
// (decision 24.2). Where missing is not empty, -s is needed, and missing says what is missing
// from the command typed as typed.
func (o Options) Check(typed, missing string) error {
	name, suffix := o.flags.Changed("name"), o.flags.Changed("suffix")
	if name {
		if err := CheckName(*o.name); err != nil {
			return err
		}
	}
	switch {
	case suffix:
		return o.checkSuffix(name)
	case missing != "":
		return fail.Usage(typed + ": missing " + missing)
	}
	return nil
}

// checkSuffix checks -s, its length, whatever its characters, before its characters, as SUFFIX
// alone names the session where NAME leaves nothing; then with -n, given where name, the length of
// the name both make.
func (o Options) checkSuffix(name bool) error {
	if utf8.RuneCountInString(*o.suffix) > session.MaxName {
		return fail.Usage(fmt.Sprintf("suffix '%s' is longer than %d characters (see cld help)",
			*o.suffix, session.MaxName))
	}
	if !session.ValidName(*o.suffix) {
		return fail.Usage(fmt.Sprintf("invalid suffix '%s' (see cld help)", *o.suffix))
	}
	if both := *o.name + "-" + *o.suffix; name && len(both) > session.MaxName {
		return tooLong(2, both)
	}
	return nil
}

// Given reports whether -n or -s was given.
func (o Options) Given() bool { return o.flags.Changed("name") || o.flags.Changed("suffix") }

// SuffixGiven reports whether -s was given: without it, join takes the next index.
func (o Options) SuffixGiven() bool { return o.flags.Changed("suffix") }

// GivenName is the session's name, NAME-SUFFIX, where the command line alone makes it: with -n
// and -s.
func (o Options) GivenName() (string, bool) {
	if !o.flags.Changed("name") || !o.flags.Changed("suffix") {
		return "", false
	}
	return *o.name + "-" + *o.suffix, true
}

// Resolve is the session's name once tmux has been checked, and the home join, detach and kill
// take the session from (see defaultName). A name too long is refused with status 1, as the
// repository's or directory's name, or the index, makes it so (decision 24.2).
func (o Options) Resolve(tmux *session.Tmux) (string, session.Home, error) {
	name, home := *o.name, session.Home{}
	if !o.flags.Changed("name") {
		name, home = defaultName()
	}
	if name != "" {
		name += "-"
	}
	if o.flags.Changed("suffix") {
		name += *o.suffix
	} else {
		var err error
		if name, err = tmux.Next(context.Background(), name); err != nil {
			return "", session.Home{}, err
		}
	}
	if len(name) > session.MaxName {
		return "", session.Home{}, tooLong(1, name)
	}
	return name, home, nil
}

// defaultName is the NAME of NAME-SUFFIX where -n gives none, and the home that sessions of it
// are taken from (decision 37): none where NAME is "", as in the root directory, where -s names
// any session whole.
func defaultName() (string, session.Home) {
	name, home := session.DefaultName()
	if name == "" {
		return "", session.Home{}
	}
	return name, home
}

// CheckName checks name, a NAME given with -n or a session's whole name: its length, whatever
// its characters, then its characters. The messages call it a name, as -s's call SUFFIX a
// suffix.
func CheckName(name string) error {
	if utf8.RuneCountInString(name) > session.MaxName {
		return fail.Usage(fmt.Sprintf("name '%s' is longer than %d characters (see cld help)",
			name, session.MaxName))
	}
	if !session.ValidName(name) {
		return &fail.Error{Status: 2, Message: fmt.Sprintf("invalid name '%s'", name),
			Advice: " (see cld help)"}
	}
	return nil
}

// tooLong refuses session name, longer than a session's name can be, with status.
func tooLong(status int, name string) error {
	return &fail.Error{Status: status,
		Message: fmt.Sprintf("session name '%s' is longer than %d characters", name,
			session.MaxName),
		Advice: "; give a shorter -n NAME or -s SUFFIX (see cld help)"}
}
