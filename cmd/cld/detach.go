package main

import (
	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/cmd/cld/internal/naming"
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

// newDetach is cld detach, C-q d for a terminal that keeps C-q from tmux (decision 44). With -s
// it detaches every terminal on the session. Without -n and -s, in one of cld's servers, it
// detaches the terminal on that session used last (see session.Inside).
func newDetach(typed string) *cobra.Command {
	detach := &cobra.Command{
		Use:   "detach [-n NAME] [-s SUFFIX]",
		Short: "detach the terminals attached to session NAME-SUFFIX",
		Long:  detachLong,
	}
	names := naming.Add(detach, "the session's `SUFFIX`, after NAME-")
	detach.RunE = func(*cobra.Command, []string) error {
		inside := session.Inside() && !names.Given()
		missing := "-s SUFFIX (see cld list)"
		if inside {
			missing = ""
		}
		if err := names.Check(typed, missing); err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		if inside {
			return tmux.DetachTerminal()
		}
		suffix, home, err := names.Resolve(tmux)
		if err != nil {
			return err
		}
		return tmux.Detach(suffix, home)
	}
	cmdline.CompleteWith(detach, "name", naming.Names(false))
	cmdline.CompleteWith(detach, "suffix", naming.Suffixes(false))
	return detach
}
