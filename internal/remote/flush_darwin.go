package remote

import (
	"os"

	"golang.org/x/sys/unix"
)

// flushInput discards what the user typed on the terminal f and no program has read. A terminal
// that cannot flush has nothing to lose.
func flushInput(f *os.File) {
	_ = unix.IoctlSetPointerInt(int(f.Fd()), unix.TIOCFLUSH, 1) //nolint:errcheck // see above
}
