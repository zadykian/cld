package session

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// marked reports whether session cld-SUFFIX has its run mark in the record's directory dir.
func marked(dir, suffix string) bool {
	_, err := os.Stat(companion(dir, suffix, runMark))
	return err == nil
}

// leaveStartMark writes the start mark beside the entry in file of the session that cld, attached,
// is about to have tmux make: cld's process ID, which the tmux client it becomes keeps (decision
// 50.4). One cld cannot write is none, as the record serves the sessions (decision 40.7).
func leaveStartMark(file string) {
	mark := strings.TrimSuffix(file, ".json") + startMark
	_ = replace(mark, []byte(strconv.Itoa(os.Getpid())+"\n")) //nolint:errcheck // explained above
}

// starting reports whether another cld is starting session cld-SUFFIX: its start mark is younger
// than lockWait and names a process that runs, as any answer of kill but ESRCH says (decision
// 50.4). One older, as a failed new-session can leave, may name another process by now.
func starting(suffix string) bool {
	dir, err := stateDir()
	if err != nil {
		return false
	}
	file := companion(dir, suffix, startMark)
	info, err := os.Stat(file)
	if err != nil || time.Since(info.ModTime()) > lockWait {
		return false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	return !errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}

// setMarks is the run-shell, as tmux's words, that follows new-session to set the marks beside
// the entry in file (decision 48.1). It removes the busy and start marks, and makes the run mark
// run unless that is "", as restore passes to keep the mark's time (decision 48.8). It prints
// nothing and exits 0, as tmux would show anything else (docs/design/findings/tmux-sessions.md).
func setMarks(file, run string) []string {
	base := strings.TrimSuffix(file, ".json")
	sh := `rm -f ` + shellWord(base+busyMark) + ` ` + shellWord(base+startMark)
	if run != "" {
		sh += `; touch ` + shellWord(run)
	}
	return []string{"run-shell", unexpanded(`{ ` + sh + `; } 2>/dev/null || true`)}
}

// unmark is the sh command, run-shell's argument, that removes session cld-SUFFIX's run mark in
// the command that ends the session, before kill-session (decision 48.1); "" where cld has no
// record's directory. It prints nothing and exits 0, as the session ends all the same, and has
// every "#" doubled, as run-shell expands a format.
func unmark(suffix string) string {
	dir, err := stateDir()
	if err != nil {
		return ""
	}
	return unexpanded(`rm -f ` + shellWord(companion(dir, suffix, runMark)) + ` 2>/dev/null || true`)
}
