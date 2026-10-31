package tabs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/ghostty"
)

// dryRun reports what a restore would do of plan p, made against the record in dir as seen. left
// is the losses it would not reopen. Ghostty and the watcher come first. Then come the panes to
// type into, the windows to reopen, the losses left and those that wait for Ghostty's undo. Notes
// on the plan and on Ghostty's config end it.
func dryRun(ctx context.Context, dir string, seen *observed, p *Planned, left []*Loss,
	report *Report,
) {
	c := clock{now: time.Now()}
	c.home, _ = os.UserHomeDir() //nolint:errcheck // without one, directories stay whole
	r := seen.record
	report.Lines = append(report.Lines, c.ghostty(seen))
	report.Lines = append(report.Lines, c.watcher(WatcherStatus(dir))...)
	c.wouldType(r, p, report)
	c.wouldReopen(p, report)
	report.section("Losses:", c.losses(left))
	for _, l := range r.Losses {
		if l.Cause == CauseQuit && len(l.Windows) == 0 {
			report.Lines = append(report.Lines,
				"Ghostty brought back every window as it quit at "+c.at(l.At))
		}
	}
	report.section("Waiting for Ghostty's undo:", c.losses(r.Pending))
	var notes []string
	for _, note := range p.notes() {
		notes = append(notes, "  "+note)
	}
	report.section("Notes:", append(notes, settingsNotes(ctx, seen.app)...))
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

// wouldType adds a line per pane Ghostty kept that plan p joins again: its directory, what cut its
// binding, and the line typed.
func (c clock) wouldType(r *Record, p *Planned, report *Report) {
	var lines []string
	for i, j := range p.typed {
		where := "in " + c.dir(paneCwd(r, j.pane))
		if p := r.Panes[j.pane]; p != nil && p.Last != nil {
			where += ", " + c.cut(p.Last)
		}
		lines = append(lines, "  "+where+": "+p.script.Typed[i].Line)
	}
	report.section("Would type into "+count(len(lines), "pane")+":", lines)
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

// wouldReopen adds a section per loss that plan p rebuilds: when it was lost, then a line per
// window. Each pane shows what its line joins, or a shell in its directory.
func (c clock) wouldReopen(p *Planned, report *Report) {
	for i := 0; i < len(p.builds); {
		l := p.builds[i].loss
		var lines []string
		n := i
		for ; n < len(p.builds) && p.builds[n].loss == l; n++ {
			lines = append(lines, "  "+c.panes(p.script.Builds[n], p.builds[n]))
		}
		report.section("Would reopen "+count(n-i, "window")+", "+c.when(l)+":", lines)
		i = n
	}
}

// panes is what build b of info's loss makes: its tabs, each its panes split by |. A pane names
// the session or host its line joins, or a shell in its directory.
func (c clock) panes(b ghostty.Build, info building) string {
	var tabs []string
	for _, tab := range b.Tabs {
		var panes []string
		for _, leaf := range buildLeaves(tab.Tree) {
			if bind, ok := info.bindings[leaf.Key]; ok {
				panes = append(panes, bind.String())
			} else {
				panes = append(panes, "a shell in "+c.dir(leaf.Cwd))
			}
		}
		tabs = append(tabs, strings.Join(panes, " | "))
	}
	return count(len(tabs), "tab") + ": " + strings.Join(tabs, ", ")
}

// losses is a line per loss not restored, with its size and time, but for a quit that lost
// nothing.
func (c clock) losses(list []*Loss) []string {
	var lines []string
	for _, l := range list {
		if l.Restored != nil || len(l.Windows) == 0 {
			continue
		}
		lines = append(lines, "  "+size(l)+", "+c.when(l))
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
			" TERM="+fallbackTerm+" for hosts Ghostty has no terminfo cached for")
	}
	if s.HiddenTitlebar {
		notes = append(notes, "  macos-titlebar-style is hidden: rebuilt tabs open as windows")
	}
	return notes
}
