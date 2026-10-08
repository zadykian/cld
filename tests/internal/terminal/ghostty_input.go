//go:build ghostty

package terminal

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"

	ghostty "go.mitchellh.com/libghostty"
	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// A keyPress is a key event as the app hands it to the encoder. consumed are the modifiers the
// layout used up, and text what the layout gives.
type keyPress struct {
	key             ghostty.Key
	mods, consumed  ghostty.Mods
	text            string
	unshiftedLetter rune
}

// ghosttyKeys are the keys the tests type that are not a lone letter or digit, on a US layout.
var ghosttyKeys = map[string]keyPress{
	"Enter":   {key: ghostty.KeyEnter},
	"S-Enter": {key: ghostty.KeyEnter, mods: ghostty.ModShift},
	"Up":      {key: ghostty.KeyArrowUp},
	"Down":    {key: ghostty.KeyArrowDown},
	"Escape":  {key: ghostty.KeyEscape},
	// The encoder sends nothing for Ctrl+Shift+- without the layout's "_".
	"C-_": {key: ghostty.KeyMinus, mods: ghostty.ModCtrl | ghostty.ModShift,
		consumed: ghostty.ModShift, text: "_", unshiftedLetter: '-'},
	"<": {key: ghostty.KeyComma, mods: ghostty.ModShift, consumed: ghostty.ModShift, text: "<",
		unshiftedLetter: ','},
	">": {key: ghostty.KeyPeriod, mods: ghostty.ModShift, consumed: ghostty.ModShift, text: ">",
		unshiftedLetter: '.'},
}

// press is the key event for a key named the tmux way: a key of ghosttyKeys, "C-" and a letter,
// or a lone letter or digit.
func (g *ghosttyTerminal) press(key string) keyPress {
	g.t.Helper()
	if press, found := ghosttyKeys[key]; found {
		return press
	}
	press := keyPress{}
	if letter, found := strings.CutPrefix(key, "C-"); found && len(letter) == 1 {
		press.mods, key = ghostty.ModCtrl, letter
	} else {
		press.text = key
	}
	name := "key_" + key
	if key >= "0" && key <= "9" {
		name = "digit_" + key
	}
	parsed, err := ghostty.ParseKey(name)
	if len(key) != 1 || err != nil {
		g.t.Fatalf("ghostty: no key %q", key)
	}
	press.key, press.unshiftedLetter = parsed, rune(key[0])
	return press
}

// encode is what the encoder sends for a press, set from the terminal's modes as the app does. A
// press the encoder sends nothing for fails the test.
func (g *ghosttyTerminal) encode(press keyPress) []byte {
	g.t.Helper()
	input := locked(g, func() ([]byte, error) {
		event, err := ghostty.NewKeyEvent()
		if err != nil {
			return nil, err
		}
		defer event.Close()
		event.SetAction(ghostty.KeyActionPress)
		event.SetKey(press.key)
		event.SetMods(press.mods)
		event.SetConsumedMods(press.consumed)
		event.SetUTF8(press.text)
		event.SetUnshiftedCodepoint(press.unshiftedLetter)
		g.keys.SetOptFromTerminal(g.term)
		return g.keys.Encode(event)
	})
	if len(input) == 0 {
		g.t.Fatalf("ghostty: the key %v encodes to nothing", press.key)
	}
	return input
}

func (g *ghosttyTerminal) Keys(keys ...string) {
	g.t.Helper()
	for _, key := range keys {
		g.write(g.encode(g.press(key)))
		time.Sleep(50 * time.Millisecond)
	}
}

// Hold times the repeats in the driver, as the app repeats a key the system repeats.
func (g *ghosttyTerminal) Hold(key string, delay, interval time.Duration, repeats int) {
	g.t.Helper()
	input, wait := g.encode(g.press(key)), delay
	g.write(input)
	for range repeats {
		time.Sleep(wait)
		g.write(input)
		wait = interval
	}
}

// Paste pastes as the app's paste command does. Ghostty refuses text that could run commands
// until the user confirms it, which the driver does at once.
func (g *ghosttyTerminal) Paste(text string) {
	g.t.Helper()
	paste := ghostty.Paste{
		Location: ghostty.ClipboardLocationStandard,
		Source:   ghostty.PasteSourceClipboard,
		MIMEs:    []string{"text/plain"},
		Reader: func(_ string, writer io.Writer) error {
			_, err := io.WriteString(writer, text)
			return err
		},
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	_, err := g.term.Paste(paste)
	if errors.Is(err, ghostty.ErrRejected) {
		paste.AllowUnsafe = true
		_, err = g.term.Paste(paste)
	}
	if err != nil {
		g.t.Fatal(err)
	}
}

// mouseEvent sends a mouse event over the cell the tmux driver clicks, 10 by 10, and reports
// whether it sent anything: the encoder sends nothing while mouse reporting is off.
func (g *ghosttyTerminal) mouseEvent(
	action ghostty.MouseAction, button ghostty.MouseButton, mods ghostty.Mods,
) bool {
	g.t.Helper()
	input := locked(g, func() ([]byte, error) {
		event, err := ghostty.NewMouseEvent()
		if err != nil {
			return nil, err
		}
		defer event.Close()
		event.SetAction(action)
		event.SetButton(button)
		event.SetMods(mods)
		event.SetPosition(ghostty.MousePosition{X: 9*cellWidth + 4, Y: 9*cellHeight + 8})
		g.mouse.SetOptFromTerminal(g.term)
		return g.mouse.Encode(event)
	})
	g.write(input)
	return len(input) > 0
}

// WheelUp waits for mouse reporting, as the JediTerm driver does. The app reports a notch of
// the wheel up as three presses of button 4, one per row it scrolls.
func (g *ghosttyTerminal) WheelUp() {
	g.t.Helper()
	sandbox.WaitFor(g.t, 10*time.Second, "mouse reporting to scroll the wheel in ghostty",
		func() bool { return g.mouseEvent(ghostty.MouseActionPress, ghostty.MouseButtonFour, 0) })
	for range 2 {
		g.mouseEvent(ghostty.MouseActionPress, ghostty.MouseButtonFour, 0)
	}
}

// Click waits for mouse reporting too; a release whose press went out is sent whatever follows.
// The app keeps a click with Shift held for its own selection, and opens a link under a Ctrl+click
// without reporting the release: the driver clicks off any link.
func (g *ghosttyTerminal) Click(key string) {
	g.t.Helper()
	code := mouseButton(g.t, key)
	button := []ghostty.MouseButton{
		ghostty.MouseButtonLeft, ghostty.MouseButtonMiddle, ghostty.MouseButtonRight,
	}[code&3]
	var mods ghostty.Mods
	for bit, mod := range map[int]ghostty.Mods{4: ghostty.ModShift, 8: ghostty.ModAlt,
		16: ghostty.ModCtrl} {
		if code&bit != 0 {
			mods |= mod
		}
	}
	if mods&ghostty.ModShift != 0 {
		return
	}
	sandbox.WaitFor(g.t, 10*time.Second, "mouse reporting to click in ghostty", func() bool {
		return g.mouseEvent(ghostty.MouseActionPress, button, mods)
	})
	g.mouseEvent(ghostty.MouseActionRelease, button, mods)
}

// setSize gives the pty the terminal's size. Called with mu held, or before the reader starts.
func (g *ghosttyTerminal) setSize(tty *os.File) {
	g.t.Helper()
	size := &unix.Winsize{Row: uint16(g.rows), Col: uint16(g.columns),
		Xpixel: uint16(g.columns * cellWidth), Ypixel: uint16(g.rows * cellHeight)}
	if err := unix.IoctlSetWinsize(int(tty.Fd()), unix.TIOCSWINSZ, size); err != nil {
		g.t.Fatal(err)
	}
}

// mouseSize is the screen as the mouse encoder needs it. Called with mu held, or before the
// reader starts.
func (g *ghosttyTerminal) mouseSize() ghostty.MouseEncoderSize {
	return ghostty.MouseEncoderSize{
		ScreenWidth: uint32(g.columns * cellWidth), ScreenHeight: uint32(g.rows * cellHeight),
		CellWidth: cellWidth, CellHeight: cellHeight,
	}
}

// Resize resizes the terminal, its mouse encoder and its pty, which signals the program.
func (g *ghosttyTerminal) Resize(columns, rows int) {
	g.t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.columns, g.rows = columns, rows
	if g.term == nil {
		return
	}
	if err := g.term.Resize(uint16(columns), uint16(rows), cellWidth, cellHeight); err != nil {
		g.t.Fatal(err)
	}
	g.mouse.SetOptSize(g.mouseSize())
	g.setSize(g.controller)
}
