package tabs

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// dryRun reports what the record in dir holds after a restore's merge, seen. Ghostty and the
// watcher come first, then the panes a restore would join again, the losses, and those that wait
// for Ghostty's undo. Notes on Ghostty's config end it.
func dryRun(ctx context.Context, dir string, seen *observed, report *Report) {
	c := clock{now: time.Now()}
	c.home, _ = os.UserHomeDir() //nolint:errcheck // without one, directories stay whole
	r := seen.record
	report.Lines = append(report.Lines, c.ghostty(seen))
	report.Lines = append(report.Lines, c.watcher(WatcherStatus(dir))...)
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

// watcher is the watcher's lines: its pid and last tick, or that none runs, and its last line.
func (c clock) watcher(s Status) []string {
	line := "The watcher: none runs; cld restore tabs --watch in a Ghostty pane starts one"
	if s.PID != 0 {
		line = fmt.Sprintf("The watcher: pid %d, last read at %s", s.PID, c.atSecond(s.LastTick))
	}
	if s.LastLine == "" {
		return []string{line}
	}
	return []string{line, "The watcher's last line: " + s.LastLine}
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
