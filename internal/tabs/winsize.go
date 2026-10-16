package tabs

import (
	"strings"

	"golang.org/x/sys/unix"
)

// winsize is the size of the pane on tty by TIOCGWINSZ: the columns, rows and pixels Ghostty
// gives the pty. It gives nil where it reads no tty, columns or rows. Opening the device as its
// owner, without making it a controlling terminal, changes nothing in the pane.
func winsize(tty string) *Size {
	if !strings.HasPrefix(tty, "/dev/") {
		return nil
	}
	fd, err := unix.Open(tty, unix.O_RDONLY|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	size, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	_ = unix.Close(fd) //nolint:errcheck // a device only read
	if err != nil || size.Col == 0 || size.Row == 0 {
		return nil
	}
	return &Size{Cols: int(size.Col), Rows: int(size.Row), Width: int(size.Xpixel),
		Height: int(size.Ypixel)}
}
