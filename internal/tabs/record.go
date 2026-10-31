package tabs

import "time"

// Format is tabs.json's format. A record of a newer one is refused, and one of 0 is corrupt.
const Format = 1

// Record is tabs.json: what cld keeps of Ghostty's current run. Times are in UTC, to the second,
// so that a record written again has the same bytes (decision 59.4).
type Record struct {
	Format int              `json:"format"`
	Run    Run              `json:"run"`
	Panes  map[string]*Pane `json:"panes"` // by terminal id
}

// Run is one run of Ghostty, from its launch to its quit.
type Run struct {
	ID      string    `json:"id"` // PID@START, START ps's lstart in UTC, as RFC 3339
	Version string    `json:"version"`
	Ended   time.Time `json:"ended,omitzero"` // when the watcher saw Ghostty's pid go
}

// Pane is a terminal as the record keeps it, by its id. UNVERIFIED until #177's checks 7 and 8
// (decision 59.12): that the id lasts through an undo, a tab's drag and Ghostty's restoration.
type Pane struct {
	Title string    `json:"title,omitempty"`
	Cwd   string    `json:"cwd,omitempty"`
	TTY   string    `json:"tty,omitempty"`
	Since time.Time `json:"since"` // when the pane was first seen
}
