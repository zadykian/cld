// Package fail carries the way cld ends up to main, which alone exits: an exit status, and the
// message to print first, if tmux has not printed its own. Print writes cld's own output, and a
// write that fails is one of these ends.
package fail

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
)

// Error ends cld with Status, after "cld: Message" on stderr, followed by Advice: what to do
// about it on cld's command line, such as " (see cld help)", kept apart for a caller that shows
// the message elsewhere - the session list's footer.
type Error struct {
	Status  int
	Message string
	Advice  string
}

func (e *Error) Error() string { return e.Message + e.Advice }

// Usage is a mistake on cld's command line: exit status 2.
func Usage(message string) error { return &Error{Status: 2, Message: message} }

// Runtime is anything else that stops cld: exit status 1.
func Runtime(message string) error { return &Error{Status: 1, Message: message} }

// Status ends cld with a tmux command's exit status, printing nothing: tmux has said why.
type Status int

func (s Status) Error() string { return "exit status " + strconv.Itoa(int(s)) }

// Print writes text to stdout. A write that fails is a Runtime error, as the script's printf and
// cat failing under set -e were, so that output cut short - on a full disk, say - does not pass
// for whole. A write to a pipe whose reader has gone never gets here: Go's runtime ends cld with
// SIGPIPE, as the signal ended the script - also when cld started with SIGPIPE ignored, where the
// script failed the write instead. Go does not tell that disposition apart from the default.
func Print(text string) error {
	if _, err := os.Stdout.WriteString(text); err != nil {
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}
		return Runtime("write error: " + err.Error())
	}
	return nil
}
