package tests

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// The terminal contract, C1 to C10 of docs/design/testing.md, runs in each terminal of
// CLD_TERMINALS. Its tests are here and in contract_*_test.go. Where terminals differ, the
// difference is in expectations, never a skip.

// startContract starts cld in the named terminal and waits for claude and the attached client.
func startContract(
	t *testing.T, name string,
) (*sandbox.Sandbox, terminal.Terminal, *sandbox.Probe) {
	t.Helper()
	s := sandbox.New(t)
	term := startCld(t, s, name, nil, "join", "-s", "contract")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	waitScreen(t, term, "probe --name cld-contract")
	return s, term, probe
}

// C1: the tab shows the session's name after claude's marker, whatever title claude sets: ✳, or ◐
// and ◑ in turn while claude is busy, a second each. " [w]" follows in a linked git worktree.
// Decisions 25 and 26 give the reasons; TestStatusHooks and TestWorktreeHooks test the hooks.
func TestContractTitle(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
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

// C2: what tmux learned about the terminal decides what it forwards to claude. tmux learns some
// features from the terminal's answers to its queries, which may come after the client attached.
func TestContractClientFeatures(t *testing.T) {
	forEachTerminal(t, func(t *testing.T, name string) { //nolint:thelper // a subtest, not a helper
		s, _, _ := startContract(t, name)
		client, missing := clientFeatures(t, s, name)
		deadline := time.Now().Add(10 * time.Second)
		for len(missing) > 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			client, missing = clientFeatures(t, s, name)
		}
		t.Logf("client: %s", client)
		for _, feature := range missing {
			t.Errorf("tmux does not detect %q in %s", feature, name)
		}
	})
}

// clientFeatures returns how tmux sees the contract's client, and the features expected of the
// named terminal that tmux has not detected yet.
func clientFeatures(t *testing.T, s *sandbox.Sandbox, name string) (string, []string) {
	t.Helper()
	client := s.MustTmux("cld-contract", "list-clients", "-F",
		"#{client_termname}|#{client_termtype}|#{client_termfeatures}")
	detected := strings.Split(strings.SplitN(client, "|", 3)[2], ",")
	missing := slices.DeleteFunc(slices.Clone(expectations[name].features), func(f string) bool {
		return slices.Contains(detected, f)
	})
	return client, missing
}
