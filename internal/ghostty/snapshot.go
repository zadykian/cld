package ghostty

import (
	"context"
	"time"
)

// SnapshotTimeout bounds one snapshot.js run. A first Automation prompt can block osascript, so
// the message on a timeout says to answer it and run again.
const SnapshotTimeout = 15 * time.Second

// Snapshot is what snapshot.js reads of Ghostty. Where Running is false, no Ghostty process runs
// and the script sent no Apple Event. Error is set where the script failed, and Take returns it as
// an *Error instead.
type Snapshot struct {
	Running bool         `json:"running"`
	Apps    []App        `json:"apps"`              // more than one is Ghostty run by open -n
	Version string       `json:"version,omitempty"` // "" where the read failed
	Windows []Window     `json:"windows,omitempty"` // front to back
	Error   *ScriptError `json:"error,omitempty"`
}

// App is a Ghostty process, as NSRunningApplication gives it without an Apple Event.
type App struct {
	PID        int     `json:"pid"`
	Launched   float64 `json:"launched"`   // launchDate, seconds since 1970; 0 unknown
	Executable string  `json:"executable"` // executableURL's path, "" unknown
}

// Window is a window, as Ghostty's AppleScript sees one: an AppKit tab group.
type Window struct {
	Tabs []Tab `json:"tabs"` // in tab-bar order
}

// Tab is a tab and its split tree's leaves. Ghostty's AppleScript gives no split's direction or
// ratio, nor any id that lasts (decision 59.4).
type Tab struct {
	Selected  bool       `json:"selected"`
	Focused   string     `json:"focused"`   // the focused terminal's id, "" where missing
	Terminals []Terminal `json:"terminals"` // the leaves, left or top first
}

// Terminal is a pane: a surface, whose id Ghostty keeps for the surface's life.
type Terminal struct {
	ID   string `json:"id"`
	Name string `json:"name"` // the title
	Cwd  string `json:"cwd"`  // the working directory Ghostty last accepted
	PID  int    `json:"pid"`  // the foreground process group, 0 unknown
	TTY  string `json:"tty"`  // /dev/ttysNNN, "" not ready
}

// Take runs snapshot.js. A Snapshot whose Error is set is returned as an *Error instead.
func Take(ctx context.Context) (*Snapshot, error) {
	snap := &Snapshot{}
	if err := run(ctx, SnapshotTimeout, snapshotJS, struct{}{}, snap); err != nil {
		return nil, err
	}
	if snap.Error != nil {
		return nil, classify(snap.Error)
	}
	return snap, nil
}

// Ghostty is the one running Ghostty: an *Error of kind Several where more run, and of kind Gone
// where none does.
func (s *Snapshot) Ghostty() (App, error) {
	switch {
	case len(s.Apps) == 0:
		return App{}, &Error{Kind: Gone}
	case len(s.Apps) > 1:
		return App{}, &Error{Kind: Several, apps: len(s.Apps)}
	}
	return s.Apps[0], nil
}
