package ghostty

import "github.com/zadykian/cld/internal/fail"

// Kind is what went wrong in a script's run, as cld tells it apart.
type Kind int

// The kinds of Error.
const (
	NoOsascript Kind = iota + 1 // osascript is not on the PATH
)

// Error is a failure to read or drive Ghostty.
type Error struct {
	Kind    Kind
	Message string
}

// Error is cld's message, which main prints after "cld: ".
func (e *Error) Error() string {
	if e.Kind == NoOsascript {
		return "restore tabs runs on a Mac: osascript is not installed"
	}
	return e.Message
}

// Fail is e as cld ends with it: status 1.
func (e *Error) Fail() error {
	return &fail.Error{Status: 1, Message: e.Error()}
}
