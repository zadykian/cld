package tests

import (
	"bytes"
	"regexp"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// C5: a copy claude makes through tmux passthrough lands in the terminal's clipboard.
func TestContractClipboard(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		probe.Send("osc52 copied by claude")
		if clipboard := term.Clipboard(); clipboard != "copied by claude" {
			t.Errorf("clipboard %q, want %q", clipboard, "copied by claude")
		}
	})
}

// C5: so does a copy claude makes with tmux load-buffer -w, its way of copying inside tmux.
func TestContractClipboardThroughTmux(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		probe.Send("loadbuffer copied through tmux")
		if clipboard := term.Clipboard(); clipboard != "copied through tmux" {
			t.Errorf("clipboard %q, want %q", clipboard, "copied through tmux")
		}
	})
}

// C5: claude's notifications reach the terminal: iTerm2's OSC 9, kitty's OSC 99 and Ghostty's
// OSC 777 taken out of tmux's passthrough, and the bell. The probe sends each on the channel that
// claude's setting preferredNotifChannel names; its default, auto, sends none under tmux
// (decision 29).
func TestContractNotifications(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		for _, notification := range []struct{ channel, what, want string }{
			{"iterm2", "iTerm2's OSC 9", "\x1b]9;claude needs you\x07"},
			{"kitty", "kitty's OSC 99", "\x1b]99;i=1:p=body;claude needs you\x07"},
			{"ghostty", "Ghostty's OSC 777", "\x1b]777;notify;Claude Code;claude needs you\x07"},
		} {
			probe.Send("notify " + notification.channel + " claude needs you")
			sandbox.WaitFor(t, 10*time.Second, notification.what+" in the terminal", func() bool {
				return bytes.Contains(term.Output(), []byte(notification.want))
			})
		}
		if bytes.Contains(term.Output(), []byte("\x1bPtmux;")) {
			t.Error("the terminal got tmux's passthrough, not what it wraps")
		}
		rung := bells(term.Output())
		probe.Send("notify terminal_bell")
		sandbox.WaitFor(t, 10*time.Second, "the bell in the terminal", func() bool {
			return bells(term.Output()) > rung
		})
	})
}

// C5: an OSC 8 link claude writes reaches the terminal as a link, not only as its text. tmux writes
// links only to a terminal with the hyperlinks feature, which JediTerm gets from cld's entry for
// xterm* (decision 30).
func TestContractLink(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		probe.Send("link https://example.com/cld the link")
		link := regexp.MustCompile(`\x1b\]8;[^;\x07\x1b]*;` +
			`https://example\.com/cld(?:\x07|\x1b\\)the link`)
		sandbox.WaitFor(t, 10*time.Second, "the link in the terminal's output", func() bool {
			return link.Match(term.Output())
		})
	})
}

// osc matches an OSC sequence - a title, an OSC 52 copy - which a BEL or ST ends.
var osc = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

// bells counts the bells rung in a terminal's output: the BELs that end no OSC sequence.
func bells(output []byte) int {
	return bytes.Count(osc.ReplaceAll(output, nil), []byte("\x07"))
}
