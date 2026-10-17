package tabs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/ghostty"
)

// A restore that opens Ghostty waits at most openWait from the open for the windows it brings
// back, reading them every openPoll.
const (
	openWait = 10 * time.Second
	openPoll = 500 * time.Millisecond
)

// opened is the Ghostty that open starts, once two snapshots in a row agree (see settled): it has
// brought its windows back. Past openWait, the last goes on, with a note. UNVERIFIED until #177's
// check 8 (decision 59.12): that open -b starts Ghostty, which brings back its windows within
// openWait.
func opened(ctx context.Context, report *Report) (*ghostty.Snapshot, error) {
	if err := ghostty.Open(ctx); err != nil {
		return nil, fail.Runtime("cannot open Ghostty: " + err.Error())
	}
	deadline := time.Now().Add(openWait)
	var last *ghostty.Snapshot
	var waited error // the last snapshot's error, where Ghostty was starting
	for time.Now().Before(deadline) {
		time.Sleep(openPoll)
		snap, err := ghostty.Take(ctx)
		switch {
		case starting(err):
			waited = err
			continue
		case err != nil:
			return nil, failure(err)
		case !snap.Running:
			continue
		case last != nil && settled(last, snap):
			return snap, nil
		}
		last = snap
	}
	return gaveUp(last, waited, report)
}

// starting reports whether err is what a snapshot meets while Ghostty starts: its process not
// ready, windows that change as it reads them, or a late answer.
func starting(err error) bool {
	e, ok := errors.AsType[*ghostty.Error](err)
	return ok && (e.Kind == ghostty.Gone || e.Kind == ghostty.Changed || e.Kind == ghostty.Timeout)
}

// gaveUp ends opened's wait past openWait: with last, the last snapshot of a Ghostty that runs,
// and a note. Without one, it fails with waited, the error of a Ghostty that did not settle, or
// as Ghostty did not start.
func gaveUp(last *ghostty.Snapshot, waited error, report *Report) (*ghostty.Snapshot, error) {
	if last != nil {
		report.note("Ghostty was still opening windows")
		return last, nil
	}
	if e, ok := errors.AsType[*ghostty.Error](waited); ok && e.Kind != ghostty.Gone {
		return nil, failure(waited)
	}
	return nil, fail.Runtime(fmt.Sprintf("Ghostty did not start in %d s",
		int(openWait.Seconds())))
}

// settled reports whether snapshots a and b, taken one after the other, show the same terminals
// in the same tabs and windows, each with its tty. A snapshot reads no window's or tab's id.
func settled(a, b *ghostty.Snapshot) bool {
	before, _ := layout(a)
	after, ready := layout(b)
	return ready && before == after
}

// layout is snap's terminal ids, grouped by tab and window, in order, and whether each terminal
// has its tty.
func layout(snap *ghostty.Snapshot) (string, bool) {
	var text strings.Builder
	ready := true
	for _, w := range snap.Windows {
		text.WriteString("[")
		for _, tab := range w.Tabs {
			text.WriteString("(")
			for _, t := range tab.Terminals {
				text.WriteString(t.ID + " ")
				ready = ready && t.TTY != ""
			}
			text.WriteString(")")
		}
		text.WriteString("]")
	}
	return text.String(), ready
}
