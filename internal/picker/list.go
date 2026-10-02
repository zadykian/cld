package picker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/session"
)

// armWait is how long the kill stays armed for the second Ctrl+X, as in agent view. A key that
// came within it counts as such although cld reads it late (decision 15.1).
const armWait = 2 * time.Second

// repeatWait is how long Ctrl+X does nothing after the Ctrl+X that killed, and after each one
// that comes meanwhile. A Ctrl+X held down then kills nothing more (decision 15.1).
const repeatWait = time.Second

// list is the state of the list on screen.
type list struct {
	source Source
	rows   []session.Session
	// selected is the selected row; top is the first row in view.
	selected, top int
	// message takes the place of the footer's hints until the next key.
	message string
	// armed is whether Ctrl+X has armed the kill of the selected session. expiry fires once the
	// wait for the second Ctrl+X is over, and is nil when the kill is not armed.
	armed  bool
	expiry <-chan time.Time
	// hold fires once no Ctrl+X has come for repeatWait after a kill, and is nil otherwise: until
	// then, or another key, Ctrl+X does nothing.
	hold            <-chan time.Time
	columns, height int
	// acting gets the outcome of Enter's lookup, or of the kill or forget, while it runs, and is
	// nil otherwise. cancel abandons it, and running counts it until it has ended.
	acting  <-chan outcome
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// outcome is the outcome of Enter's lookup of the session on row, or of its kill or forget.
// refused is why join cannot take the session, or why the kill or forget did nothing; rows are the
// sessions read again, or unread why they could not be.
type outcome struct {
	row     session.Session
	kill    bool
	refused error
	rows    []session.Session
	unread  error
}

// press does what k does, and reports whether the user left. Ctrl+X arms the kill, or the forget
// on a row that has ended, and kills once armed. Any other key disarms it, then does what it does,
// but Esc only disarms it. While Enter's lookup, the kill or the forget runs, only leaving acts.
func (l *list) press(k key) bool {
	// After a kill or forget, Ctrl+X does nothing until none has come for repeatWait.
	if k == kill && l.hold != nil {
		l.hold = time.After(repeatWait)
		return false
	}
	l.hold = nil
	if l.acting != nil && k != esc && k != interrupt {
		return false
	}
	armed := l.armed
	l.disarm()
	l.message = ""
	switch k {
	case up:
		l.selected = max(l.selected-1, 0)
	case down:
		l.selected = max(min(l.selected+1, len(l.rows)-1), 0)
	case esc:
		return !armed
	case interrupt:
		return true
	case enter:
		if len(l.rows) > 0 {
			l.lookUp(l.rows[l.selected])
		}
	case kill:
		switch {
		case len(l.rows) == 0:
		case armed:
			l.kill(l.rows[l.selected])
			l.hold = time.After(repeatWait)
		default:
			l.armed, l.expiry = true, time.After(armWait)
		}
	}
	return false
}

// disarm disarms the kill.
func (l *list) disarm() {
	l.armed, l.expiry = false, nil
}

// start runs task beside the list, which goes on taking keys: acting gets its outcome.
func (l *list) start(task func(ctx context.Context) outcome) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan outcome, 1)
	l.acting, l.cancel = done, cancel
	l.running.Go(func() {
		done <- task(ctx)
	})
}

// lookUp starts Enter's lookup of the session on row, join's, and the read of the sessions again
// when join would refuse it.
func (l *list) lookUp(row session.Session) {
	l.start(func(ctx context.Context) outcome {
		result := outcome{row: row}
		result.refused = l.source.Joinable(ctx, row.Name)
		if result.refused != nil {
			result.rows, result.unread = l.read(ctx)
		}
		return result
	})
}

// kill starts the kill of the session on row, or the forget where it has ended, and the read of
// the sessions after it. The read passes over the killed session's server as that exits, and the
// session shows as one that has ended (decision 15.5).
func (l *list) kill(row session.Session) {
	l.start(func(ctx context.Context) outcome {
		result := outcome{row: row, kill: true}
		if row.State == session.Ended {
			result.refused = l.source.Forget(ctx, row.Name)
		} else {
			result.refused = l.source.Kill(ctx, row.Name, row.PIDs)
		}
		result.rows, result.unread = l.read(ctx)
		return result
	})
}

// read reads the sessions again for Enter's lookup or the kill or forget, unless the list has
// abandoned it: then they are unread, as ctx says.
func (l *list) read(ctx context.Context) ([]session.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.source.Sessions(ctx)
}

// settle takes the outcome of Enter's lookup or of the kill or forget, and returns the session to
// join, with no name where there is none. Otherwise it says why join, the kill or the forget was
// refused, and shows the rows read again, the selection following its session (decision 14.6).
// Rows it could not read stay, less the session its kill ended or its forget forgot.
func (l *list) settle(result outcome) session.Session {
	l.cancel()
	l.acting, l.cancel = nil, nil
	if !result.kill && result.refused == nil {
		return result.row
	}
	if result.refused != nil {
		l.message = describe(result.refused)
	}
	rows := result.rows
	if result.unread != nil {
		if problem := describe(result.unread); problem != l.message {
			l.message = strings.TrimPrefix(l.message+" · "+problem, " · ")
		}
		rows = l.rows
		if result.kill && result.refused == nil {
			rows = slices.DeleteFunc(slices.Clone(rows), func(row session.Session) bool {
				return row.Name == result.row.Name
			})
		}
	}
	l.selected = follow(l.rows, l.selected, rows)
	l.rows = rows
	return session.Session{}
}

// abandon ends Enter's lookup, the kill or the forget if one still runs, killing its tmux, and
// waits for it. Its outcome then counts for the rows as settle takes it, joining nothing. A kill
// cut short may or may not have ended the session, whose row stays.
func (l *list) abandon() {
	if l.acting == nil {
		return
	}
	l.cancel()
	l.running.Wait()
	select {
	case result := <-l.acting:
		l.settle(result)
	default:
	}
}

// follow is the row to select in rows, read again, where selected was the selected row of old.
// That is the same session where listed, or else the next of old still listed, the one that took
// its place, or else the one above it.
func follow(old []session.Session, selected int, rows []session.Session) int {
	find := func(s session.Session) int {
		return slices.IndexFunc(rows, func(row session.Session) bool { return row.Name == s.Name })
	}
	for i := selected; i < len(old); i++ {
		if found := find(old[i]); found >= 0 {
			return found
		}
	}
	for i := selected - 1; i >= 0; i-- {
		if found := find(old[i]); found >= 0 {
			return found
		}
	}
	return max(min(selected, len(rows)-1), 0)
}

// describe is what the list says about err: cld's message, without the advice meant for the
// command line.
func describe(err error) string {
	if failure, ok := errors.AsType[*fail.Error](err); ok {
		return failure.Message
	}
	return err.Error()
}
