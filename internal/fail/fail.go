// Package fail carries the way cld ends up to main, which alone exits: an exit status, and the
// message to print first, if tmux has not printed its own. Every value it makes is one of these
// ends; cld's own output is internal/output's, whose failed write is one of them too.
package fail

import "strconv"

// Error ends cld with Status, after "cld: Message" on stderr, followed by Advice. Advice says what
// to do about it on cld's command line, such as " (see cld help)". It stays apart for a caller
// that shows the message elsewhere: the session list's footer.
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

// Status ends cld with an exit status, printing nothing. Its reason has been shown already, as
// when a tmux command failed and tmux has said why, or needs none, as when a signal ended what cld
// was doing. That status is 128 plus the signal's number.
type Status int

func (s Status) Error() string { return "exit status " + strconv.Itoa(int(s)) }
