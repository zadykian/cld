package main

import (
	"context"
	"errors"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/picker"
	"github.com/zadykian/cld/internal/session"
)

// switchList is list --switch CLIENT, which the keys run for tmux client CLIENT (decision 51.2).
// With to, it moves the terminal to the previous, next or last session. Without, it shows the
// list in a popup, whose Enter moves it. It ends no idle session, and says what goes wrong on the
// terminal's message line too (decision 51.1).
func switchList(client, to string) error {
	sw, err := session.SwitchClient(client)
	if err != nil {
		return err
	}
	tmux, err := session.Check()
	if err != nil {
		// tmux's checks refuse the tmux found, which can still tell the terminal why.
		if found, ferr := session.Find(); ferr == nil {
			tell(found, sw, err)
		}
		return err
	}
	err = moveByKey(tmux, sw, to)
	tell(tmux, sw, err)
	return err
}

// moveByKey moves the terminal of sw to the session to names, or else to the one picked in the
// list, leaving the popup to close with it. Esc closes the popup, and moves nothing.
func moveByKey(tmux *session.Tmux, sw *session.Switch, to string) error {
	if to != "" {
		return tmux.Step(context.Background(), sw, to)
	}
	sessions, err := tmux.Sessions(context.Background())
	if err != nil {
		return err
	}
	if len(sessions) == 0 || !picker.Available() {
		return nil
	}
	picked, _, err := picker.Run(listSource{tmux}, sessions, true)
	if err != nil || picked.Name == "" {
		return err
	}
	return tmux.SwitchTo(sw, picked.Name)
}

// tell shows err on the message line of the terminal of sw (see session.Tmux.Tell), where err
// carries cld's own message. An exit status of tmux's follows tmux's message on stderr.
func tell(tmux *session.Tmux, sw *session.Switch, err error) {
	var failure *fail.Error
	switch {
	case errors.As(err, &failure):
		tmux.Tell(sw, failure.Message)
	case err != nil && !errors.As(err, new(fail.Status)):
		tmux.Tell(sw, err.Error())
	}
}
