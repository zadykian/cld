package sandbox

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// openPty opens a pseudo-terminal's controller and names its terminal, as glibc's posix_openpt,
// unlockpt and ptsname do.
func openPty() (*os.File, string, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	controller := os.NewFile(uintptr(fd), "/dev/ptmx")
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		controller.Close()
		return nil, "", err
	}
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		controller.Close()
		return nil, "", err
	}
	return controller, "/dev/pts/" + strconv.FormatUint(uint64(n), 10), nil
}
