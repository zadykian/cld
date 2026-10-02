package picker

import (
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
	var t typing
	for {
		// The outcome of an action, and the end of the wait for the second Ctrl+X, wait for a key
		// that has begun, which may be Esc.
		acting, expiry := l.acting, l.expiry
		if len(t.pending) > 0 {
			acting, expiry = nil, nil
		}
		var picked session.Session
		var err error
		done, draw := false, true
		select {
		case ready := <-in.waiting:
			done, err = l.typed(in, &t, ready)
		case <-t.escape:
			done = l.press(t.expire())
		case result := <-acting:
			picked = l.settle(result)
			done = picked.Name != ""
		case <-expiry:
			l.armOver(in)
		case <-l.hold:
			l.holdOver(in)
			draw = false // nothing has changed
		case <-in.resized:
			l.measure()
		case sig := <-in.stops:
			err = stopped(sig)
		case <-in.paused:
			err = l.suspend(in, tty)
		case <-in.continued:
			// Back from a SIGSTOP the list did not see coming: the shell may have put back its own
			// mode, as bash does, and written over the list.
			err = l.takeBack(tty)
		}
		if done || err != nil {
			return picked, err
		}
		// Nothing is drawn halfway through a key, nor before the keys that came with it.
		if draw && len(t.pending) == 0 && !in.more() {
			if err := output.Print(l.frame()); err != nil {
				return session.Session{}, err
			}
		}
	}
}

// typed reads the byte that watch saw waiting, and presses the keys it ends in turn until one
// leaves. It reports whether the user left.
func (l *list) typed(in *input, t *typing, ready error) (bool, error) {
	b, err := in.take(ready)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(t.add(b), l.press), nil
}

// armOver ends the wait for the second Ctrl+X. A byte waiting to be read may have come within it,
// while cld was held up: the kill then stays armed for the key it begins (decision 15.1).
func (l *list) armOver(in *input) {
	l.expiry = nil
	if !in.more() {
		l.disarm()
	}
}

// holdOver ends the hold after a kill. A byte waiting to be read may have come within it, while
// cld was held up. The hold then starts again, and the byte, read next, ends it or starts it again.
func (l *list) holdOver(in *input) {
	l.hold = nil
	if in.more() {
		l.hold = time.After(repeatWait)
	}
}
