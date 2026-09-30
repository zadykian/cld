package session

// cld restore brings back the sessions that a reboot ended: the entries of cld's record with a run
// mark and no server (see the top of record.go). A reboot ends every tmux server, and each claude
// with its session, but neither the conversations nor cld's record: a session that ran then keeps
// its run mark, where kill, the interactive list's Ctrl+X, the sweep of the idle sessions and
// claude's own exit with status 0 remove it (see unmark and died). The user's systemd runs
// restore as it starts, where cld setup restore set that up (see internal/restore).

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// ContinuePrompt is the prompt restore gives the claude of a session whose busy mark says it was
// in a turn when the machine stopped: claude resumes the conversation and submits the prompt as
// its first turn there (claude 2.1.232 and 2.1.285, read; see Findings in docs/design.md). claude
// asks the permissions it asks for as in any turn.
const ContinuePrompt = "The machine restarted while you were working; continue where you left off."

// Marked are the NAMEs of the sessions of cld's record that have a run mark, in the order of their
// names: those that run, and those that ran when the machine stopped, which restore brings back.
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

// Restored is what Restore did with a session: where Idle is 0, it brought the session back, its
// claude running in Directory, and continuing the turn it was in where Busy (see ContinuePrompt);
// otherwise it left the session ended, idle for that long.
type Restored struct {
	Directory string
	Busy      bool
	Idle      time.Duration
}

// Restore brings session cld-SUFFIX back, for cld restore, where its entry in cld's record has its
// run mark and no server runs as cld-SUFFIX, and returns what it did; nil where it left the
// session as it is: one that runs, or whose server does, or that another cld is starting (see
// starting), or that has lost its entry or its mark meanwhile - to kill, say, or to the list's
// forget. A session idle for longer than limit, where that is not 0, by its run mark - the time
// the mark was made, as join made the session, or last touched, as claude took a prompt (see
// recordHooks) - it leaves ended, removing the mark: the sweep of the idle sessions goes by tmux's
// times, which a reboot takes with the server, and the new server's would count the session as
// used at every restore (see EndIdle). Otherwise it makes the session as join -n NAME -s SUFFIX
// would bring it back, but detached: tmux makes the session with new-session -d, on no terminal,
// and cld waits for tmux, which returns once the session is made, instead of becoming it; claude
// resumes the conversation of the session's entry by its ID, or else the one named cld-SUFFIX (see
// resumed), in the directory of the entry (see enter), with the claude and the environment the
// session's server started with (see started) - but for TMUX_TMPDIR, which is cld's own: the
// server's socket is where cld looks for it, and where the hooks reach it - and, for a session
// whose busy mark is there, ContinuePrompt after --resume; the words given to claude after "--"
// are not given again, as with join. The run mark keeps its time, so that a session restore alone
// keeps bringing back ends all the same (see setMarks). claude is checked as join checks it (see
// CheckClaude), but in the session's directory and environment, where it starts. The caller holds
// the record's lock (see Lock), so that another restore, or a join of the session, at once makes
// no second one. What refuses the session is an error - no environment recorded, its directory
// gone or closed (see enterable), a claude that is too old or does not run, a name tmux refuses,
// tmux's own failure, with its message - and restore goes on with the others.
func (t *Tmux) Restore(suffix string, limit time.Duration) (*Restored, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, nil
	}
	r, ok := recorded(suffix)
	mark, err := os.Stat(companion(dir, suffix, runMark))
	if !ok || err != nil {
		return nil, nil
	}
	// The start mark before the lookup, as join reads it (see settled): one gone since was removed
	// by a tmux that had made the session by then, which the lookup finds.
	if starting(suffix) {
		return nil, nil
	}
	server, _, _, _, err := t.lookup(context.Background(), suffix)
	if err != nil || server {
		return nil, err
	}
	if idle := time.Since(mark.ModTime()); limit > 0 && idle > limit {
		// A mark cld cannot remove stays, and the next restore says the same.
		_ = os.Remove(companion(dir, suffix, runMark))
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
	env := slices.DeleteFunc(slices.Clone(s.Environment), func(variable string) bool { return strings.HasPrefix(variable, "PWD=") })
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
