package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/term"
)

// modes are the terminal modes claude enters.
const modes = "\x1b[?1049h" + // alternate screen
	"\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h" + // SGR all-motion mouse
	"\x1b[?2004h" + // bracketed paste
	"\x1b[?1004h" + // focus reports
	"\x1b[>4;1m" // modifyOtherKeys mode 1

// removedDirectory is what claude 2.1.282 says to --version in a directory that has been removed.
const removedDirectory = "error: The current working directory was deleted, so that command " +
	"didn't work. Please cd into a different directory and try again."

// claude stands in for claude: it enters claude's modes, draws its screen, obeys PID.ctl and logs
// every input byte to PID.in until the terminal goes away.
func claude() error {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		return version()
	}
	if message := os.Getenv("CLD_PROBE_FAIL"); message != "" {
		fmt.Println(message)
		os.Exit(1)
	}
	base := filepath.Join(os.Getenv("CLD_PROBE_DIR"), strconv.Itoa(os.Getpid()))
	control, input, err := openFiles(base)
	if err != nil {
		return err
	}
	if _, err := term.MakeRaw(int(os.Stdin.Fd())); err != nil {
		return err
	}

	var mu sync.Mutex
	write := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = os.Stdout.WriteString(s) //nolint:errcheck // the read below sees the terminal go
	}
	write(modes + "\x1b]0;probe\x07" + screen())
	go obey(control, &controller{write: write, base: base})

	buffer := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buffer)
		if n > 0 {
			if _, err := input.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			return nil //nolint:nilerr // the terminal went away, which ends claude
		}
	}
}

// openFiles makes the FIFO PID.ctl and opens it and PID.in, base being their path less the
// extension. It writes PID.json last, so that once it exists the other files do too.
func openFiles(base string) (control, input *os.File, err error) {
	if err := syscall.Mkfifo(base+".ctl", 0o600); err != nil {
		return nil, nil, err
	}
	// Opened read-write so that the FIFO never reports end of file between two writers.
	control, err = os.OpenFile(base+".ctl", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	input, err = os.OpenFile(base+".in", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	if err := writeRecord(base + ".json"); err != nil {
		return nil, nil, err
	}
	return control, input, nil
}

// version answers claude --version; it fails in a directory that has been removed, as claude
// does (docs/design/findings/claude.md).
func version() error {
	if _, err := os.Getwd(); err != nil {
		fmt.Fprintln(os.Stderr, removedDirectory)
		os.Exit(1)
	}
	reported, set := os.LookupEnv("CLD_FAKE_CLAUDE_VERSION")
	if !set {
		reported = "99.0.0 (Claude Code)"
	}
	_, err := fmt.Println(reported)
	return err
}

// screen is what the probe draws: its arguments, the settings cut short, then a line in the
// attributes claude uses.
func screen() string {
	// The settings, some two thousand bytes of JSON with their hooks, go as {...}: the arguments
	// stay within a line of the smallest terminal a test gives claude, as they did before them.
	args := slices.Clone(os.Args[1:])
	if i := slices.Index(args, "--settings"); i >= 0 && i+1 < len(args) {
		args[i+1] = "{...}"
	}
	return "probe " + strings.Join(args, " ") + "\r\n" +
		"\x1b[1mbold\x1b[22m \x1b[2mdim\x1b[22m \x1b[3mitalic\x1b[23m \x1b[7minverse\x1b[27m " +
		"\x1b[38;5;208mcolour\x1b[39m\r\n"
}
