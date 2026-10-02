package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// fakeTmux fakes tmux for the checks cld makes before starting it: "tmux -V" prints
// $CLD_FAKE_TMUX_VERSION, list-sessions is listSessions, and any other call is recorded in
// tmux.json. With $CLD_FAKE_TMUX_REAL, the path of a real tmux, it runs that for all but
// list-sessions.
func fakeTmux() error {
	real := os.Getenv("CLD_FAKE_TMUX_REAL")
	if len(os.Args) == 2 && os.Args[1] == "-V" && real == "" {
		fmt.Println(os.Getenv("CLD_FAKE_TMUX_VERSION"))
		return nil
	}
	if slices.Contains(os.Args[1:], "list-sessions") {
		listSessions()
		return nil
	}
	if real != "" {
		return syscall.Exec(real, append([]string{"tmux"}, os.Args[1:]...), os.Environ())
	}
	return writeRecord(filepath.Join(os.Getenv("CLD_PROBE_DIR"), "tmux.json"))
}

// listSessions prints $CLD_FAKE_TMUX_SESSIONS for any server but those failServer fails on. Where
// that is unset it fails as tmux does with no server running.
func listSessions() {
	if i := slices.Index(os.Args, "-L"); i > 0 && i+1 < len(os.Args) {
		failServer(os.Args[i+1])
	}
	sessions := os.Getenv("CLD_FAKE_TMUX_SESSIONS")
	if sessions == "" {
		fmt.Fprintln(os.Stderr, "no server running on /fake/tmux")
		os.Exit(1)
	}
	fmt.Println(sessions)
}

// failServer fails as tmux does where the server exits while it asks, for a server named in
// $CLD_FAKE_TMUX_EXITED, or where it may not connect, for one in $CLD_FAKE_TMUX_DENIED. Both
// separate the names, of -L, by spaces.
func failServer(server string) {
	if slices.Contains(strings.Fields(os.Getenv("CLD_FAKE_TMUX_EXITED")), server) {
		fmt.Fprintln(os.Stderr, "server exited unexpectedly")
		os.Exit(1)
	}
	if slices.Contains(strings.Fields(os.Getenv("CLD_FAKE_TMUX_DENIED")), server) {
		fmt.Fprintf(os.Stderr, "error connecting to /fake/tmux/%s (Permission denied)\n", server)
		os.Exit(1)
	}
}
