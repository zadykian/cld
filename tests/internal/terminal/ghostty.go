//go:build ghostty

package terminal

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	ghostty "go.mitchellh.com/libghostty"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The cell size the driver reports, in pixels, as a font of 8 by 16 would give.
const cellWidth, cellHeight = 8, 16

// ghosttyTerminal is Ghostty's terminal core, libghostty-vt, on a pty of the test's. The program's
// output goes through Ghostty's parser, and keys, the mouse and pastes through its encoders. The
// app's keybindings and link clicks stay unchecked (decision 54.1). mu guards the fields below it,
// libghostty-vt among them, which the reader and the test share.
type ghosttyTerminal struct {
	t             testing.TB
	controller    *os.File
	path          string
	cmd           *exec.Cmd
	mu            sync.Mutex
	columns, rows int
	term          *ghostty.Terminal
	keys          *ghostty.KeyEncoder
	mouse         *ghostty.MouseEncoder
	output        bytes.Buffer
	clipboard     string
	// reporting is whether the program asks for focus reports, unfocused the focus to report,
	// and tail the end of the last output, where a request for them may begin.
	reporting, unfocused bool
	tail                 []byte
	// exited closes once the program has exited, drained once the reader has fed everything
	// the program wrote; gate stops the reader while a Freeze holds it.
	exited, drained chan struct{}
	gate            sync.Mutex
}

// The driver links libghostty-vt, so only a build with the tag ghostty has it (decision 54.3).
func init() {
	newGhostty = func(tb testing.TB, _ *sandbox.Sandbox) Terminal {
		tb.Helper()
		return &ghosttyTerminal{t: tb, columns: 120, rows: 40}
	}
}

func (g *ghosttyTerminal) Name() string { return "ghostty" }

func (g *ghosttyTerminal) Start(argv []string, env map[string]string, dir string) {
	g.t.Helper()
	home, err := ghosttyDir()
	if err != nil {
		g.t.Fatal(err)
	}
	version, err := ghosttyVersion()
	if err != nil {
		g.t.Fatal(err)
	}
	g.open(version)
	// Close closes the controller, whatever fails from here.
	if g.controller, g.path, err = sandbox.OpenController(); err != nil {
		g.t.Fatal(err)
	}
	tty, err := os.OpenFile(g.path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		g.t.Fatal(err)
	}
	// The program holds the terminal once started: once it and its children close it, reads fail
	// with EIO.
	defer func() {
		if err := tty.Close(); err != nil {
			g.t.Error(err)
		}
	}()
	g.setSize(tty)
	g.cmd = exec.Command(argv[0], argv[1:]...)
	g.cmd.Env = ghosttyEnviron(env, home, version)
	g.cmd.Dir = dir
	g.cmd.Stdin, g.cmd.Stdout, g.cmd.Stderr = tty, tty, tty
	g.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := g.cmd.Start(); err != nil {
		g.t.Fatal(err)
	}
	g.exited, g.drained = make(chan struct{}), make(chan struct{})
	go func() {
		g.cmd.Wait() //nolint:errcheck // Running reports the exit, not its status
		close(g.exited)
	}()
	go g.read()
}

// open creates the terminal and its encoders, with the effects that answer the program.
func (g *ghosttyTerminal) open(version string) {
	g.t.Helper()
	term, err := ghostty.NewTerminal(append(ghosttyOptions(version),
		ghostty.WithSize(uint16(g.columns), uint16(g.rows)),
		ghostty.WithWritePty(func(_ *ghostty.Terminal, data []byte) { g.write(data) }),
		ghostty.WithClipboardWrite(g.copied),
		ghostty.WithSizeReport(func(*ghostty.Terminal) (ghostty.SizeReportSize, bool) {
			return ghostty.SizeReportSize{Rows: uint16(g.rows), Columns: uint16(g.columns),
				CellWidth: cellWidth, CellHeight: cellHeight}, true
		}),
	)...)
	if err != nil {
		g.t.Fatal(err)
	}
	g.term = term
	g.setDefaults()
	if err := term.Resize(uint16(g.columns), uint16(g.rows), cellWidth, cellHeight); err != nil {
		g.t.Fatal(err)
	}
	if g.keys, err = ghostty.NewKeyEncoder(); err != nil {
		g.t.Fatal(err)
	}
	if g.mouse, err = ghostty.NewMouseEncoder(); err != nil {
		g.t.Fatal(err)
	}
	g.mouse.SetOptSize(g.mouseSize())
}

// copied keeps the text of an OSC 52 copy, as the app's default clipboard-write = allow does.
func (g *ghosttyTerminal) copied(
	_ *ghostty.Terminal, write ghostty.ClipboardWrite,
) ghostty.ClipboardWriteReply {
	for _, content := range write.Contents {
		if content.MIME == "text/plain" || content.MIME == "text/plain;charset=utf-8" {
			g.clipboard = string(content.Data)
		}
	}
	return ghostty.ClipboardWriteReply{Result: ghostty.ClipboardWriteSuccess}
}

// read feeds what the program writes to the terminal until the pty fails, as it does once
// nothing has the terminal open.
func (g *ghosttyTerminal) read() {
	defer close(g.drained)
	buf := make([]byte, 65536)
	for {
		g.gate.Lock()
		g.gate.Unlock() //nolint:staticcheck // waits out a Freeze
		n, err := g.controller.Read(buf)
		if n > 0 {
			g.mu.Lock()
			g.output.Write(buf[:n])
			g.term.VTWrite(buf[:n])
			g.reportFocus(buf[:n])
			g.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// write sends bytes to the program, as typed or as the terminal's answers. A program that has
// gone leaves nobody to read them.
func (g *ghosttyTerminal) write(data []byte) {
	_, _ = g.controller.Write(data) //nolint:errcheck // the program may have exited
}

// Freeze stops the reader: the terminal then neither takes in what the program writes nor
// answers it. A read already waiting still takes in one chunk.
func (g *ghosttyTerminal) Freeze() (thaw func()) {
	g.gate.Lock()
	var once sync.Once
	thaw = func() { once.Do(g.gate.Unlock) }
	g.t.Cleanup(thaw)
	return thaw
}

// locked runs read under the lock and fails the test on its error.
func locked[T any](g *ghosttyTerminal, read func() (T, error)) T {
	g.t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	value, err := read()
	if err != nil {
		g.t.Fatal(err)
	}
	return value
}

func (g *ghosttyTerminal) Title() string { return locked(g, g.term.Title) }

func (g *ghosttyTerminal) Screen() string { return g.format(ghostty.FormatterFormatPlain) }

func (g *ghosttyTerminal) Styled() string { return g.format(ghostty.FormatterFormatVT) }

// format is the visible screen in the given format, a line per row. The formatter writes the
// active screen's history first, a line per row too, and leaves out blank rows at the end.
func (g *ghosttyTerminal) format(format ghostty.FormatterFormat) string {
	g.t.Helper()
	return locked(g, func() (string, error) {
		formatter, err := ghostty.NewFormatter(g.term, ghostty.WithFormatterFormat(format))
		if err != nil {
			return "", err
		}
		defer formatter.Close()
		history, err := g.term.ScrollbackRows()
		if err != nil {
			return "", err
		}
		text, err := formatter.FormatString()
		lines := strings.Split(text, "\n")
		return strings.Join(lines[min(int(history), len(lines)):], "\n"), err
	})
}

func (g *ghosttyTerminal) Output() []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return bytes.Clone(g.output.Bytes())
}

func (g *ghosttyTerminal) Modes() Modes {
	g.t.Helper()
	return locked(g, func() (Modes, error) {
		screen, err := g.term.ActiveScreen()
		if err != nil {
			return Modes{}, err
		}
		mouse, err := g.term.MouseTracking()
		if err != nil {
			return Modes{}, err
		}
		cursor, err := g.term.CursorVisible()
		return Modes{AltScreen: screen == ghostty.ScreenAlternate, Mouse: mouse,
			Cursor: cursor}, err
	})
}

func (g *ghosttyTerminal) Clipboard() string {
	g.t.Helper()
	sandbox.WaitFor(g.t, 10*time.Second, "an OSC 52 copy in ghostty", func() bool {
		return locked(g, func() (string, error) { return g.clipboard, nil }) != ""
	})
	return locked(g, func() (string, error) { return g.clipboard, nil })
}

func (g *ghosttyTerminal) Running() bool {
	select {
	case <-g.drained:
		select {
		case <-g.exited:
			return false
		default:
		}
	default:
	}
	return true
}

// Close kills the program, as closing the app's tab does. It frees the terminal once the reader
// has stopped, as it does once nothing has the terminal open, within 10 seconds.
func (g *ghosttyTerminal) Close() {
	if g.exited != nil {
		if err := g.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			g.t.Error(err)
		}
		<-g.exited
	}
	if g.controller != nil {
		if err := g.controller.Close(); err != nil {
			g.t.Error(err)
		}
	}
	if g.drained != nil {
		select {
		case <-g.drained:
		case <-time.After(10 * time.Second):
			g.t.Errorf("ghostty: %s still open 10s after the program exited", g.path)
			return
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.term != nil {
		g.keys.Close()
		g.mouse.Close()
		g.term.Close()
	}
}
