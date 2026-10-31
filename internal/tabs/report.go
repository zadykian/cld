package tabs

import (
	"context"
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

// losses is a line per loss not restored, with its size and time.
func (c clock) losses(list []*Loss) []string {
	var lines []string
	for _, l := range list {
		if l.Restored != nil || len(l.Windows) == 0 {
			continue
		}
		when := "closed at " + c.at(l.At)
		if l.Cause == CauseQuit {
			when = "lost as Ghostty quit at " + c.at(l.At)
		}
		lines = append(lines, "  "+size(l)+", "+when)
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

// settingsNotes are the notes on the settings in Ghostty's config that keep a restore from working
// as it should, as ghostty +show-config gives them. app is the Ghostty that runs, if any.
func settingsNotes(ctx context.Context, app ghostty.App) []string {
	bin, ok := ghostty.Binary(app)
	if !ok {
		return []string{"  cld cannot read Ghostty's config: ghostty is not installed"}
	}
	s, err := ghostty.ReadSettings(ctx, bin)
	if err != nil {
		return []string{"  cld cannot read Ghostty's config: " + err.Error()}
	}
	var notes []string
	switch s.SaveState {
	case "always":
	case "default":
		notes = append(notes, "  window-save-state is default: whether Ghostty brings back its"+
			" windows after Cmd+Q rests on macOS's own setting; set it to always")
	default:
		notes = append(notes, "  window-save-state is "+s.SaveState+
			": Ghostty brings back no window after Cmd+Q")
	}
	if s.Title {
		notes = append(notes, "  title is set in Ghostty's config: it hides cld's title,"+
			" and with it each pane's session")
	}
	if !s.SSHTerminfo {
		notes = append(notes, "  shell-integration-features lacks ssh-terminfo: cld types"+
			" TERM=xterm-256color for hosts Ghostty has no terminfo cached for")
	}
	if s.HiddenTitlebar {
		notes = append(notes, "  macos-titlebar-style is hidden: rebuilt tabs open as windows")
	}
	return notes
}
