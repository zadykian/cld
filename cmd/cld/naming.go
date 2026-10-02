package main

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

// naming is the -n and -s of join, detach and kill, which name the session NAME-SUFFIX
// (decision 24): -n's NAME, or else the repository's or directory's name, and -s's SUFFIX, or
// else, for join, the next index.
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

// check checks -n and -s, where given, once the options have been read, and the name both make
// (decision 24.2). Where missing is not empty, -s is needed, and missing says what is missing
// from the command typed as typed.
func (n naming) check(typed, missing string) error {
	name, suffix := n.flags.Changed("name"), n.flags.Changed("suffix")
	if name {
		if err := checkName(*n.name); err != nil {
			return err
		}
	}
	switch {
	case suffix:
		return n.checkSuffix(name)
	case missing != "":
		return fail.Usage(typed + ": missing " + missing)
	}
	return nil
}

// checkSuffix checks -s, its length, whatever its characters, before its characters, as SUFFIX
// alone names the session where NAME leaves nothing; then with -n, given where name, the length of
// the name both make.
func (n naming) checkSuffix(name bool) error {
	if utf8.RuneCountInString(*n.suffix) > session.MaxName {
		return fail.Usage(fmt.Sprintf("suffix '%s' is longer than %d characters (see cld help)",
			*n.suffix, session.MaxName))
	}
	if !session.ValidName(*n.suffix) {
		return fail.Usage(fmt.Sprintf("invalid suffix '%s' (see cld help)", *n.suffix))
	}
	if both := *n.name + "-" + *n.suffix; name && len(both) > session.MaxName {
		return tooLong(2, both)
	}
	return nil
}

// given reports whether -n or -s was given.
func (n naming) given() bool { return n.flags.Changed("name") || n.flags.Changed("suffix") }

// givenName is the session's name, NAME-SUFFIX, where the command line alone makes it: with -n
// and -s.
func (n naming) givenName() (string, bool) {
	if !n.flags.Changed("name") || !n.flags.Changed("suffix") {
		return "", false
	}
	return *n.name + "-" + *n.suffix, true
}

// resolve is the session's name once tmux has been checked, and the home join, detach and kill
// take the session from (see defaultName). A name too long is refused with status 1, as the
// repository's or directory's name, or the index, makes it so (decision 24.2).
func (n naming) resolve(tmux *session.Tmux) (string, session.Home, error) {
	name, home := *n.name, session.Home{}
	if !n.flags.Changed("name") {
		name, home = defaultName()
	}
	if name != "" {
		name += "-"
	}
	if n.flags.Changed("suffix") {
		name += *n.suffix
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

// checkName checks name, a NAME given with -n or a session's whole name: its length, whatever
// its characters, then its characters. The messages call it a name, as -s's call SUFFIX a
// suffix.
func checkName(name string) error {
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
