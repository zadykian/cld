package tabs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// Report is what a restore did or would do, for cmd to print: Lines on stdout, then Warnings and
// Notes on stderr. Failed is a step that failed, which makes the status 1 (decision 48.8). Own is
// cld's own pane, which cmd joins again by exec once the report is out, nil for none.
type Report struct {
	Lines    []string
	Warnings []string
	Notes    []string
	Failed   bool
	Own      *Own
}

// warn adds warning, where there is one: a failed step.
func (r *Report) warn(warning string) {
	if warning != "" {
		r.Warnings = append(r.Warnings, warning)
		r.Failed = true
	}
}

// note adds note: what cld did, or left, beside what was asked of it.
func (r *Report) note(note string) { r.Notes = append(r.Notes, note) }

// section adds a heading and its lines, where it has any.
func (r *Report) section(heading string, lines []string) {
	if len(lines) > 0 {
		r.Lines = append(append(r.Lines, heading), lines...)
	}
}

// paneCwd is the directory of record r's pane id, "" where unknown.
func paneCwd(r *Record, id string) string {
	if p := r.Panes[id]; p != nil {
		return p.Cwd
	}
	return ""
}

// clock writes the report's times, in local time: of today as 15:04, of another day with its
// date. A directory under home starts with ~.
type clock struct {
	now  time.Time
	home string
}

// newClock is the clock of a report written now.
func newClock() clock {
	c := clock{now: time.Now()}
	c.home, _ = os.UserHomeDir() //nolint:errcheck // without one, directories stay whole
	return c
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

// when is when loss l was lost: closed, or as Ghostty quit.
func (c clock) when(l *Loss) string {
	if l.Cause == CauseQuit {
		return "lost as Ghostty quit at " + c.at(l.At)
	}
	return "closed at " + c.at(l.At)
}

// noun is what loss l is, in the singular: a window, a tab or a split.
func noun(l *Loss) string {
	switch l.Cause {
	case CauseTab:
		return "a tab"
	case CausePane:
		return "a split"
	}
	return "a window"
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
		what := noun(l)
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

// windowSize is a window of tabs, with their panes.
func windowSize(tabs []ghostty.TabBuild) string {
	panes := 0
	for _, tab := range tabs {
		panes += len(buildLeaves(tab.Tree))
	}
	what := "a window of " + count(len(tabs), "tab")
	if panes > len(tabs) {
		what += " and " + count(panes, "pane")
	}
	return what
}

// buildLeaves are the leaves of apply.js's tree n, left or top first.
func buildLeaves(n *ghostty.Node) []*ghostty.Leaf {
	switch {
	case n == nil:
		return nil
	case n.Leaf != nil:
		return []*ghostty.Leaf{n.Leaf}
	}
	return append(buildLeaves(n.First), buildLeaves(n.Second)...)
}

// count is n of thing, in the plural but for one.
func count(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}
