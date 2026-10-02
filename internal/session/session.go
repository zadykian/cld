package session

import (
	"strconv"
	"time"
)

// Session is one of the sessions cld started.
type Session struct {
	// Name is the session's NAME, without "cld-".
	Name string
	// State is "attached" or "detached", whether a terminal is attached, "exited" once claude
	// has, or Ended once its server no longer runs, for a session of cld's record.
	State string
	// Attached is whether a terminal is attached, claude exited or not.
	Attached bool
	// Status is what claude is doing, as statusHooks keep it: "busy", "waiting" or "idle". Status
	// is "" where no hook has set it, once claude has exited, and for a session that has ended. It
	// goes by the active pane, and has the hooks' limits (decisions 49.1 and 49.5).
	Status string
	// PIDs are the process ids of the programs in the session's panes, claude's among them. A pane
	// keeps its pid once its program has exited, and a session made again has others (see End).
	PIDs []string
	// Directory is the directory claude is in now, or once claude has exited, the one its session
	// started in. A session that has ended has the one its entry names.
	Directory string
	// Home is where join made the session, as @cld-home has it (see Home). A session that has ended
	// has "", as does one of a cld that recorded none, 0.8.2 or earlier.
	Home string
	// Idle is how long the session had been idle when Sessions read it, 0 while a terminal is
	// attached (decision 46.1). A time tmux does not give as a number, or a time to come, counts
	// as now, so that the sweep ends nothing on it. A session that has ended has none.
	Idle time.Duration
}

// ShownState is State as list shows it: with claude's Status after it, where there is one -
// "detached, waiting".
func (s Session) ShownState() string {
	if s.Status == "" {
		return s.State
	}
	return s.State + ", " + s.Status
}

// LastActive is Idle as list shows it: "now" under a minute, and otherwise in whole minutes, hours
// or days, the largest that fits. A session that has ended shows "-" (decision 46.5).
func (s Session) LastActive() string {
	switch {
	case s.State == Ended:
		return "-"
	case s.Idle < time.Minute:
		return "now"
	case s.Idle < time.Hour:
		return strconv.Itoa(int(s.Idle/time.Minute)) + "m"
	case s.Idle < 24*time.Hour:
		return strconv.Itoa(int(s.Idle/time.Hour)) + "h"
	}
	return strconv.Itoa(int(s.Idle/(24*time.Hour))) + "d"
}

// endedSession is the session of entry r as Sessions reads it once it has ended.
func endedSession(r entry) Session {
	return Session{Name: r.Name, State: Ended, Directory: r.Directory}
}

// EndedSession is session cld-SUFFIX as Sessions would read it now that the idle sweep has ended
// it, without reading every server again. The bool says whether there is one: none where cld's
// record keeps no entry of it.
func EndedSession(suffix string) (Session, bool) {
	r, ok := recorded(suffix)
	if !ok {
		return Session{}, false
	}
	return endedSession(r), true
}
