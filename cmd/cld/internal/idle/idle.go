// Package idle is CLD_IDLE_DAYS, how long a session may stay idle, and the sweep that list and
// join run, ending the sessions idle for longer (decision 46).
package idle

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/session"
)

// days matches the days CLD_IDLE_DAYS takes: decimal digits, with a fraction or without.
var days = regexp.MustCompile(`^([0-9]+\.?[0-9]*|\.[0-9]+)$`)

// Limit is how long a session may stay idle before the sweep ends it: CLD_IDLE_DAYS days, 30
// where unset or empty. A CLD_IDLE_DAYS of 0, or one longer than a time.Duration holds, gives 0,
// no limit. Anything else is refused rather than read as another limit (decision 46.3).
func Limit() (time.Duration, error) {
	value := os.Getenv("CLD_IDLE_DAYS")
	if value == "" {
		return 30 * 24 * time.Hour, nil
	}
	if !days.MatchString(value) {
		return 0, fail.Runtime(fmt.Sprintf("CLD_IDLE_DAYS is not a number of days: '%s'", value))
	}
	// days took value, so ParseFloat fails only on a number too large for a float64, giving +Inf.
	count, _ := strconv.ParseFloat(value, 64) //nolint:errcheck // explained above
	limit := math.Ceil(count * float64(24*time.Hour))
	if limit >= math.MaxInt64 {
		return 0, nil
	}
	return time.Duration(limit), nil
}

// Sweep ends each of sessions idle for longer than limit, none where limit is 0 (decision 46).
// It returns the sessions as list then shows them, those it ended as ended where the record keeps
// them. It keeps the session whose server cld runs on, and left, the one a move has just left
// (decisions 46.2 and 51.5).
func Sweep(tmux *session.Tmux, sessions []session.Session, limit time.Duration,
	left string) []session.Session {
	if limit == 0 {
		return sessions
	}
	own, inside := session.OwnServer()
	var kept []session.Session
	for _, s := range sessions {
		spared := s.Idle <= limit || inside && s.Name == own || s.Name == left
		if spared || !end(tmux, s, limit) {
			kept = append(kept, s)
		} else if row, recorded := session.EndedSession(s.Name); recorded {
			kept = append(kept, row)
		}
	}
	return kept
}

// end ends session s, idle for longer than limit, with a note on stderr, and reports whether it
// did. The kill checks again that the session is idle, and one that fails is a warning
// (decision 46.4).
func end(tmux *session.Tmux, s session.Session, limit time.Duration) bool {
	ended, err := tmux.EndIdle(context.Background(), s.Name, limit)
	if err != nil {
		output.Warn(fmt.Sprintf("cannot end session '%s', idle for %s: %s", s.Name, For(s.Idle),
			err))
	}
	if ended {
		output.Note(fmt.Sprintf("ended session '%s', idle for %s", s.Name, For(s.Idle)))
	}
	return ended
}

// For is how long a session has been idle, for the notes: in whole days, hours, minutes or
// seconds, the largest that fits, as "31 days" or "1 hour".
func For(idle time.Duration) string {
	unit, name := time.Second, "second"
	for _, larger := range []struct {
		size time.Duration
		name string
	}{{24 * time.Hour, "day"}, {time.Hour, "hour"}, {time.Minute, "minute"}} {
		if idle >= larger.size {
			unit, name = larger.size, larger.name
			break
		}
	}
	count := int64(idle / unit)
	if count != 1 {
		name += "s"
	}
	return strconv.FormatInt(count, 10) + " " + name
}
