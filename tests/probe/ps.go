package main

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zadykian/cld/tests/internal/mac"
)

// fakePs fakes ps listing the model's processes (internal/mac) as cld asks: lstart in UTC as C's
// locale writes it, and ?? for no tty, as macOS's ps does. It logs each call, with TZ and LC_ALL,
// to ps.jsonl. With no model it lists nothing.
func fakePs() error {
	dir := os.Getenv("CLD_PROBE_DIR")
	call := mac.PsCall{Argv: os.Args[1:], Env: map[string]string{}}
	for _, name := range []string{"TZ", "LC_ALL"} {
		if value, set := os.LookupEnv(name); set {
			call.Env[name] = value
		}
	}
	if err := appendJSON(filepath.Join(dir, mac.PsLog), call); err != nil {
		return err
	}
	if !slices.Equal(os.Args[1:], mac.PsArgs) {
		fmt.Fprintln(os.Stderr, "fake ps: unexpected arguments")
		os.Exit(1)
	}
	m, err := mac.Load(dir)
	if err != nil {
		return err
	}
	var rows strings.Builder
	for _, p := range m.Processes {
		fmt.Fprintf(&rows, "%5d %5d %5d %-8s %s %s\n", p.PID, p.PPID, p.PGID, cmp.Or(p.TTY, "??"),
			p.Start.UTC().Format("Mon Jan _2 15:04:05 2006"), strings.Join(p.Args, " "))
	}
	_, err = os.Stdout.WriteString(rows.String())
	return err
}
