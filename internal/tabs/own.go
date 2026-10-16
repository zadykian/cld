package tabs

import (
	"golang.org/x/sys/unix"
	"golang.org/x/term"
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
