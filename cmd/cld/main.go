// Command cld runs Claude Code in named sessions on a private tmux server, so a conversation can
// be detached and rejoined from any terminal. The command line is here; how cld uses tmux, and
// why, is in internal/session, the interactive list, cld list on a terminal, in internal/picker,
// and cld setup telemetry, a local OpenTelemetry collector for claude, in internal/telemetry.
// cld completion SHELL prints cobra's completion script for the shell, with which cld join -n
// completes the names cld list shows (see commandLine).
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/zadykian/cld/internal/fail"
)

// version is what cld version prints; make dist and make install set it with -ldflags.
var version = "dev"

// main is the only place cld exits: new, resume and join end in tmux, and everything else comes
// back here with its exit status, printing cld's message first, if tmux has not printed its own.
func main() {
	err := run(os.Args[1:])
	var failure *fail.Error
	var status fail.Status
	switch {
	case err == nil:
	case errors.As(err, &failure):
		fmt.Fprintf(os.Stderr, "cld: %s%s\n", failure.Message, failure.Advice)
		os.Exit(failure.Status)
	case errors.As(err, &status):
		os.Exit(int(status))
	default:
		fmt.Fprintf(os.Stderr, "cld: %v\n", err)
		os.Exit(1)
	}
}
