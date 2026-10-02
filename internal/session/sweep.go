package session

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// EndIdle ends session cld-SUFFIX with its server, and its run mark, where still idle for longer
// than limit, for the sweep of list and join. tmux checks again in the same command, before
// and after the mark's rm, so that a terminal attaching meanwhile keeps the session (decision
// 46.4). It reports whether it ended the session. Once ctx is done, its tmux is killed.
func (t *Tmux) EndIdle(ctx context.Context, suffix string, limit time.Duration) (bool, error) {
	idle := idleFormat(time.Now().Add(-limit))
	name := "=cld-" + suffix
	kill := "kill-session -t " + name + " ; kill-server"
	if rm := unmark(suffix); rm != "" {
		kill = "run-shell " + shellWord(rm) + " ; if -F -t " + name + ": " + shellWord(idle) + " " +
			shellWord(kill) + " 'display-message -p kept'"
	}
	out, err := combinedOutput(t.serverContext(ctx, suffix, "if", "-F", "-t", name+":", idle, kill,
		"display-message -p kept"))
	switch {
	case err != nil && noServer(out):
		return false, nil
	case err != nil:
		return false, fail.Runtime(out)
	}
	if out != "" {
		return false, nil
	}
	return true, nil
}

// idleFormat is a format that tmux makes 1 for a session with no terminal attached, and neither a
// key nor an attach from since on (see Session's Idle).
func idleFormat(since time.Time) string {
	// tmux compares whole seconds, so a time with a fraction counts from its next whole second.
	cutoff := since.Unix()
	if since.Nanosecond() > 0 {
		cutoff++
	}
	before := func(format string) string {
		return "#{e|<:#{" + format + "}," + strconv.FormatInt(cutoff, 10) + "}"
	}
	return "#{&&:#{==:#{session_attached},0},#{&&:" + before("session_activity") + "," +
		before("session_last_attached") + "}}"
}

// OwnServer names the session whose server cld runs on, where TMUX names the socket of one of
// cld's servers, which the sweep passes over (decision 46.2). It compares the socket as a file, as
// TMUX names it with symbolic links resolved. Unlike OwnPane it needs no terminal.
func OwnServer() (string, bool) {
	socket, suffix, found := ownServer()
	if !found {
		return "", false
	}
	own, err := os.Stat(socket)
	if err != nil {
		return "", false
	}
	listed, err := os.Stat(filepath.Join(socketDir(), "cld-"+suffix))
	if err != nil || !os.SameFile(own, listed) {
		return "", false
	}
	return suffix, true
}
