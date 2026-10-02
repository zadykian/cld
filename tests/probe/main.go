// Command probe stands in for claude in cld's tests, and for tmux, docker, systemctl and
// loginctl where a test checks cld's calls. The name it runs under picks which. The reasons
// behind the fakes are in docs/design/testing.md.
//
// As claude it enters the terminal modes claude enters, and records what cld and the terminal
// hand it in $CLD_PROBE_DIR, in files named after its PID:
//
//	PID.json  argv, working directory and environment, written once at start
//	PID.in    every input byte, appended as it arrives
//	PID.ctl   a FIFO of commands, a line each (see controls)
//
// "claude --version" answers first, writing no file, so that cld's version check counts as no
// claude. It prints "99.0.0 (Claude Code)" unless $CLD_FAKE_CLAUDE_VERSION says otherwise
// (decision 6). With $CLD_PROBE_FAIL set, it prints that and exits 1, as claude does when it
// cannot start.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var err error
	switch filepath.Base(os.Args[0]) {
	case "tmux":
		err = fakeTmux()
	case "docker":
		err = fakeDocker()
	case "systemctl", "loginctl":
		err = fakeSystemd()
	case "receiver":
		err = receiver()
	default:
		err = claude()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}
