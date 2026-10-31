package tabs

import (
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/ghostty"
)

// Seen is what a pane shows at an observation (decision 59.5).
type Seen struct {
	Kind  string
	Title string
	Login int
	Keep  bool // ps cannot tell: keep the earlier observation
}

// classify reads what terminal t shows from ps's rows, Ghostty's pid being g. The leader of the
// pane's foreground group tells it: ssh, a shell at its prompt, or another program.
func classify(t ghostty.Terminal, procs Table, g int) Seen {
	if t.PID == 0 || t.TTY == "" {
		return Seen{Kind: KindDead, Title: t.Name}
	}
	tty := strings.TrimPrefix(t.TTY, "/dev/")
	login := procs.Login(tty, g)
	// UNVERIFIED until #177's check 9 (decision 59.12). pid is the foreground group's, ssh leading
	// it where ssh runs, and the pane's login is Ghostty's child.
	leader, ok := procs.ByPID(t.PID)
	if !ok || leader.TTY != tty {
		return Seen{Keep: true, Title: t.Name, Login: login}
	}
	seen := front(leader)
	seen.Title, seen.Login = t.Name, login
	return seen
}

// front is what the leader of a pane's foreground group shows.
func front(leader Process) Seen {
	switch name := program(leader); {
	case name == "ssh":
		return Seen{Kind: KindSSH}
	case isShell(name):
		return Seen{Kind: KindLocal}
	}
	return Seen{Kind: KindOther}
}

// shells are the programs whose pane is at its prompt.
var shells = []string{"sh", "bash", "zsh", "fish", "ksh", "dash", "tcsh", "csh", "nu"}

// isShell reports whether the program named name is a shell.
func isShell(name string) bool { return slices.Contains(shells, name) }
