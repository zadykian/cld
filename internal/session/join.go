package session

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// Joining is what cld join asks of session cld-SUFFIX beyond a terminal on it (see Tmux.Join).
// Worktree, New, Conversation, Fork and Args go only to a session that join makes or brings back,
// and are refused where they would be lost (see lost).
type Joining struct {
	// Home takes a session that runs only where join made it (see foreign).
	Home Home
	// DetachOthers detaches the other terminals on the session.
	DetachOthers bool
	// Worktree has claude work in git worktree cld-SUFFIX, named as the session is.
	Worktree bool
	// New starts a new conversation where the session has ended, rather than resume its own.
	New bool
	// Conversation is what claude resumes, whatever claude --resume takes, and Fork resumes a copy
	// of it under a new ID instead (decision 45).
	Conversation string
	Fork         bool
	// Args, the words given after "--", go to claude after cld's own arguments.
	Args []string
	// SwitchedFrom is the session a switch moved the terminal from, which the session joined
	// records for C-q L (decision 51.4).
	SwitchedFrom string
	// Typed are the words given after join, which join passes on where it moves a terminal instead
	// of attaching this one (see Tmux.move).
	Typed []string
}

// launch is how create makes session cld-SUFFIX for a join that makes it, with kept, the keys the
// tmux cld runs in keeps from claude. claude starts a new conversation, or resumes Conversation,
// taking the session's name either way (decision 16.2).
func (j Joining) launch(kept []string) launch {
	l := launch{worktree: j.Worktree, args: j.Args, kept: kept, from: j.SwitchedFrom}
	if j.Conversation != "" {
		l.resume = []string{"--resume", j.Conversation}
		if j.Fork {
			l.resume = append(l.resume, "--fork-session")
		}
	}
	return l
}

// lost refuses the first of what j asks that would be lost on session cld-SUFFIX (decision 50.2).
// Those are -w, --new, --resume and the words for claude where it runs, and -w where it has ended.
// The advice for the command line is kept apart (fail.Error's Advice); nil where nothing is lost.
func (j Joining) lost(suffix string, ended bool) error {
	if ended {
		if !j.Worktree {
			return nil
		}
		return &fail.Error{Status: 1,
			Message: fmt.Sprintf("session '%s' has ended, and -w would be lost: "+
				"claude takes its conversation back to its worktree itself", suffix),
			Advice: "; resume it with cld join " + Options(suffix) +
				", or give --new for a new conversation"}
	}
	var option string
	switch {
	case j.Worktree:
		option = "-w"
	case j.New:
		option = "--new"
	case j.Conversation != "":
		option = "--resume"
	case len(j.Args) > 0:
		option = "the words after --"
	default:
		return nil
	}
	return &fail.Error{Status: 1,
		Message: fmt.Sprintf("session '%s' exists, and %s would be lost: its claude has started",
			suffix, option),
		Advice: "; attach to it with cld join " + Options(suffix) + ", or give another -s SUFFIX"}
}

// Create makes session cld-SUFFIX as j asks, for cld join without -s, and becomes a tmux client
// attached to it. The caller has readied the client, which gave kept and no terminal to move (see
// ReadyClient), and holds the record's lock. It returns only when it does not get as far.
func (t *Tmux) Create(c *Claude, suffix string, j Joining, kept []string) error {
	return t.create(c, suffix, j.launch(kept))
}

// Join is cld join with -s, once the caller has readied the client, which gave kept or sw. It
// attaches to session cld-SUFFIX where it runs, brings it back where it has ended, and makes it
// otherwise (decision 50.1). With sw, it moves that terminal instead (see switchJoin). It returns
// only when it does not get as far.
func (t *Tmux) Join(suffix string, j Joining, kept []string, sw *Switch) error {
	return t.join(suffix, j, kept, true, sw)
}

// JoinPicked is the interactive list's Enter on session cld-SUFFIX, once the list has handed the
// terminal over. It joins as Join does, but refuses a session that neither runs nor has ended, as
// the row picked has gone. It returns only when it does not get as far.
func (t *Tmux) JoinPicked(suffix string) error {
	return t.join(suffix, Joining{}, t.keptKeys(), false, nil)
}

// join is Join's and JoinPicked's work. With makes, a session that neither runs nor has ended is
// made, and otherwise refused; with sw, the terminal of sw moves to the session.
func (t *Tmux) join(suffix string, j Joining, kept []string, makes bool, sw *Switch) error {
	if sw != nil {
		return t.switchJoin(sw, suffix, j)
	}
	unlock, server, exists, made, err := t.settled(suffix)
	if err != nil || exists || server {
		unlock()
	}
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
		return t.attach(suffix, j, kept)
	case server:
		_, refused := t.lingering(context.Background(), suffix)
		return refused
	}
	defer unlock()
	r, ended := recorded(suffix)
	if !ended && !makes {
		return noSession(suffix)
	}
	l := j.launch(kept)
	l.looked = true
	if ended && !j.New && j.Conversation == "" {
		if err := j.lost(suffix, true); err != nil {
			return err
		}
		if err := enter(r); err != nil {
			return err
		}
		l = resumed(suffix, l)
	}
	c, err := CheckClaude()
	if err != nil {
		return err
	}
	return t.create(c, suffix, l)
}

// settled takes the record's lock and looks session cld-SUFFIX up under it, returning what lets
// the lock go. It waits lockWait at most for a session that another cld is starting, so that two
// joins make it once. The start mark is read before the lookup (decision 50.4).
func (t *Tmux) settled(suffix string) (unlock func(), server, exists bool, made string, err error) {
	deadline := time.Now().Add(lockWait)
	for {
		unlock = Lock()
		wait := starting(suffix)
		server, exists, _, made, err = t.lookup(context.Background(), suffix)
		if err != nil || exists || time.Now().After(deadline) || !wait {
			return unlock, server, exists, made, err
		}
		unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

// resumed is l with claude resuming the conversation of session cld-SUFFIX, which has ended. claude
// resumes the one its entry names by ID, or else the one named cld-SUFFIX (decision 40.4). join
// and restore resume it so.
func resumed(suffix string, l launch) launch {
	l.resume, l.id = []string{"--resume", "cld-" + suffix}, ""
	if r, ok := recorded(suffix); ok && r.Conversation != "" {
		l.resume, l.id = []string{"--resume", r.Conversation}, r.Conversation
	}
	return l
}

// attach is join's last step for a session that runs. Once TMUX is emptied and the terminal
// checked, it becomes a tmux client attached to session cld-SUFFIX, and shows the keys kept (see
// showKept). It returns only when it does not get as far.
func (t *Tmux) attach(suffix string, j Joining, kept []string) error {
	if err := emptyTMUX(); err != nil {
		return err
	}
	if err := checkTerminal("join"); err != nil {
		return err
	}
	name := "cld-" + suffix
	if err := printTitle(suffix); err != nil {
		return err
	}
	// -u takes the terminal for UTF-8 whatever the locale, as create's client does.
	attach := []string{"tmux", "-u", "-L", name, "attach-session"}
	if j.DetachOthers {
		attach = append(attach, "-d")
	}
	attach = append(attach, "-t", "="+name)
	// What follows attach-session, which tmux skips where the attach fails, goes to this terminal,
	// attached by then. A claude that exited shows the hint in place of the keys.
	if j.SwitchedFrom != "" && j.SwitchedFrom != suffix {
		attach = append(attach, ";", "set", "-t", "="+name+":", "@cld-last", j.SwitchedFrom)
	}
	attach = append(attach, showKept(kept)...)
	return t.become(append(attach, ";", "if", "-F", "#{pane_dead}", hint(suffix)), os.Environ())
}
