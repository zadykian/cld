package tests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup's commands, and setup telemetry's docker.

// setupCommands and setupShells end the messages for a missing or unknown argument of setup, and
// of setup completion.
const (
	setupCommands = "cld setup project, cld setup telemetry, cld setup completion SHELL or " +
		"cld setup restore (see cld help)\n"
	setupShells = "bash, zsh or fish (see cld help)\n"
)

// setupCommandCases are setup's command lines without one of its commands, or setup
// completion's without a shell, and the message.
var setupCommandCases = []struct {
	args []string
	want string
}{
	{[]string{"setup"}, "cld: setup: missing command: " + setupCommands},
	{[]string{"setup", ""}, "cld: setup: missing command: " + setupCommands},
	{[]string{"setup", "", "telemetry"}, "cld: setup: missing command: " + setupCommands},
	{[]string{"setup", "other"}, "cld: setup: unknown command 'other': " + setupCommands},
	{[]string{"setup", "other", "telemetry"}, "cld: setup: unknown command 'other': " + setupCommands},
	{[]string{"setup", "--local", "http://127.0.0.1:4319", "telemetry"},
		"cld: setup: unknown command '--local': " + setupCommands},
	{[]string{"setup", "-x"}, "cld: setup: unknown command '-x': " + setupCommands},
	{[]string{"setup", "--help=false", "telemetry"},
		"cld: setup: unknown command '--help=false': " + setupCommands},
	{[]string{"setup", "--"}, "cld: setup: unknown command '--': " + setupCommands},
	{[]string{"setup", "--mcp", "goland", "project"},
		"cld: setup: unknown command '--mcp': " + setupCommands},
	{[]string{"setup", "Project"}, "cld: setup: unknown command 'Project': " + setupCommands},
	{[]string{"setup", "completion"}, "cld: setup completion: missing shell: " + setupShells},
	{[]string{"setup", "completion", ""}, "cld: setup completion: missing shell: " + setupShells},
	{[]string{"setup", "completion", "", "zsh"},
		"cld: setup completion: missing shell: " + setupShells},
	{[]string{"setup", "completion", "tcsh"},
		"cld: setup completion: unknown shell 'tcsh': " + setupShells},
	{[]string{"setup", "completion", "powershell"},
		"cld: setup completion: unknown shell 'powershell': " + setupShells},
	{[]string{"setup", "completion", "Zsh"},
		"cld: setup completion: unknown shell 'Zsh': " + setupShells},
	// cobra would run zsh's for setup completion --help=false zsh.
	{[]string{"setup", "completion", "--help=false", "zsh"},
		"cld: setup completion: unknown shell '--help=false': " + setupShells},
	{[]string{"setup", "completion", "--no-descriptions", "bash"},
		"cld: setup completion: unknown shell '--no-descriptions': " + setupShells},
	{[]string{"setup", "completion", "--", "fish"},
		"cld: setup completion: unknown shell '--': " + setupShells},
}

// setup's first argument is one of its commands, checked as cld's first is: no option comes
// before it (decision 18.8). The shell after setup completion is checked the same way. A mistake
// writes nothing, in the work directory or the home directory.
func TestSetupRequiresCommand(t *testing.T) {
	t.Parallel()
	for _, test := range setupCommandCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(nil, test.args...), 2, test.want)
			if calls := s.DockerCalls(); len(calls) != 0 {
				t.Errorf("docker ran: %q", calls[0].Argv)
			}
			if entries, err := os.ReadDir(s.Work); err != nil || len(entries) != 0 {
				t.Errorf("the work directory holds %v (%v), want nothing", entries, err)
			}
			if entries, err := os.ReadDir(s.Home); err != nil || len(entries) != 1 {
				t.Errorf("the home directory holds %v (%v), want .tmux.conf alone", entries, err)
			}
		})
	}
}

// setup telemetry looks for docker first, as the other commands look for tmux: before it reads
// the settings, here not valid JSON.
func TestSetupTelemetryRequiresDocker(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	s := sandbox.New(t)
	settings := writeSettings(t, s, "{")
	result := s.RunCld(map[string]string{"PATH": s.Tools()},
		"setup", "telemetry", "--remote", "https://otel.example.com:4317")
	checkFailed(t, result, 1, "cld: docker is not installed\n")
	checkSettings(t, settings, "{")
}

// A docker the system cannot run ends cld as a tmux that cannot run does (see TestCannotRunTmux),
// from its first call, which looks for the collector: nothing has changed.
func TestCannotRunDocker(t *testing.T) {
	t.Parallel()
	linuxOnly(t)
	for _, broken := range unrunnable("docker") {
		t.Run(broken.what, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			fake := filepath.Join(s.Root, "fake")
			if err := os.Mkdir(fake, 0o755); err != nil {
				t.Fatal(err)
			}
			docker := filepath.Join(fake, "docker")
			s.WriteProgram(docker, broken.content, broken.mode)
			result := s.RunCld(map[string]string{"PATH": fake},
				"setup", "telemetry", "--remote", "https://otel.example.com:4317")
			want := "cld: cannot run " + docker + ": " + broken.reason + "\n"
			checkFailed(t, result, broken.code, want)
			if _, err := os.Stat(filepath.Join(s.Home, ".claude")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("~/.claude: %v, want none", err)
			}
		})
	}
}
