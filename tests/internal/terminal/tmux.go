package terminal

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

var outerSockets atomic.Int64

// tmuxTerminal is the baseline terminal: a pane of an outer tmux server on its own socket, typing
// what an xterm-compatible terminal with CSI u keys would send. The outer server keeps OSC 52
// copies in its paste buffers and pipes the program's output to a file. It keeps the pane once the
// program exits, so that the state left behind can still be inspected.
type tmuxTerminal struct {
	unsupported
	sandbox       *sandbox.Sandbox
	socket        string
	columns, rows int
	started       bool
}

const outerPane = "outer:0.0"

func newTmux(tb testing.TB, s *sandbox.Sandbox) *tmuxTerminal {
	tb.Helper()
	return &tmuxTerminal{
		unsupported: unsupported{t: tb, name: "tmux"},
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
		o.t.Fatalf("outer tmux %s: %v: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
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
		// A real terminal sets no TMUX, though the outer server does: env -u drops it, and only
		// an entry in env sets it again.
		"new-session", "-d", "-x", strconv.Itoa(o.columns), "-y", strconv.Itoa(o.rows), "-s", "outer",
		"-c", literal(unexpanded(dir)), "env", "-u", "TMUX",
	}
	for name, value := range env {
		args = append(args, literal(name+"="+value))
	}
	for _, word := range argv {
		args = append(args, literal(word))
	}
	// In new-session's command list, so that the pipe is in place before the first output. dd
	// writes each read as it comes. uutils' cat (0.8.0, Ubuntu 26.04) holds the last read back
	// until the next, so the log would miss what the program wrote last.
	pipe := "dd bs=65536 2>/dev/null >> '" + o.outputFile() + "'"
	args = append(args, ";", "pipe-pane", "-O", "-t", outerPane, pipe)
	o.tmux(args...)
	o.started = true
}

// literal is word as the outer tmux takes it back from its command line, as cld's own tmux does
// (see literal in internal/session). tmux ends a command at a word that ends in ";", and turns a
// "\;" at the end of a word into ";".
func literal(word string) string {
	if before, found := strings.CutSuffix(word, ";"); found {
		return before + `\;`
	}
	return word
}

// unexpanded is text as a tmux format that expands to text itself, as for cld's own tmux (see
// unexpanded in internal/session). The outer tmux expands new-session's -c as a format too, in
// which "##" is a "#".
func unexpanded(text string) string {
	return strings.ReplaceAll(text, "#", "##")
}

func (o *tmuxTerminal) outputFile() string {
	return filepath.Join(o.sandbox.Root, o.socket+".out")
}

// older reports whether the outer server runs a tmux older than major.minor, from #{version}. It
// reads "3.7c" as 3.7 and a development build's "next-3.8" as 3.8, and one without a version,
// "master", as not older.
func (o *tmuxTerminal) older(major, minor int) bool {
	o.t.Helper()
	match := regexp.MustCompile(`([0-9]+)\.([0-9]+)`).FindStringSubmatch(o.format("#{version}"))
	if match == nil {
		return false
	}
	var version []int
	for _, digits := range match[1:] {
		n, _ := strconv.Atoi(digits) //nolint:errcheck // the pattern matched only digits
		version = append(version, n)
	}
	return slices.Compare(version, []int{major, minor}) < 0
}

func (o *tmuxTerminal) Resize(columns, rows int) {
	o.t.Helper()
	if !o.started {
		o.columns, o.rows = columns, rows
		return
	}
	o.tmux("resize-window", "-t", "outer", "-x", strconv.Itoa(columns), "-y", strconv.Itoa(rows))
}

// Freeze stops the outer tmux server with SIGSTOP: it then reads nothing from the pane and
// answers nothing. The server goes on when thaw is called, or when the test ends.
func (o *tmuxTerminal) Freeze() (thaw func()) {
	o.t.Helper()
	pid, err := strconv.Atoi(o.format("#{pid}"))
	if err != nil {
		o.t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		o.t.Fatal(err)
	}
	thaw = func() {
		_ = syscall.Kill(pid, syscall.SIGCONT) //nolint:errcheck // fails only once the server has gone
	}
	o.t.Cleanup(thaw)
	return thaw
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
	flags := strings.Fields(o.format("#{alternate_on} #{mouse_any_flag} #{cursor_flag}"))
	return Modes{AltScreen: flags[0] == "1", Mouse: flags[1] == "1", Cursor: flags[2] == "1"}
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
	_ = cmd.Run() //nolint:errcheck // a server never started, or gone, has nothing to kill
}
