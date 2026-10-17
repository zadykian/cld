package tests

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// --host HOST (decision 62): the ssh that cld becomes, as the fake ssh logs it. Where
// CLD_FAKE_SSH_EXIT=0, that ssh runs nothing more.

// logOnly is the environment of a run whose fake ssh logs its call and exits 0.
var logOnly = map[string]string{"CLD_FAKE_SSH_EXIT": "0"}

// hostSSH is the ssh that runs cld with words on host, with a terminal for the host's cld or
// none, ssh's arguments after its name.
func hostSSH(host string, terminal bool, words ...string) []string {
	argv := []string{"-T", "-n"}
	if terminal {
		argv = []string{"-t", "-o", "LogLevel=ERROR"}
	}
	return append(argv, "-o", "RemoteCommand=none", "--", host,
		"exec $SHELL -lc 'exec cld "+strings.Join(words, " ")+"'")
}

// hostValue is how --host sends words that are not all plain: __host and their VALUE, the words
// joined by NULs, in URL-safe base64 without padding.
func hostValue(words ...string) []string {
	joined := []byte(strings.Join(words, "\x00"))
	return []string{"__host", base64.RawURLEncoding.EncodeToString(joined)}
}

// checkSSH reports s's calls of the fake ssh, unless there was one, with argv want; with want
// nil, unless there was none.
func checkSSH(t *testing.T, s *sandbox.Sandbox, want []string) {
	t.Helper()
	calls := s.SSHCalls()
	if want == nil && len(calls) != 0 {
		t.Errorf("ssh ran: %+v", calls)
	}
	if want != nil && (len(calls) != 1 || !slices.Equal(calls[0].Argv, want)) {
		t.Errorf("ssh ran %+v, want once, with %q", calls, want)
	}
}

// hostCase is a command line with --host, and the words it has the host's cld run.
type hostCase struct {
	host  string
	args  []string
	words []string
}

// hostCases are TestHostWords' command lines: each option as --NAME=VALUE, in pflag's order, and
// --mcp once per value. A word a shell would read otherwise sends them all as VALUE. Such a word
// is empty, has a space, a quote or a "$", or starts with "=", which zsh expands.
var hostCases = []hostCase{
	{"ws", []string{"join", "--host", "ws", "-n", "app", "-s", "1", "-w", "--", "--model", "opus"},
		[]string{"join", "--name=app", "--suffix=1", "--worktree=true", "--", "--model", "opus"}},
	{"me@ws.example", []string{"join", "-C", "/srv/app", "--host=me@ws.example", "--new"},
		[]string{"join", "--directory=/srv/app", "--new=true"}},
	{"ws", []string{"join", "--host", "ws", "-n", "app", "--"}, []string{"join", "--name=app", "--"}},
	{"ws", []string{"detach", "--host", "ws", "-n", "app"}, []string{"detach", "--name=app"}},
	{"ws", []string{"kill", "-s", "1", "--host", "ws", "-n", "app"},
		[]string{"kill", "--name=app", "--suffix=1"}},
	{"ws", []string{"list", "--host", "ws"}, []string{"list"}},
	{"ws", []string{"restore", "sessions", "--host", "ws"}, []string{"restore", "sessions"}},
	{"ws", []string{"update", "--host", "ws"}, []string{"update"}},
	{"ws", []string{"setup", "config", "project", "--host", "ws", "-C", "/srv/app", "--permissions",
		"cld", "--mcp", "rider", "--mcp", "goland,jbcontext"}, []string{"setup", "config", "project",
		"--directory=/srv/app", "--mcp=rider", "--mcp=goland,jbcontext", "--permissions=cld"}},
	{"ws", []string{"setup", "config", "user", "--host", "ws", "--permissions", "none",
		"--notifications", "kitty"},
		[]string{"setup", "config", "user", "--notifications=kitty", "--permissions=none"}},
	// Off Linux too, which refuses it here: the host may be Linux.
	{"ws", []string{"setup", "restore", "--host", "ws"}, []string{"setup", "restore"}},
	{"ws", []string{"join", "--host", "ws", "-n", "app", "--", "two words"},
		hostValue("join", "--name=app", "--", "two words")},
	{"ws", []string{"join", "--host", "ws", "-n", "app", "--resume", "a b"},
		hostValue("join", "--name=app", "--resume=a b")},
	{"ws", []string{"join", "--host", "ws", "-n", "app", "--", "$HOME", "it's"},
		hostValue("join", "--name=app", "--", "$HOME", "it's")},
	{"ws", []string{"join", "--host", "ws", "-n", "app", "--", "", "=x"},
		hostValue("join", "--name=app", "--", "", "=x")},
	{"ws", []string{"setup", "config", "project", "--host", "ws", "-C", "/srv/my app"},
		hostValue("setup", "config", "project", "--directory=/srv/my app")},
}

// With --host, cld becomes ssh, which needs no tmux, claude or git here, nor -C's directory: the
// host's cld runs the command, with its words. Where cld's input is no terminal, ssh reads none.
func TestHostWords(t *testing.T) {
	t.Parallel()
	for _, test := range hostCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			env := map[string]string{"PATH": s.Tools("ssh"), "CLD_FAKE_SSH_EXIT": "0"}
			checkExit(t, "cld", s.RunCld(env, test.args...), 0, "", "")
			checkSSH(t, s, hostSSH(test.host, false, test.words...))
		})
	}
}

// runOnTerminal runs cld with args, its input and output a pty of the test's, as on a terminal,
// and its stderr captured. What it writes to the pty is not in the result.
func runOnTerminal(
	t *testing.T, s *sandbox.Sandbox, extra map[string]string, args ...string,
) sandbox.Result {
	t.Helper()
	p := sandbox.OpenPty(t)
	argv := s.CldArgv(args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env, cmd.Dir, cmd.Stdin, cmd.Stdout = s.Environ(extra), s.Work, p.Terminal, p.Terminal
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var exit *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exit) {
		t.Fatalf("run cld: %v", err)
	}
	return sandbox.Result{Code: cmd.ProcessState.ExitCode(), Stderr: stderr.String()}
}

// ssh gives the host's join and list a terminal where cld's input and output are both terminals:
// join attaches there, and list draws its picker. With its input alone a terminal, ssh gives none,
// and never to the other commands.
func TestHostTerminal(t *testing.T) {
	t.Parallel()
	for _, test := range []hostCase{
		{"ws", []string{"join", "--host", "ws", "-n", "app"}, []string{"join", "--name=app"}},
		{"ws", []string{"list", "--host", "ws"}, []string{"list"}},
		{"ws", []string{"kill", "--host", "ws", "-n", "app"}, []string{"kill", "--name=app"}},
		{"ws", []string{"update", "--host", "ws"}, []string{"update"}},
	} {
		s := sandbox.New(t)
		checkExit(t, test.args[0]+", its input a terminal", s.RunCldWithInput(
			sandbox.OpenPty(t).Terminal, logOnly, test.args...), 0, "", "")
		checkSSH(t, s, hostSSH(test.host, false, test.words...))
		s = sandbox.New(t)
		checkExit(t, test.args[0]+" on a terminal", runOnTerminal(t, s, logOnly, test.args...), 0,
			"", "")
		attaches := test.args[0] == "join" || test.args[0] == "list"
		checkSSH(t, s, hostSSH(test.host, attaches, test.words...))
	}
}

// cld's status is ssh's: the host cld's, which says why it fails there, or ssh's own 255. Without
// ssh, cld fails with status 1.
func TestHostStatus(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeProfile(t, s, filepath.Dir(sandbox.Cld))
	checkExit(t, "detach", s.RunCld(nil, "detach", "--host", "ws", "-n", "app"), 2, "",
		"cld: detach: missing -s SUFFIX (see cld list)\n")
	dropped := map[string]string{"CLD_FAKE_SSH_EXIT": "255"}
	checkExit(t, "kill", s.RunCld(dropped, "kill", "--host", "ws", "-n", "app"), 255, "", "")
	checkFailed(t, s.RunCld(map[string]string{"PATH": s.Tools()}, "list", "--host", "ws"), 1,
		"cld: ssh is not installed\n")
}

// On the host, list prints nothing where no session runs, and setup config project writes the
// files of -C's directory, whose name holds a space.
func TestHostRuns(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	writeProfile(t, s, filepath.Dir(sandbox.Cld))
	checkExit(t, "list", s.RunCld(nil, "list", "--host", "ws"), 0, "", "")
	dir := filepath.Join(s.Root, "my app")
	makeDirs(t, dir)
	checkExit(t, "setup config project", s.RunCld(nil, "setup", "config", "project", "--host",
		"ws", "-C", dir, "--permissions", "cld"), 0, "Created .claude/settings.json\n"+
		"Created .claude/settings.local.json\nCreated .gitignore\n", "")
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err != nil {
		t.Errorf("the host's settings: %v", err)
	}
}

// __host VALUE runs the words of VALUE, as the host's cld does for --host: one word with a space
// stays one. It refuses no VALUE or two, and one --host never sends: empty, padded or no base64.
func TestHostValue(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	version := s.RunCld(nil, "version")
	checkExit(t, "version", s.RunCld(nil, hostValue("version")...), 0, version.Stdout, "")
	checkFailed(t, s.RunCld(nil, hostValue("kill", "--name=a b")...), 2,
		"cld: invalid name 'a b' (see cld help)\n")
	takes := "cld: __host takes one VALUE, which --host sends (see cld help)\n"
	checkFailed(t, s.RunCld(nil, "__host"), 2, takes)
	checkFailed(t, s.RunCld(nil, append(hostValue("version"), "x")...), 2, takes)
	for _, value := range []string{"", hostValue("version")[1] + "==", "!!"} {
		checkFailed(t, s.RunCld(nil, "__host", value), 2,
			"cld: invalid value '"+value+"' for __host (see cld help)\n")
	}
}

// Each command that takes --host offers it, and its HOST completes nothing; the others never offer
// it (decision 62.6).
func TestCompleteHost(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	none := map[string]string{"PATH": s.Tools()}
	check := func(args []string, want string) {
		t.Helper()
		result := s.RunCld(none, append([]string{"__complete"}, args...)...)
		checkExit(t, strings.Join(args, " "), result, 0, want, noFileReport)
	}
	for _, command := range [][]string{{"join"}, {"detach"}, {"kill"}, {"list"}, {"update"},
		{"restore", "sessions"}, {"setup", "config", "project"}, {"setup", "config", "user"},
		{"setup", "restore"}} {
		check(append(slices.Clone(command), "--ho"), offeredHost+noFileDirective)
		check(append(slices.Clone(command), "--host", ""), noFileDirective)
	}
	for _, command := range [][]string{{"version"}, {"completion"}, {"help"},
		{"setup", "completion"}} {
		check(append(slices.Clone(command), "--ho"), noFileDirective)
	}
}
