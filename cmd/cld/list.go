package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/cmd/cld/internal/idle"
	"github.com/zadykian/cld/cmd/cld/internal/naming"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/picker"
	"github.com/zadykian/cld/internal/session"
)

// listLong is list's help.
const listLong = `list the sessions cld started: name, whether a terminal is attached (or claude
exited, or the session ended) and whether claude is busy, waiting for an answer
or idle, when it was last active (a terminal attaching, or a key typed in one)
and the directory claude is in or ran in. cld keeps a session that has ended -
by cld kill, claude's /exit, a reboot - for 30 days. A session idle for longer
than $CLD_IDLE_DAYS days, 30 where unset or empty, is ended first, as cld kill
ends it, with a line on stderr; cld join brings its conversation back.
CLD_IDLE_DAYS=0 ends none.

On a terminal, pick one to join or kill: Up and Down select a session, Enter
joins it as cld join does, C-x twice within two seconds kills it as cld kill
does - Esc after the first C-x keeps it - and Esc or C-c leaves, printing the
list. On a session that has ended, Enter resumes it as cld join does, and C-x
twice forgets it. cld list | cat prints the list only.

In a session, C-q s shows the list over it: Enter moves the terminal to the
session picked, as ! cld join does, and Esc closes the list. So does Enter in
cld list run in a shell on the session's tmux server.`

// newList is cld list: the interactive list on a terminal it can draw on (decision 14), and else
// the table. It ends the idle sessions first (decision 46.2). --switch CLIENT and --to are the
// keys' own options, hidden (decision 51.2).
func newList(typed string) *cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "list cld's sessions; on a terminal, join or kill one",
		Long:  listLong,
	}
	var client, to string
	list.Flags().StringVar(&client, "switch", "", "")
	list.Flags().StringVar(&to, "to", "", "")
	cmdline.Hide(list.Flags(), "switch", "to")
	list.RunE = func(c *cobra.Command, _ []string) error {
		if err := switchOptions(c, typed, client, to); err != nil {
			return err
		}
		if c.Flags().Changed("switch") {
			return switchList(client, to)
		}
		return listSessions()
	}
	return list
}

// switchOptions refuses a --switch without its CLIENT, and a --to without --switch or with
// another value than previous, next or last.
func switchOptions(c *cobra.Command, typed, client, to string) error {
	switching, toGiven := c.Flags().Changed("switch"), c.Flags().Changed("to")
	switch {
	case switching && client == "":
		return fail.Usage("option '--switch' needs a value (see cld help)")
	case toGiven && !switching:
		return fail.Usage(typed + ": --to needs --switch CLIENT (see cld help)")
	case toGiven && !slices.Contains([]string{"previous", "next", "last"}, to):
		return fail.Usage(fmt.Sprintf("invalid value '%s' for --to: previous, next or last "+
			"(see cld help)", to))
	}
	return nil
}

// listSessions ends the idle sessions, then shows the sessions, those ended among them, in the
// interactive list where it can, and else, or once the list is left, in the table.
func listSessions() error {
	limit, err := idle.Limit()
	if err != nil {
		return err
	}
	tmux, err := session.Check()
	if err != nil {
		return err
	}
	sessions, err := tmux.Sessions(context.Background())
	if err != nil {
		return err
	}
	sessions = idle.Sweep(tmux, sessions, limit, "")
	if len(sessions) > 0 && picker.Available() {
		var done bool
		if sessions, done, err = pick(tmux, sessions); done {
			return err
		}
	}
	return output.Print(table(sessions))
}

// pick runs the interactive list over sessions, and reports whether that ends list. It joins the
// session picked as join does, or in a pane of cld's servers moves the pane's terminal there
// (decision 51.6). Left with none picked, it returns the sessions it read last, for the table.
func pick(tmux *session.Tmux, sessions []session.Session) ([]session.Session, bool, error) {
	sw := tmux.Switching()
	picked, last, err := picker.Run(listSource{tmux}, sessions, sw != nil)
	switch {
	case err != nil:
		return nil, true, err
	case picked.Name != "" && sw != nil:
		return nil, true, tmux.SwitchTo(sw, picked.Name)
	case picked.Name != "":
		return nil, true, tmux.JoinPicked(picked.Name)
	}
	return last, false, nil
}

// table lays out sessions for list under a header. NAME and STATE, with claude's status, are as
// wide as their longest, at least four and eight, before LAST ACTIVE and DIRECTORY. No sessions
// make no table.
func table(sessions []session.Session) string {
	if len(sessions) == 0 {
		return ""
	}
	width, stateWidth := 4, 8
	for _, s := range sessions {
		width = max(width, utf8.RuneCountInString(s.Name))
		stateWidth = max(stateWidth, utf8.RuneCountInString(s.ShownState()))
	}
	const row = "%-*s  %-*s  %-11s  %s\n"
	var out strings.Builder
	fmt.Fprintf(&out, row, width, "NAME", stateWidth, "STATE", "LAST ACTIVE", "DIRECTORY")
	for _, s := range sessions {
		fmt.Fprintf(&out, row, width, s.Name, stateWidth, s.ShownState(), s.LastActive(),
			s.Directory)
	}
	return out.String()
}

// listSource is what the interactive list reads and acts through: the sessions, join's checks for
// Enter, and kill's steps or the forget for the second Ctrl+X (decisions 14.5, 15.4 and 40.3).
// The list names a session whole, as -n does, and takes it from anywhere.
type listSource struct{ tmux *session.Tmux }

func (l listSource) Sessions(ctx context.Context) ([]session.Session, error) {
	return l.tmux.Sessions(ctx)
}

func (l listSource) Joinable(ctx context.Context, name string) error {
	if err := naming.CheckName(name); err != nil {
		return err
	}
	return l.tmux.Joinable(ctx, name)
}

// Forget forgets the session under the record's lock, which a restore or a join bringing it back
// holds (decisions 48.6 and 50.4).
func (l listSource) Forget(ctx context.Context, name string) error {
	if err := naming.CheckName(name); err != nil {
		return err
	}
	unlock := session.Lock()
	defer unlock()
	return l.tmux.Forget(ctx, name)
}

// Kill is kill's steps, checking that the session still holds one of pids (decision 15.3). What
// tmux says where the kill fails becomes the error, for the list's footer, rather than going to
// the terminal the list draws on (decision 15.4).
func (l listSource) Kill(ctx context.Context, name string, pids []string) error {
	if err := naming.CheckName(name); err != nil {
		return err
	}
	var said bytes.Buffer
	err := l.tmux.End(ctx, name, session.Home{}, pids, &said, &said)
	if status, ok := errors.AsType[fail.Status](err); ok {
		if message := strings.TrimSpace(said.String()); message != "" {
			return fail.Runtime(message)
		}
		return fail.Runtime("tmux kill-session: " + status.Error())
	}
	return err
}
