package picker

import (
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
)

// input is what the list waits for: the terminal's bytes, its resizes, and the signals that end
// or stop cld.
type input struct {
	fd int
	// waiting says that the terminal has a byte to read, or why watch could not tell; next lets
	// watch look again.
	waiting   <-chan error
	next      chan<- struct{}
	resized   chan os.Signal
	stops     chan os.Signal // SIGTERM, SIGHUP, SIGINT and SIGQUIT
	paused    chan os.Signal // SIGTSTP
	continued chan os.Signal // SIGCONT
}

// take reads the byte that watch saw waiting - ready is what watch said - and lets watch look
// again.
func (in *input) take(ready error) (byte, error) {
	if ready == nil {
		var b [1]byte
		if _, ready = os.Stdin.Read(b[:]); ready == nil {
			in.next <- struct{}{}
			return b[0], nil
		}
	}
	if errors.Is(ready, io.EOF) {
		return 0, fail.Runtime("read error: the terminal closed")
	}
	return 0, fail.Runtime("read error: " + ready.Error())
}

// more reports whether the terminal has more to read now, without reading it.
func (in *input) more() bool {
	var set unix.FdSet
	set.Set(in.fd)
	n, err := unix.Select(in.fd+1, &set, nil, nil, &unix.Timeval{})
	return err == nil && n > 0
}

// signalled is how cld ends for a signal that has come and not been handled yet, if one has.
func (in *input) signalled() error {
	select {
	case sig := <-in.stops:
		return stopped(sig)
	default:
		return nil
	}
}

// pause stops cld, as SIGTSTP does by default, until SIGCONT. Go drops SIGTSTP once os/signal has
// had it, so cld stops itself with SIGSTOP, which takes effect after Kill returns, and waits for
// the SIGCONT (decision 14). A signal that ends cld, coming meanwhile, ends it once it goes on.
func (in *input) pause() error {
	// SIGTSTP stops nothing in an orphaned process group. cld goes on at once in its session
	// leader's process group, which only a shell with job control takes a job out of.
	if leader, err := unix.Getsid(0); err != nil || leader == unix.Getpgrp() {
		return nil //nolint:nilerr // with no session to tell by, cld goes on rather than stop for good
	}
	select {
	case <-in.continued: // one from before the stop
	default:
	}
	if err := unix.Kill(unix.Getpid(), unix.SIGSTOP); err != nil {
		return nil //nolint:nilerr // cld has not stopped, so it goes on
	}
	select {
	case <-in.continued:
		return nil
	case sig := <-in.stops:
		return stopped(sig)
	}
}

// stopped is how cld ends for sig: 128 and its number, as the shell reports a command it ended.
func stopped(sig os.Signal) error {
	return fail.Status(128 + int(sig.(syscall.Signal)))
}

// watch sends on waiting each time fd has a byte to read, or why it cannot tell, then waits for
// next before it looks again, until next closes. It reads nothing: the list reads a byte at a
// time itself, so that nothing reads the bytes after its last key.
func watch(fd int, waiting chan<- error, next <-chan struct{}) {
	for {
		waiting <- readable(fd)
		if _, more := <-next; !more {
			return
		}
	}
}

// readable waits until fd has something to read, or has closed. It uses select, as macOS's poll
// does not support a terminal (decision 14.7).
func readable(fd int) error {
	for {
		var set unix.FdSet
		set.Set(fd)
		if _, err := unix.Select(fd+1, &set, nil, nil, nil); !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}
