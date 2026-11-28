package tests

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join --host on a terminal runs ssh as its child, and joins again after a drop (decision 68).

// leaveAlternate is the heart of the reset after a drop: ESC 7, then the alternate screen's end.
const leaveAlternate = "\x1b7\x1b[?1049l"

// dropNote is cld's note after a drop of ssh to ws, where a try follows.
const dropNote = "cld: ssh to ws ended with status 255: trying again in 5 s (Ctrl-C to stop)\n"

// After a drop, ssh's 255, join resets the terminal, and joins again 5 s later with --existing,
// -C, -n and -s alone. ssh's log follows each try, and its file goes once join ends.
func TestHostJoinAgain(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	line := "client_loop: send disconnect: Broken pipe"
	result := s.RunCldOnTerminal(map[string]string{"PATH": s.Tools("ssh"),
		"CLD_FAKE_SSH_EXIT": "255,0", "CLD_FAKE_SSH_LOG": line}, "join", "--host", "ws", "-C",
		"/srv/app", "-s", "1", "-w", "--detach-others", "--", "--model", "opus")
	if want := line + "\n" + dropNote + line + "\n"; result.Code != 0 || result.Stderr != want {
		t.Errorf("exit %d, stderr %q, want exit 0, stderr %q", result.Code, result.Stderr, want)
	}
	if n := strings.Count(result.Stdout, leaveAlternate); n != 1 {
		t.Errorf("the terminal left the alternate screen %d times, want once: %q", n, result.Stdout)
	}
	want := [][]string{
		joinSSH("ws", "join", "--detach-others=true", "--directory=/srv/app", "--suffix=1",
			"--worktree=true", "--", "--model", "opus"),
		joinSSH("ws", "join", "--directory=/srv/app", "--existing=true", "--suffix=1"),
	}
	if calls := sshCalls(s); !slices.EqualFunc(calls, want, slices.Equal) {
		t.Errorf("ssh ran %q, want %q", calls, want)
	}
	for _, call := range s.SSHCalls() {
		log := call.Argv[slices.Index(call.Argv, "-E")+1]
		if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("ssh's log %s stays: %v", log, err)
		}
	}
}

// Any status but 255 is the host cld's, and a refusal in ssh's log ends the tries, as each
// would count towards a fail2ban's ban. Without -s, the host's cld chose the session's SUFFIX,
// which no try can name. Each makes one call of ssh, with ssh's log after it.
func TestHostJoinOnce(t *testing.T) {
	t.Parallel()
	refusal, hostKey := "me@ws: Permission denied (publickey).", "Host key verification failed."
	dropLine := "client_loop: send disconnect: Broken pipe"
	for _, test := range []struct {
		name, exit, log, suffix string
		code                    int
		stderr                  string
	}{
		{"a status of the host's cld", "1", "", "1", 1, ""},
		{"a login refused", "255", refusal, "1", 255, refusal + "\n"},
		{"a host key refused", "255", hostKey, "1", 255, hostKey + "\n"},
		{"no -s", "255", dropLine, "", 255, dropLine + "\ncld: ssh to ws ended with status 255: " +
			"join again with -s SUFFIX (see cld list --host ws)\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			env := map[string]string{"PATH": s.Tools("ssh"), "CLD_FAKE_SSH_EXIT": test.exit}
			if test.log != "" {
				env["CLD_FAKE_SSH_LOG"] = test.log
			}
			args, words := []string{"join", "--host", "ws", "-n", "app"}, []string{"join", "--name=app"}
			if test.suffix != "" {
				args, words = append(args, "-s", test.suffix), append(words, "--suffix="+test.suffix)
			}
			result := s.RunCldOnTerminal(env, args...)
			if result.Code != test.code || result.Stderr != test.stderr {
				t.Errorf("exit %d, stderr %q, want exit %d, stderr %q", result.Code, result.Stderr,
					test.code, test.stderr)
			}
			if reset := strings.Contains(result.Stdout, leaveAlternate); reset != (test.code == 255) {
				t.Errorf("reset %t after status %d: %q", reset, test.code, result.Stdout)
			}
			checkSSH(t, s, joinSSH("ws", words...))
		})
	}
}

// Where ssh never connected, the host's cld has seen no join. A try then runs the first words
// again, with -w and ARGS, no --existing, and with or without -s.
func TestHostJoinUnreached(t *testing.T) {
	t.Parallel()
	for _, line := range []string{"ssh: connect to host ws port 22: Connection timed out",
		"ssh: Could not resolve hostname ws: Temporary failure in name resolution"} {
		for _, suffix := range [][]string{{"-s", "2"}, nil} {
			t.Run(line[:strings.Index(line, " ws")]+" "+strings.Join(suffix, " "), func(t *testing.T) {
				t.Parallel()
				s := sandbox.New(t)
				env := map[string]string{"PATH": s.Tools("ssh"), "CLD_FAKE_SSH_EXIT": "255,0",
					"CLD_FAKE_SSH_LOG": line}
				args := append([]string{"join", "--host", "ws", "-n", "app", "-w"}, suffix...)
				result := s.RunCldOnTerminal(env, append(args, "--", "fix-it")...)
				if want := line + "\n" + dropNote + line + "\n"; result.Code != 0 || result.Stderr != want {
					t.Errorf("exit %d, stderr %q, want exit 0, stderr %q", result.Code, result.Stderr, want)
				}
				first := []string{"join", "--name=app"}
				if suffix != nil {
					first = append(first, "--suffix=2")
				}
				first = append(first, "--worktree=true", "--", "fix-it")
				want := [][]string{joinSSH("ws", first...), joinSSH("ws", first...)}
				if calls := sshCalls(s); !slices.EqualFunc(calls, want, slices.Equal) {
					t.Errorf("ssh ran %q, want %q", calls, want)
				}
			})
		}
	}
}

// ssh logs "Permission denied, please try again." after a mistyped password, and goes on.
// That is no refusal, and a later drop joins again.
func TestHostJoinMistypedPassword(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	log := "Permission denied, please try again.\nLoad key \"k\": Permission denied"
	result := s.RunCldOnTerminal(map[string]string{"PATH": s.Tools("ssh"),
		"CLD_FAKE_SSH_EXIT": "255,0", "CLD_FAKE_SSH_LOG": log}, "join", "--host", "ws", "-n", "app",
		"-s", "1")
	if result.Code != 0 {
		t.Errorf("exit %d, stderr %q, want exit 0 after a second try", result.Code, result.Stderr)
	}
	if n := len(s.SSHCalls()); n != 2 {
		t.Errorf("ssh ran %d times, want 2", n)
	}
}
