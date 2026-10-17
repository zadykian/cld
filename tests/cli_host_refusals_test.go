package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What --host refuses, before ssh runs (decision 62.2).

// Refusals of --host's words.
const (
	hostEmpty    = "cld: option '--host' needs a value (see cld help)\n"
	joinNeeds    = "cld: join: --host needs -n NAME or -C DIR, the directory there (see cld help)\n"
	projectNeeds = "cld: setup config project: --host needs -C DIR, the project's directory " +
		"there (see cld help)\n"
)

// invalidHost is the refusal of host, which --host does not take.
func invalidHost(host string) string {
	return "cld: invalid host '" + host + "' for --host (see cld help)\n"
}

// hostUnexpected is the refusal of --host by the command typed as typed.
func hostUnexpected(typed string) string {
	return "cld: " + typed + ": unexpected argument '--host' (see cld help)\n"
}

// hostRefusals are the command lines --host refuses with status 2, and what cld says. HOST is an
// ssh destination that neither ssh nor a shell reads otherwise, of 255 characters at most.
var hostRefusals = []struct {
	args   []string
	stderr string
}{
	{[]string{"kill", "-n", "app", "--host="}, hostEmpty},
	{[]string{"kill", "-n", "app", "--host", ""}, hostEmpty},
	{[]string{"kill", "-n", "app", "--host"}, hostEmpty},
	{[]string{"kill", "-n", "app", "--host", "-oProxyCommand=sh"}, invalidHost("-oProxyCommand=sh")},
	{[]string{"kill", "-n", "app", "--host", "a b"}, invalidHost("a b")},
	{[]string{"kill", "-n", "app", "--host", "ws;true"}, invalidHost("ws;true")},
	{[]string{"kill", "-n", "app", "--host", ".ws"}, invalidHost(".ws")},
	{[]string{"kill", "-n", "app", "--host", "ws:22"}, invalidHost("ws:22")},
	{[]string{"kill", "-n", "app", "--host", "[::1]"}, invalidHost("[::1]")},
	{[]string{"kill", "-n", "app", "--host", strings.Repeat("w", 256)},
		invalidHost(strings.Repeat("w", 256))},
	{[]string{"kill", "--host", "a b"}, invalidHost("a b")},
	// The options cld's own commands give.
	{[]string{"join", "--host", "ws", "-n", "app", "--moved=x"}, hostUnexpected("join")},
	{[]string{"join", "--host", "ws", "-n", "app", "--switched-from", "a"},
		hostUnexpected("join")},
	{[]string{"join", "--host", "ws", "--moved=x"}, hostUnexpected("join")},
	{[]string{"list", "--host", "ws", "--switch", "c"}, hostUnexpected("list")},
	{[]string{"list", "--host", "ws", "--to", "next"}, hostUnexpected("list")},
	// The host's cld would take the name or directory from the home ssh starts it in.
	{[]string{"join", "--host", "ws"}, joinNeeds},
	{[]string{"join", "--host", "ws", "-s", "1", "--", "--model", "opus"}, joinNeeds},
	{[]string{"kill", "--host", "ws", "-s", "1"},
		"cld: kill: --host needs -n NAME (see cld help)\n"},
	{[]string{"detach", "--host", "ws"}, "cld: detach: --host needs -n NAME (see cld help)\n"},
	{[]string{"setup", "config", "project", "--host", "ws", "--permissions", "cld"},
		projectNeeds},
	// The commands that take no --host.
	{[]string{"version", "--host", "ws"}, hostUnexpected("version")},
	{[]string{"-V", "--host", "ws"}, hostUnexpected("-V")},
	{[]string{"help", "--host", "ws"}, hostUnexpected("help")},
	{[]string{"completion", "bash", "--host", "ws"}, hostUnexpected("completion bash")},
	{[]string{"setup", "completion", "bash", "--host", "ws"},
		hostUnexpected("setup completion bash")},
}

// --host refuses an empty HOST, one ssh or a shell would read otherwise, the hidden options, and a
// name or directory left to the host's home. Where none of these is refused, a HOST of 255
// characters runs ssh.
func TestHostRefusals(t *testing.T) {
	t.Parallel()
	for _, test := range hostRefusals {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(logOnly, test.args...), 2, test.stderr)
			checkSSH(t, s, nil)
		})
	}
	s := sandbox.New(t)
	host := strings.Repeat("w", 255)
	checkExit(t, "255 characters", s.RunCld(logOnly, "kill", "--host", host, "-n", "app"), 0, "",
		"")
	checkSSH(t, s, hostSSH(host, false, "kill", "--name=app"))
}

// In a pane of cld's servers, join and list refuse --host on a terminal, with status 1: the host's
// session would show inside this one. Without terminals they run ssh, which gives none, as kill
// does on a terminal, and join on one in another tmux's pane.
func TestHostInsideASession(t *testing.T) {
	t.Parallel()
	pane := map[string]string{"TMUX": "/tmp/tmux-0/cld-app,1,0", "CLD_FAKE_SSH_EXIT": "0"}
	joinX := hostCase{"ws", []string{"join", "--host", "ws", "-n", "x"}, []string{"join", "--name=x"}}
	list := hostCase{"ws", []string{"list", "--host", "ws"}, []string{"list"}}
	s := sandbox.New(t)
	for _, test := range []hostCase{joinX, list} {
		checkFailed(t, runOnTerminal(t, s, pane, test.args...), 1, "cld: "+test.args[0]+
			": --host would show the host's session inside this one: run it outside cld's "+
			"sessions\n")
	}
	checkFailed(t, runOnTerminal(t, s, pane, "join", "--host", "ws"), 2, joinNeeds)
	checkSSH(t, s, nil)
	kill := hostCase{"ws", []string{"kill", "--host", "ws", "-n", "x"}, []string{"kill", "--name=x"}}
	for _, test := range []hostCase{joinX, list, kill} {
		s := sandbox.New(t)
		checkExit(t, test.args[0], s.RunCld(pane, test.args...), 0, "", "")
		checkSSH(t, s, hostSSH("ws", false, test.words...))
		s = sandbox.New(t)
		checkExit(t, test.args[0]+", its input a terminal", s.RunCldWithInput(
			sandbox.OpenPty(t).Terminal, pane, test.args...), 0, "", "")
		checkSSH(t, s, hostSSH("ws", false, test.words...))
	}
	s = sandbox.New(t)
	checkExit(t, "kill on a terminal", runOnTerminal(t, s, pane, kill.args...), 0, "", "")
	checkSSH(t, s, hostSSH("ws", false, kill.words...))
	s = sandbox.New(t)
	elsewhere := map[string]string{"TMUX": "/tmp/tmux-0/default,1,0", "CLD_FAKE_SSH_EXIT": "0"}
	checkExit(t, "join", runOnTerminal(t, s, elsewhere, joinX.args...), 0, "", "")
	checkSSH(t, s, hostSSH("ws", true, joinX.words...))
}

// --help shows this cld's help, the same as without --host, and runs no ssh.
func TestHostHelp(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	help := s.RunCld(nil, "kill", "--help")
	checkExit(t, "kill --host ws --help", s.RunCld(logOnly, "kill", "--host", "ws", "-n", "app",
		"--help"), 0, help.Stdout, "")
	checkSSH(t, s, nil)
}
