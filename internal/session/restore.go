package session

// cld restore brings back the sessions a reboot ended, the record's entries with a run mark and no
// server (see the top of record.go; decision 48). A reboot leaves the mark, which kill, the list's
// Ctrl+X, the idle sweep and claude's exit with status 0 remove (see unmark and died). cld setup
// restore has the user's systemd run restore at startup (see internal/restore).

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// ContinuePrompt is the prompt restore gives claude where the busy mark shows a turn that the
// machine's stop cut off (decision 48.7). claude resumes the conversation and submits the prompt
// as its first turn there (claude 2.1.232 and 2.1.285, read; see docs/design/findings/claude.md).
// claude asks the permissions it asks for as in any turn.
const ContinuePrompt = "The machine restarted while you were working; continue where you left off."

// Marked are the NAMEs of the sessions of cld's record that have a run mark, in the order of their
// file names. Those are the sessions that run, and those that ran when the machine stopped, which
// restore brings back.
func Marked() []string {
	dir, err := stateDir()
	if err != nil {
		return nil
	}
	var names []string
	for _, r := range entries() {
		if marked(dir, r.Name) {
			names = append(names, r.Name)
		}
	}
	return names
}

// Restored is what Restore did with a session. Where Idle is 0, it brought the session back, its
// claude running in Directory, and continuing an unfinished turn where Busy (see ContinuePrompt).
// Otherwise it left the session ended, idle for that long.
type Restored struct {
	Directory string
	Busy      bool
	Idle      time.Duration
}

// Restore brings session cld-SUFFIX back, detached, as join brings back an ended one (decision
// 48.5). It returns nil for a session that runs, or whose server does, that a join is starting,
// or that has lost its entry or mark. Where limit is not 0, it leaves ended a session whose run
// mark is older than limit (decision 48.8). The caller holds the record's lock (decision 48.6).
func (t *Tmux) Restore(suffix string, limit time.Duration) (*Restored, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	r, ok := recorded(suffix)
	mark, err := os.Stat(companion(dir, suffix, runMark))
	if !ok || err != nil {
		return nil, nil //nolint:nilerr // an entry or mark it cannot read is none, as in Marked
	}
	// The start mark is read before the lookup, as join reads it (see settled). A tmux that removed
	// it since had made the session by then, which the lookup finds.
	if starting(suffix) {
		return nil, nil
	}
	server, _, _, _, err := t.lookup(context.Background(), suffix)
	if err != nil || server {
		return nil, err
	}
	if idle := time.Since(mark.ModTime()); limit > 0 && idle > limit {
		// A mark cld cannot remove stays, and the next restore says the same.
		_ = os.Remove(companion(dir, suffix, runMark)) //nolint:errcheck // explained above
		return &Restored{Idle: idle}, nil
	}
	s, err := readStarted(dir, suffix)
	if err != nil {
		return nil, fail.Runtime("cld keeps no environment of it: " + err.Error())
	}
	// As join does: the directory is where tmux starts claude, and where the session's home comes
	// from (see DefaultName).
	if err := enter(r); err != nil {
		return nil, err
	}
	env := slices.DeleteFunc(slices.Clone(s.Environment), func(variable string) bool {
		return strings.HasPrefix(variable, "PWD=")
	})
	env = append(env, "PWD="+r.Directory)
	c, err := checkClaude(s.Claude, r.Directory, env)
	if err != nil {
		return nil, err
	}
	restored := &Restored{Directory: r.Directory}
	l := launch{env: env, detached: true, restored: true}
	if _, err := os.Stat(companion(dir, suffix, busyMark)); err == nil {
		restored.Busy, l.args = true, []string{ContinuePrompt}
	}
	if err := t.create(c, suffix, resumed(suffix, l)); err != nil {
		return nil, err
	}
	return restored, nil
}
