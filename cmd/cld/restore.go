package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/restore"
	"github.com/zadykian/cld/internal/session"
)

// restoreLong is restore's help.
const restoreLong = `bring back the sessions that ran when the machine stopped: a reboot or a crash
ends them, and cld list shows them as ended, beside those that cld kill, C-x in
cld list, the idle sweep or claude's /exit ended, which stay ended. Each comes
back as cld join -n NAME -s SUFFIX brings it back, but without a terminal:
claude resumes its conversation in the directory it ran in, with the
environment the session started with and without the words given after --,
and is told to continue the turn it was in, if any. A session not started or
given a prompt for longer than $CLD_IDLE_DAYS days stays ended, with a note. A
session that cannot come back is a warning, and the status 1. cld setup
restore has your systemd run cld restore as it starts; cld join attaches to a
session.`

// newRestore is cld restore, which brings back the sessions a reboot ended (decision 48.5).
func newRestore() *cobra.Command {
	return &cobra.Command{
		Use:   "restore",
		Short: "bring back the sessions that ran when the machine stopped",
		Long:  restoreLong,
		RunE:  func(*cobra.Command, []string) error { return restoreAll() },
	}
}

// restoreAll brings back each session with a run mark, checking tmux as every command does and
// CLD_IDLE_DAYS as list does. A session it cannot bring back is a warning, and the status 1
// (decision 48.8).
func restoreAll() error {
	tmux, err := session.Check()
	if err != nil {
		return err
	}
	limit, err := idleLimit()
	if err != nil {
		return err
	}
	failed := false
	for _, name := range session.Marked() {
		restored, err := restoreOne(tmux, name, limit)
		if err != nil {
			return err
		}
		failed = failed || !restored
	}
	if failed {
		return fail.Status(1)
	}
	return nil
}

// restoreOne brings back session name under the record's lock (decision 48.6), and says how it
// went. It reports false where the session cannot come back, which is a warning, and fails where
// cld's output does.
func restoreOne(tmux *session.Tmux, name string, limit time.Duration) (bool, error) {
	unlock := session.Lock()
	restored, err := tmux.Restore(name, limit)
	unlock()
	switch {
	case err != nil:
		output.Warn(fmt.Sprintf("cannot restore session '%s': %v", name, err))
		return false, nil
	case restored != nil && restored.Idle > 0:
		output.Note(fmt.Sprintf("left session '%s' ended, idle for %s", name,
			idleFor(restored.Idle)))
	case restored != nil:
		line := fmt.Sprintf("Restored session '%s' in %s", name, restored.Directory)
		if restored.Busy {
			line += ", continuing its turn"
		}
		return true, output.Print(line + "\n")
	}
	return true, nil
}

// setupRestore is cld setup restore, named in its messages as typed (decision 48.9). Off Linux it
// refuses to run before it looks at anything but -h and --help (see linuxOnly); its checks of
// systemd are in its RunE, which completion never runs.
func setupRestore(typed string) *cobra.Command {
	only := linuxOnly{typed: typed, supported: restore.Supported}
	command := &cobra.Command{
		Use:   "restore",
		Short: "have your systemd run cld restore at login, or at boot",
		Long: `have your user's systemd run cld restore as it starts, which brings back the
sessions that ran when the machine stopped: cld writes the unit
~/.config/systemd/user/cld-restore.service, which runs this cld with the PATH,
TMUX_TMPDIR, XDG_STATE_HOME and CLD_IDLE_DAYS it has now, and enables it. Your
systemd starts at your first login, and at your last logout ends the sessions
cld restore brought back, unless lingering is on for you (loginctl
enable-linger): then it starts at boot, and keeps them. Run it again after
moving cld, or to change those variables. Needs systemd; Linux only.`,
		Args: only.arguments,
		RunE: func(*cobra.Command, []string) error {
			// The unit hands CLD_IDLE_DAYS on to cld restore, which would refuse a value that
			// is no number of days at every start.
			if _, err := idleLimit(); err != nil {
				return err
			}
			// The file cld runs from, with its symbolic links resolved, as cld update replaces it.
			cld, err := os.Executable()
			if err != nil {
				return fail.Runtime("cannot find the file cld runs from: " + err.Error())
			}
			report, err := restore.Setup(cld)
			if err != nil {
				return err
			}
			return output.Print(report)
		},
	}
	command.SetFlagErrorFunc(only.flagError)
	return command
}
