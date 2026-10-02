package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zadykian/cld/internal/fail"
)

// launch is how create makes a session, as join and restore ask.
type launch struct {
	// worktree has claude work in git worktree cld-SUFFIX.
	worktree bool
	// resume is claude's --resume and what goes with it, and id the conversation's ID where it
	// came from the session's entry.
	resume []string
	id     string
	// args are the words for claude, which go after cld's own arguments.
	args []string
	// env is the environment of tmux, which its server and claude keep: where nil, cld's own
	// without the variables that name the terminal (see withoutTerminal).
	env []string
	// kept are the keys the tmux cld runs in keeps from claude, shown once attached (see showKept).
	kept []string
	// looked says that the caller has looked the session up under the record's lock, and found no
	// server, so create does not look again.
	looked bool
	// detached makes the session without a terminal, cld waiting for tmux instead of becoming its
	// client. restored brings it back as it ran, its run mark keeping its time (see setMarks).
	detached bool
	restored bool
	// from is the session a switch moved the terminal from, which the session records.
	from string
}

// creation is what create has worked out for tmux's command that makes session cld-SUFFIX. It
// holds the claude CheckClaude checked, claude's directory, the session's home (see Home) and the
// server's socket by an absolute path. It also holds the keys that move the terminal (see
// switchKeys), and the run mark that the pane-died hook removes (see died).
type creation struct {
	t       *Tmux
	claude  string
	suffix  string
	dir     string
	home    string
	socket  string
	keys    []string
	diedRun string
	l       launch
}

// create makes session cld-SUFFIX for join and restore, as l asks, under the record's lock, which
// the caller holds. It writes the entry and the environment once nothing is left to refuse it, and
// tmux sets the marks once it has made the session (decision 40). Attached, it leaves the start
// mark and becomes tmux's client (decision 50.4), and returns only when it does not get as far.
func (t *Tmux) create(c *Claude, suffix string, l launch) error {
	cr, plannedFile, err := t.prepare(c, suffix, l)
	if err != nil {
		return err
	}
	// The command is counted with the hooks and marks of the planned files, before the entry is
	// written: one too long leaves the record unchanged (decision 41.5). Where cld then cannot
	// write them, the command goes without them, and is only shorter.
	argv, size, err := cr.command(plannedFile, cr.diedRun)
	if err != nil {
		return err
	}
	if size > commandLimit {
		return fail.Usage(fmt.Sprintf("claude's arguments make tmux's command %d bytes, and tmux "+
			"takes %d at most: give claude long text in a file, as with --append-system-prompt-file",
			size, commandLimit))
	}
	if !cr.l.detached {
		if err := checkTerminal("join"); err != nil {
			return err
		}
	}
	env := cr.l.env
	if cr.l.detached {
		env = withTmuxTmpdir(env)
	}
	file, run := remember(entry{Name: suffix, Directory: cr.dir, Conversation: cr.l.id},
		started{Claude: c.path, Environment: cr.l.env}, t.runs)
	if file != plannedFile || run != cr.diedRun {
		if argv, _, err = cr.command(file, run); err != nil {
			return err
		}
	}
	if cr.l.detached {
		return t.runDetached(argv, env, cr.dir)
	}
	if err := printTitle(suffix); err != nil {
		return err
	}
	if file != "" {
		leaveStartMark(file)
	}
	// The server keeps the environment of the client that starts it, cld's (decision 13).
	return t.become(argv, env)
}

// prepare works out what create needs for session cld-SUFFIX, refusing what it refuses before
// tmux's command, and the file of the entry it plans to write. It empties TMUX first (see
// emptyTMUX).
func (t *Tmux) prepare(c *Claude, suffix string, l launch) (*creation, string, error) {
	if err := emptyTMUX(); err != nil {
		return nil, "", err
	}
	if l.env == nil {
		l.env = withoutTerminal(os.Environ())
	}
	if !l.looked {
		if err := t.occupied(context.Background(), suffix); err != nil {
			return nil, "", err
		}
	}
	dir, err := workingDirectory()
	if err != nil {
		return nil, "", err
	}
	_, home := DefaultName()
	if l.worktree {
		if err := workTree(); err != nil {
			return nil, "", err
		}
	}
	socket, err := filepath.Abs(filepath.Join(socketDir(), "cld-"+suffix))
	if err != nil {
		return nil, "", fail.Runtime(err.Error())
	}
	cr := &creation{t: t, claude: c.path, suffix: suffix, dir: dir, home: home.Dir, socket: socket,
		l: l}
	// The pane-died hook removes the run mark even where cld writes none: the session can have one
	// from before, a reboot's (decision 48.2).
	plannedFile := ""
	if state, err := stateDir(); err == nil {
		plannedFile, cr.diedRun = entryFile(state, suffix), companion(state, suffix, runMark)
	}
	cld, _ := self() //nolint:errcheck // without its file, tmux's own keys stay (see switchKeys)
	cr.keys = switchKeys(t.path, socket, cld)
	return cr, plannedFile, nil
}

// runs reports, for the record's expiry, whether the server of session cld-SUFFIX runs, or might.
func (t *Tmux) runs(suffix string) bool {
	server, _, _, _, err := t.lookup(context.Background(), suffix)
	return server || err != nil
}

// runDetached runs argv, tmux's command that makes a session without a terminal, with env in dir.
func (t *Tmux) runDetached(argv, env []string, dir string) error {
	// tmux's client returns once the server has run the command, the marks' run-shell included.
	// The server leaves cld's output as it starts, so nothing holds the output cld reads.
	cmd := exec.Command(t.path, argv[1:]...)
	cmd.Args[0] = "tmux"
	cmd.Env, cmd.Dir = env, dir
	if out, err := combinedOutput(cmd); err != nil {
		return fail.Runtime(out)
	}
	return nil
}
