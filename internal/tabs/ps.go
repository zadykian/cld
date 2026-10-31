package tabs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/tool"
)

// psTimeout bounds ps.
const psTimeout = 5 * time.Second

// lstart is C's %c, which ps writes a start in under LC_ALL=C. parsePS joins its fields by single
// spaces, and the day's 2 takes both 6 and 16.
const lstart = "Mon Jan 2 15:04:05 2006"

// Process is a row of ps.
type Process struct {
	PID, PPID, PGID int
	TTY             string    // as ps names it: ttys016, pts/3; ?? or ? for none
	Start           time.Time // in UTC
	Args            []string  // ps's args, split at spaces
}

// Table is ps's rows.
type Table []Process

// Processes lists every process, by one run of ps. It asks for each keyword with its own -o,
// since BSD ps lets the text after a = run on through the commas of the last one. LC_ALL=C keeps
// lstart in C's form and TZ=UTC in UTC, on macOS and procps alike.
func Processes(ctx context.Context) (Table, error) {
	ctx, cancel := context.WithTimeout(ctx, psTimeout)
	defer cancel()
	cmd, err := tool.CommandContext(ctx, "ps", "-A", "-ww", "-o", "pid=", "-o", "ppid=",
		"-o", "pgid=", "-o", "tty=", "-o", "lstart=", "-o", "args=")
	if err != nil {
		return nil, err
	}
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("ps did not answer in %d s", int(psTimeout.Seconds()))
		}
		return nil, errors.New("ps: " + tool.Said(stderr.String(), err.Error()))
	}
	return parsePS(stdout.String()), nil
}

// parsePS reads ps's output, a row a line. A line it cannot read is passed over. An argument's
// spaces are lost, as ps prints the words joined.
func parsePS(out string) Table {
	var table Table
	for line := range strings.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		var ids [3]int
		var err error
		for i := range ids {
			if ids[i], err = strconv.Atoi(fields[i]); err != nil {
				break
			}
		}
		start, timeErr := time.Parse(lstart, strings.Join(fields[4:9], " "))
		if err != nil || timeErr != nil {
			continue
		}
		table = append(table, Process{PID: ids[0], PPID: ids[1], PGID: ids[2], TTY: fields[3],
			Start: start.UTC(), Args: fields[9:]})
	}
	return table
}

// ByPID is the process pid.
func (t Table) ByPID(pid int) (Process, bool) {
	for _, p := range t {
		if p.PID == pid {
			return p, true
		}
	}
	return Process{}, false
}
