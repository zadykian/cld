package tests

import (
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The command, and arguments that no command takes.

func TestRequiresCommand(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	for _, args := range [][]string{{}, {""}, {"", "join"}} {
		result := s.RunCld(nil, args...)
		if result.Code != 2 || !strings.HasPrefix(result.Stderr, "cld: missing command") ||
			result.Stdout != "" {
			t.Errorf("cld %q: exit %d, stdout %q, stderr %q",
				args, result.Code, result.Stdout, result.Stderr)
		}
	}
	if sessions := s.Sessions(); len(sessions) != 0 {
		t.Errorf("sessions %q, want none", sessions)
	}
}

// Every unknown word is an unknown command, with no pointer to join: a name, as cld NAME once
// took, and new and resume, which join took over (decision 50.6). The first argument is the
// command: an option there is no command either, and -v is one only elsewhere.
func TestRejectsUnknownCommands(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"review"}, "cld: unknown command 'review' (see cld help)\n"},
		{[]string{"new"}, "cld: unknown command 'new' (see cld help)\n"},
		{[]string{"resume", "-s", "x"}, "cld: unknown command 'resume' (see cld help)\n"},
		{[]string{"-x"}, "cld: unknown command '-x' (see cld help)\n"},
		{[]string{"a.b"}, "cld: unknown command 'a.b' (see cld help)\n"},
		{[]string{"-v"}, "cld: unknown command '-v' (see cld help)\n"},
		{[]string{"-n", "x", "join"}, "cld: unknown command '-n' (see cld help)\n"},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(nil, test.args...), 2, test.want)
			if sessions := s.Sessions(); len(sessions) != 0 {
				t.Errorf("sessions %q, want none", sessions)
			}
		})
	}
}

// unexpected is the message for command given word, an argument it does not take.
func unexpected(command, word string) string {
	return "cld: " + command + ": unexpected argument '" + word + "' (see cld help)\n"
}

// unknownCommand is the message for command given word, which names no command it takes.
func unknownCommand(command, word string) string {
	return "cld: " + command + ": unknown command '" + word + "' (see cld help)\n"
}

// argumentCases are command lines with an argument their command does not take, and the message.
var argumentCases = []struct {
	args []string
	want string
}{
	{[]string{"join", "review"}, unexpected("join", "review")},
	{[]string{"join", "-x"}, unexpected("join", "-x")},
	{[]string{"join", "-n"}, "cld: option '-n' needs a value (see cld help)\n"},
	{[]string{"join", "--name"}, "cld: option '--name' needs a value (see cld help)\n"},
	{[]string{"kill", "a"}, unexpected("kill", "a")},
	{[]string{"detach", "-w"}, unexpected("detach", "-w")},
	{[]string{"kill", "-n", "a", "--worktree"}, unexpected("kill", "--worktree")},
	{[]string{"kill", "--new"}, unexpected("kill", "--new")},
	{[]string{"detach", "--resume", "x"}, unexpected("detach", "--resume")},
	{[]string{"list", "-n", "a"}, unexpected("list", "-n")},
	{[]string{"update", "x"}, unexpected("update", "x")},
	{[]string{"update", "--check"}, unexpected("update", "--check")},
	{[]string{"update", "--"}, unexpected("update", "--")},
	{[]string{"help", "nope"}, unknownCommand("help", "nope")},
	{[]string{"help", "new"}, unknownCommand("help", "new")},
	{[]string{"help", "resume"}, unknownCommand("help", "resume")},
	{[]string{"help", "join", "kill"}, unexpected("help", "kill")},
	{[]string{"help", "nope", "join"}, unknownCommand("help", "nope")},
	{[]string{"help", "-V"}, unexpected("help", "-V")},
	{[]string{"help", ""}, unknownCommand("help", "")},
	{[]string{"help", "--", "join"}, unexpected("help", "--")},
	{[]string{"help", "join", "--"}, unexpected("help", "--")},
	{[]string{"help", "join", "-h"}, unexpected("help", "-h")},
	{[]string{"help", "join", "project"}, unexpected("help", "project")},
	{[]string{"help", "setup", "nope"}, unknownCommand("help", "setup nope")},
	{[]string{"help", "setup", "telemetry"}, unknownCommand("help", "setup telemetry")},
	{[]string{"help", "setup", "restore", "x"}, unexpected("help", "x")},
	{[]string{"help", "setup", "config", "project", "restore"}, unexpected("help", "restore")},
	{[]string{"help", "setup", "project"}, unknownCommand("help", "setup project")},
	{[]string{"help", "setup", "config", "nope"}, unknownCommand("help", "setup config nope")},
	{[]string{"help", "setup", "config", "user", "x"}, unexpected("help", "x")},
	{[]string{"help", "project"}, unknownCommand("help", "project")},
	{[]string{"help", "telemetry"}, unknownCommand("help", "telemetry")},
	{[]string{"help", "completion", "tcsh"}, unknownCommand("help", "completion tcsh")},
	{[]string{"help", "completion", "bash", "x"}, unexpected("help", "x")},
	{[]string{"-h", "-n", "x"}, unexpected("-h", "-n")},
	{[]string{"--help", "x"}, unknownCommand("--help", "x")},
	{[]string{"version", "-n", "a"}, unexpected("version", "-n")},
	{[]string{"-V", "x"}, unexpected("-V", "x")},
	{[]string{"join", "review", "--", "-p"}, unexpected("join", "review")},
	{[]string{"kill", "--", "-x"}, unexpected("kill", "--")},
	{[]string{"list", "--"}, unexpected("list", "--")},
	{[]string{"join", "a", "-x"}, unexpected("join", "a")},
	{[]string{"join", "review", "-n"}, unexpected("join", "review")},
	{[]string{"join", "review", "-h"}, unexpected("join", "review")},
	{[]string{"join", "-d"}, unexpected("join", "-d")},
	{[]string{"join", "--detach-others=maybe"}, unexpected("join", "--detach-others=maybe")},
	{[]string{"join", "--detach-others", "a"}, unexpected("join", "a")},
	{[]string{"join", "--new=maybe"}, unexpected("join", "--new=maybe")},
	{[]string{"join", "--resume", "a", "b"}, unexpected("join", "b")},
	{[]string{"join", "--resume", "a", "b", "--", "-p"}, unexpected("join", "b")},
	{[]string{"join", "x", "-n", "y"}, unexpected("join", "x")},
	{[]string{"join", "x", "-h", "--"}, unexpected("join", "x")},
	{[]string{"join", "-n", "x", ""}, unexpected("join", "")},
	{[]string{"join", "", "--", "-p"}, unexpected("join", "")},
	{[]string{"join", "-"}, unexpected("join", "-")},
	{[]string{"join", "x", "--fork"}, unexpected("join", "x")},
	{[]string{"join", "--fork=maybe", "--resume", "x"}, unexpected("join", "--fork=maybe")},
	{[]string{"join", "-f", "x"}, unexpected("join", "-f")},
	{[]string{"detach", "x"}, unexpected("detach", "x")},
	{[]string{"detach", "-s", "x", "y"}, unexpected("detach", "y")},
	{[]string{"detach", "--detach-others"}, unexpected("detach", "--detach-others")},
	{[]string{"detach", "--others"}, unexpected("detach", "--others")},
	{[]string{"detach", "--"}, unexpected("detach", "--")},
	// An empty argument is one too (cld's shell script took it for none).
	{[]string{"list", ""}, unexpected("list", "")},
	// cobra would show its help and exit 0 for an unknown shell, fail with exit status 1 for
	// an argument after it, and read an option after an argument.
	{[]string{"completion", "tcsh"}, "cld: completion: unknown shell 'tcsh' (see cld help)\n"},
	{[]string{"completion", ""}, "cld: completion: unknown shell '' (see cld help)\n"},
	{[]string{"completion", "bash", "x"}, unexpected("completion bash", "x")},
	{[]string{"completion", "zsh", "x", "--bogus"}, unexpected("completion zsh", "x")},
	{[]string{"completion", "fish", "-x"}, unexpected("completion fish", "-x")},
	{[]string{"completion", "--no-descriptions", "bash"},
		unexpected("completion", "--no-descriptions")},
	{[]string{"completion", "--", "bash"}, unexpected("completion", "--")},
	{[]string{"completion", "bash", "--"}, unexpected("completion bash", "--")},
	{[]string{"setup", "completion", "zsh", "x"}, unexpected("setup completion zsh", "x")},
	{[]string{"setup", "completion", "bash", "--"}, unexpected("setup completion bash", "--")},
	{[]string{"setup", "completion", "fish", "--no-descriptions"},
		unexpected("setup completion fish", "--no-descriptions")},
	{[]string{"setup", "completion", "zsh", "x", "-h"}, unexpected("setup completion zsh", "x")},
	{[]string{"help", "setup", "completion", "tcsh"}, unknownCommand("help", "setup completion tcsh")},
	{[]string{"help", "setup", "completion", "zsh", "x"}, unexpected("help", "x")},
}

// Arguments are read left to right, and the first wrong one decides the message. An option after
// an argument is not read, and -- ends only join's options (decision 12.4). As arguments, help
// takes a command of cld's, join only the words after "--", and completion a SHELL. Messages name
// help and version as typed, and completion's commands with their SHELL.
func TestRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()
	for _, test := range argumentCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(nil, test.args...), 2, test.want)
		})
	}
}
