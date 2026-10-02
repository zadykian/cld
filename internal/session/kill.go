package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"

	"github.com/zadykian/cld/internal/fail"
)

// Kill ends session cld-SUFFIX with its server, for cld kill: End, whatever its panes' pids, with
// tmux's messages on cld's stdout and stderr.
func (t *Tmux) Kill(suffix string, home Home) error {
	return t.End(context.Background(), suffix, home, nil, os.Stdout, os.Stderr)
}

// End is kill's steps after the name's check, which the list's Ctrl+X takes too (decision 15.4).
// It looks session cld-SUFFIX up, then ends it and its server in one tmux command, without
// waiting for claude (decisions 13 and 32). With pids, only a session that holds one of them counts
// (decision 15.3). A kill that fails is its exit status. Once ctx is done, its tmux is killed.
func (t *Tmux) End(ctx context.Context, suffix string, home Home, pids []string,
	stdout, stderr io.Writer) error {
	server, exists, found, made, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	var command []string
	switch {
	case !exists && server:
		if outlived, refused := t.lingering(ctx, suffix); !outlived {
			return refused
		}
		command = outlivedKill(suffix)
	case !exists:
		if err := ended(suffix); err != nil {
			return err
		}
		return noSession(suffix)
	case len(pids) > 0 && !holdsAny(found, pids):
		return noSession(suffix)
	default:
		command = sessionKill(suffix)
	}
	if err := foreign("kill", suffix, made, home); err != nil {
		return err
	}
	kill := t.serverContext(ctx, suffix, command...)
	kill.Stdout, kill.Stderr = stdout, stderr
	if err := kill.Run(); err != nil {
		return t.exitStatus(err)
	}
	return nil
}

// holdsAny reports whether found, the pids of a session's panes, holds any of pids.
func holdsAny(found, pids []string) bool {
	return slices.ContainsFunc(found, func(pid string) bool { return slices.Contains(pids, pid) })
}

// sessionKill is the tmux command that ends session cld-SUFFIX and then its server, so that a
// terminal attached exits with status 0 (decision 13). It first removes the run mark, while the
// session holds its name (decision 48.1).
func sessionKill(suffix string) []string {
	kill := []string{"kill-session", "-t", "=cld-" + suffix, ";", "kill-server"}
	if rm := unmark(suffix); rm != "" {
		return append([]string{"run-shell", rm, ";"}, kill...)
	}
	return kill
}

// outlivedKill is the tmux command that ends a server that has outlived session cld-SUFFIX, under
// lingering's check made again (see outlives). No pane of the session is left to check, nor its
// home (decision 15.3).
func outlivedKill(suffix string) []string {
	kill := "kill-server"
	if rm := unmark(suffix); rm != "" {
		kill = "run-shell " + shellWord(rm) + " ; kill-server"
	}
	return []string{"if", "-F", outlives(suffix), kill}
}

// Forget forgets session cld-SUFFIX, which has ended, for the list's Ctrl+X. Its entry goes with
// its environment and marks, but the index its name ends in stays given (decision 40). The caller
// holds the record's lock (decision 48.6), and a session another cld is starting is refused
// (decision 50.4). Once ctx is done, its tmux is killed.
func (t *Tmux) Forget(ctx context.Context, suffix string) error {
	starts := starting(suffix)
	_, exists, _, _, err := t.lookup(ctx, suffix)
	if err != nil {
		return err
	}
	if exists {
		return &fail.Error{Status: 1, Message: fmt.Sprintf("session '%s' runs again", suffix),
			Advice: " (see cld list)"}
	}
	if starts {
		return &fail.Error{Status: 1, Message: fmt.Sprintf("session '%s' is starting", suffix),
			Advice: " (see cld list)"}
	}
	if _, ok := recorded(suffix); !ok {
		return noSession(suffix)
	}
	dir, _ := stateDir() //nolint:errcheck // recorded found the entry there
	files := []string{entryFile(dir, suffix), companion(dir, suffix, runMark),
		companion(dir, suffix, busyMark), companion(dir, suffix, startMark),
		companion(dir, suffix, environment)}
	for _, file := range files {
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fail.Runtime("cannot forget session '" + suffix + "': " + reason(err).Error())
		}
	}
	return nil
}
