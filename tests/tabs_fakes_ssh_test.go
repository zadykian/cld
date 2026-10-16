package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The fake ssh, to a host that is the sandbox itself.

// writeProfile writes the sandbox's ~/.profile, which puts dir and then ProbeBin first on a
// login shell's PATH. The fake ssh runs no remote shell without it.
func writeProfile(t *testing.T, s *sandbox.Sandbox, dir string) {
	t.Helper()
	s.WriteFile(filepath.Join(s.Home, ".profile"),
		`PATH="`+dir+`:`+sandbox.ProbeBin+`:$PATH"; export PATH`+"\n")
}

// The fake ssh: -G's configuration, a remote command and a login shell run by bash, and
// CLD_FAKE_SSH_EXIT, each call logged. It refuses words as ssh does, with status 255, and -G in
// another form than cld's, and a remote shell where HOME has no .profile.
func TestFakeSSH(t *testing.T) {
	t.Parallel()
	t.Run("-G", testSSHConfig)
	t.Run("a remote command", testSSHCommand)
	t.Run("a login shell", testSSHLogin)
	t.Run("CLD_FAKE_SSH_EXIT", testSSHExit)
	for _, test := range []struct {
		args   []string
		stderr string
	}{
		{nil, "fake ssh: no destination\n"},
		{[]string{"-t", "--"}, "fake ssh: no destination\n"},
		{[]string{"-p"}, "fake ssh: option requires an argument -- p\n"},
		{[]string{"-tto"}, "fake ssh: option requires an argument -- o\n"},
		{[]string{"-G"}, configForm},
		{[]string{"-G", "-l", "bob", "ws"}, configForm},
		{[]string{"-G", "ws", "-p", "2222"}, configForm},
		{[]string{"-t", "-G", "ws"}, configForm},
		{[]string{"-Gp2222", "ws"}, configForm},
		{[]string{"ws", "true"}, noProfile},
		{[]string{"ws"}, noProfile},
	} {
		t.Run(strings.Join(append([]string{"ssh"}, test.args...), " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkExit(t, "ssh", runFake(t, s, nil, "ssh", test.args...), 255, "", test.stderr)
		})
	}
}

// What the fake ssh refuses: -G in another form than cld's, and a remote shell where HOME has no
// .profile.
const (
	configForm = "fake ssh: -G takes a destination and nothing else, as cld runs it\n"
	noProfile  = "fake ssh: HOME has no .profile to put the tools under test first on the PATH\n"
)

// ssh -G DEST, as cld runs it, prints the user of user@host, else $USER or me, the host in lower
// case, as ssh does, and port 22.
func testSSHConfig(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		dest, user, host string
		withUser         bool
	}{
		{"ws", "me", "ws", false},
		{"ws", "alice", "ws", true},
		{"root@ws", "root", "ws", true},
		{"Me@WS.example", "Me", "ws.example", false},
	} {
		s := sandbox.New(t)
		extra := map[string]string{}
		if test.withUser {
			extra["USER"] = "alice"
		}
		checkExit(t, "ssh -G "+test.dest, runFake(t, s, extra, "ssh", "-G", test.dest), 0,
			"user "+test.user+"\nhostname "+test.host+"\nport 22\n", "")
	}
}

// A remote command runs in HOME, by the bash first on the PATH with SHELL set to it, as sshd runs
// the login shell. So the typed line's exec $SHELL -lc reads ~/.profile, whose PATH finds the
// real tmux, which makes a session on the sandbox's server.
func testSSHCommand(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	tools, tmux := s.Tools("ssh", "bash"), s.RealTools("tmux")
	writeProfile(t, s, tmux)
	remote := `exec $SHELL -lc 'echo "$SHELL"; pwd; command -v tmux; ` +
		`tmux -f /dev/null -L ssh new-session -d -s x'`
	result := runFake(t, s, map[string]string{"PATH": tools + ":" + s.Env["PATH"]}, "ssh",
		"-t", "-o", "ConnectTimeout=10", "ws", remote)
	checkExit(t, "ssh", result, 0, filepath.Join(tools, "bash")+"\n"+s.Home+"\n"+
		filepath.Join(tmux, "tmux")+"\n", "")
	if sessions := s.MustTmux("ssh", "list-sessions", "-F", "#S"); sessions != "x" {
		t.Errorf("the sandbox's server ssh has sessions %q, want x", sessions)
	}
}

// With no remote command, the fake ssh runs bash as a login shell, -bash, which reads its commands
// from stdin and its PATH from ~/.profile. -l takes its value, the user, before the destination.
func testSSHLogin(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	tools, tmux := s.Tools("ssh", "bash"), s.RealTools("tmux")
	writeProfile(t, s, tmux)
	cmd := fakeCommand(t.Context(), s, map[string]string{"PATH": tools + ":" + s.Env["PATH"]},
		filepath.Join(sandbox.ProbeBin, "ssh"), "-l", "me", "ws")
	cmd.Stdin = strings.NewReader(`echo "$0"; echo "$SHELL"; pwd; command -v tmux` + "\n")
	checkExit(t, "ssh -l me ws", runCommand(t, cmd), 0, "-bash\n"+filepath.Join(tools, "bash")+
		"\n"+s.Home+"\n"+filepath.Join(tmux, "tmux")+"\n", "")
}

// CLD_FAKE_SSH_EXIT exits with its status where ssh would connect, once the call is logged, and
// runs nothing: 255 as a dropped connection does, 0 as a call that only logs. -G, which never
// connects, answers all the same. A status that is no number fails.
func testSSHExit(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, test := range []struct{ status, term string }{
		{"255", "xterm-256color"}, {"0", "xterm-ghostty"},
	} {
		extra := map[string]string{"CLD_FAKE_SSH_EXIT": test.status, "TERM": test.term}
		result := runFake(t, s, extra, "ssh", "-t", "ws", "touch ran")
		if result.Code != map[string]int{"255": 255, "0": 0}[test.status] ||
			result.Stdout != "" || result.Stderr != "" {
			t.Errorf("CLD_FAKE_SSH_EXIT=%s: exit %d, stdout %q, stderr %q, want that status alone",
				test.status, result.Code, result.Stdout, result.Stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Home, "ran")); err == nil {
		t.Error("the remote command ran")
	}
	calls := s.SSHCalls()
	if len(calls) != 2 || calls[0].Term != "xterm-256color" || calls[1].Term != "xterm-ghostty" ||
		!slices.Equal(calls[1].Argv, []string{"-t", "ws", "touch ran"}) {
		t.Errorf("ssh.jsonl holds %+v, want both calls, -t ws 'touch ran', and their TERMs", calls)
	}
	checkExit(t, "ssh -G ws, CLD_FAKE_SSH_EXIT=255", runFake(t, s,
		map[string]string{"CLD_FAKE_SSH_EXIT": "255"}, "ssh", "-G", "ws"), 0,
		"user me\nhostname ws\nport 22\n", "")
	checkExit(t, "CLD_FAKE_SSH_EXIT=x", runFake(t, s, map[string]string{"CLD_FAKE_SSH_EXIT": "x"},
		"ssh", "ws", "true"), 1, "", `probe: CLD_FAKE_SSH_EXIT: strconv.Atoi: parsing "x": `+
		"invalid syntax\n")
}
