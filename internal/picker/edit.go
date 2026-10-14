package picker

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/zadykian/cld/internal/session"
)

// editor is Ctrl+R's editor in the footer: the name typed for the session on row, which has ended.
// It keeps every byte typed, for the rename to check the name whole (decision 57.2).
type editor struct {
	row  session.Session
	text []byte
}

// openEditor opens the editor on row, with its name typed, where it has ended. A row that runs gets
// the message instead (decision 57.3).
func (l *list) openEditor(row session.Session) {
	if row.State != session.Ended {
		l.message = fmt.Sprintf("end session '%s' with ctrl+x twice first, to rename it", row.Name)
		return
	}
	l.edit = &editor{row: row, text: []byte(row.Name)}
}

// typeIn does what k does in the editor, and reports whether the user left. Enter renames the
// session to the name typed, closing the editor; Esc closes it, and Ctrl+C leaves the list.
// Backspace takes the last character off the name, and Ctrl+U the whole name. A pasted CR is an
// Enter (decision 57.2). The arrows, Ctrl+X, Ctrl+R and the other sequences do nothing.
func (l *list) typeIn(k key) bool {
	switch k {
	case interrupt:
		return true
	case esc:
		l.edit = nil
	case enter:
		row, to := l.edit.row, string(l.edit.text)
		l.edit = nil
		if to != row.Name {
			l.rename(row, to)
		}
	case backspace:
		_, size := utf8.DecodeLastRune(l.edit.text)
		l.edit.text = l.edit.text[:len(l.edit.text)-size]
	case discard:
		l.edit.text = l.edit.text[:0]
	default:
		if b, ok := k.text(); ok {
			l.edit.text = append(l.edit.text, b)
		}
	}
	return false
}

// reopen opens the editor again after the rename of the session on row to the name to was refused,
// with that name typed. It does so only where the rows still list the session as ended.
func (l *list) reopen(row session.Session, to string) {
	if slices.ContainsFunc(l.rows, func(listed session.Session) bool {
		return listed.Name == row.Name && listed.State == session.Ended
	}) {
		l.edit = &editor{row: row, text: []byte(to)}
	}
}

// renamed is rows with the row of session from named to.
func renamed(rows []session.Session, from, to string) []session.Session {
	rows = slices.Clone(rows)
	for i := range rows {
		if rows[i].Name == from {
			rows[i].Name = to
		}
	}
	return rows
}

// editFooter is the footer while the editor is open: "rename to ", the name typed and the cursor,
// a cell in inverse video. The terminal's width cuts that from the left. The message, or else the
// hints, follow where they fit whole; a message that does not takes the footer alone.
func (l *list) editFooter() string {
	line := cutLeft("rename to "+string(l.edit.text), l.columns-1)
	room := l.columns - cells(line) - 1
	line += inverse + " " + noInverse
	switch message := oneLine(l.message); {
	case message == "":
		if hints := " · enter to rename · esc to cancel"; cells(hints) <= room {
			return line + dim + hints + noDim
		}
		return line
	case cells(" · "+message) <= room:
		return line + " · " + cut(message, room)
	default:
		return cut(message, l.columns)
	}
}

// cutLeft is text in at most columns cells, as cut has it, but what does not fit is left out at
// its start.
func cutLeft(text string, columns int) string {
	shown := cut(text, cells(text))
	for cells(shown) > max(columns, 0) {
		_, size := utf8.DecodeRuneInString(shown)
		shown = shown[size:]
	}
	return shown
}
