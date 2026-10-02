package picker

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/session"
)

// escapeWait is how long a lone Esc waits for the rest of a sequence, which can arrive in two
// reads: the arrow keys start with Esc too (decision 14.7).
const escapeWait = 100 * time.Millisecond

// typing holds the bytes of a key that has begun, and the wait for the rest of its sequence, nil
// while no key has begun.
type typing struct {
	pending []byte
	escape  <-chan time.Time
}

// add takes byte b and returns the keys the pending bytes now make. The wait starts with the byte
// that begins a key, and again with each byte after it.
func (t *typing) add(b byte) []key {
	t.pending = append(t.pending, b)
	var keys []key
	for len(t.pending) > 0 {
		k, n := parse(t.pending)
		if n == 0 {
			t.escape = time.After(escapeWait)
			break
		}
		t.pending, t.escape = t.pending[n:], nil
		keys = append(keys, k)
	}
	return keys
}

// expire ends the wait: Esc alone, or twice, is Esc; a sequence cut short is dropped.
func (t *typing) expire() key {
	k := other
	if strings.Trim(string(t.pending), "\x1b") == "" {
		k = esc
	}
	t.pending, t.escape = nil, nil
	return k
}

// keys takes keys until the list is done: it returns the session Enter picked, or one with no
// name where the user left. It draws the list again once it has taken all that has come: once
// for the bytes of a key, and once for keys that come together, pasted say.
func (l *list) keys(in *input, tty *terminal) (session.Session, error) {
	w := &waiter{list: l, in: in, tty: tty}
	for {
		s := wait(w.feeds())
		if s.done || s.err != nil {
			return s.picked, s.err
		}
		// Nothing is drawn halfway through a key, nor before the keys that came with it.
		if !s.unchanged && len(w.typing.pending) == 0 && !in.more() {
			if err := output.Print(l.frame()); err != nil {
				return session.Session{}, err
			}
		}
	}
}

// step is what the list made of what came: the session Enter picked, whether the list is done,
// and the error that ends it. unchanged says that nothing on screen has changed.
type step struct {
	picked    session.Session
	done      bool
	unchanged bool
	err       error
}

// A feed is a channel the list waits on, and the method that takes what comes on it. A nil
// channel never has anything.
type feed struct {
	channel reflect.Value
	take    func(received reflect.Value) step
}

// on is the feed of channel c, whose values go to take.
func on[T any](c <-chan T, take func(T) step) feed {
	return feed{reflect.ValueOf(c), func(received reflect.Value) step {
		// A nil error fails the assertion, and value is nil all the same.
		value, _ := reflect.TypeAssert[T](received)
		return take(value)
	}}
}

// wait waits until one of feeds has something, and returns what its method made of it. Where
// several have, it takes one at random, as a select statement does: none is preferred.
func wait(feeds []feed) step {
	cases := make([]reflect.SelectCase, len(feeds))
	for i, f := range feeds {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: f.channel}
	}
	chosen, received, _ := reflect.Select(cases)
	return feeds[chosen].take(received)
}

// waiter is the list waiting for keys: what it waits on, the terminal to put back and take again,
// and the key that has begun.
type waiter struct {
	list   *list
	in     *input
	tty    *terminal
	typing typing
}

// feeds are what the list waits on. The outcome of an action, and the end of the wait for the
// second Ctrl+X, wait for a key that has begun, which may be Esc.
func (w *waiter) feeds() []feed {
	acting, expiry := w.list.acting, w.list.expiry
	if len(w.typing.pending) > 0 {
		acting, expiry = nil, nil
	}
	return []feed{
		on(w.in.waiting, w.typed),
		on(w.typing.escape, w.escaped),
		on(acting, w.settled),
		on(expiry, w.armOver),
		on(w.list.hold, w.holdOver),
		on(w.in.resized, w.resized),
		on(w.in.stops, w.ended),
		on(w.in.paused, w.paused),
		on(w.in.continued, w.continued),
	}
}

// typed reads the byte that watch saw waiting, and presses the keys it ends in turn until one
// leaves.
func (w *waiter) typed(ready error) step {
	b, err := w.in.take(ready)
	if err != nil {
		return step{err: err}
	}
	return step{done: slices.ContainsFunc(w.typing.add(b), w.list.press)}
}

// escaped presses the key the pending bytes make once the wait for the rest is over.
func (w *waiter) escaped(time.Time) step {
	return step{done: w.list.press(w.typing.expire())}
}

// settled takes the outcome of Enter's lookup, or of the kill or forget (see settle).
func (w *waiter) settled(result outcome) step {
	picked := w.list.settle(result)
	return step{picked: picked, done: picked.Name != ""}
}

// armOver ends the wait for the second Ctrl+X. A byte waiting to be read may have come within it,
// while cld was held up: the kill then stays armed for the key it begins (decision 15.1).
func (w *waiter) armOver(time.Time) step {
	w.list.expiry = nil
	if !w.in.more() {
		w.list.disarm()
	}
	return step{}
}

// holdOver ends the hold after a kill. A byte waiting to be read may have come within it, while
// cld was held up. The hold then starts again, and the byte, read next, ends it or starts it again.
func (w *waiter) holdOver(time.Time) step {
	w.list.hold = nil
	if w.in.more() {
		w.list.hold = time.After(repeatWait)
	}
	return step{unchanged: true}
}

// resized reads the terminal's new size, for the list to fit it.
func (w *waiter) resized(os.Signal) step {
	w.list.measure()
	return step{}
}

// ended ends cld for sig, one of endSignals.
func (w *waiter) ended(sig os.Signal) step {
	return step{err: stopped(sig)}
}

// paused stops cld for SIGTSTP, the terminal put back meanwhile (see suspend).
func (w *waiter) paused(os.Signal) step {
	return step{err: w.list.suspend(w.in, w.tty)}
}

// continued takes the terminal back after a SIGSTOP the list did not see coming: the shell may
// have put back its own mode, as bash does, and written over the list.
func (w *waiter) continued(os.Signal) step {
	return step{err: w.list.takeBack(w.tty)}
}
