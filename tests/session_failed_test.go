package tests

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
	"github.com/zadykian/cld/tests/internal/terminal"
)

// A claude that fails with no terminal attached leaves the hint to join, which shows it on the
// message line. tmux would show the hook's over the next session a terminal attaches to (decision
// 5). The border line is there all the same, on claude's window only, and stays past a key.
// Joining a live session shows no hint.
func TestClaudeFailingDetached(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	first := startCld(t, s, "tmux", nil, "join", "-s", "bad")
	probe := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	probe.Send("tmux new-session -d -s side sleep 600")
	sandbox.WaitFor(t, 10*time.Second, "claude's tmux to make a session", func() bool {
		return slices.Contains(s.Sessions(), "cld-bad/side")
	})
	first.Keys("C-q", "d")
	sandbox.WaitFor(t, 10*time.Second, "cld to detach", func() bool { return !first.Running() })
	probe.Send("exit 1")
	sandbox.WaitFor(t, 10*time.Second, "claude to exit", func() bool {
		return s.Format("cld-bad", "#{pane_dead}") == "1"
	})
	for session, want := range map[string]string{"cld-bad": "bottom", "side": ""} {
		status := s.MustTmux("cld-bad", "show", "-wv", "-t", "="+session+":", "pane-border-status")
		if status != want {
			t.Errorf("pane-border-status of %s's window %q, want %q", session, status, want)
		}
	}

	other := startCld(t, s, "tmux", nil, "join", "-s", "other")
	s.WaitProbes(2)
	waitClients(t, s, 1)
	if mode := s.Format("cld-other", "#{pane_mode}"); mode != "" {
		t.Errorf("a new session opens in %s:\n%s", mode, other.Screen())
	}

	hint := "claude exited with status 1: cld kill -s bad ends the session, " +
		"C-q d or cld detach -s bad detaches"
	joined := startCld(t, s, "tmux", nil, "join", "-s", "bad")
	waitScreen(t, joined, hint)
	if screen := joined.Screen(); !strings.HasSuffix(strings.TrimRight(screen, " \n"), "\n"+hint) {
		t.Errorf("the hint is not on the message line:\n%s", screen)
	}
	if mode := s.Format("cld-bad", "#{pane_mode}"); mode != "" {
		t.Errorf("the joined session is in %s", mode)
	}
	joined.Keys("x")
	waitBorderLine(t, joined, hint)

	live := startCld(t, s, "tmux", nil, "join", "-s", "other")
	waitScreen(t, live, "probe --name cld-other")
	if screen := live.Screen(); strings.Contains(screen, "claude exited") {
		t.Errorf("joining a live session shows the hint:\n%s", screen)
	}
}

// The lines that say how claude exited and how to end the session say what fits the terminal's
// width whole, rather than be cut at it (decision 5). A command cut short could name another
// session, -s 1 of -s 12. As the terminal narrows, the line leaves out the detach, then the kill.
func TestFailedClaudeLinesFit(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	// Wide enough for the whole line from the start. A resize racing claude's exit would have the
	// message line, which covers the border line until a key is pressed, say less.
	term := terminal.New(t, "tmux", s)
	term.Resize(150, 40)
	term.Start(s.CldArgv("join", "-n", "my-awesome-proj", "-s", "12"), s.Env, s.Work)
	waitClients(t, s, 1)
	s.WaitProbes(1)[0].Send("exit 1")
	exited := "claude exited with status 1"
	kill := exited + ": cld kill -n my-awesome-proj -s 12 ends the session"
	all := kill + ", C-q d or cld detach -n my-awesome-proj -s 12 detaches"
	waitScreen(t, term, all)
	term.Keys("x")
	for _, width := range []struct {
		columns int
		text    string
	}{{150, all}, {120, kill}, {90, kill}, {80, exited}} {
		term.Resize(width.columns, 40)
		waitBorderLine(t, term, width.text)
	}
}

// waitBorderLine waits until the screen's last line is a line of the pane's border with text in
// it, as the pane-died hook leaves it below a claude that failed. The text is between the border's
// ─, unlike on the message line, which starts with it.
func waitBorderLine(t *testing.T, term terminal.Terminal, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen := term.Screen()
		last := screen[strings.LastIndexByte(screen, '\n')+1:]
		if strings.HasPrefix(last, "─") && strings.Trim(last, "─ ") == text {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after 10s waiting for the last line to be the pane's border "+
				"with %q; the screen shows\n%s", text, screen)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
