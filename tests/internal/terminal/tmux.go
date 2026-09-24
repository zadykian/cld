package terminal

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

var outerSockets atomic.Int64

// tmuxTerminal is the baseline terminal: a pane of an outer tmux server on its own socket, typing
// what an xterm-compatible terminal with CSI u keys would send. The outer server keeps OSC 52
// copies in its paste buffers, pipes the program's output to a file, and keeps the pane after the
// program exits so that the state it left behind can still be inspected.
type tmuxTerminal struct {
	unsupported
	sandbox       *sandbox.Sandbox
	socket        string
	columns, rows int
	started       bool
}

const outerPane = "outer:0.0"

// Input an xterm-compatible terminal sends; the outer tmux types it as raw bytes because its own
// key names and focus reporting differ between versions and need an attached client.
var xtermInput = map[string]string{
	"S-Enter":   "\x1b[13;2u",
	"focus-in":  "\x1b[I",
	"focus-out": "\x1b[O",
	"wheel-up":  "\x1b[<64;10;10M",
}

func newTmux(t testing.TB, s *sandbox.Sandbox) *tmuxTerminal {
	return &tmuxTerminal{
		unsupported: unsupported{t: t, name: "tmux"},
		sandbox:     s,
		socket:      "outer" + strconv.FormatInt(outerSockets.Add(1), 10),
		columns:     120,
		rows:        40,
	}
}

func (o *tmuxTerminal) tmux(args ...string) string {
	o.t.Helper()
	cmd := exec.Command("tmux", append([]string{"-L", o.socket}, args...)...)
	cmd.Env = o.sandbox.Environ(nil)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		o.t.Fatalf("outer tmux %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(string(out), "\n")
}

func (o *tmuxTerminal) format(format string) string {
	o.t.Helper()
	return o.tmux("display", "-p", "-t", outerPane, format)
}

func (o *tmuxTerminal) Start(argv []string, env map[string]string, dir string) {
	o.t.Helper()
	args := []string{
		"-f", "/dev/null",
		"set", "-g", "default-terminal", "xterm-256color", ";",
		"set", "-g", "set-clipboard", "on", ";",
		"set", "-g", "remain-on-exit", "on", ";",
		"set", "-g", "status", "off", ";",
		// A real terminal does not set TMUX, the outer server does: it is unset first and set
		// again only when env asks for it.
		"new-session", "-d", "-x", strconv.Itoa(o.columns), "-y", strconv.Itoa(o.rows), "-s", "outer",
		"-c", dir, "env", "-u", "TMUX",
	}
	for name, value := range env {
		args = append(args, name+"="+value)
	}
	args = append(args, argv...)
	// In the same command list as new-session, so the pipe is in place before the first output.
	args = append(args, ";", "pipe-pane", "-O", "-t", outerPane, "cat >> '"+o.outputFile()+"'")
	o.tmux(args...)
	o.started = true
}

func (o *tmuxTerminal) outputFile() string {
	return filepath.Join(o.sandbox.Root, o.socket+".out")
}

func (o *tmuxTerminal) Keys(keys ...string) {
	o.t.Helper()
	for _, key := range keys {
		if input, found := xtermInput[key]; found {
			o.send(input)
		} else {
			o.tmux("send-keys", "-t", outerPane, key)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Paste goes through a paste buffer: paste-buffer -p brackets the text only if the program asked
// for bracketed paste, and turns line feeds into carriage returns, as terminals do. Since tmux 3.7
// it also writes control characters as ^X unless given -S, which a terminal does not do.
func (o *tmuxTerminal) Paste(text string) {
	o.t.Helper()
	o.tmux("set-buffer", "-b", "paste", "--", text)
	args := []string{"paste-buffer", "-p", "-d", "-b", "paste", "-t", outerPane}
	if pasteRaw.MatchString(o.tmux("list-commands", "paste-buffer")) {
		args = append(args, "-S")
	}
	o.tmux(args...)
}

// pasteRaw matches the usage of a paste-buffer that knows -S.
var pasteRaw = regexp.MustCompile(`\[-[a-zA-Z]*S[a-zA-Z]*\]`)

func (o *tmuxTerminal) send(input string) {
	o.t.Helper()
	args := []string{"send-keys", "-t", outerPane, "-H"}
	for _, b := range []byte(input) {
		args = append(args, hex.EncodeToString([]byte{b}))
	}
	o.tmux(args...)
}

func (o *tmuxTerminal) WheelUp() {
	o.t.Helper()
	o.send(xtermInput["wheel-up"])
}

func (o *tmuxTerminal) Focus(focused bool) {
	o.t.Helper()
	if focused {
		o.send(xtermInput["focus-in"])
	} else {
		o.send(xtermInput["focus-out"])
	}
}

func (o *tmuxTerminal) Resize(columns, rows int) {
	o.t.Helper()
	if !o.started {
		o.columns, o.rows = columns, rows
		return
	}
	o.tmux("resize-window", "-t", "outer", "-x", strconv.Itoa(columns), "-y", strconv.Itoa(rows))
}

func (o *tmuxTerminal) Title() string {
	o.t.Helper()
	return o.format("#{pane_title}")
}

func (o *tmuxTerminal) Screen() string {
	o.t.Helper()
	return o.tmux("capture-pane", "-p", "-t", outerPane)
}

func (o *tmuxTerminal) Styled() string {
	o.t.Helper()
	return o.tmux("capture-pane", "-p", "-e", "-t", outerPane)
}

func (o *tmuxTerminal) Output() []byte {
	o.t.Helper()
	data, err := os.ReadFile(o.outputFile())
	if err != nil && !os.IsNotExist(err) {
		o.t.Fatal(err)
	}
	return data
}

func (o *tmuxTerminal) Modes() Modes {
	o.t.Helper()
	flags := strings.Fields(o.format("#{alternate_on} #{mouse_any_flag}"))
	return Modes{AltScreen: flags[0] == "1", Mouse: flags[1] == "1"}
}

func (o *tmuxTerminal) Clipboard() string {
	o.t.Helper()
	sandbox.WaitFor(o.t, 10*time.Second, "an OSC 52 copy in the outer paste buffers", func() bool {
		return o.tmux("list-buffers") != ""
	})
	return o.tmux("show-buffer")
}

func (o *tmuxTerminal) Running() bool {
	o.t.Helper()
	return o.format("#{pane_dead}") == "0"
}

func (o *tmuxTerminal) Close() {
	cmd := exec.Command("tmux", "-L", o.socket, "kill-server")
	cmd.Env = o.sandbox.Environ(nil)
	_ = cmd.Run()
}
