package picker

import (
	"os"
	"regexp"
	"time"

	"golang.org/x/term"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/session"
)

// answerWait is how long Enter waits for the terminal's answer before it hands the terminal over
// all the same: long enough for the round trip of a slow link (decision 14).
const answerWait = 5 * time.Second

// What the list writes to the terminal besides text and styles.
const (
	openScreen  = "\x1b[?1049h" + "\x1b[?25l" // the alternate screen, the cursor hidden
	closeScreen = "\x1b[?25h" + "\x1b[?1049l" // the cursor shown, the main screen back
	// askAttributes asks for the terminal's primary device attributes (DA1). Every terminal the
	// list runs on answers `CSI ? ATTRIBUTES c`, once it has read what came before.
	askAttributes = "\x1b[c"
)

// attributes matches the end of the terminal's answer to askAttributes.
var attributes = regexp.MustCompile(`\x1b\[\?[0-9;]*c$`)

// terminal is what the list changes in the terminal, to put back.
type terminal struct {
	fd    int
	saved *term.State // the terminal's mode before the list
	raw   bool        // the terminal is in raw mode
	shown bool        // the list is on the alternate screen
}

// makeRaw puts the terminal in raw mode, keeping the mode it had the first time to put back. In
// the background, where bg puts a stopped cld, SIGTTOU stops cld here until the shell brings it to
// the foreground, before the list draws anything.
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
	return output.Print(openScreen)
}

// restore puts back the main screen, the cursor and the terminal's mode. Each step is best
// effort, on the way out: a failure has nowhere to show but the terminal.
func (t *terminal) restore() {
	if t.shown {
		t.shown = false
		_, _ = os.Stdout.WriteString(closeScreen) //nolint:errcheck // best effort, as above
	}
	if t.raw {
		t.raw = false
		_ = term.Restore(t.fd, t.saved) //nolint:errcheck // best effort, as above
	}
}

// takeBack takes the terminal again once cld goes on after a stop, raw mode and the alternate
// screen, and reads its size, which may have changed meanwhile, for the list to draw it all.
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

// suspend puts the terminal back while SIGTSTP stops cld, as less and vim do, and takes it again
// once cld goes on (decision 14).
func (l *list) suspend(in *input, tty *terminal) error {
	tty.restore()
	if err := in.pause(); err != nil {
		return err
	}
	return l.takeBack(tty)
}

// handOver readies the terminal for tmux to take over for session name: the main screen, the
// cursor and join's title. It then asks for DA1 and waits for the answer, up to answerWait, so
// that tmux throws none of that away as it starts (decision 14). Keys typed before the answer
// are dropped; a signal meanwhile ends cld with the title on the tab.
func handOver(in *input, tty *terminal, name string) error {
	tty.shown = false
	if err := output.Print(closeScreen + session.Title(name) + askAttributes); err != nil {
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
			// A later answer reaches claude as keys (docs/design/findings/tmux-terminal.md).
			return nil
		case <-in.resized:
			// Nothing is on the screen to fit.
		case sig := <-in.stops:
			return stopped(sig)
		case <-in.continued:
			// Back from SIGSTOP, the answer is still to be read in raw mode. A SIGTSTP waits until
			// the terminal is handed over (see finish).
			if err := tty.makeRaw(); err != nil {
				return err
			}
		}
	}
}
