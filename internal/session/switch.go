package session

// A terminal on one of cld's sessions moves to another without leaving claude (decision 51). The
// keys C-q s, C-q (, C-q ) and C-q L move it, and so does cld join in a pane of the session. Each
// session has a server of its own, so detach-client -E has the terminal's client run cld join in
// its place (decision 51.3). switch_*.go hold the keys, the steps, the move and its command.

import (
	"context"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/zadykian/cld/internal/fail"
)

// Switch is a terminal on a session of one of cld's servers, which cld moves to another session.
// It has the server's socket, as TMUX names it, and the session's NAME. client is the terminal's
// tmux client where a key named it, and "" where tmux takes the terminal (see DetachTerminal).
type Switch struct {
	socket, suffix, client string
}

// Switching is the terminal that join and the interactive list move where cld runs in a pane of
// one of cld's servers, and nil elsewhere (decision 51.5). A terminal on stdin has to be a live
// pane there (see OwnPane). With none, as under claude's !, the server TMUX names needs cld's mark.
func (t *Tmux) Switching() *Switch {
	socket, suffix, found := ownServer()
	if !found {
		return nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if _, own := t.OwnPane(); !own {
			return nil
		}
		return &Switch{socket: socket, suffix: suffix}
	}
	ask := t.command("-S", socket, "display-message", "-p", mark)
	ask.Stderr = nil
	marked, err := ask.Output()
	if err != nil || strings.TrimSuffix(string(marked), "\n") != "1" {
		return nil
	}
	return &Switch{socket: socket, suffix: suffix}
}

// SwitchClient is the terminal of the tmux client named client on the server TMUX names, which the
// keys hand cld list --switch (see switchKeys). Their popup and run-shell have TMUX and no
// TMUX_PANE (docs/design/findings/tmux-terminal.md).
func SwitchClient(client string) (*Switch, error) {
	socket, suffix, found := ownServer()
	if !found {
		return nil, fail.Runtime(
			"list --switch moves a terminal on one of cld's sessions, and TMUX names none")
	}
	return &Switch{socket: socket, suffix: suffix, client: client}, nil
}

// SwitchJoin is join without -s in a pane of one of cld's servers. It moves the terminal there to
// a new session, which the terminal's cld join makes under the next index. Only -w outside a git
// work tree is refused first (decision 51.5).
func (t *Tmux) SwitchJoin(sw *Switch, j Joining) error {
	if j.Worktree {
		if err := workTree(); err != nil {
			return err
		}
	}
	return t.move(sw, j)
}

// switchJoin is join -s in a pane of one of cld's servers. It moves the terminal there to session
// cld-SUFFIX, once it has refused what join refuses before claude starts (decision 51.5). Its
// lookup takes no lock, and leaves a session another cld is starting to the terminal's cld join.
func (t *Tmux) switchJoin(sw *Switch, suffix string, j Joining) error {
	if starting(suffix) {
		return t.move(sw, j)
	}
	server, exists, _, made, err := t.lookup(context.Background(), suffix)
	switch {
	case err != nil:
		return err
	case exists:
		if err := foreign("join", suffix, made, j.Home); err != nil {
			return err
		}
		if err := j.lost(suffix, false); err != nil {
			return err
		}
	case server:
		_, refused := t.lingering(context.Background(), suffix)
		return refused
	default:
		if r, ended := recorded(suffix); ended && !j.New && j.Conversation == "" {
			if err := j.lost(suffix, true); err != nil {
				return err
			}
			if err := enterable(r); err != nil {
				return err
			}
		} else if j.Worktree {
			if err := workTree(); err != nil {
				return err
			}
		}
	}
	return t.move(sw, j)
}

// move moves the terminal of sw as a join with the words Typed asks, run in a pane of sw's server.
// The terminal's cld join --moved runs those words in the directory this join runs in, which has
// to be there (decision 51.3).
func (t *Tmux) move(sw *Switch, j Joining) error {
	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	return t.switchTerminal(sw, "--moved="+encodeMove(dir, j.Typed))
}

// SwitchTo moves the terminal of sw to session cld-SUFFIX, which runs or has ended, for the list's
// Enter and for C-q (, C-q ) and C-q L. The terminal's cld join names it by -n and -s, or with no
// split by -s in /, where NAME's default is empty (decisions 24 and 51.7). The terminal's own
// session moves nothing.
func (t *Tmux) SwitchTo(sw *Switch, suffix string) error {
	if suffix == sw.suffix {
		return nil
	}
	if name, index, ok := split(suffix); ok {
		return t.switchTerminal(sw, "-n", name, "-s", index)
	}
	return t.switchTerminal(sw, "--moved="+encodeMove("/", []string{"-s", suffix}))
}

// workTree refuses, for -w, a current directory in no git work tree, as create does (decision 4).
// cld reports it in the terminal, where claude would report it in a session left to kill.
func workTree() error {
	if inWorkTree() {
		return nil
	}
	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	return fail.Runtime("--worktree needs a git repository, and " + dir + " is not in one")
}

// split splits session NAME into -n's NAME and -s's SUFFIX at its last "-", where both are NAMEs,
// as cld's messages name a session (see Options). The bool is false where NAME has no such split.
func split(name string) (string, string, bool) {
	if i := strings.LastIndexByte(name, '-'); i > 0 && ValidName(name[:i]) && ValidName(name[i+1:]) {
		return name[:i], name[i+1:], true
	}
	return "", "", false
}
