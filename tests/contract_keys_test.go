package tests

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// C3: Shift+Enter reaches claude distinct from Enter.
func TestContractShiftEnter(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		if enter := between(t, term, probe, "Enter"); enter != "\r" {
			t.Errorf("Enter arrives as %s, want \"\\r\"", strconv.Quote(enter))
		}
		shiftEnter := between(t, term, probe, "S-Enter")
		if !slices.Contains(expectations[name].shiftEnter, shiftEnter) {
			t.Errorf("Shift+Enter arrives as %s, want one of %q",
				strconv.Quote(shiftEnter), expectations[name].shiftEnter)
		}
	})
}

// C3: tmux asks the terminal for modified keys - what makes Shift+Enter distinct in terminals
// that send it only on request - and takes the request back when the client detaches.
func TestContractModifiedKeys(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, _ := startContract(t, name)
		sandbox.WaitFor(t, 10*time.Second, "tmux to ask the terminal for modified keys", func() bool {
			return modifiedKeysOn(term.Output())
		})
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to exit", func() bool { return !term.Running() })
		sandbox.WaitFor(t, 10*time.Second, "tmux to turn modified keys off", func() bool {
			return !modifiedKeysOn(term.Output())
		})
	})
}

// C3: Ctrl keys claude binds pass through - C-b backgrounds a task, C-_ undoes an edit (what a
// Cmd+Z mapping sends) - and the prefix pressed twice sends itself.
func TestContractControlKeys(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		keys := []string{"C-b", "C-_", "C-q", "C-q"}
		if got := between(t, term, probe, keys...); got != "\x02\x1f\x11" {
			t.Errorf("%s arrives as %s, want \"\\x02\\x1f\\x11\"",
				strings.Join(keys, " "), strconv.Quote(got))
		}
	})
}

// C8: a paste reaches claude whole and bracketed, so claude inserts it instead of submitting it
// line by line. A prefix key inside it stays text, not a tmux binding.
func TestContractPaste(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		s, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.Paste("first line\nsecond line \x11d")
		probe.WaitInput(mark, expectations[name].pasted)
		if !term.Running() || !slices.Equal(s.Sessions(), []string{"cld-contract"}) {
			t.Error("cld did not stay attached through the paste")
		}
	})
}

// modifiedKeysOn reports whether the last modifyOtherKeys sequence in a terminal's output turns
// modified keys on rather than off. On is CSI > 4 ; 1 m or CSI > 4 ; 2 m, and off CSI > 4 m.
func modifiedKeysOn(output []byte) bool {
	on := max(bytes.LastIndex(output, []byte("\x1b[>4;1m")),
		bytes.LastIndex(output, []byte("\x1b[>4;2m")))
	return on > bytes.LastIndex(output, []byte("\x1b[>4m"))
}
