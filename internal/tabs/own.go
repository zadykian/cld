package tabs

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

// ownDevice is the device of cld's stdin, where that is a terminal: the pane cld runs in, which a
// restore never types into (decision 59.9). The device number matches on any system, without
// running tty.
func ownDevice() (uint64, bool) {
	if !term.IsTerminal(0) {
		return 0, false
	}
	var st unix.Stat_t
	if unix.Fstat(0, &st) != nil {
		return 0, false
	}
	return uint64(st.Rdev), true // dev_t: int32 on macOS, uint64 on Linux
}

// sameDevice reports whether tty is the terminal device dev.
func sameDevice(tty string, dev uint64) bool {
	var st unix.Stat_t
	return unix.Stat(tty, &st) == nil && uint64(st.Rdev) == dev
}

// Own is the pane cld runs in, where a restore joins it again: cld execs there the ssh it would
// type, as join execs tmux (decision 59.9). Env is cld's environment, its TERM fallbackTerm where
// Ghostty has not cached the host's terminfo.
type Own struct {
	Binding Binding
	Words   []string // ssh's argv
	Env     []string
}

// ownJoin is the Own that joins j's binding again.
func ownJoin(j *joining) *Own {
	env := os.Environ()
	if !j.cached {
		env = append(slices.DeleteFunc(env, func(v string) bool {
			return strings.HasPrefix(v, "TERM=")
		}), "TERM="+fallbackTerm)
	}
	return &Own{Binding: j.binding, Words: Words(j.binding), Env: env}
}

// Joining is the line cmd prints before Exec.
func (o *Own) Joining() string {
	if o.Binding.Session == "" {
		return "Joining " + o.Binding.Host + " here"
	}
	return fmt.Sprintf("Joining session '%s' on %s here", o.Binding.Session, o.Binding.Host)
}

// Exec replaces cld with ssh, run with Words and Env: the pane's process is ssh from then on. It
// returns only where that fails.
func (o *Own) Exec() error {
	path, err := tool.LookPath("ssh")
	if err != nil {
		return fail.Runtime("ssh is not installed")
	}
	return tool.CannotRun(path, syscall.Exec(path, o.Words, o.Env))
}
