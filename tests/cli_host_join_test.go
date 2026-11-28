package tests

import (
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// join --host on a terminal runs ssh as its child, and joins again after a drop (decision 68).

// Any status is the host cld's, or ssh's own 255, with ssh's log after it. Each makes one call
// of ssh.
func TestHostJoinOnce(t *testing.T) {
	t.Parallel()
	timeout := "ssh: connect to host ws port 22: Connection timed out"
	for _, test := range []struct {
		name, exit, log, suffix string
		code                    int
		stderr                  string
	}{
		{"a status of the host's cld", "1", "", "1", 1, ""},
		{"no -s", "255", timeout, "", 255, timeout + "\n"},
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
			checkSSH(t, s, joinSSH("ws", words...))
		})
	}
}
