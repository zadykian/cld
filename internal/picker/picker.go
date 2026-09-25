// Package picker is cld list on a terminal: the sessions on the alternate screen, one of them
// selected, to join with Enter as cld join does, or to kill with Ctrl+X pressed twice as cld kill
// does.
//
// The list opens with the first row selected; ↑ and ↓ move the selection, stopping at the first and
// the last row, Enter joins the selected session, and Esc or Ctrl+C leave, joining nothing. Ctrl+X
// arms the kill of the selected session, and a second Ctrl+X within armWait kills it. Esc, or the
// wait running out, disarms it and does nothing more; any other key disarms it and then does what
// it does. After a kill, Ctrl+X does nothing until it has not come for repeatWait, or another key
// has come, so that the repeats of a Ctrl+X held down arm and kill nothing more (see repeatWait).
// Other keys do nothing, keys with Alt among them: terminals send those as Esc and the key, so
// that Esc and a key typed within the wait for a lone Esc count as that key with Alt. The footer
// under the rows says what the keys do, but for the arrows where the terminal is too narrow for
// them all, and warns when Enter or the kill would detach a terminal attached to the session, one
// whose claude has exited included; once the kill is armed, it asks for the second Ctrl+X
// instead. A message, such as why Enter could not join, takes the place of its hints until the
// next key.
//
// Enter looks the session up while the list still owns the terminal, in raw mode. When the
// lookup fails, the list shows why, reads the sessions again and stays open; the selection stays
// on the same session or, once it is gone, moves to the next row the list showed that is still
// there, or else to the one above. With no rows left, the header stays over "no sessions". The
// second Ctrl+X runs cld kill's steps, with a check that the session is still the one on the row,
// with the same claude (see Source): the list then reads the sessions again, the selection moving
// by the same rule, and shows why the kill ended nothing, if it did - the session gone, made again
// under its name, or ended with its server running on. The list reads the sessions only when it opens and after its own
// actions, never on a timer, so a row does not change under a key. While Enter looks the session
// up, or the kill runs, and the list reads the sessions again, Esc, Ctrl+C and the signals below
// still leave - its tmux is killed - and other keys do nothing: a lookup that hangs, on a server
// that does, does not hold the list. A session the kill has ended by then is not in the sessions
// the list leaves with.
//
// The list redraws the whole screen after each key and each resize - once for the bytes of a key,
// and once for keys that come together, pasted say - and cuts every line at the terminal's
// width, counted in cells, so that none wraps. Leaving puts the terminal back as it was - the
// main screen, the cursor and the terminal's mode - on every way out: Esc, Ctrl+C, Enter before
// the handover to tmux, an error, SIGTERM, SIGHUP, SIGINT and SIGQUIT. A signal that comes
// before cld becomes tmux ends it with 128 and the signal's number, joining nothing; one that
// comes after Enter's lookup may leave the session's title on the tab (see handOver). The
// terminal is as it was while SIGTSTP stops cld, too (see pause), and once cld goes on after any
// stop (SIGCONT) the list takes the terminal again and draws it all, as less and vim do: SIGSTOP
// leaves the list on the screen, and the shell may put its own mode back meanwhile, as bash does.
// Ctrl+Z and Ctrl+\ are keys in raw mode, and do nothing.
//
// The list reads the terminal a byte at a time, and only once there is one to read, so that what
// is typed after its last key stays with the terminal: for the shell after Esc or Ctrl+C, for
// tmux after Enter - but for what comes before the terminal's answer to the list's last question,
// or an answer later than answerWait, which reaches claude (see handOver).
package picker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"golang.org/x/text/width"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/session"
)

// Source is where the list reads its rows, looks a session up before joining it, and kills one.
// Each gives up once ctx is done.
type Source interface {
	// Sessions reads the sessions to list.
	Sessions(ctx context.Context) ([]session.Session, error)
	// Joinable is nil when join can attach to session NAME, and otherwise why it cannot.
	Joinable(ctx context.Context, name string) error
	// Kill ends session NAME, as cld kill -n NAME does, if it is still the session the list read,
	// one of whose panes' pids is among pids (see session.Session), and otherwise says why it
	// ended nothing.
	Kill(ctx context.Context, name string, pids []string) error
}

// Available reports whether cld's stdin and stdout are a terminal the list can run on: both
// terminals, TERM set and not dumb - a dumb terminal cannot move the cursor - and cld in the
// terminal's foreground. A job in the background, as with cld list &, would stop as it set the
// terminal up (SIGTTOU), where it printed the table before. The foreground of a terminal that is
// not cld's controlling terminal is not cld's to know (TIOCGPGRP fails): no list there either.
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

// escapeWait is how long a lone Esc waits for the rest of a sequence: the arrow keys start with
// Esc too, and a sequence can arrive in two reads.
const escapeWait = 100 * time.Millisecond

// armWait is how long the kill stays armed for the second Ctrl+X, as in Claude Code's agent view.
// A key that has begun when it is over, or has come and not been read yet, counts as typed within
// it: an Esc keeps the session although the wait for a lone Esc ends after it, and a Ctrl+X that
// cld, held up, reads late kills it.
const armWait = 2 * time.Second

// repeatWait is how long Ctrl+X does nothing after the Ctrl+X that killed, and after each one that
// comes meanwhile. A key held down repeats: the terminal types it again after a delay, half a
// second or so by default, and then many times a second. Held a little too long, the second
// Ctrl+X would otherwise arm the kill of the session that took the killed one's place, and kill
// it, and the next, none of which the user selected.
const repeatWait = time.Second

// answerWait is how long Enter waits for the terminal's answer before it hands the terminal over
// all the same (see handOver), where a later answer goes to claude: long enough for the round
// trip of a slow link. Every terminal the list runs on answers, so only one that does not waits
// it out.
const answerWait = 5 * time.Second

// What the list writes to the terminal besides text.
const (
	openScreen  = "\x1b[?1049h" + "\x1b[?25l" // the alternate screen, the cursor hidden
	closeScreen = "\x1b[?25h" + "\x1b[?1049l" // the cursor shown, the main screen back
	inverse     = "\x1b[7m"
	noInverse   = "\x1b[27m"
	dim         = "\x1b[2m"
	noDim       = "\x1b[22m"
	// askAttributes asks the terminal what it is (primary device attributes, DA1): every terminal
	// the list runs on answers, CSI ? and its attributes then c, once it has read what came before.
	askAttributes = "\x1b[c"
)

// attributes matches the end of the terminal's answer to askAttributes.
var attributes = regexp.MustCompile(`\x1b\[\?[0-9;]*c$`)

// Run shows sessions until the user leaves or picks one to join. It returns the name of the
// session picked, "" when the user left, and the sessions the list last read, less one its kill
// ended since (see abandon). The terminal is back as it was when Run returns; after a pick, it
// also has the session's title (see handOver).
func Run(source Source, sessions []session.Session) (picked string, last []session.Session, err error) {
	// Signals are caught before the terminal changes, and let go after it is back.
	in := &input{
		resized:   make(chan os.Signal, 1),
		stops:     make(chan os.Signal, 1),
		paused:    make(chan os.Signal, 1),
		continued: make(chan os.Signal, 1),
	}
	signal.Notify(in.resized, syscall.SIGWINCH)
	defer signal.Stop(in.resized)
	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT} {
		// A signal cld was started with ignored, under nohup say, stays ignored.
		if !signal.Ignored(sig) {
			signal.Notify(in.stops, sig)
		}
	}
	defer signal.Stop(in.stops)
	signal.Notify(in.paused, syscall.SIGTSTP)
	defer signal.Stop(in.paused)
	signal.Notify(in.continued, syscall.SIGCONT)
	defer signal.Stop(in.continued)

	// Raw mode comes first: a key typed once the footer shows is neither echoed nor held for a
	// line. It clears ISIG too, so Ctrl+C arrives as a key.
	in.fd = int(os.Stdin.Fd())
	tty := &terminal{fd: in.fd}
	if err := tty.makeRaw(); err != nil {
		return "", sessions, err
	}
	l := &list{source: source, rows: sessions}
	// A lookup or a kill still running when the list ends is abandoned and waited for, once the
	// terminal is back: its tmux does not outlive cld. What it did by then counts for the sessions
	// Run returns (see abandon).
	defer func() {
		l.abandon()
		last = l.rows
	}()
	defer tty.restore()

	waiting, next := make(chan error, 1), make(chan struct{})
	in.waiting, in.next = waiting, next
	go watch(in.fd, waiting, next)
	defer close(next)

	if err := tty.open(); err != nil {
		return "", l.rows, err
	}
	l.measure()
	if err := fail.Print(l.frame()); err != nil {
		return "", l.rows, err
	}
	name, err := l.keys(in, tty)
	if err != nil || name == "" {
		return "", l.rows, err
	}
	// A signal that came while Enter looked the session up ends cld, as it would have after.
	if err := in.signalled(); err != nil {
		return "", l.rows, err
	}
	if err := handOver(in, tty, name); err != nil {
		return "", l.rows, err
	}
	tty.restore()
	// From here on these signals end cld as they would without the list. os/signal passes on
	// those it took before Stop by the time Stop returns: they end cld too, joining nothing.
	signal.Stop(in.stops)
	if err := in.signalled(); err != nil {
		return "", l.rows, err
	}
	// A SIGTSTP that came during the handover stops cld now, with the terminal back: tmux takes
	// it once cld goes on. Until cld becomes tmux, SIGTSTP then does nothing (see pause).
	signal.Stop(in.paused)
	select {
	case <-in.paused:
		if err := in.pause(); err != nil {
			return "", l.rows, err
		}
	default:
	}
	return name, l.rows, nil
}

// keys takes keys until the list is done: it returns the session Enter picked, or "" when the
// user left. It draws the list again once it has taken all that has come.
func (l *list) keys(in *input, tty *terminal) (string, error) {
	var pending []byte
	var escape <-chan time.Time
	for {
		// The outcome of Enter's lookup or of the kill, and the end of the wait for the second
		// Ctrl+X, wait for a key that has begun, which may be Esc.
		acting, expiry := l.acting, l.expiry
		if len(pending) > 0 {
			acting, expiry = nil, nil
		}
		select {
		case ready := <-in.waiting:
			b, err := in.take(ready)
			if err != nil {
				return "", err
			}
			pending = append(pending, b)
			for len(pending) > 0 {
				k, n := parse(pending)
				if n == 0 {
					// The wait starts with the byte that began the key, and again with each byte.
					escape = time.After(escapeWait)
					break
				}
				pending, escape = pending[n:], nil
				if done, name := l.press(k); done {
					return name, nil
				}
			}
		case <-escape:
			// The wait is over: Esc alone, or twice, is Esc; a sequence cut short is dropped.
			k := other
			if strings.Trim(string(pending), "\x1b") == "" {
				k = esc
			}
			pending, escape = nil, nil
			if done, name := l.press(k); done {
				return name, nil
			}
		case result := <-acting:
			if name := l.settle(result); name != "" {
				return name, nil
			}
		case <-expiry:
			// A byte waiting to be read may have come within the wait, while cld was held up: the
			// kill stays armed for the key it begins, read next - Esc keeps the session, Ctrl+X
			// kills it, and another key disarms the kill.
			l.expiry = nil
			if !in.more() {
				l.disarm()
			}
		case <-l.hold:
			// A byte waiting to be read may have come within the wait, while cld was held up: the
			// wait starts again, and the byte, read next, ends it or starts it again.
			l.hold = nil
			if in.more() {
				l.hold = time.After(repeatWait)
			}
			continue // nothing to draw
		case <-in.resized:
			l.measure()
		case sig := <-in.stops:
			return "", stopped(sig)
		case <-in.paused:
			// SIGTSTP: the terminal is as it was while cld is stopped.
			tty.restore()
			if err := in.pause(); err != nil {
				return "", err
			}
			if err := l.takeBack(tty); err != nil {
				return "", err
			}
		case <-in.continued:
			// Back from a stop the list did not see coming (SIGSTOP): the shell may have put its
			// own mode back, as bash does, and written over the list.
			if err := l.takeBack(tty); err != nil {
				return "", err
			}
		}
		// Nothing is drawn halfway through a key, nor before the keys that came with it.
		if len(pending) == 0 && !in.more() {
			if err := fail.Print(l.frame()); err != nil {
				return "", err
			}
		}
	}
}

// handOver readies the terminal for tmux to take it over for session name. It brings the main
// screen back, shows the cursor and writes the session's title, as join does, then asks the
// terminal a question. tmux throws away what the terminal has not read yet as its client starts
// (tcflush(TCOFLUSH) in tty_start_tty, tmux 3.3a to 3.7c); the terminal answers once it has read
// all that. Keys typed with Enter, which arrive before the answer, are dropped; what comes after
// it stays for tmux. A terminal that has not answered after answerWait is handed over all the
// same, and its answer, should it come later, goes to claude as keys: tmux asks the terminal the
// same question as it starts, and takes the first answer for its own. Attach, the rest of join,
// writes the title again, which tmux may throw away: this one has arrived. A signal that comes
// meanwhile ends cld with the title written: the title has to come before the question, and the
// question after the lookup.
func handOver(in *input, tty *terminal, name string) error {
	tty.shown = false
	if err := fail.Print(closeScreen + session.Title(name) + askAttributes); err != nil {
		return err
	}
	unanswered := time.After(answerWait)
	var answer []byte
	for {
		select {
		case ready := <-in.waiting:
			b, err := in.take(ready)
			if err != nil {
				return err
			}
			if b == 0x1b {
				answer = answer[:0]
			}
			if answer = append(answer, b); attributes.Match(answer) {
				return nil
			}
		case <-unanswered:
			return nil
		case <-in.resized:
			// Nothing is on the screen to fit.
		case sig := <-in.stops:
			return stopped(sig)
		case <-in.continued:
			// Back from a stop (SIGSTOP), the answer is still to be read in raw mode. A SIGTSTP
			// waits until the terminal is handed over (see Run).
			if err := tty.makeRaw(); err != nil {
				return err
			}
		}
	}
}

// terminal is what the list changes in the terminal, to put back.
type terminal struct {
	fd    int
	saved *term.State // the terminal's mode before the list
	raw   bool        // the terminal is in raw mode
	shown bool        // the list is on the alternate screen
}

// makeRaw puts the terminal in raw mode, keeping the mode it had the first time to put back. In
// the background, where bg puts a stopped cld, that stops cld (SIGTTOU) until it is back in the
// foreground: it comes before the list draws anything.
func (t *terminal) makeRaw() error {
	state, err := term.MakeRaw(t.fd)
	if err != nil {
		return fail.Runtime("cannot set up the terminal: " + err.Error())
	}
	if t.saved == nil {
		t.saved = state
	}
	t.raw = true
	return nil
}

// open brings up the alternate screen, with the cursor hidden, for the list to draw on.
func (t *terminal) open() error {
	t.shown = true
	return fail.Print(openScreen)
}

// restore puts the terminal back as it was: the main screen, the cursor and the terminal's mode.
func (t *terminal) restore() {
	if t.shown {
		t.shown = false
		_, _ = os.Stdout.WriteString(closeScreen)
	}
	if t.raw {
		t.raw = false
		_ = term.Restore(t.fd, t.saved)
	}
}

// takeBack takes the terminal again once cld goes on after a stop - raw mode, the alternate
// screen - and reads its size, which may have changed meanwhile, for the list to draw it all.
func (l *list) takeBack(tty *terminal) error {
	if err := tty.makeRaw(); err != nil {
		return err
	}
	if err := tty.open(); err != nil {
		return err
	}
	l.measure()
	return nil
}

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

// pause stops cld, as SIGTSTP does by default, until the shell has it go on (SIGCONT). Go keeps
// its own handler for SIGTSTP once os/signal has had the signal, and drops it when nothing
// wants it (Go 1.27.1): cld stops with SIGSTOP instead, which takes effect some time after Kill
// returns, so pause waits for the SIGCONT that ends the stop. SIGTSTP stops nothing in an
// orphaned process group, where nothing would have it go on: cld goes on at once where its
// process group is its session leader's, which only a shell with job control takes a job out of.
// A signal that ends cld, if one comes meanwhile, ends it once it goes on.
func (in *input) pause() error {
	if leader, err := unix.Getsid(0); err != nil || leader == unix.Getpgrp() {
		return nil
	}
	select {
	case <-in.continued: // one from before the stop
	default:
	}
	if err := unix.Kill(unix.Getpid(), unix.SIGSTOP); err != nil {
		return nil
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

// watch sends on waiting each time the terminal fd has a byte to read, or why it cannot tell,
// and then waits for next before it looks again, until next is closed. It reads nothing: the
// list reads a byte at a time itself, once watch has seen one, and nothing reads the bytes
// after its last key.
func watch(fd int, waiting chan<- error, next <-chan struct{}) {
	for {
		waiting <- readable(fd)
		if _, more := <-next; !more {
			return
		}
	}
}

// readable waits until fd has something to read, or has closed. It uses select: macOS's poll
// does not support devices, a terminal among them (poll(2), BUGS).
func readable(fd int) error {
	for {
		var set unix.FdSet
		set.Set(fd)
		if _, err := unix.Select(fd+1, &set, nil, nil, nil); !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

// key is what the list makes of the bytes of one key.
type key int

const (
	other key = iota // anything the list does nothing with
	up
	down
	enter
	esc
	interrupt // Ctrl+C
	kill      // Ctrl+X
)

// parse takes the first key off input: the key and its length in bytes, or a length of 0 when
// input only starts one - an Esc, or a sequence that may still be arriving. The arrows are
// ESC [ A and ESC [ B, or ESC O A and ESC O B once a program has turned on application cursor
// keys - the list accepts both, and sets neither mode itself. Enter is CR, and Ctrl+C and Ctrl+X
// are 0x03 and 0x18 in raw mode. Terminals send a key with Alt as Esc and the key: Esc followed by
// another key is that key with Alt, which does nothing - but for Esc itself, where the first Esc
// stands alone unless a sequence follows the second (Alt+Up as ESC ESC [ A).
func parse(input []byte) (key, int) {
	switch input[0] {
	case '\r':
		return enter, 1
	case 0x03:
		return interrupt, 1
	case 0x18:
		return kill, 1
	case 0x1b:
	default:
		return other, 1
	}
	if len(input) == 1 {
		return other, 0
	}
	switch input[1] {
	case '[':
		// A control sequence: parameter bytes, intermediate bytes, then a final byte.
		i := 2
		for i < len(input) && input[i] >= 0x30 && input[i] <= 0x3f {
			i++
		}
		for i < len(input) && input[i] >= 0x20 && input[i] <= 0x2f {
			i++
		}
		if i == len(input) {
			return other, 0
		}
		if input[i] < 0x40 || input[i] > 0x7e {
			return other, i // not a sequence after all: what came before goes
		}
		// A modified arrow, such as Shift+Up (ESC [ 1 ; 2 A), does nothing.
		if params := string(input[2:i]); params == "" || params == "1" {
			return arrow(input[i]), i + 1
		}
		return other, i + 1
	case 'O':
		if len(input) == 2 {
			return other, 0
		}
		return arrow(input[2]), 3
	case 0x1b:
		if len(input) == 2 {
			return other, 0
		}
		if input[2] != '[' && input[2] != 'O' {
			return esc, 1
		}
		if _, n := parse(input[1:]); n > 0 {
			return other, 1 + n
		}
		return other, 0
	}
	return other, 2
}

// arrow is the key a cursor key sequence ending in final is.
func arrow(final byte) key {
	switch final {
	case 'A':
		return up
	case 'B':
		return down
	}
	return other
}

// list is the state of the list on screen.
type list struct {
	source Source
	rows   []session.Session
	// selected is the selected row; top is the first row in view.
	selected, top int
	// message takes the place of the footer's hints until the next key.
	message string
	// armed is whether Ctrl+X has armed the kill of the selected session; expiry fires once the
	// wait for the second Ctrl+X is over, and is nil when the kill is not armed.
	armed  bool
	expiry <-chan time.Time
	// hold, from the Ctrl+X that killed on, fires once no Ctrl+X has come for repeatWait: until
	// then, or another key, Ctrl+X does nothing (see repeatWait). It is nil otherwise.
	hold            <-chan time.Time
	columns, height int
	// acting gets the outcome of Enter's lookup, or of the kill, while it runs, and is nil
	// otherwise; cancel abandons it, and running counts it until it has ended.
	acting  <-chan outcome
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// outcome is the outcome of Enter's lookup of session name, or of its kill: refused is why join
// cannot attach to it, or why the kill ended nothing, and nil when join can or the kill ended it;
// then rows are the sessions read again, or unread why they could not be.
type outcome struct {
	name    string
	kill    bool
	refused error
	rows    []session.Session
	unread  error
}

// press does what k does. It reports whether the list is done, and with which session picked.
// Ctrl+X arms the kill, or once armed kills. Any other key disarms it, and then does what it does
// - but for Esc, which only disarms it. After a kill, Ctrl+X does nothing until it has not come
// for repeatWait, or another key has come. While Enter's lookup or the kill runs, only leaving
// does anything.
func (l *list) press(k key) (done bool, picked string) {
	if k == kill && l.hold != nil {
		l.hold = time.After(repeatWait)
		return false, ""
	}
	l.hold = nil
	if l.acting != nil && k != esc && k != interrupt {
		return false, ""
	}
	armed := l.armed
	l.disarm()
	l.message = ""
	switch k {
	case up:
		l.selected = max(l.selected-1, 0)
	case down:
		l.selected = max(min(l.selected+1, len(l.rows)-1), 0)
	case esc:
		return !armed, ""
	case interrupt:
		return true, ""
	case enter:
		if len(l.rows) > 0 {
			l.lookUp(l.rows[l.selected].Name)
		}
	case kill:
		switch {
		case len(l.rows) == 0:
		case armed:
			l.kill(l.rows[l.selected])
			l.hold = time.After(repeatWait)
		default:
			l.armed, l.expiry = true, time.After(armWait)
		}
	}
	return false, ""
}

// disarm disarms the kill.
func (l *list) disarm() {
	l.armed, l.expiry = false, nil
}

// start runs task beside the list, which goes on taking keys: acting gets its outcome.
func (l *list) start(task func(ctx context.Context) outcome) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan outcome, 1)
	l.acting, l.cancel = done, cancel
	l.running.Add(1)
	go func() {
		defer l.running.Done()
		done <- task(ctx)
	}()
}

// lookUp starts Enter's lookup of session name, and the read of the sessions again when join
// would refuse it.
func (l *list) lookUp(name string) {
	l.start(func(ctx context.Context) outcome {
		result := outcome{name: name, refused: l.source.Joinable(ctx, name)}
		if result.refused != nil {
			result.rows, result.unread = l.read(ctx)
		}
		return result
	})
}

// kill starts the kill of the session on row, and the read of the sessions again after it. The
// killed session's server exits after it, and the read passes over a server that exits as it is
// asked (see session.Tmux.Sessions).
func (l *list) kill(row session.Session) {
	l.start(func(ctx context.Context) outcome {
		result := outcome{name: row.Name, kill: true, refused: l.source.Kill(ctx, row.Name, row.PIDs)}
		result.rows, result.unread = l.read(ctx)
		return result
	})
}

// read reads the sessions again for Enter's lookup or the kill, unless the list has abandoned it:
// then they are unread, as ctx says.
func (l *list) read(ctx context.Context) ([]session.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.source.Sessions(ctx)
}

// settle takes the outcome of Enter's lookup or of the kill: the session to join, or "" when
// there is none - join would refuse it, or the list killed or tried to kill it. Then the list says
// why join or the kill was refused, if it was, and shows the sessions read again, keeping the
// selection on the same session where it can (see follow); when they could not be read, it says
// that too and keeps its rows, but for the session its kill ended.
func (l *list) settle(result outcome) string {
	l.cancel()
	l.acting, l.cancel = nil, nil
	if !result.kill && result.refused == nil {
		return result.name
	}
	if result.refused != nil {
		l.message = describe(result.refused)
	}
	rows := result.rows
	if result.unread != nil {
		if problem := describe(result.unread); problem != l.message {
			l.message = strings.TrimPrefix(l.message+" · "+problem, " · ")
		}
		rows = l.rows
		if result.kill && result.refused == nil {
			rows = slices.DeleteFunc(slices.Clone(rows), func(row session.Session) bool { return row.Name == result.name })
		}
	}
	l.selected = follow(l.rows, l.selected, rows)
	l.rows = rows
	return ""
}

// abandon ends Enter's lookup or the kill if it is still running, killing its tmux, and waits for
// it. Its outcome then counts for the rows, as settle takes it, but for joining: a kill that ended
// the session - its tmux command, kill-session and kill-server, returned - takes the row away, and
// a read of the sessions that ended gives the rows. A kill cut short may or may not have ended the
// session, whose row stays.
func (l *list) abandon() {
	if l.acting == nil {
		return
	}
	l.cancel()
	l.running.Wait()
	select {
	case result := <-l.acting:
		l.settle(result)
	default:
	}
}

// follow is the row to select in rows, read again, when selected was the selected one of old:
// the same session if it is still listed, or else the next of old still listed - the one that
// took its place - or else the one above it.
func follow(old []session.Session, selected int, rows []session.Session) int {
	find := func(s session.Session) int {
		return slices.IndexFunc(rows, func(row session.Session) bool { return row.Name == s.Name })
	}
	for i := selected; i < len(old); i++ {
		if found := find(old[i]); found >= 0 {
			return found
		}
	}
	for i := selected - 1; i >= 0; i-- {
		if found := find(old[i]); found >= 0 {
			return found
		}
	}
	return max(min(selected, len(rows)-1), 0)
}

// describe is what the list says about err: cld's message, without the advice meant for the
// command line.
func describe(err error) string {
	var failure *fail.Error
	if errors.As(err, &failure) {
		return failure.Message
	}
	return err.Error()
}

// measure reads the terminal's size: 80 by 24 where it cannot.
func (l *list) measure() {
	columns, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || columns <= 0 || height <= 0 {
		columns, height = 80, 24
	}
	l.columns, l.height = columns, height
}

// frame draws the list over the whole screen: its lines from the top, each cleared before it
// is drawn, then the rest of the screen cleared. Every line is cut to the terminal's width, and
// a line moved to by position ends whatever wrap a full line before it had pending.
func (l *list) frame() string {
	var out strings.Builder
	lines := l.lines()
	for i, line := range lines {
		fmt.Fprintf(&out, "\x1b[%d;1H\x1b[2K%s", i+1, line)
	}
	if len(lines) < l.height {
		fmt.Fprintf(&out, "\x1b[%d;1H\x1b[J", len(lines)+1)
	}
	return out.String()
}

// lines are the lines the list shows, styled: the header, the rows in view with the selected
// one marked and in inverse video - or "no sessions" once none are left - a blank line and the
// footer. A terminal too short for the header, a row, the blank line and the footer loses the
// blank line, then the header, then the footer.
func (l *list) lines() []string {
	nameWidth := 4
	for _, row := range l.rows {
		nameWidth = max(nameWidth, cells(row.Name))
	}
	format := func(marker string, row session.Session) string {
		return marker + " " + pad(row.Name, nameWidth) + "  " + pad(row.State, 8) + "  " + row.Directory
	}
	title := format(" ", session.Session{Name: "NAME", State: "STATE", Directory: "DIRECTORY"})
	header := []string{cut(title, l.columns)}
	var rows []string
	if len(l.rows) == 0 {
		rows = []string{cut("no sessions", l.columns)}
	} else {
		// The selected row's inverse video spans the table, as far as the terminal shows it.
		tableWidth := cells(title)
		for _, row := range l.rows {
			tableWidth = max(tableWidth, cells(format(">", row)))
		}
		// The rows in view: as many as fit, the selected one among them.
		fits := max(l.height-3, 1)
		l.top = min(l.top, l.selected)
		l.top = max(l.top, l.selected-fits+1)
		l.top = max(min(l.top, len(l.rows)-fits), 0)
		for i := l.top; i < min(l.top+fits, len(l.rows)); i++ {
			if i == l.selected {
				rows = append(rows, inverse+pad(cut(format(">", l.rows[i]), l.columns), min(tableWidth, l.columns))+noInverse)
			} else {
				rows = append(rows, cut(format(" ", l.rows[i]), l.columns))
			}
		}
	}
	blank, footer := []string{""}, []string{l.footer()}
	for _, left := range []*[]string{&blank, &header, &footer} {
		if len(header)+len(rows)+len(blank)+len(footer) > l.height {
			*left = nil
		}
	}
	return slices.Concat(header, rows, blank, footer)
}

// footer is the line under the rows: the message, the kill's question once it is armed, or the
// hints for the keys, cut at the terminal's width.
func (l *list) footer() string {
	if l.message != "" {
		return cut(strings.Join(strings.Fields(l.message), " "), l.columns)
	}
	detach := ""
	if len(l.rows) > 0 && l.rows[l.selected].Attached {
		detach = " and detach its terminal"
	}
	hints := "esc to quit"
	switch {
	case l.armed:
		hints = "ctrl+x again to kill" + detach + " · esc to keep"
	case len(l.rows) > 0:
		// The arrows' hint goes first where the terminal is short of cells for them all, as on
		// a row with a terminal attached in 80 columns: the others then fit whole.
		hints = "enter to join" + detach + " · ctrl+x to kill · " + hints
		if all := "↑/↓ to navigate · " + hints; cells(all) <= l.columns {
			hints = all
		}
	}
	return dim + cut(hints, l.columns) + noDim
}

// cells is the number of terminal cells text takes: two for a wide character, none for a
// combining one, and one for any other, an ambiguous one - such as ↑, · or é - included.
func cells(text string) int {
	count := 0
	for _, r := range text {
		count += runeCells(r)
	}
	return count
}

func runeCells(r rune) int {
	switch {
	case r == '\u00ad': // the soft hyphen shows as one
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// cut is text in at most columns cells: what does not fit is left out, a wide character that
// would straddle the edge included. A control character shows as "?", so that none moves the
// cursor or changes the terminal; bytes that are not UTF-8 show as U+FFFD.
func cut(text string, columns int) string {
	var out strings.Builder
	used := 0
	for _, r := range text {
		if unicode.IsControl(r) {
			r = '?'
		}
		size := runeCells(r)
		if used+size > columns {
			break
		}
		used += size
		out.WriteRune(r)
	}
	return out.String()
}

// pad is text followed by spaces up to columns cells.
func pad(text string, columns int) string {
	return text + strings.Repeat(" ", max(columns-cells(text), 0))
}
