// Command probe stands in for claude in cld's tests, and for the tools cld runs where a test checks
// its calls: tmux, systemctl, loginctl, osascript, ps and ssh. The name it runs under picks which.
// The fakes' reasons are in docs/design/testing.md and testing-2.md; the Mac's model, internal/mac.
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
	case "systemctl", "loginctl":
		err = fakeSystemd()
	case "osascript":
		err = fakeOsascript()
	case "ps":
		err = fakePs()
	case "ssh":
		err = fakeSSH()
	default:
		err = claude()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}
