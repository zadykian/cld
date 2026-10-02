package tests

import (
	"maps"
	"os/exec"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The terminal join needs, and the title it prints only to one.

// needsTerminal starts join's message where it has no terminal to hand tmux.
const needsTerminal = "cld: join needs a terminal, and "

// terminalCase is a join of session x without a terminal to hand tmux.
type terminalCase struct {
	args []string
	// attach is whether the fake tmux finds session x, which join then attaches to
	attach bool
	// terminal is whether cld runs on a terminal; term is TERM, unset for "unset"
	terminal bool
	term     string
	want     string
}

// name names the case by its arguments, what join would do and the terminal.
func (c terminalCase) name() string {
	name := strings.Join(c.args, " ")
	if c.attach {
		name += ", attaching"
	}
	switch {
	case !c.terminal:
		return name + ", no terminal"
	case c.term == "unset":
		return name + ", TERM unset"
	}
	return name + ", TERM=" + c.term
}

var terminalCases = []terminalCase{
	{[]string{"join", "-s", "x"}, false, false, "xterm-256color",
		needsTerminal + "its input is not one\n"},
	{[]string{"join", "-s", "x", "--resume", "SESSION"}, false, false, "xterm-256color",
		needsTerminal + "its input is not one\n"},
	{[]string{"join", "-s", "x"}, true, false, "xterm-256color",
		needsTerminal + "its input is not one\n"},
	{[]string{"join", "-s", "x"}, false, true, "dumb", needsTerminal + "TERM is dumb\n"},
	{[]string{"join", "-s", "x"}, false, true, "unset", needsTerminal + "TERM is not set\n"},
	{[]string{"join", "-s", "x"}, false, true, "", needsTerminal + "TERM is empty\n"},
	{[]string{"join", "-s", "x", "--resume", "SESSION"}, false, true, "dumb",
		needsTerminal + "TERM is dumb\n"},
	{[]string{"join", "-s", "x"}, true, true, "dumb", needsTerminal + "TERM is dumb\n"},
	{[]string{"join", "-s", "x"}, true, true, "unset", needsTerminal + "TERM is not set\n"},
}

// join refuses without a terminal, or with TERM unset, empty or dumb, whether it would create the
// session or attach (decision 31). It ends with status 1, saying so, nothing on stdout and no
// socket left. It creates session x with the real tmux, and attaches to x on the fake tmux's
// server. TestJoinStates and TestOnlyJoinRunsClaude pin the checks before, with no terminal.
func TestRefusesWithoutATerminal(t *testing.T) {
	t.Parallel()
	for _, test := range terminalCases {
		t.Run(test.name(), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			extra := map[string]string{"TERM": test.term}
			if test.term == "unset" {
				delete(s.Env, "TERM")
				delete(extra, "TERM")
			}
			if test.attach {
				maps.Copy(extra, fakeTmuxEnv(s, map[string]string{"CLD_FAKE_TMUX_SESSIONS": "cld-x"}))
			}
			run := s.RunCld
			if test.terminal {
				run = s.RunCldOnTerminal
			}
			checkFailed(t, run(extra, test.args...), 1, test.want)
			if servers := s.Servers(); len(servers) != 0 {
				t.Errorf("sockets %q left behind, want none", servers)
			}
			if fakeTmuxRan(s) {
				t.Error("cld handed over to tmux")
			}
		})
	}
}

// join prints the title only where stdout is a terminal (decision 31.4): a pipe, as in cld join |
// tee, would take the escape in as text. With stdin a terminal and stdout a pipe, neither gets
// the title, and cld hands over to the fake tmux, which finds session x where join attaches.
func TestTitleOnlyToATerminal(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args     []string
		sessions string
	}{
		{[]string{"join", "-s", "x"}, ""},
		{[]string{"join", "-s", "x", "--resume", "SESSION"}, ""},
		{[]string{"join", "-s", "x"}, "cld-x"},
	} {
		t.Run(strings.Join(test.args, " ")+" "+test.sessions, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			pty := sandbox.OpenPty(t)
			cmd := exec.Command(sandbox.Cld, test.args...)
			cmd.Env = s.Environ(fakeTmuxEnv(s, map[string]string{
				"CLD_FAKE_TMUX_SESSIONS": test.sessions,
			}))
			cmd.Dir, cmd.Stdin = s.Work, pty.Terminal
			result := runCommand(t, cmd)
			if result.Code != 0 || result.Stdout != "" || result.Stderr != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and no output",
					result.Code, result.Stdout, result.Stderr)
			}
			if written := pty.Output(); written != "" {
				t.Errorf("the terminal got %q, want nothing", written)
			}
			if !fakeTmuxRan(s) {
				t.Error("cld did not hand over to tmux")
			}
		})
	}
}
