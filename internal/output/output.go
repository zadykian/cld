// Package output writes cld's own output, and its warnings and notes. A write of the output that
// fails is one of the ways cld ends (see internal/fail), so that output cut short does not pass
// for whole.
package output

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/zadykian/cld/internal/fail"
)

// Print writes text to stdout. Its error is only that of a write that failed, a fail.Runtime, so
// that output cut short does not pass for whole. A reader gone from a pipe ends cld by SIGPIPE
// before then, even where SIGPIPE was ignored at startup (decision 11.9).
func Print(text string) error {
	if _, err := os.Stdout.WriteString(text); err != nil {
		if pathError, ok := errors.AsType[*fs.PathError](err); ok {
			err = pathError.Err
		}
		return fail.Runtime("write error: " + err.Error())
	}
	return nil
}

// Warn writes warning to stderr, as "cld: warning: WARNING", for what goes wrong without ending
// cld. A write that fails is passed over, as main passes over one of cld's last message.
func Warn(warning string) {
	_, _ = fmt.Fprintf(os.Stderr, "cld: warning: %s\n", warning)
}

// Note writes note to stderr, as "cld: NOTE", for what cld did beside what was asked of it, away
// from its output - a session it ended for being idle, say. A write that fails is passed over, as
// for Warn.
func Note(note string) {
	_, _ = fmt.Fprintf(os.Stderr, "cld: %s\n", note)
}
