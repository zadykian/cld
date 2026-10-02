package terminal

import (
	"encoding/hex"
	"strconv"
	"time"
)

// xtermInput is what an xterm-compatible terminal sends, typed as raw bytes: the outer tmux's key
// names and focus reports differ between versions and need an attached client. Alt is Esc before
// the key, as with xterm's metaSendsEscape, so Alt+Esc is Esc twice at once. Alt+Up is Esc and Up,
// as rxvt sends Alt with any key, where xterm sends CSI 1;3A.
var xtermInput = map[string]string{
	"S-Enter":   "\x1b[13;2u",
	"S-Up":      "\x1b[1;2A",
	"M-j":       "\x1bj",
	"M-Escape":  "\x1b\x1b",
	"M-Up":      "\x1b\x1b[A",
	"focus-in":  "\x1b[I",
	"focus-out": "\x1b[O",
	"wheel-up":  "\x1b[<64;10;10M",
}

func (o *tmuxTerminal) Keys(keys ...string) {
	o.t.Helper()
	for _, key := range keys {
		o.key(key)
		time.Sleep(50 * time.Millisecond)
	}
}

func (o *tmuxTerminal) key(key string) {
	o.t.Helper()
	o.tmux(typeKey(key)...)
}

// typeKey is the tmux command that types key.
func typeKey(key string) []string {
	if input, found := xtermInput[key]; found {
		return typeRaw(input)
	}
	return []string{"send-keys", "-t", outerPane, key}
}

// Hold types the keys in one command list that the outer server times. Its run-shell -d with no
// command only waits, and the list goes on once it has (tmux 3.7c). A tmux client per key would
// space them out by its start, a tenth of a second or more, and far more under load.
func (o *tmuxTerminal) Hold(key string, delay, interval time.Duration, repeats int) {
	o.t.Helper()
	args, wait := typeKey(key), delay
	for range repeats {
		args = append(args, ";", "run-shell", "-d", strconv.FormatFloat(wait.Seconds(), 'f', -1, 64), ";")
		args, wait = append(args, typeKey(key)...), interval
	}
	o.tmux(args...)
}

// Paste goes through a paste buffer: paste-buffer -p brackets the text only if the program asked
// for bracketed paste, and turns line feeds into carriage returns, as terminals do. -S keeps 3.7
// from writing control characters as ^X; 3.5a writes them as they are, and refuses it (see
// docs/design/findings/tmux-terminal.md).
func (o *tmuxTerminal) Paste(text string) {
	o.t.Helper()
	args := []string{"paste-buffer", "-p", "-d", "-b", "paste", "-t", outerPane}
	if !o.older(3, 7) {
		args = append(args, "-S")
	}
	o.tmux("set-buffer", "-b", "paste", "--", text)
	o.tmux(args...)
}

func (o *tmuxTerminal) send(input string) {
	o.t.Helper()
	o.tmux(typeRaw(input)...)
}

// typeRaw is the tmux command that types input as raw bytes.
func typeRaw(input string) []string {
	args := []string{"send-keys", "-t", outerPane, "-H"}
	for _, b := range []byte(input) {
		args = append(args, hex.EncodeToString([]byte{b}))
	}
	return args
}

func (o *tmuxTerminal) WheelUp() {
	o.t.Helper()
	o.send(xtermInput["wheel-up"])
}

// Click types the press and the release as an xterm reports them in SGR mode, which tmux asks
// for, over the cell the wheel turns over.
func (o *tmuxTerminal) Click(key string) {
	o.t.Helper()
	button := strconv.Itoa(mouseButton(o.t, key))
	o.send("\x1b[<" + button + ";10;10M" + "\x1b[<" + button + ";10;10m")
}

func (o *tmuxTerminal) Focus(focused bool) {
	o.t.Helper()
	if focused {
		o.send(xtermInput["focus-in"])
	} else {
		o.send(xtermInput["focus-out"])
	}
}
