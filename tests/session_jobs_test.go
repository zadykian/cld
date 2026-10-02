package tests

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// program is the name of the program process pid runs: from /proc where there is one, which is
// quicker than ps. A tmux client may name itself "tmux: client" there (prctl in tmux's
// setproctitle, where the system has no setproctitle of its own).
func program(pid string) string {
	if name, err := os.ReadFile("/proc/" + pid + "/comm"); err == nil {
		return strings.TrimSpace(string(name))
	}
	name, err := exec.Command("ps", "-o", "comm=", "-p", pid).Output()
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimSpace(string(name)))
}

// isStopped reports whether process pid is stopped, every thread of it: from /proc where there is
// one, or else from ps.
func isStopped(pid int) bool {
	tasks, err := filepath.Glob("/proc/" + strconv.Itoa(pid) + "/task/*/stat")
	if err == nil && len(tasks) > 0 {
		return allStopped(tasks)
	}
	state, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(state)), "T")
}

// allStopped reports whether every task whose stat file tasks lists is stopped.
func allStopped(tasks []string) bool {
	for _, task := range tasks {
		stat, err := os.ReadFile(task)
		if err != nil {
			return false
		}
		// The state follows the program's name, which is in parentheses and may hold any.
		fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
		if len(fields) == 0 || fields[0] != "T" {
			return false
		}
	}
	return true
}

// gone reports whether process pid has ended and been waited for.
func gone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// pidScript is listScript that also records cld's pid: the inner sh writes its own, which exec
// hands on to cld.
var pidScript = strings.Replace(listScript, `"$@";`,
	`sh -c 'echo $$ >"$0.pid" && exec "$@"' "$0" "$@";`, 1)

// jobScript is pidScript run as a job of a shell with job control (see startJob). As cld stops, it
// records cld's status (stopped) and the terminal's mode (during), and writes a line of its own.
// Once the file go exists, it puts its own mode back, as bash does and dash does not, and cld back
// in the foreground (fg).
const jobScript = `tty >"$0.tty"; stty -g >"$0.before"; ` +
	`sh -c 'echo $$ >"$0.pid" && exec "$@"' "$0" "$@"; ` +
	`echo $? >"$0.stopped"; stty -g >"$0.during"; echo "the shell's line"; ` +
	`until [ -e "$0.go" ]; do sleep 0.05; done; stty "$(cat "$0.before")"; ` +
	`fg >/dev/null; echo $? >"$0.code"; stty -g >"$0.after"`

// startJob runs jobScript in term under an interactive sh (-i), which has job control. On macOS,
// sh hears of a job that stops only when interactive (docs/design/findings/environment.md). There,
// bash puts its own mode back as the job stops, before the script reads it (during).
func startJob(t *testing.T, s *sandbox.Sandbox, term terminal.Terminal) listRun {
	t.Helper()
	return startListIn(t, s, term, []string{"sh", "-i"}, jobScript, nil)
}
