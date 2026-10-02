package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The arguments that print a help, and which.

// helpCases are arguments that print a help, and its topic.
var helpCases = []struct {
	args  []string
	topic string
}{
	{[]string{"help"}, ""},
	{[]string{"-h"}, ""},
	{[]string{"--help"}, ""},
	{[]string{"help", "join"}, "join"},
	{[]string{"-h", "join"}, "join"},
	{[]string{"--help", "version"}, "version"},
	{[]string{"help", "help"}, "help"},
	{[]string{"join", "--help"}, "join"},
	{[]string{"join", "--resume", "x", "-h"}, "join"},
	{[]string{"join", "-h", "a", "b"}, "join"},
	{[]string{"join", "-s", "x", "-h"}, "join"},
	{[]string{"help", "detach"}, "detach"},
	{[]string{"detach", "-h"}, "detach"},
	{[]string{"detach", "-s", "x", "--help"}, "detach"},
	{[]string{"kill", "--help"}, "kill"},
	{[]string{"list", "-h"}, "list"},
	{[]string{"update", "-h"}, "update"},
	{[]string{"help", "update"}, "update"},
	{[]string{"update", "--help", "x"}, "update"},
	{[]string{"help", "-h"}, "help"},
	{[]string{"version", "--help"}, "version"},
	{[]string{"-V", "-h"}, "version"},
	{[]string{"--version", "--help"}, "version"},
	{[]string{"join", "-h", "-x"}, "join"},
	{[]string{"join", "-h", "-w"}, "join"},
	{[]string{"join", "-h", "--help=x"}, "join"},
	{[]string{"list", "-h", "-h=no"}, "list"},
	{[]string{"join", "--help", "--help=maybe"}, "join"},
	{[]string{"kill", "-h", "a"}, "kill"},
	{[]string{"help", "-h", "nope"}, "help"},
	{[]string{"help", "--help", "-x"}, "help"},
	{[]string{"completion"}, "completion"},
	{[]string{"completion", "--help"}, "completion"},
	{[]string{"-h", "completion"}, "completion"},
	{[]string{"completion", "-h", "tcsh"}, "completion"},
	{[]string{"completion", "-h", "--help=x"}, "completion"},
	{[]string{"completion", "--help", "-h=no"}, "completion"},
	// setup's commands: after it, or after help setup; -h after setup is setup's.
	{[]string{"help", "setup"}, "setup"},
	{[]string{"-h", "setup"}, "setup"},
	{[]string{"setup", "-h"}, "setup"},
	{[]string{"setup", "--help"}, "setup"},
	{[]string{"setup", "-h", "x"}, "setup"},
	{[]string{"help", "setup", "telemetry"}, "setup telemetry"},
	{[]string{"--help", "setup", "telemetry"}, "setup telemetry"},
	{[]string{"setup", "-h", "telemetry"}, "setup telemetry"},
	{[]string{"setup", "telemetry", "-h"}, "setup telemetry"},
	{[]string{"setup", "telemetry", "--help"}, "setup telemetry"},
	{[]string{"setup", "telemetry", "-h", "x"}, "setup telemetry"},
	{[]string{"setup", "telemetry", "--local", "x", "-h"}, "setup telemetry"},
	{[]string{"setup", "telemetry", "-h", "--bogus"}, "setup telemetry"},
	{[]string{"help", "setup", "project"}, "setup project"},
	{[]string{"-h", "setup", "project"}, "setup project"},
	{[]string{"setup", "-h", "project"}, "setup project"},
	{[]string{"setup", "project", "-h"}, "setup project"},
	{[]string{"setup", "project", "--help", "x"}, "setup project"},
	{[]string{"setup", "project", "--mcp", "idea", "-h"}, "setup project"},
	{[]string{"setup", "project", "-h", "--bogus"}, "setup project"},
	// setup completion's shells: after it, or after help setup completion; -h after setup
	// completion is its own.
	{[]string{"help", "setup", "completion"}, "setup completion"},
	{[]string{"setup", "-h", "completion"}, "setup completion"},
	{[]string{"setup", "completion", "-h"}, "setup completion"},
	{[]string{"setup", "completion", "--help", "x"}, "setup completion"},
	{[]string{"help", "setup", "completion", "zsh"}, "setup completion zsh"},
	{[]string{"-h", "setup", "completion", "bash"}, "setup completion bash"},
	{[]string{"setup", "completion", "-h", "fish"}, "setup completion fish"},
	{[]string{"setup", "completion", "zsh", "--help"}, "setup completion zsh"},
	{[]string{"setup", "completion", "bash", "-h", "x"}, "setup completion bash"},
	{[]string{"setup", "completion", "fish", "-h", "--no-descriptions"}, "setup completion fish"},
}

// help, -h and --help print the help of cld, or of the command they are given to: the one after
// them, or before -h. Given before a wrong argument, -h shows the help too, as arguments are read
// left to right. With no shell, completion shows its help, as cobra's does.
func TestHelp(t *testing.T) {
	t.Parallel()
	for _, test := range helpCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(goldenHelp(test.topic))
			if err != nil {
				t.Fatal(err)
			}
			s := sandbox.New(t)
			result := s.RunCld(nil, test.args...)
			if result.Code != 0 || result.Stdout != string(want) || result.Stderr != "" {
				t.Errorf("exit %d, stderr %q, stdout\n%s\nwant exit 0, stdout as in %s",
					result.Code, result.Stderr, result.Stdout, goldenHelp(test.topic))
			}
		})
	}
}
