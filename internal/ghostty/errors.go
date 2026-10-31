package ghostty

import (
	"fmt"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// Kind is what went wrong in a script's run, as cld tells it apart.
type Kind int

// The kinds of Error. NoOsascript is permanent: another try finds it again until the user acts.
const (
	NoOsascript Kind = iota + 1 // osascript is not on the PATH
	Gone                        // Ghostty quit or went away during the run
	Several                     // more than one Ghostty process
	Timeout                     // no answer before the deadline, or JXA's own timeout
	Failed                      // anything else: osascript's own failure, or another of Ghostty's
)

// JXA's error numbers that cld tells apart.
const (
	procNotFound      = -600  // the application is not running
	connectionInvalid = -609  // the connection to the application went away
	timedOut          = -1712 // errAETimeout: no answer to an Apple Event
)

// ScriptError is the error document a script prints in place of its result. Number is JXA's
// error number, 0 where it has none, as for a TypeError. Message is JXA's, which is Ghostty's
// scriptErrorString where Ghostty set one. Version is Ghostty's, where read before the failure.
type ScriptError struct {
	Step    string `json:"step"` // running|count|probe|tty|read|changed
	Number  int    `json:"number"`
	Message string `json:"message"`
	Version string `json:"version"`
}

// Error is a failure to read or drive Ghostty. Step, Number, Message and Version are the script's
// ScriptError's, or else Step is "" and Message is osascript's stderr: cld saw the failure itself.
type Error struct {
	Kind    Kind
	Step    string
	Number  int
	Message string
	Version string
	apps    int           // Several: the Ghostty processes
	limit   time.Duration // Timeout: the deadline cld gave osascript, 0 for JXA's own
}

// classify tells what a script's error document means, by its step and JXA's error number.
func classify(se *ScriptError) *Error {
	e := &Error{Kind: Failed, Step: se.Step, Number: se.Number, Message: se.Message,
		Version: se.Version}
	switch {
	case se.Number == procNotFound || se.Number == connectionInvalid || se.Step == "running":
		e.Kind = Gone
	case se.Number == timedOut:
		e.Kind = Timeout
	}
	return e
}

// Error is cld's message, which main prints after "cld: ".
func (e *Error) Error() string {
	switch e.Kind {
	case NoOsascript:
		return "restore tabs runs on a Mac: osascript is not installed"
	case Gone:
		return "Ghostty quit while cld read it"
	case Several:
		return fmt.Sprintf("Ghostty runs %d times, as open -n starts it: quit all but one", e.apps)
	case Timeout:
		if e.limit == 0 {
			return "Ghostty did not answer an Apple Event in time"
		}
		return fmt.Sprintf("Ghostty did not answer in %d s", int(e.limit.Seconds()))
	}
	if e.Step == "" {
		return "osascript: " + e.Message
	}
	if e.Number == 0 {
		return fmt.Sprintf("Ghostty: %s at %s", e.Message, e.Step)
	}
	return fmt.Sprintf("Ghostty: %s (%d) at %s", e.Message, e.Number, e.Step)
}

// Fail is e as cld ends with it: status 1, with what to do about it where cld can say.
func (e *Error) Fail() error {
	advice := ""
	if e.Kind == Timeout {
		advice = "; if macOS asked whether the app cld runs in may control Ghostty, answer, then" +
			" run cld restore tabs again"
	}
	return &fail.Error{Status: 1, Message: e.Error(), Advice: advice}
}
