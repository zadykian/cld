package main

import (
	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/session"
)

// killLong is kill's help.
const killLong = `end session NAME-SUFFIX and its tmux server: claude exits as when its terminal
closes, and what claude started through tmux ends too, also where it keeps the
server running after claude has exited. claude runs its SessionEnd hooks with
the reason "other", and may still run them when cld kill returns. Without -n,
a session made in another repository or directory of the same name is refused.
After a kill, cld list shows the session as ended, and cld join brings its
conversation back; cld restore leaves it ended.`

// newKill is cld kill, which ends a session and its server (decision 32), and needs -s
// (decision 24.5).
func newKill(typed string) *cobra.Command {
	kill := &cobra.Command{
		Use:   "kill [-n NAME] -s SUFFIX",
		Short: "end session NAME-SUFFIX and its tmux server",
		Long:  killLong,
	}
	names := addNaming(kill, "the session's `SUFFIX`, after NAME-")
	kill.RunE = func(*cobra.Command, []string) error {
		if err := names.check(typed, "-s SUFFIX (see cld list)"); err != nil {
			return err
		}
		tmux, err := session.Check()
		if err != nil {
			return err
		}
		suffix, home, err := names.resolve(tmux)
		if err != nil {
			return err
		}
		return tmux.Kill(suffix, home)
	}
	return kill
}
