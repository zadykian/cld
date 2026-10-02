package tests

import (
	"regexp"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// C4: the mouse wheel reaches claude, which scrolls its transcript with it.
func TestContractMouseWheel(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.WheelUp()
		probe.WaitInput(mark, "\x1b[<64;")
	})
}

// C4: over a program drawing in the main screen without the mouse, as claude outside fullscreen
// or a shell, the wheel scrolls the pane's history in copy mode. cld sets mouse on for this:
// claude's fullscreen transcript gets the wheel either way (docs/design/overview.md).
func TestContractWheelScrollsHistory(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
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

// C4: a click with a modifier reaches claude whole, press and release, as claude opens a link on a
// Ctrl+click's release only after its press. cld unbinds tmux's Ctrl+click and Alt+right-click,
// which take the press whatever the pane's program asks (decision 35; TestServerOptions).
func TestContractClicks(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		clicks := []struct{ key, button string }{{"C-MouseDown1", "16"}, {"M-MouseDown3", "10"}}
		for _, click := range clicks {
			mark := probe.Mark()
			term.Click(click.key)
			whole := regexp.MustCompile(`\x1b\[<` + click.button + `;\d+;\d+M` +
				`\x1b\[<` + click.button + `;\d+;\d+m`)
			what := click.key + " pressed and let go in the probe input"
			sandbox.WaitFor(t, 10*time.Second, what, func() bool {
				return whole.Match(probe.Input()[mark:])
			})
		}
	})
}

// C4: focus changes reach claude.
func TestContractFocus(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		_, term, probe := startContract(t, name)
		mark := probe.Mark()
		term.Focus(false)
		probe.WaitInput(mark, "\x1b[O")
		mark = probe.Mark()
		term.Focus(true)
		probe.WaitInput(mark, "\x1b[I")
	})
}
