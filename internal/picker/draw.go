package picker

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/term"
	"golang.org/x/text/width"

	"github.com/zadykian/cld/internal/session"
)

// The styles the list draws in.
const (
	inverse   = "\x1b[7m"
	noInverse = "\x1b[27m"
	dim       = "\x1b[2m"
	noDim     = "\x1b[22m"
	bold      = "\x1b[1m"
	noBold    = "\x1b[22m"
)

// measure reads the terminal's size: 80 by 24 where it cannot.
func (l *list) measure() {
	columns, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || columns <= 0 || height <= 0 {
		columns, height = 80, 24
	}
	l.columns, l.height = columns, height
}

// frame draws the list over the whole screen, clearing each line before drawing it, then the rest.
// Each line is cut at the terminal's width, and moving to a line by position ends any wrap that a
// full line before it left pending.
func (l *list) frame() string {
	var out strings.Builder
	lines := l.lines()
	for i, line := range lines {
		fmt.Fprintf(&out, "\x1b[%d;1H\x1b[2K%s", i+1, line)
	}
	if len(lines) < l.height {
		fmt.Fprintf(&out, "\x1b[%d;1H\x1b[J", len(lines)+1)
	}
	return out.String()
}

// lines are the lines the list shows, styled: the header, the rows in view with the selected one
// marked and in inverse video, or "no sessions", then a blank line and the footer. A terminal too
// short for them all loses the blank line, then the header, then the footer.
func (l *list) lines() []string {
	nameWidth, stateWidth := 4, 8
	for _, row := range l.rows {
		nameWidth = max(nameWidth, cells(row.Name))
		stateWidth = max(stateWidth, cells(row.ShownState()))
	}
	format := func(marker, name, state, active, directory string) string {
		return marker + " " + pad(name, nameWidth) + "  " + pad(state, stateWidth) + "  " +
			pad(active, 11) + "  " + directory
	}
	row := func(marker string, s session.Session) string {
		return format(marker, s.Name, s.ShownState(), s.LastActive(), s.Directory)
	}
	title := format(" ", "NAME", "STATE", "LAST ACTIVE", "DIRECTORY")
	header := []string{cut(title, l.columns)}
	var rows []string
	if len(l.rows) == 0 {
		rows = []string{cut("no sessions", l.columns)}
	} else {
		// The selected row's inverse video spans the table, as far as the terminal shows it.
		tableWidth := cells(title)
		for _, s := range l.rows {
			tableWidth = max(tableWidth, cells(row(">", s)))
		}
		// The rows in view: as many as fit, the selected one among them.
		fits := max(l.height-3, 1)
		l.top = min(l.top, l.selected)
		l.top = max(l.top, l.selected-fits+1)
		l.top = max(min(l.top, len(l.rows)-fits), 0)
		for i := l.top; i < min(l.top+fits, len(l.rows)); i++ {
			if i == l.selected {
				line := pad(cut(row(">", l.rows[i]), l.columns), min(tableWidth, l.columns))
				rows = append(rows, inverse+waiting(line, l.rows[i], nameWidth)+noInverse)
			} else {
				rows = append(rows, waiting(cut(row(" ", l.rows[i]), l.columns), l.rows[i], nameWidth))
			}
		}
	}
	blank, footer := []string{""}, []string{l.footer()}
	for _, left := range []*[]string{&blank, &header, &footer} {
		if len(header)+len(rows)+len(blank)+len(footer) > l.height {
			*left = nil
		}
	}
	return slices.Concat(header, rows, blank, footer)
}

// waiting is line, the row of session s cut at the terminal's width, with the word waiting in bold
// as far as line holds it, where claude waits for an answer (decision 49). The word follows the
// marker, the name nameWidth wide and the state, in cells.
func waiting(line string, s session.Session, nameWidth int) string {
	if s.Status != "waiting" {
		return line
	}
	return embolden(line, 2+nameWidth+2+cells(s.State+", "), cells(s.Status))
}

// embolden is line with the count cells from cell from on in bold, as many of them as line has.
func embolden(line string, from, count int) string {
	start, end, used := -1, len(line), 0
	for i, r := range line {
		if used >= from+count {
			end = i
			break
		}
		if used >= from && start < 0 {
			start = i
		}
		used += runeCells(r)
	}
	if start < 0 {
		return line
	}
	return line[:start] + bold + line[start:end] + noBold + line[end:]
}

// footer is the line under the rows: the message, the kill's question once armed, or the hints
// for the keys, cut at the terminal's width. On a row that has ended, Enter resumes and the kill
// is a forget.
func (l *list) footer() string {
	if l.message != "" {
		return cut(strings.Join(strings.Fields(l.message), " "), l.columns)
	}
	enter, kill, detach := "join", "kill", ""
	if len(l.rows) > 0 {
		switch row := l.rows[l.selected]; {
		case row.State == session.Ended:
			enter, kill = "resume", "forget"
		case row.Attached:
			detach = " and detach its terminal"
		}
	}
	hints := "esc to quit"
	switch {
	case l.armed:
		hints = "ctrl+x again to " + kill + detach + " · esc to keep"
	case len(l.rows) > 0:
		// The arrows' hint goes first where the terminal is short of cells for them all: 61 columns
		// or fewer on a row to join. From 44 columns on, the others then fit whole.
		hints = "enter to " + enter + " · ctrl+x to " + kill + " · " + hints
		if all := "↑/↓ to navigate · " + hints; cells(all) <= l.columns {
			hints = all
		}
	}
	return dim + cut(hints, l.columns) + noDim
}

// cells is the number of terminal cells text takes: two for a wide character, none for a
// combining one, and one for any other, ambiguous ones such as ↑, · or é included (decision 14.7).
func cells(text string) int {
	count := 0
	for _, r := range text {
		count += runeCells(r)
	}
	return count
}

// runeCells is the number of cells r takes, as cells counts them.
func runeCells(r rune) int {
	switch {
	case r == '\u00ad': // the soft hyphen shows as one
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// cut is text in at most columns cells: what does not fit is left out, a wide character that
// would straddle the edge included. A control character shows as "?", so that none moves the
// cursor or changes the terminal; bytes that are not UTF-8 show as U+FFFD.
func cut(text string, columns int) string {
	var out strings.Builder
	used := 0
	for _, r := range text {
		if unicode.IsControl(r) {
			r = '?'
		}
		size := runeCells(r)
		if used+size > columns {
			break
		}
		used += size
		out.WriteRune(r)
	}
	return out.String()
}

// pad is text followed by spaces up to columns cells.
func pad(text string, columns int) string {
	return text + strings.Repeat(" ", max(columns-cells(text), 0))
}
