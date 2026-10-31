package tabs

import "time"

// Format is tabs.json's format. A record of a newer one is refused, and one of 0 is corrupt.
const Format = 1

// What a pane shows, as classify reads it.
const (
	KindCld   = "cld"   // ssh, with cld's title naming a session
	KindSSH   = "ssh"   // ssh, with any other title
	KindLocal = "local" // a shell at its prompt
	KindOther = "other" // any other program in front: an editor, a local tmux, mosh
	KindDead  = "dead"  // no program: the pane is not ready, or its program exited
)

// Why a loss was lost: Ghostty's quit, or the close of a window, a tab or a split.
const (
	CauseQuit   = "quit"
	CauseClosed = "closed"
	CauseTab    = "tab"
	CausePane   = "pane"
)

// How a tab's split tree came to be: cld built it, worked it out from the panes' sizes, or made
// it a row.
const (
	ShapeMade     = "made"
	ShapeInferred = "inferred"
	ShapeFlat     = "flat"
)

// Record is tabs.json: the panes of Ghostty's current run, its windows, and what it lost. Times are
// in UTC, to the second, so that a record written again has the same bytes (decision 59.4).
type Record struct {
	Format   int              `json:"format"`
	Run      Run              `json:"run"`
	Previous string           `json:"previous,omitempty"` // the run before Run, by id
	Panes    map[string]*Pane `json:"panes"`              // by terminal id: the panes of Windows
	Windows  []Window         `json:"windows"`            // front to back; hidden-only ones last
	Pending  []*Loss          `json:"pending"`            // gone, their logins still alive
	Losses   []*Loss          `json:"losses"`             // oldest first
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
	Kind     string    `json:"kind"`
	Host     string    `json:"host,omitempty"`    // ssh's destination as typed: cld and ssh only
	Session  string    `json:"session,omitempty"` // the whole NAME: cld only
	Worktree bool      `json:"worktree,omitempty"`
	Title    string    `json:"title,omitempty"`
	Cwd      string    `json:"cwd,omitempty"`
	TTY      string    `json:"tty,omitempty"`
	Login    int       `json:"login,omitempty"` // the pane's /usr/bin/login, Ghostty's child
	Size     *Size     `json:"size,omitempty"`
	Since    time.Time `json:"since"` // when Host or Session last changed
}

// Size is a pane's winsize. The pixels are the surface's, less its padding.
type Size struct {
	Cols   int `json:"cols"`
	Rows   int `json:"rows"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Window is a window's tabs, in tab-bar order.
type Window struct {
	Tabs []Tab `json:"tabs"`
}

// Tab is a tab and its split tree.
type Tab struct {
	Selected bool   `json:"selected,omitempty"`
	Focused  string `json:"focused,omitempty"` // a terminal id
	Shape    string `json:"shape"`
	Tree     *Node  `json:"tree"`
}

// Node is a split tree. A leaf names its pane by terminal id. A split has a direction, right or
// down as Ghostty's split words go, and two halves, left or top first.
type Node struct {
	Pane   string  `json:"pane,omitempty"`
	Copy   *Pane   `json:"copy,omitempty"` // in Pending and Losses: the pane as last seen
	Split  string  `json:"split,omitempty"`
	Ratio  float64 `json:"ratio,omitempty"` // the first half's share; 0 unknown
	First  *Node   `json:"first,omitempty"`
	Second *Node   `json:"second,omitempty"`
}

// Loss is what a quit or a close took: whole windows, or one tab or split as a window of one tab
// with its Anchor.
type Loss struct {
	At       time.Time  `json:"at"`
	Run      string     `json:"run"`
	Cause    string     `json:"cause"`
	Restored *time.Time `json:"restored"`
	Anchor   *Anchor    `json:"anchor,omitempty"`
	Windows  []Window   `json:"windows"`
}

// Anchor is a pane still open beside a lost tab or split. A tab goes back at Index in the pane's
// window, and a split beside the pane, on the side Split names.
type Anchor struct {
	Pane  string `json:"pane"`
	Index int    `json:"index,omitempty"` // a tab's place in its window, from 1
	Split string `json:"split,omitempty"` // right, down, left or up
}
