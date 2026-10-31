package tabs

import (
	"fmt"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Report is what a restore did or would do, for cmd to print: Lines on stdout, then Warnings and
// Notes on stderr. Failed is a step that failed, which makes the status 1 (decision 48.8).
type Report struct {
	Lines    []string
	Warnings []string
	Notes    []string
	Failed   bool
}

// clock writes the report's times, in local time: of today as 15:04, of another day with its
// date.
type clock struct {
	now time.Time
}

func (c clock) at(t time.Time) string {
	t = t.In(c.now.Location())
	layout := "15:04"
	year, month, day := t.Date()
	if y, m, d := c.now.Date(); y != year || m != month || d != day {
		layout = "2006-01-02 " + layout
	}
	return t.Format(layout)
}

// ghostty is the report's first line: Ghostty's version, pid and start, or that none runs. A
// start is the one NSRunningApplication gave, else the one of ps's row for the pid.
func (c clock) ghostty(snap *ghostty.Snapshot, app ghostty.App, procs Table) string {
	if !snap.Running {
		return "Ghostty is not running"
	}
	line := "Ghostty"
	if snap.Version != "" {
		line += " " + snap.Version
	}
	line += fmt.Sprintf(", pid %d", app.PID)
	if app.Launched > 0 {
		return line + ", started " + c.at(time.UnixMilli(int64(app.Launched*1000)))
	}
	if p, ok := procs.ByPID(app.PID); ok {
		return line + ", started " + c.at(p.Start)
	}
	return line
}
