package tests

import (
	"bytes"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// The terminal contract: what cld promises wherever it runs, checked against every terminal in
// CLD_TERMINALS. Where terminals legitimately differ, the difference is an expectation below
// rather than a skip, so a terminal gaining or losing support fails a test.

type expectation struct {
	// features tmux must detect in the terminal (#{client_termfeatures}).
	features []string
	// shiftEnter lists what claude may receive for Shift+Enter; plain Enter is always "\r".
	shiftEnter []string
}

var expectations = map[string]expectation{
	// The outer tmux announces itself through XTVERSION, and the inner tmux knows its features,
	// hyperlinks among them; extkeys comes from cld's terminal-features entry for xterm*.
	"tmux": {
		features:   []string{"clipboard", "extkeys", "focus", "hyperlinks", "mouse", "title"},
		shiftEnter: []string{"\x1b[13;2u", "\x1b[27;2;13~"},
	},
	// JediTerm answers no XTVERSION, so tmux falls back to its defaults for xterm*: they claim
	// clipboard and focus, which the emulator ignores (see the skipped tests), and cld adds
	// extkeys and hyperlinks. The emulator ignores the modifyOtherKeys request that follows;
	// Shift+Enter becomes ESC CR through its own setting, which tmux passes on as Meta+Enter.
	"jediterm": {
		features:   []string{"bpaste", "clipboard", "extkeys", "focus", "hyperlinks", "title"},
		shiftEnter: []string{"\x1b\r"},
	},
}

// startContract starts cld in the named terminal and waits for claude and the attached client.
func startContract(t *testing.T, name string) (*sandbox.Sandbox, terminal.Terminal, *sandbox.Probe) {
	t.Helper()
	s := sandbox.New(t)
	term := startCld(t, s, name, nil, "join", "-s", "contract")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	waitScreen(t, term, "probe --name cld-contract")
	return s, term, probe
}

// between types keys between two markers and returns what claude received between them.
func between(t *testing.T, term terminal.Terminal, probe *sandbox.Probe, keys ...string) string {
	t.Helper()
	markers := func() int { return bytes.Count(probe.Input(), []byte(">")) }
	before := markers()
	term.Keys(append(append([]string{"<"}, keys...), ">")...)
	sandbox.WaitFor(t, 10*time.Second, "the keys to reach claude", func() bool { return markers() > before })
	input := probe.Input()
	end := bytes.LastIndexByte(input, '>')
	start := bytes.LastIndexByte(input[:end], '<')
	return string(input[start+1 : end])
}

// C1: the tab shows the session name after claude's marker, whatever claude sets as its own title:
// ✳, and while claude is busy - from the prompt it was given, as its hooks tell tmux (see
// TestStatusHooks) - ◐ and ◑ in turn, a second each, until its turn is done; then " [w]" after
// the name while claude is in a linked git worktree (see TestWorktreeHooks).
func TestContractTitle(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, term, probe := startContract(t, name)
		probe.Send("title claude's own title")
		sandbox.WaitFor(t, 10*time.Second, "claude's title in its pane", func() bool {
			return s.Format("cld-contract", "#{pane_title}") == "claude's own title"
		})
		if title := term.Title(); title != "\u2733 cld-contract" {
			t.Errorf("terminal title %q, want %q", title, "\u2733 cld-contract")
		}
		probe.Hook("UserPromptSubmit", `{"prompt":"go"}`)
		for _, marker := range []string{"\u25d0", "\u25d1", "\u25d0", "\u25d1"} {
			want := marker + " cld-contract"
			sandbox.WaitFor(t, 5*time.Second, "the terminal title "+strconv.Quote(want), func() bool {
				return term.Title() == want
			})
		}
		probe.Hook("Stop", `{}`)
		sandbox.WaitFor(t, 5*time.Second, "the terminal title back at \u2733", func() bool {
			return term.Title() == "\u2733 cld-contract"
		})
		// The job that would turn the marker next finds claude idle.
		time.Sleep(1500 * time.Millisecond)
		if title := term.Title(); title != "\u2733 cld-contract" {
			t.Errorf("terminal title %q a while after the turn, want %q", title, "\u2733 cld-contract")
		}
		// In a linked git worktree the title ends in [w], busy or not.
		gitInit(t, s)
		worktree := gitWorktree(t, s, "contract")
		probe.Send("cd " + worktree)
		probe.Hook("CwdChanged", `{"new_cwd":"`+worktree+`"}`)
		sandbox.WaitFor(t, 5*time.Second, "[w] in the terminal title", func() bool {
			return term.Title() == "✳ cld-contract [w]"
		})
		probe.Hook("UserPromptSubmit", `{"prompt":"go"}`)
		for _, marker := range []string{"◐", "◑"} {
			want := marker + " cld-contract [w]"
			sandbox.WaitFor(t, 5*time.Second, "the terminal title "+strconv.Quote(want), func() bool {
				return term.Title() == want
			})
		}
		// A claude that fails in a turn leaves it busy, and its pane on screen: the tab says ✳.
		probe.Send("exit 1")
		sandbox.WaitFor(t, 5*time.Second, "the terminal title back at ✳ once claude failed", func() bool {
			return term.Title() == "✳ cld-contract [w]"
		})
	})
}

// C2: what tmux learned about the terminal decides what it forwards to claude.
func TestContractClientFeatures(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, _, _ := startContract(t, name)
		client := s.MustTmux("cld-contract", "list-clients", "-F", "#{client_termname}|#{client_termtype}|#{client_termfeatures}")
		t.Logf("client: %s", client)
		detected := strings.Split(strings.SplitN(client, "|", 3)[2], ",")
		for _, feature := range expectations[name].features {
			if !slices.Contains(detected, feature) {
				t.Errorf("tmux does not detect %q in %s", feature, name)
			}
		}
	})
}

// C3: Shift+Enter reaches claude distinct from Enter.
func TestContractShiftEnter(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		if enter := between(t, term, probe, "Enter"); enter != "\r" {
			t.Errorf("Enter arrives as %s, want \"\\r\"", strconv.Quote(enter))
		}
		shiftEnter := between(t, term, probe, "S-Enter")
		if !slices.Contains(expectations[name].shiftEnter, shiftEnter) {
			t.Errorf("Shift+Enter arrives as %s, want one of %q", strconv.Quote(shiftEnter), expectations[name].shiftEnter)
		}
	})
}

// C3: tmux asks the terminal for modified keys - what makes Shift+Enter distinct in terminals
// that send it only on request - and takes the request back when the client detaches.
func TestContractModifiedKeys(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
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
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		if got := between(t, term, probe, "C-b", "C-_", "C-q", "C-q"); got != "\x02\x1f\x11" {
			t.Errorf("C-b C-_ C-q C-q arrives as %s, want \"\\x02\\x1f\\x11\"", strconv.Quote(got))
		}
	})
}

// C3, C7: the prefix and d detach, claude keeps running, and the terminal is left clean.
func TestContractDetach(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, term, probe := startContract(t, name)
		waitModes(t, term, "while attached", "the alternate screen and mouse reporting on", func(modes terminal.Modes) bool {
			return modes.AltScreen && modes.Mouse
		})
		term.Keys("C-q", "d")
		sandbox.WaitFor(t, 10*time.Second, "cld to exit", func() bool { return !term.Running() })
		if modes := term.Modes(); modes.AltScreen || modes.Mouse {
			t.Errorf("modes after detaching %+v, want everything off", modes)
		}
		if !probe.Alive() || !slices.Equal(s.Sessions(), []string{"cld-contract"}) {
			t.Error("claude did not survive the detach")
		}
	})
}

// C4: the mouse wheel reaches claude, which scrolls its transcript with it.
func TestContractMouseWheel(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.WheelUp()
		probe.WaitInput(mark, "\x1b[<64;")
	})
}

// C4: over a program that draws in the main screen without the mouse - claude outside fullscreen,
// a shell - the wheel scrolls the pane's history in tmux's copy mode. This is what mouse on is
// for: claude's fullscreen transcript gets the wheel either way, as tmux passes its mouse
// reporting on. (Since tmux 3.6 a program in the alternate screen gets the wheel, mouse or not.)
func TestContractWheelScrollsHistory(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, term, probe := startContract(t, name)
		probe.Send("inline")
		sandbox.WaitFor(t, 10*time.Second, "claude to draw inline", func() bool {
			return s.Format("cld-contract", "#{mouse_any_flag} #{alternate_on}") == "0 0"
		})
		term.WheelUp()
		sandbox.WaitFor(t, 10*time.Second, "the pane to enter copy mode", func() bool {
			return s.Format("cld-contract", "#{pane_in_mode}") == "1"
		})
	})
}

// C4: a click with a modifier reaches claude whole, the press and the release: claude opens the
// link under a Ctrl+click as it is let go, and only after a press it saw. tmux binds a Ctrl+click
// and an Alt+right-click on a pane whether or not its program takes the mouse, and cld unbinds
// them (see TestServerOptions).
func TestContractClicks(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		for _, click := range []struct{ key, button string }{{"C-MouseDown1", "16"}, {"M-MouseDown3", "10"}} {
			mark := probe.Mark()
			term.Click(click.key)
			whole := regexp.MustCompile(`\x1b\[<` + click.button + `;\d+;\d+M\x1b\[<` + click.button + `;\d+;\d+m`)
			sandbox.WaitFor(t, 10*time.Second, click.key+" pressed and let go in the probe input", func() bool {
				return whole.Match(probe.Input()[mark:])
			})
		}
	})
}

// C4: focus changes reach claude.
func TestContractFocus(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.Focus(false)
		probe.WaitInput(mark, "\x1b[O")
		mark = probe.Mark()
		term.Focus(true)
		probe.WaitInput(mark, "\x1b[I")
	})
}

// C5: a copy claude makes through tmux passthrough lands in the terminal's clipboard.
func TestContractClipboard(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		probe.Send("osc52 copied by claude")
		if clipboard := term.Clipboard(); clipboard != "copied by claude" {
			t.Errorf("clipboard %q, want %q", clipboard, "copied by claude")
		}
	})
}

// C5: so does a copy claude makes with tmux load-buffer -w, its way of copying inside tmux.
func TestContractClipboardThroughTmux(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		probe.Send("loadbuffer copied through tmux")
		if clipboard := term.Clipboard(); clipboard != "copied through tmux" {
			t.Errorf("clipboard %q, want %q", clipboard, "copied through tmux")
		}
	})
}

// C5: a notification claude sends on the channel its setting preferredNotifChannel names -
// iTerm2's OSC 9, kitty's OSC 99 or Ghostty's OSC 777, in tmux passthrough, or the bell - reaches
// the terminal, the passthrough taken off. (Its default channel, auto, sends none under tmux.)
func TestContractNotifications(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
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

// C5: a link claude writes - OSC 8, as claude marks file paths and URLs under tmux - reaches the
// terminal as a link, not only as its text: tmux writes it only to a terminal with the hyperlinks
// feature, which the outer tmux has from its XTVERSION and JediTerm from cld's terminal-features
// entry for xterm*.
func TestContractLink(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		_, term, probe := startContract(t, name)
		probe.Send("link https://example.com/cld the link")
		link := regexp.MustCompile(`\x1b\]8;[^;\x07\x1b]*;https://example\.com/cld(?:\x07|\x1b\\)the link`)
		sandbox.WaitFor(t, 10*time.Second, "the link in the terminal's output", func() bool {
			return link.Match(term.Output())
		})
	})
}

// C8: a paste reaches claude whole and bracketed, so claude inserts it instead of submitting it
// line by line, and a prefix key inside it is text rather than a tmux binding.
func TestContractPaste(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.Paste("first line\nsecond line \x11d")
		probe.WaitInput(mark, "\x1b[200~first line\rsecond line \x11d\x1b[201~")
		if !term.Running() || !slices.Equal(s.Sessions(), []string{"cld-contract"}) {
			t.Error("cld did not stay attached through the paste")
		}
	})
}

// C9: claude exiting - /exit - ends its session, and with it the session's server; cld returns,
// and the terminal is left clean although claude restored none of its modes.
func TestContractClaudeExit(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		s, term, probe := startContract(t, name)
		probe.Send("exit")
		sandbox.WaitFor(t, 10*time.Second, "cld to exit", func() bool { return !term.Running() })
		if modes := term.Modes(); modes.AltScreen || modes.Mouse {
			t.Errorf("modes after claude exited %+v, want everything off", modes)
		}
		sandbox.WaitFor(t, 10*time.Second, "tmux to turn modified keys off", func() bool {
			return !modifiedKeysOn(term.Output())
		})
		sandbox.WaitFor(t, 10*time.Second, "the tmux server to exit", func() bool {
			_, err := s.Tmux("cld-contract", "list-sessions")
			return err != nil
		})
	})
}

// C10: the session list reads the terminal's own keys, not tmux's: Down and Enter join the second
// session, with the terminal handed to tmux as it was before the list, which a detach shows;
// Ctrl+X twice kills the selected session, which then shows as ended; Esc leaves the terminal as
// it was. (Whether a JetBrains
// IDE passes Esc and Ctrl+X on to its terminal depends on its keymap, which the driver cannot see;
// Ctrl+C also leaves.)
func TestContractList(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) {
		t.Run("join", func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a", "b")
			term := terminal.New(t, name, s)
			list := startList(t, s, term, listScript, nil)
			waitScreen(t, term, listHints)
			if selected := selectedRow(term); selected != "a" {
				t.Errorf("row %q selected, want a", selected)
			}
			term.Keys("Down")
			sandbox.WaitFor(t, 10*time.Second, "the selection to move to b", func() bool { return selectedRow(term) == "b" })
			term.Keys("Enter")
			waitScreen(t, term, "probe --name cld-b")
			if title := term.Title(); title != "✳ cld-b" {
				t.Errorf("terminal title %q, want %q", title, "✳ cld-b")
			}
			term.Keys("C-q", "d")
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			list.checkRestored(t, term)
		})
		t.Run("kill", func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			probes := detachedSessions(t, s, "a", "b")
			term := terminal.New(t, name, s)
			list := startList(t, s, term, listScript, nil)
			waitScreen(t, term, listHints)
			armThen(t, term, func() {
				waitScreen(t, term, killArmed)
				if !probes["a"].Alive() {
					t.Error("the first Ctrl+X killed a")
				}
			}, "C-x")
			sandbox.WaitFor(t, 10*time.Second, "claude a to exit", func() bool { return !probes["a"].Alive() })
			sandbox.WaitFor(t, 10*time.Second, "the list to show a as ended, selected", func() bool {
				return selectedRow(term) == "a" && strings.Contains(term.Screen(), "> a     ended")
			})
			if !probes["b"].Alive() {
				t.Error("claude b exited")
			}
			term.Keys("Escape")
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			list.checkRestored(t, term)
		})
		t.Run("leave", func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			detachedSessions(t, s, "a", "b")
			term := terminal.New(t, name, s)
			list := startList(t, s, term, listScript, nil)
			waitScreen(t, term, listHints)
			if modes := term.Modes(); !modes.AltScreen || modes.Cursor {
				t.Errorf("modes while the list is open %+v, want the alternate screen and the cursor hidden", modes)
			}
			term.Keys("Escape")
			if code := list.code(t); code != "0" {
				t.Errorf("exit %s, want 0", code)
			}
			list.checkRestored(t, term)
		})
	})
}

// selectedRow is the name on the row the list marks as selected, or "" when it marks none.
func selectedRow(term terminal.Terminal) string {
	for _, line := range strings.Split(term.Screen(), "\n") {
		if row, marked := strings.CutPrefix(line, "> "); marked {
			if fields := strings.Fields(row); len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}

// modifiedKeysOn reports whether the last modifyOtherKeys sequence in a terminal's output turns
// modified keys on (CSI > 4 ; 1 m or CSI > 4 ; 2 m) rather than off (CSI > 4 m).
func modifiedKeysOn(output []byte) bool {
	on := max(bytes.LastIndex(output, []byte("\x1b[>4;1m")), bytes.LastIndex(output, []byte("\x1b[>4;2m")))
	return on > bytes.LastIndex(output, []byte("\x1b[>4m"))
}

// osc matches an OSC sequence - a title, an OSC 52 copy - which a BEL or ST ends.
var osc = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

// bells counts the bells rung in a terminal's output: the BELs that end no OSC sequence.
func bells(output []byte) int {
	return bytes.Count(osc.ReplaceAll(output, nil), []byte("\x07"))
}
