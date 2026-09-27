// Package output writes cld's own output. A write that fails is one of the ways cld ends (see
// internal/fail), so that output cut short does not pass for whole.
package output

import (
	"errors"
	"io/fs"
	"os"

	"github.com/zadykian/cld/internal/fail"
)

// Print writes text to stdout. Its error is only that of a write that failed: a fail.Runtime, as
// the script's printf and cat failing under set -e were, so that output cut short - on a full
// disk, say - does not pass for whole. A write to a pipe whose reader has gone never gets here:
// Go's runtime ends cld with SIGPIPE, as the signal ended the script - also when cld started with
// SIGPIPE ignored, where the script failed the write instead. Go does not tell that disposition
// apart from the default.
func Print(text string) error {
	if _, err := os.Stdout.WriteString(text); err != nil {
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}
		return fail.Runtime("write error: " + err.Error())
	}
	return nil
}
