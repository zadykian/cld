// Package mac is the Mac that the tests give cld restore tabs: Ghostty's windows, tabs and
// terminals, and the processes ps lists. A test saves a Model in $CLD_PROBE_DIR. The probe, as
// osascript, answers cld's scripts from it and saves what an apply builds there; as ps, it lists
// the model's processes. files.go names the files the fakes read and the logs they append.
package mac

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Model is Ghostty and its panes' processes. Ghostty does not run where Running is false, as where
// a test saved no model.
type Model struct {
	Running    bool                 `json:"running"`
	Apps       int                  `json:"apps,omitempty"` // Ghostty processes; 0 means 1
	PID        int                  `json:"pid"`            // Ghostty's, a row of Processes
	Launched   float64              `json:"launched"`       // seconds since 1970
	Executable string               `json:"executable"`
	Version    string               `json:"version"`
	Windows    []Window             `json:"windows"`   // front to back
	Hidden     []string             `json:"hidden"`    // app-wide terminals in no window
	Terminals  map[string]*Terminal `json:"terminals"` // by id
	Processes  []Process            `json:"processes"`
	Serial     int                  `json:"serial"` // the last id and pid the fake made
	// FailAt is the terminal, counted from 1 in each apply, that Ghostty fails to make: a new
	// window's or tab's never appears, and a split fails. 0 makes every one.
	FailAt int `json:"failAt,omitempty"`
	// DropMoves is how many move_tab actions Ghostty drops before it moves a tab again, as it drops
	// one sent to a terminal its tab does not focus.
	DropMoves int `json:"dropMoves,omitempty"`
}

// Window is a window's tabs, in tab-bar order.
type Window struct {
	Tabs []Tab `json:"tabs"`
}

// Tab is a tab and its split tree.
type Tab struct {
	Selected bool   `json:"selected"`
	Focused  string `json:"focused"` // a terminal's id, "" for none
	Tree     *Node  `json:"tree"`
}

// Node is a split tree: a leaf names its pane's terminal, a split its direction and its halves.
type Node struct {
	Pane   string `json:"pane,omitempty"`
	Split  string `json:"split,omitempty"` // right or down
	First  *Node  `json:"first,omitempty"`
	Second *Node  `json:"second,omitempty"`
}

// Terminal is a pane, as Ghostty's scripting shows it.
type Terminal struct {
	Name  string   `json:"name"` // the title
	Cwd   string   `json:"cwd"`
	PID   int      `json:"pid"`   // the foreground process group, 0 unknown
	TTY   string   `json:"tty"`   // /dev/ttysNNN, "" not ready
	Typed []string `json:"typed"` // in place: "^U", the line, "\r", in order
	Input string   `json:"input"` // its surface configuration's initial input
}

// Process is a row of ps.
type Process struct {
	PID   int       `json:"pid"`
	PPID  int       `json:"ppid"`
	PGID  int       `json:"pgid"`
	TTY   string    `json:"tty"` // as ps names it, ttys016 or pts/3; "" for none
	Start time.Time `json:"start"`
	Args  []string  `json:"args"`
}

// Load reads the model in dir: one whose Ghostty does not run where none was saved.
func Load(dir string) (*Model, error) {
	data, err := os.ReadFile(filepath.Join(dir, ModelFile))
	if errors.Is(err, os.ErrNotExist) {
		return &Model{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Model
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", ModelFile, err)
	}
	return &m, nil
}

// Save writes m in dir, as WriteFile does.
func Save(dir string, m *Model) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(dir, ModelFile, string(data)+"\n")
}

// TerminalID is the id of the terminal numbered n, as the fake Ghostty numbers those it makes.
func TerminalID(n int) string {
	return fmt.Sprintf("A1000000-0000-4000-8000-%012d", n)
}

// NewID takes the next serial, and is the id of a terminal the fake Ghostty makes.
func (m *Model) NewID() string {
	m.Serial++
	return TerminalID(m.Serial)
}

// NewPID takes the next serial, and is the pid of a process the fake starts.
func (m *Model) NewPID() int {
	m.Serial++
	return 50000 + m.Serial
}

// AddTerminal makes a terminal in no window yet, whose shell waits at its prompt, and returns its
// id. Its login is Ghostty's child, and runs -zsh in front, on a tty that no device backs. The id
// is NewID's, and the login's and the shell's pids the two NewPID takes after it.
func (m *Model) AddTerminal(cwd, input string) string {
	id := m.NewID()
	tty := fmt.Sprintf("/dev/ttys9%02d", m.Serial)
	login, shell := m.NewPID(), m.NewPID()
	start := time.Now().UTC().Truncate(time.Second)
	name := strings.TrimPrefix(tty, "/dev/")
	m.Processes = append(m.Processes,
		Process{PID: login, PPID: m.PID, PGID: login, TTY: name, Start: start,
			Args: []string{"/usr/bin/login", "-flp", "me", "/bin/bash", "--noprofile", "--norc",
				"-c", "exec -l /bin/zsh"}},
		Process{PID: shell, PPID: login, PGID: shell, TTY: name, Start: start,
			Args: []string{"-zsh"}})
	if m.Terminals == nil {
		m.Terminals = map[string]*Terminal{}
	}
	m.Terminals[id] = &Terminal{Cwd: cwd, PID: shell, TTY: tty, Input: input}
	return id
}

// IDs are the app-wide terminals: every window's, front to back and tab by tab, then Hidden.
func (m *Model) IDs() []string {
	var ids []string
	for _, window := range m.Windows {
		for _, tab := range window.Tabs {
			ids = append(ids, tab.Tree.Leaves()...)
		}
	}
	return append(ids, m.Hidden...)
}

// Locate is the window and the tab whose tree holds the terminal id, or -1 and -1.
func (m *Model) Locate(id string) (window, tab int) {
	for w := range m.Windows {
		for t := range m.Windows[w].Tabs {
			if slices.Contains(m.Windows[w].Tabs[t].Tree.Leaves(), id) {
				return w, t
			}
		}
	}
	return -1, -1
}

// Leaves are the tree's terminals, left or top first, as Ghostty lists a tab's terminals.
func (n *Node) Leaves() []string {
	switch {
	case n == nil:
		return nil
	case n.Pane != "":
		return []string{n.Pane}
	}
	return append(n.First.Leaves(), n.Second.Leaves()...)
}
