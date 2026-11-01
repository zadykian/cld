package tabs

import (
	"fmt"
	"os"
	"time"
)

// logger writes the watcher's log, its stderr, a line per event after the time in UTC. An error
// that repeats is logged once, and counted until another line comes.
type logger struct {
	last   string // the error logged last, while it repeats
	repeat int    // how many times it came
}

// line logs text, after the count of the error before it.
func (l *logger) line(text string) {
	l.flush()
	l.write(text)
}

// failed logs text, an error, unless it repeats the last.
func (l *logger) failed(text string) {
	if text == l.last {
		l.repeat++
		return
	}
	l.line(text)
	l.last, l.repeat = text, 1
}

// flush logs how many times the last error came, where more than once, and forgets it.
func (l *logger) flush() {
	if l.repeat > 1 {
		l.write(fmt.Sprintf("the last error repeated %d times", l.repeat))
	}
	l.last, l.repeat = "", 0
}

// write writes text after the time in UTC. A write that fails is passed over: the log is the
// watcher's only voice, and a full disk must not stop the record.
func (l *logger) write(text string) {
	_, _ = fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().UTC().Format(time.RFC3339), text)
}
