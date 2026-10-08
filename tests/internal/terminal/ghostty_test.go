//go:build ghostty

package terminal

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	ghostty "go.mitchellh.com/libghostty"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The ghostty driver's own tests, for what the terminal contract does not exercise in it. Each
// script ends on a sentinel key the test types last, not on a timeout.

// startScript runs the sh script on a ghostty terminal, with the path of a file in a temporary
// directory as $1, and returns the terminal and that path.
func startScript(t *testing.T, script string) (*ghosttyTerminal, string) {
	t.Helper()
	if err := GhosttyReady(); err != nil {
		t.Fatal(err)
	}
	g, ok := New(t, "ghostty", nil).(*ghosttyTerminal)
	if !ok {
		t.Fatal("ghostty: not the ghostty driver")
	}
	file := filepath.Join(t.TempDir(), "out")
	g.Start([]string{"sh", "-c", script, "sh", file}, map[string]string{"PATH": os.Getenv("PATH")},
		t.TempDir())
	return g, file
}

// waitFile waits until the script has exited and the terminal has taken in what it wrote, and
// returns the file's contents, which a timeout reports too.
func waitFile(t *testing.T, g *ghosttyTerminal, file string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for g.Running() {
		if time.Now().After(deadline) {
			data, err := os.ReadFile(file)
			t.Fatalf("the script still runs after 15s, having read %q (%v)", data, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// waitText waits until the screen shows text.
func waitText(t *testing.T, g *ghosttyTerminal, text string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, text+" on the screen", func() bool {
		return strings.Contains(g.Screen(), text)
	})
}

// waitOutput waits until what the program wrote ends in end.
func waitOutput(t *testing.T, g *ghosttyTerminal, end string) {
	t.Helper()
	sandbox.WaitFor(t, 10*time.Second, end+" at the end of the output", func() bool {
		return bytes.HasSuffix(g.Output(), []byte(end))
	})
}

// A request for focus reports that two reads split gets one report, while reporting is already
// on: as tmux's requests at its queries' answers come. The second half waits for a key, which the
// test types once the terminal has taken in the first.
func TestGhosttySplitFocusRequest(t *testing.T) {
	t.Parallel()
	g, file := startScript(t, `stty raw -echo; printf '\033[?1004h\033[?10'
dd bs=1 count=4 2>/dev/null >"$1"; printf '04h'; dd bs=1 count=4 2>/dev/null >>"$1"`)
	waitOutput(t, g, "\x1b[?1004h\x1b[?10")
	g.Keys("y")
	waitOutput(t, g, "04h")
	g.Keys("z")
	if got, want := waitFile(t, g, file), "\x1b[Iy\x1b[Iz"; got != want {
		t.Errorf("the program read %q, want a focus report before each key, %q", got, want)
	}
}

// Resize gives the mouse encoder its size, which sends nothing for a cell beyond the screen. Cell
// 10;10 takes a click before the resize to 8 by 6, and none after it, as the old size would send.
// The pty gets the size too.
func TestGhosttyResize(t *testing.T) {
	t.Parallel()
	g, file := startScript(t, `printf '\033[?1000h\033[?1006h'; stty raw -echo
dd bs=1 count=23 2>/dev/null >"$1"; stty size >"$1.size"`)
	g.Click("MouseDown1")
	g.Resize(8, 6)
	if g.mouseEvent(ghostty.MouseActionPress, ghostty.MouseButtonLeft, 0) {
		t.Error("a press on cell 10;10 went out after a resize to 8 by 6")
	}
	g.Keys("z")
	if got, want := waitFile(t, g, file), "\x1b[<0;10;10M\x1b[<0;10;10mz"; got != want {
		t.Errorf("the program read %q, want %q", got, want)
	}
	if size, err := os.ReadFile(file + ".size"); err != nil || string(size) != "6 8\n" {
		t.Errorf("stty size %q (%v), want 6 rows and 8 columns", size, err)
	}
}

// Freeze stops the terminal reading: what the program writes after a read it waited on shows only
// once the terminal thaws. The second's gap between them keeps them apart, the one timing this
// test assumes; the script touches $1 once it has written the second.
func TestGhosttyFreeze(t *testing.T) {
	t.Parallel()
	g, file := startScript(t, `stty -echo; printf ready; read -r x
printf one; sleep 1; printf two; : >"$1"; sleep 30`)
	waitText(t, g, "ready")
	thaw := g.Freeze()
	g.Keys("Enter")
	sandbox.WaitFor(t, 10*time.Second, "the program to write two", func() bool {
		_, err := os.Stat(file)
		return err == nil
	})
	if strings.Contains(g.Screen(), "two") {
		t.Error("a frozen terminal took in what the program wrote")
	}
	thaw()
	waitText(t, g, "two")
}

// Hold types a key once, and again at each repeat.
func TestGhosttyHold(t *testing.T) {
	t.Parallel()
	g, file := startScript(t, `stty raw -echo; printf ready; dd bs=1 count=5 2>/dev/null >"$1"`)
	waitText(t, g, "ready")
	g.Hold("x", 100*time.Millisecond, 50*time.Millisecond, 3)
	g.Keys("z")
	if got := waitFile(t, g, file); got != "xxxxz" {
		t.Errorf("the program read %q, want %q", got, "xxxxz")
	}
}

// Styled has the screen's attributes as SGR sequences, and the same screen gives the same string.
func TestGhosttyStyled(t *testing.T) {
	t.Parallel()
	g, _ := startScript(t, `printf '\033[1mbold\033[0m plain'; sleep 30`)
	waitText(t, g, "bold plain")
	styled := g.Styled()
	if !regexp.MustCompile(`\x1b\[(?:[0-9]+;)*1(?:;[0-9]+)*mbold`).MatchString(styled) {
		t.Errorf("styled screen %q, want bold's SGR right before it", styled)
	}
	if again := g.Styled(); again != styled {
		t.Errorf("styled screen %q, then %q", styled, again)
	}
}
