package main

import (
	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/session"
)

// detachLong is detach's help.
const detachLong = `detach every terminal attached to session NAME-SUFFIX; claude keeps running.
Without -n and -s, as ! cld detach in claude, or in a shell on the session's
tmux server, detach the terminal used last on that session, as C-q d does, for
a terminal that keeps C-q from tmux: the one it was typed in, unless the mouse
moved over another since, or another took the focus. Elsewhere, -s is needed.
With -s and without -n, a session made in another repository or directory of
the same name is refused.`

// newDetach is cld detach, C-q d for a terminal that keeps C-q from tmux (decision 44): with -s
// it detaches every terminal on the session, and without -n and -s, in one of cld's servers, the
// terminal on that session used last (see session.Inside).
func newDetach(typed string) *cobra.Command {
	detach := &cobra.Command{
		Use:   "detach [-n NAME] [-s SUFFIX]",
		Short: "detach the terminals attached to session NAME-SUFFIX",
		Long:  detachLong,
	}
	names := addNaming(detach, "the session's `SUFFIX`, after NAME-")
	detach.RunE = func(*cobra.Command, []string) error {
		inside := session.Inside() && !names.given()
		missing := "-s SUFFIX (see cld list)"
		if inside {
			missing = ""
		}
		if err := names.check(typed, missing); err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		if inside {
			return tmux.DetachTerminal()
		}
		suffix, home, err := names.resolve(tmux)
		if err != nil {
			return err
		}
		return tmux.Detach(suffix, home)
	}
	completeWith(detach, "name", sessionNames(false))
	completeWith(detach, "suffix", sessionSuffixes(false))
	return detach
}
