package tabs

import (
	"slices"
	"time"
)

// What a restore reopens by default: the losses of the last recentSpan, and the run before's
// quit (decision 59.6). --older reopens a burst at a time, the losses within burstSpan of the
// newest one left.
const (
	recentSpan = 15 * time.Minute
	burstSpan  = 10 * time.Minute
)

// selectLosses is what a restore reopens of r's losses at time now, as opts asks, and what it
// leaves, both oldest first. A loss restored or empty is neither. A tab or split closed alone
// counts only with --closed, and is left otherwise.
func selectLosses(r *Record, opts Options, now time.Time) (chosen, left []*Loss) {
	var older []*Loss
	for _, l := range r.Losses {
		if candidate(l, opts.Closed) && !opts.All && !byDefault(r, l, now) {
			older = append(older, l)
		}
	}
	var next []*Loss
	if opts.Older {
		next = burst(older)
	}
	for _, l := range r.Losses {
		switch {
		case candidate(l, opts.Closed) &&
			(opts.All || byDefault(r, l, now) || slices.Contains(next, l)):
			chosen = append(chosen, l)
		case candidate(l, true):
			left = append(left, l)
		}
	}
	return chosen, left
}

// candidate reports whether a restore may reopen loss l: not restored, with a window, and a
// window's close or a quit. A tab's or a split's close counts where closed holds.
func candidate(l *Loss, closed bool) bool {
	if l.Restored != nil || len(l.Windows) == 0 {
		return false
	}
	switch l.Cause {
	case CauseClosed, CauseQuit:
		return true
	case CauseTab, CausePane:
		return closed
	}
	return false
}

// byDefault reports whether a restore of r without options at time now reopens loss l: lost in the
// last recentSpan, or the quit of the run before, whatever its age.
func byDefault(r *Record, l *Loss, now time.Time) bool {
	recent := now.Sub(l.At) <= recentSpan
	return recent || (l.Cause == CauseQuit && r.Previous != "" && l.Run == r.Previous)
}

// burst is the newest burst of losses: the newest one, and those up to burstSpan before it.
func burst(losses []*Loss) []*Loss {
	if len(losses) == 0 {
		return nil
	}
	newest := slices.MaxFunc(losses, func(a, b *Loss) int { return a.At.Compare(b.At) }).At
	return slices.DeleteFunc(slices.Clone(losses), func(l *Loss) bool {
		return newest.Sub(l.At) > burstSpan
	})
}

// reopenWith is the option that reopens loss l, which a restore of r at time now with opts left.
func reopenWith(r *Record, l *Loss, opts Options, now time.Time) string {
	alone := l.Cause == CauseTab || l.Cause == CausePane
	switch {
	case alone && !opts.Closed && byDefault(r, l, now):
		return "--closed"
	case alone && !opts.Closed:
		return "--closed with --older or --all"
	}
	return "--older or --all"
}
