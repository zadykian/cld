package tests

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/mac"
	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The model Mac that the tabs_* files give cld restore tabs: a Ghostty and its windows.

// ghosttyStart is when the model Ghostty started, as its ps row says: a day no test runs on, so
// that the dry-run writes it with its date.
var ghosttyStart = time.Date(2025, time.January, 2, 9, 12, 0, 0, time.UTC)

// ghosttyAt is a running Ghostty 1.4.0 with no window, of pid, which started at start.
//
//nolint:unparam // the tests that restart Ghostty pass other pids
func ghosttyAt(pid int, start time.Time) *mac.Model {
	return &mac.Model{Running: true, PID: pid, Launched: float64(start.Unix()),
		Executable: ghosttyExecutable, Version: "1.4.0", Terminals: map[string]*mac.Terminal{},
		Processes: []mac.Process{{PID: pid, PPID: 1, PGID: pid, Start: start,
			Args: []string{ghosttyExecutable}}}}
}

// quietConfig is what ghostty +show-config prints where Ghostty's config gives no note.
const quietConfig = "window-save-state = always\nshell-integration-features = cursor,ssh-terminfo\n"

// ghosttyApp is ghosttyAt for s, whose ghostty prints quietConfig.
func ghosttyApp(t *testing.T, s *sandbox.Sandbox) *mac.Model {
	t.Helper()
	s.WriteProbeFile(mac.ShowConfig, quietConfig)
	return ghosttyAt(ghosttyPID, ghosttyStart)
}

// tabOf is a tab of the panes ids in a row, left to right, the first focused.
func tabOf(ids ...string) mac.Tab {
	var tree *mac.Node
	for _, id := range slices.Backward(ids) {
		leaf := &mac.Node{Pane: id}
		if tree == nil {
			tree = leaf
		} else {
			tree = &mac.Node{Split: "right", First: leaf, Second: tree}
		}
	}
	return mac.Tab{Focused: ids[0], Tree: tree}
}

// windowOf is a window of tabs, the first selected.
func windowOf(tabs ...mac.Tab) mac.Window {
	tabs[0].Selected = true
	return mac.Window{Tabs: tabs}
}

// addPane adds a pane to m, in no window yet, titled title, and returns its terminal's id. Its
// login runs -zsh, with jobs in front of it as front puts them.
func addPane(m *mac.Model, title string, jobs ...[]string) string {
	id := m.AddTerminal("/Users/me", "")
	m.Terminals[id].Name = title
	front(m, id, jobs...)
	return id
}

// front puts jobs in front of pane id's shell, in place of those there. The first, the shell's
// child, leads the foreground group, and each other is a child of the first, in its group. With
// no job, the shell is at its prompt.
func front(m *mac.Model, id string, jobs ...[]string) {
	terminal := m.Terminals[id]
	tty, login := strings.TrimPrefix(terminal.TTY, "/dev/"), loginOf(m, id)
	var kept []mac.Process
	for _, p := range m.Processes {
		if p.TTY != tty || p.PID == login || p.PPID == login {
			kept = append(kept, p)
		}
		if p.TTY == tty && p.PPID == login {
			terminal.PID = p.PID
		}
	}
	m.Processes = kept
	if len(jobs) == 0 {
		return
	}
	shell, start := terminal.PID, time.Now().UTC().Truncate(time.Second)
	terminal.PID = m.NewPID()
	m.Processes = append(m.Processes, mac.Process{PID: terminal.PID, PPID: shell,
		PGID: terminal.PID, TTY: tty, Start: start, Args: jobs[0]})
	for _, args := range jobs[1:] {
		m.Processes = append(m.Processes, mac.Process{PID: m.NewPID(), PPID: terminal.PID,
			PGID: terminal.PID, TTY: tty, Start: start, Args: args})
	}
}

// ssh is ssh's words as ps gives them, with args after its name.
func ssh(args ...string) []string { return append([]string{"ssh"}, args...) }

// loginOf is the pid of pane id's login in m: Ghostty's child on the pane's tty.
func loginOf(m *mac.Model, id string) int {
	tty := strings.TrimPrefix(m.Terminals[id].TTY, "/dev/")
	for _, p := range m.Processes {
		if p.TTY == tty && p.PPID == m.PID {
			return p.PID
		}
	}
	return 0
}
