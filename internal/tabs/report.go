package tabs

import (
	"fmt"
	"strings"
	"time"
)

// Report is what a restore did or would do, for cmd to print: Lines on stdout, then Warnings and
// Notes on stderr. Failed is a step that failed, which makes the status 1 (decision 48.8).
type Report struct {
	Lines    []string
	Warnings []string
	Notes    []string
	Failed   bool
}

// warn adds warning, where there is one: a failed step.
func (r *Report) warn(warning string) {
	if warning != "" {
		r.Warnings = append(r.Warnings, warning)
		r.Failed = true
	}
}

// section adds a heading and its lines, where it has any.
func (r *Report) section(heading string, lines []string) {
	if len(lines) > 0 {
		r.Lines = append(append(r.Lines, heading), lines...)
	}
}

// clock writes the report's times, in local time: of today as 15:04, of another day with its
// date. A directory under home starts with ~.
type clock struct {
	now  time.Time
	home string
}

func (c clock) at(t time.Time) string { return c.format(t, "15:04") }

// atSecond is at, to the second.
func (c clock) atSecond(t time.Time) string { return c.format(t, "15:04:05") }

func (c clock) format(t time.Time, layout string) string {
	t = t.In(c.now.Location())
	year, month, day := t.Date()
	if y, m, d := c.now.Date(); y != year || m != month || d != day {
		layout = "2006-01-02 " + layout
	}
	return t.Format(layout)
}

func (c clock) dir(path string) string {
	switch {
	case path == "":
		return "an unknown directory"
	case c.home == "" || c.home == "/":
		return path
	case path == c.home:
		return "~"
	case strings.HasPrefix(path, c.home+"/"):
		return "~" + path[len(c.home):]
	}
	return path
}

// joinAgain is a line per pane a restore would join again: a shell at its prompt, with a binding.
// A quit cut that binding, or a restore typed it, and it did not hold.
func (c clock) joinAgain(r *Record) []string {
	var lines []string
	for _, w := range r.Windows {
		for _, id := range windowLeaves(w) {
			p := r.Panes[id]
			if p == nil || p.Kind != KindLocal || p.Last == nil {
				continue
			}
			b := Binding{Host: p.Last.Host, Session: p.Last.Session}
			lines = append(lines, fmt.Sprintf("  in %s: %s, %s", c.dir(p.Cwd), b, c.cut(p.Last)))
		}
	}
	return lines
}

// size is what loss l holds: a tab, a split, or windows with their tabs and panes.
func size(l *Loss) string {
	tabs, panes := 0, 0
	for _, w := range l.Windows {
		tabs += len(w.Tabs)
		panes += len(windowLeaves(w))
	}
	switch l.Cause {
	case CauseTab, CausePane:
		what := map[string]string{CauseTab: "a tab", CausePane: "a split"}[l.Cause]
		if panes > 1 {
			what += " of " + count(panes, "pane")
		}
		return what
	}
	what := "a window"
	if len(l.Windows) > 1 {
		what = count(len(l.Windows), "window")
	}
	what += " of " + count(tabs, "tab")
	if panes > tabs {
		what += " and " + count(panes, "pane")
	}
	return what
}

// count is n of thing, in the plural but for one.
func count(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}
