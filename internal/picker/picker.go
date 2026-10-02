// Package picker is cld list on a terminal: the sessions as the table of cld list shows them, one
// of them selected. Enter joins it as cld join does, bringing back one that has ended. Ctrl+X
// pressed twice kills it as cld kill does, or forgets one that has ended. Decisions 14 and 15 in
// docs/design/decisions give the keys, the waits and the footer.
//
// The list reads the sessions only as it opens and after its own actions, never on a timer: no
// row changes under a key (decision 14.6).
//
// It reads the terminal a byte at a time, once one has come: the keys typed after its last stay
// with the terminal (decision 14.7). Leaving puts the terminal back on every way out, signals
// included.
//
// In C-q s's popup or a pane of cld's servers, the session picked goes to another terminal. The
// list then hands nothing over, and returns it (decisions 51.2 and 51.6).
package picker

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/session"
)

// Source is where the list reads its rows, looks a session up before joining it, and kills or
// forgets one. Each gives up once ctx is done.
type Source interface {
	// Sessions reads the sessions to list.
	Sessions(ctx context.Context) ([]session.Session, error)
	// Joinable is nil where join can attach to session NAME or bring it back, as far as the list
	// can tell without running claude, and otherwise why not.
	Joinable(ctx context.Context, name string) error
	// Kill ends session NAME as cld kill -n NAME does, while one of its panes' pids is among pids
	// or its server has outlived it (decision 15.3). Otherwise it says why it ended nothing.
	Kill(ctx context.Context, name string, pids []string) error
	// Forget forgets session NAME, which has ended, and otherwise says why it forgot nothing.
	Forget(ctx context.Context, name string) error
}

// Available reports whether stdin and stdout are a terminal the list can run on, TERM set and not
// dumb, with cld in its foreground (decision 14.1). cld cannot read the foreground of a terminal
// other than its controlling terminal (TIOCGPGRP fails): no list there either.
func Available() bool {
	stdin := int(os.Stdin.Fd())
	if !term.IsTerminal(stdin) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}
	if name := os.Getenv("TERM"); name == "" || name == "dumb" {
		return false
	}
	foreground, err := unix.IoctlGetInt(stdin, unix.TIOCGPGRP)
	return err == nil && foreground == unix.Getpgrp()
}

// endSignals end cld while the list runs, once it has put the terminal back.
var endSignals = []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT}

// Run shows sessions until the user leaves, or picks one to join or bring back. It returns the
// session picked, with no name where the user left, and the sessions last read, less one its kill
// ended or its forget forgot since (see abandon). The terminal is back as before, with the title
// of a session picked, but where switching: then the session picked goes to another terminal.
func Run(source Source, sessions []session.Session, switching bool) (
	picked session.Session, last []session.Session, err error,
) {
	in, release := catch()
	defer release()
	// Raw mode comes first, so that no key is echoed or held for a line. Without ISIG, Ctrl+C,
	// Ctrl+Z and Ctrl+\ arrive as keys, and only Ctrl+C does anything.
	in.fd = int(os.Stdin.Fd())
	tty := &terminal{fd: in.fd}
	if err := tty.makeRaw(); err != nil {
		return session.Session{}, sessions, err
	}
	l := &list{source: source, rows: sessions}
	// What still runs beside the list is abandoned and waited for, once the terminal is back, so
	// that its tmux does not outlive cld.
	defer func() {
		l.abandon()
		last = l.rows
	}()
	defer tty.restore()

	waiting, next := make(chan error, 1), make(chan struct{})
	in.waiting, in.next = waiting, next
	go watch(in.fd, waiting, next)
	defer close(next)

	row, err := l.show(in, tty)
	if err != nil || row.Name == "" {
		return session.Session{}, l.rows, err
	}
	if err := finish(in, tty, row.Name, switching); err != nil {
		return session.Session{}, l.rows, err
	}
	return row, l.rows, nil
}

// catch catches the signals the list handles, before the terminal changes; release lets them go,
// after the terminal is back.
func catch() (in *input, release func()) {
	in = &input{
		resized:   make(chan os.Signal, 1),
		stops:     make(chan os.Signal, 1),
		paused:    make(chan os.Signal, 1),
		continued: make(chan os.Signal, 1),
	}
	signal.Notify(in.resized, syscall.SIGWINCH)
	for _, sig := range endSignals {
		// A signal cld was started with ignored, under nohup say, stays ignored.
		if !signal.Ignored(sig) {
			signal.Notify(in.stops, sig)
		}
	}
	signal.Notify(in.paused, syscall.SIGTSTP)
	signal.Notify(in.continued, syscall.SIGCONT)
	return in, func() {
		signal.Stop(in.continued)
		signal.Stop(in.paused)
		signal.Stop(in.stops)
		signal.Stop(in.resized)
	}
}

// show draws the list, then takes keys until the list is done (see keys).
func (l *list) show(in *input, tty *terminal) (session.Session, error) {
	if err := tty.open(); err != nil {
		return session.Session{}, err
	}
	l.measure()
	if err := output.Print(l.frame()); err != nil {
		return session.Session{}, err
	}
	return l.keys(in, tty)
}

// finish hands the terminal over for session name, but where switching, and puts it back. From
// then on, the signals the list caught end or stop cld as they would without the list.
func finish(in *input, tty *terminal, name string, switching bool) error {
	// A signal that came while Enter looked the session up ends cld, as it would have after.
	if err := in.signalled(); err != nil {
		return err
	}
	if !switching {
		if err := handOver(in, tty, name); err != nil {
			return err
		}
	}
	tty.restore()
	// os/signal passes on the signals it took before Stop by the time Stop returns: they end cld
	// too, joining nothing.
	signal.Stop(in.stops)
	if err := in.signalled(); err != nil {
		return err
	}
	// A SIGTSTP that came during the handover stops cld now, with the terminal back: tmux takes
	// it once cld goes on. Until cld becomes tmux, SIGTSTP then does nothing (see pause).
	signal.Stop(in.paused)
	select {
	case <-in.paused:
		return in.pause()
	default:
		return nil
	}
}
