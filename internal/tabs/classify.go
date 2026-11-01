package tabs

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/ghostty"
	"github.com/zadykian/cld/internal/remote"
	"github.com/zadykian/cld/internal/session"
)

// Seen is what a pane shows at an observation (decision 59.5).
type Seen struct {
	Kind, Host, Session string
	Worktree            bool
	Title               string // busy markers as ✳
	Login               int
	Keep                bool // ps cannot tell: keep the earlier observation
}

// classify reads what terminal t shows from ps's rows, Ghostty's pid being g. The leader of the
// pane's foreground group tells it: ssh, directly or under a wrapper, a shell at its prompt, or
// another program. An ssh pane is cld's where its title is cld's. A title counts only while ssh
// is in front, so that a shell keeps no stale title's session.
func classify(t ghostty.Terminal, procs Table, g int) Seen {
	title := normalTitle(t.Name)
	if t.PID == 0 || t.TTY == "" {
		return Seen{Kind: KindDead, Title: title}
	}
	tty := strings.TrimPrefix(t.TTY, "/dev/")
	login := procs.Login(tty, g)
	// pid is the foreground group's, ssh leading it where ssh runs, and the pane's login is
	// Ghostty's child.
	leader, ok := procs.ByPID(t.PID)
	if !ok || leader.TTY != tty {
		return Seen{Keep: true, Title: title, Login: login}
	}
	seen := front(leader, procs.Group(t.PID, tty))
	seen.Title, seen.Login = title, login
	if seen.Kind == KindSSH && seen.Host != "" {
		if name, worktree, ok := titleSession(t.Name); ok {
			seen.Kind, seen.Session, seen.Worktree = KindCld, name, worktree
		}
	}
	return seen
}

// front is what the leader of a pane's foreground group shows, group being its members. cld in
// front is a restore in its own pane, before its exec of ssh, or another of cld's commands: the
// pane keeps what it showed. A wrapper's ssh is its child, and only that ssh counts: one deeper
// in the group, such as a git push's, does not.
func front(leader Process, group []Process) Seen {
	name := program(leader)
	switch {
	case name == "cld":
		return Seen{Keep: true}
	case name == "ssh":
		return sshSeen(leader.Args[1:])
	case wraps(leader, name):
		for _, p := range group {
			if p.PPID == leader.PID && program(p) == "ssh" {
				return sshSeen(p.Args[1:])
			}
		}
		return Seen{Keep: true}
	case isShell(name):
		return Seen{Kind: KindLocal}
	}
	return Seen{Kind: KindOther}
}

// wraps reports whether the leader named name runs ssh as its child: Ghostty's ssh wrapper,
// ghostty +ssh, or ws's ws-join, a sh script. The first has the shape macOS's ps shows; the
// second's is UNVERIFIED until #177's check 9 (decision 59.12).
func wraps(leader Process, name string) bool {
	switch name {
	case "ghostty":
		return len(leader.Args) > 1 && leader.Args[1] == "+ssh"
	case "ws-join":
		return true
	}
	return isShell(name) && len(leader.Args) > 1 && path.Base(leader.Args[1]) == "ws-join"
}

// sshSeen is an ssh pane, with ssh's words after its name. A host --host would refuse is no host:
// a restore types it unchanged, and never sanitises one.
func sshSeen(args []string) Seen {
	host, ok := sshHost(args)
	if !ok || !remote.ValidHost(host) {
		return Seen{Kind: KindSSH}
	}
	return Seen{Kind: KindSSH, Host: host}
}

// sshOptions are the options of ssh(1) that take a value.
const sshOptions = "BbcDEeFIiJLlmOoPpQRSWw"

// sshHost is ssh's destination: the first word that is no option, past the values of the options
// that take one. "--" ends the options. The words after it, a remote command, do not count.
func sshHost(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		word := args[i]
		switch {
		case word == "--":
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		case len(word) > 1 && word[0] == '-':
			if valueNext(word[1:]) {
				i++
			}
		default:
			return word, true
		}
	}
	return "", false
}

// valueNext reports whether the letters of an option word end in an option whose value is the
// next word. An option that takes a value ends the word: what follows it in the word is its value.
func valueNext(letters string) bool {
	for i, letter := range letters {
		if strings.ContainsRune(sshOptions, letter) {
			return i == len(letters)-1
		}
	}
	return false
}

// titleRE matches cld's title: a marker word, cld-NAME, then " [w]" in a worktree (decisions 25
// and 26). Any single marker word matches, so that a new marker breaks nothing, and any word as
// NAME, which session.ValidName then judges.
var titleRE = regexp.MustCompile(`^\S+ cld-(\S+)( \[w\])?$`)

// titleSession is the session cld's title names, and whether in a worktree. It reports false for
// any other title, and for a NAME that session.ValidName refuses.
func titleSession(title string) (name string, worktree, ok bool) {
	match := titleRE.FindStringSubmatch(title)
	if match == nil || !session.ValidName(match[1]) {
		return "", false, false
	}
	return match[1], match[2] != "", true
}

// normalTitle is title with a busy marker at its start turned into ✳. The markers turn every
// second, and would otherwise write the record again at every tick.
func normalTitle(title string) string {
	for _, marker := range []string{session.BusyEven, session.BusyOdd} {
		if rest, ok := strings.CutPrefix(title, marker); ok {
			return "✳" + rest
		}
	}
	return title
}

// shells are the programs whose pane is at its prompt.
var shells = []string{"sh", "bash", "zsh", "fish", "ksh", "dash", "tcsh", "csh", "nu"}

// isShell reports whether the program named name is a shell.
func isShell(name string) bool { return slices.Contains(shells, name) }
