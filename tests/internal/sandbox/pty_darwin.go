package sandbox

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPty opens a pseudo-terminal's controller and names its terminal, as macOS's posix_openpt,
// grantpt, unlockpt and ptsname do.
func openPty() (*os.File, string, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	controller := os.NewFile(uintptr(fd), "/dev/ptmx")
	fail := func(err error) (*os.File, string, error) {
		controller.Close()
		return nil, "", err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		return fail(err)
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		return fail(err)
	}
	// TIOCPTYGNAME writes the name, and a NUL, to a buffer of 128 bytes.
	name := make([]byte, 128)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		return fail(errno)
	}
	path, _, _ := bytes.Cut(name, []byte{0})
	return controller, string(path), nil
}
