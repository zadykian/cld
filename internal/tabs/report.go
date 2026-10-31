package tabs

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// dryRun reports what the record holds after a restore's merge, seen. Ghostty comes first, then the
// panes a restore would join again, the losses, and those that wait for Ghostty's undo. Notes on
// Ghostty's config end it.
func dryRun(ctx context.Context, seen *observed, report *Report) {
	c := clock{now: time.Now()}
	c.home, _ = os.UserHomeDir() //nolint:errcheck // without one, directories stay whole
	r := seen.record
	report.Lines = append(report.Lines, c.ghostty(seen))
	report.section("Panes to join again:", c.joinAgain(r))
	report.section("Losses:", c.losses(r.Losses))
	for _, l := range r.Losses {
		if l.Cause == CauseQuit && len(l.Windows) == 0 {
			report.Lines = append(report.Lines,
				"Ghostty brought back every window as it quit at "+c.at(l.At))
		}
	}
	report.section("Waiting for Ghostty's undo:", c.losses(r.Pending))
	report.section("Notes:", settingsNotes(ctx, seen.app))
}

// clock writes the report's times, in local time: of today as 15:04, of another day with its
// date. A directory under home starts with ~.
type clock struct {
	now  time.Time
	home string
}

func (c clock) at(t time.Time) string { return c.format(t, "15:04") }

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

// ghostty is the report's first line: Ghostty's version, pid and start, or that none runs.
func (c clock) ghostty(seen *observed) string {
	if seen.o == nil {
		return "Ghostty is not running: the record as if it brought nothing back"
	}
	line := "Ghostty"
	if seen.o.Version != "" {
		line += " " + seen.o.Version
	}
	line += fmt.Sprintf(", pid %d", seen.app.PID)
	if seen.app.Launched > 0 {
		return line + ", started " + c.at(time.UnixMilli(int64(seen.app.Launched*1000)))
	}
	if p, ok := seen.o.Procs.ByPID(seen.app.PID); ok {
		return line + ", started " + c.at(p.Start)
	}
	return line
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
			lines = append(lines, fmt.Sprintf("  in %s: %s, %s", c.dir(p.Cwd), binding(p.Last),
				c.cut(p.Last)))
		}
	}
	return lines
}

// binding is the session or the host of last.
func binding(last *Last) string {
	if last.Session != "" {
		return fmt.Sprintf("session '%s' on %s", last.Session, last.Host)
	}
	return "ssh to " + last.Host
}

// cut is what ended last's binding: a typed line that left the pane at its prompt, or a quit.
func (c clock) cut(last *Last) string {
	switch {
	case last.Typed != nil:
		return "typed at " + c.at(*last.Typed) + " and back at its prompt"
	case last.Cut == CutQuit:
		return "as Ghostty quit at " + c.at(last.At)
	}
	return "as it closed at " + c.at(last.At)
}

// losses is a line per loss not restored, with its size and time, but for a quit that lost
// nothing.
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
