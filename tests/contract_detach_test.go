package tests

import (
	"slices"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// C3, C7: the prefix and d detach, claude keeps running, and the terminal is left clean.
func TestContractDetach(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		s, term, probe := startContract(t, name)
		waitModes(t, term, "while attached", "the alternate screen and mouse reporting on",
			func(modes terminal.Modes) bool { return modes.AltScreen && modes.Mouse })
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

// C9: claude exiting - /exit - ends its session, and with it the session's server; cld returns,
// and the terminal is left clean although claude restored none of its modes.
func TestContractClaudeExit(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
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
