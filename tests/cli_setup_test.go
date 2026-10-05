package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup's commands.

// setupCommands and setupShells end the messages for a missing or unknown argument of setup, and
// of setup completion.
const (
	setupCommands = "cld setup project, cld setup completion SHELL or cld setup restore " +
		"(see cld help)\n"
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
	{[]string{"setup", "", "restore"}, "cld: setup: missing command: " + setupCommands},
	{[]string{"setup", "other"}, "cld: setup: unknown command 'other': " + setupCommands},
	{[]string{"setup", "other", "project"}, "cld: setup: unknown command 'other': " + setupCommands},
	// setup telemetry is gone, as new and resume are (decision 52.1).
	{[]string{"setup", "telemetry"}, "cld: setup: unknown command 'telemetry': " + setupCommands},
	{[]string{"setup", "telemetry", "--local", "http://127.0.0.1:4319"},
		"cld: setup: unknown command 'telemetry': " + setupCommands},
	{[]string{"setup", "-x"}, "cld: setup: unknown command '-x': " + setupCommands},
	{[]string{"setup", "--help=false", "restore"},
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
// before it (decision 52.3). The shell after setup completion is checked the same way. A mistake
// writes nothing, in the work directory or the home directory.
func TestSetupRequiresCommand(t *testing.T) {
	t.Parallel()
	for _, test := range setupCommandCases {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			checkFailed(t, s.RunCld(nil, test.args...), 2, test.want)
			if calls := s.SystemdCalls(); len(calls) != 0 {
				t.Errorf("%s ran: %q", calls[0][0], calls[0][1:])
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
