package session

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/tool"
)

// ReadyClient readies cld to become a tmux client, as join's first step once tmux is checked. In
// a pane of one of cld's servers it returns the terminal to move instead (decision 51.5), and
// elsewhere the keys another tmux keeps from claude. TMUX stays until create or attach empty it,
// so that the sweep keeps the session cld runs in (decision 50.3).
func (t *Tmux) ReadyClient() ([]string, *Switch) {
	if sw := t.Switching(); sw != nil {
		return nil, sw
	}
	return t.keptKeys(), nil
}

// emptyTMUX empties a TMUX that is set, since tmux would refuse a client on a pty named as a dead
// pane's. An empty TMUX skips that check, the terminal still taken for UTF-8 (decision 2).
func emptyTMUX() error {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	return os.Setenv("TMUX", "")
}

// OwnPane reports whether this terminal is a live pane of one of cld's servers, claude's external
// editor say, and names the server's session. There join and the list move the terminal instead
// (decision 2). It looks only on the server TMUX names, where that has cld's mark.
func (t *Tmux) OwnPane() (string, bool) {
	socket, suffix, found := ownServer()
	if !found {
		return "", false
	}
	terminal, ok := ttyName()
	if !ok {
		return "", false
	}
	panes := t.command("-S", socket, "list-panes", "-a", "-F",
		"#{?"+mark+",#{?pane_dead,,#{pane_tty}},}")
	panes.Stderr = nil
	live, err := panes.Output()
	if err != nil {
		return "", false
	}
	return suffix, slices.Contains(strings.Split(strings.TrimRight(string(live), "\n"), "\n"),
		terminal)
}

// ttyName names the terminal on cld's stdin, as tty prints it, and whether there is one. uutils'
// tty prints no newline, so the name is trimmed rather than cut (docs/design/overview.md).
func ttyName() (string, bool) {
	tty, err := tool.Command("tty")
	if err != nil {
		return "", false
	}
	terminal, err := tty.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(terminal), "\n"), true
}

// keptKeys names the keys that the tmux this terminal is a pane of keeps from claude (decision
// 43.1): its prefixes, and Shift+Enter unless it passes modified keys on. It names them only where
// TMUX names a tmux not cld's, and cld's tmux is 3.6 or newer. A tmux that does not answer, or
// finds another pane, keeps nothing cld knows of.
func (t *Tmux) keptKeys() []string {
	socket, _, _ := strings.Cut(os.Getenv("TMUX"), ",")
	if socket == "" || t.older(version{3, 6, 0}) {
		return nil
	}
	terminal, ok := ttyName()
	if !ok {
		return nil
	}
	options := t.command("-S", socket, "display", "-p",
		"#{pane_tty}\t#{extended-keys}\t#{prefix}\t#{prefix2}\t#{client_termfeatures}\t"+mark)
	options.Stderr = nil
	out, err := options.Output()
	if err != nil {
		return nil
	}
	field := strings.Split(strings.TrimRight(string(out), "\n"), "\t")
	if len(field) != 6 || field[0] != terminal {
		return nil
	}
	suffix, found := strings.CutPrefix(filepath.Base(socket), "cld-")
	if found && ValidName(suffix) && field[5] == "1" {
		return nil
	}
	return t.keysKept(field[1], field[2:4], field[4])
}

// keysKept are the keys a tmux keeps from claude by its extended-keys, its prefixes and the
// features of its client. It keeps Shift+Enter unless it passes modified keys on to cld's tmux.
func (t *Tmux) keysKept(extended string, prefixes []string, features string) []string {
	var kept []string
	for _, prefix := range prefixes {
		if prefix != "" && prefix != "None" && !slices.Contains(kept, prefix) {
			kept = append(kept, prefix)
		}
	}
	passed := extended == "always" || extended == "on" && !t.older(version{3, 7, 0})
	if !passed || !slices.Contains(strings.Split(features, ","), "extkeys") {
		kept = append(kept, "Shift+Enter")
	}
	return kept
}

// showKept is the tmux commands that show the keys kept on the message line of the terminal just
// attached, after a ";", within 80 columns; none where no key is kept. -C keeps claude drawn, and
// two redraws show the line again once claude has drawn over it (decisions 43.3 and 43.4).
func showKept(kept []string) []string {
	if len(kept) == 0 {
		return nil
	}
	keys := kept[len(kept)-1]
	if len(kept) > 1 {
		keys = strings.Join(kept[:len(kept)-1], ", ") + " and " + keys
	}
	line := "your tmux keeps " + keys + `: see "Inside your own tmux" in cld's guide`
	if len(line) > 80 {
		line = "your tmux keeps " + keys + ": see cld's guide"
	}
	return []string{";", "display", "-l", "-C", "-d", "0", line,
		";", "run-shell", "-b", "-C", "-d", "1", "refresh-client",
		";", "run-shell", "-b", "-C", "-d", "3", "refresh-client"}
}

// ownServer is the socket TMUX names and the NAME of its server, cld-NAME, where the socket is
// named like one of cld's.
func ownServer() (socket, suffix string, found bool) {
	socket, _, _ = strings.Cut(os.Getenv("TMUX"), ",")
	suffix, found = strings.CutPrefix(filepath.Base(socket), "cld-")
	if !found || !ValidName(suffix) {
		return "", "", false
	}
	return socket, suffix, true
}
