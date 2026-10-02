// Package restore is cld setup restore, which has the user's systemd run cld restore as it
// starts (decision 48.9). cld restore brings back the sessions a reboot ended (see Tmux.Restore
// in internal/session). Setup runs these steps in this order, so that nothing is written where it
// would not run:
//
//   - checks: systemctl on the PATH (see tool.LookPath), then the user's systemd answering
//     systemctl --user show-environment; cld refuses any system but Linux before (see Supported)
//   - the unit: ~/.config/systemd/user/cld-restore.service, written as internal/configfile writes
//     a file, where it differs (see unit)
//   - enable: systemctl --user daemon-reload where the unit changed, then systemctl --user enable,
//     which links the unit into default.target.wants: once more changes nothing
//   - lingering: loginctl show-user, as the user's systemd starts at boot only with lingering on;
//     the report names loginctl enable-linger, which cld does not run
//
// The package prints nothing: Setup returns what it did, for cld to report.
package restore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

// Unit is the name of the systemd user unit that runs cld restore.
const Unit = "cld-restore.service"

// Supported refuses setup restore on any system but Linux, the one with systemd.
func Supported() error {
	if runtime.GOOS != "linux" {
		return fail.Runtime("setup restore works on Linux only")
	}
	return nil
}

// variables are the variables of cld's environment the unit gives cld restore where they are set
// (decision 48.9); PATH goes whether set or not.
var variables = []string{"PATH", "TMUX_TMPDIR", "XDG_STATE_HOME", "CLD_IDLE_DAYS"}

// unit is the unit file that runs cld, the file at that path, as cld restore. A oneshot unit, it
// stays active once run, leaves the tmux servers in its cgroup when stopped, and gives cld restore
// the variables it needs (decision 48.9).
func unit(cld string) []byte {
	var environment strings.Builder
	for _, name := range variables {
		if value, set := os.LookupEnv(name); name == "PATH" || set && value != "" {
			environment.WriteString("Environment=" + quoted(name+"="+value, false) + "\n")
		}
	}
	return []byte(`# Written by cld setup restore: at login, or at boot with lingering on, cld restore
# brings back the sessions of cld that ran when the machine stopped.
[Unit]
Description=Restore cld's sessions

[Service]
Type=oneshot
RemainAfterExit=yes
KillMode=process
` + environment.String() + `ExecStart=` + quoted(cld, true) + ` restore

[Install]
WantedBy=default.target
`)
}

// quoted is value as a word of a unit file, in double quotes. In it systemd reads C's escapes,
// "%" as the start of a specifier and, in a command, "$" as the start of a variable.
func quoted(value string, command bool) string {
	var word strings.Builder
	word.WriteByte('"')
	for _, r := range value {
		switch {
		case r == '\\' || r == '"':
			word.WriteString(`\` + string(r))
		case r == '%':
			word.WriteString("%%")
		case r == '$' && command:
			word.WriteString("$$")
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&word, `\x%02x`, r)
		default:
			word.WriteRune(r)
		}
	}
	word.WriteByte('"')
	return word.String()
}

// Setup has the user's systemd run cld, the file at that path, as cld restore as it starts, and
// returns what it did.
func Setup(cld string) (string, error) {
	if _, err := tool.LookPath("systemctl"); err != nil {
		return "", fail.Runtime("setup restore needs systemd, and systemctl is not installed")
	}
	if _, err := run("systemctl", "--user", "show-environment"); err != nil {
		return "", fail.Runtime("setup restore needs your user's systemd: " + err.Error())
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fail.Runtime("cannot find where the unit goes: " + err.Error())
	}
	path := filepath.Join(home, ".config", "systemd", "user", Unit)
	file, err := configfile.Read(path)
	if err != nil {
		return "", err
	}
	var report strings.Builder
	content := unit(cld)
	// written is what a failure after the unit was written adds to its message.
	written := ""
	switch {
	case file.Exists && bytes.Equal(file.Data, content):
		report.WriteString("Left " + path + " as it was\n")
	default:
		if err := file.Write(content); err != nil {
			return "", err
		}
		written = " (" + path + " written before it)"
		if file.Exists {
			report.WriteString("Updated " + path + "\n")
		} else {
			report.WriteString("Created " + path + "\n")
		}
		if _, err := run("systemctl", "--user", "daemon-reload"); err != nil {
			return "", fail.Runtime(err.Error() + written)
		}
	}
	if _, err := run("systemctl", "--user", "enable", Unit); err != nil {
		return "", fail.Runtime(err.Error() + written)
	}
	report.WriteString("Enabled " + Unit + ": your systemd runs cld restore as it starts\n")
	report.WriteString(lingering())
	return report.String(), nil
}

// lingering is the report's line on lingering, without which the user's systemd starts at the
// first login, and stops at the last logout (decision 48.9).
func lingering() string {
	const without = "your systemd starts, and cld restore with it, at your first login, " +
		"and at your last logout ends the sessions cld restore brought back; " +
		"loginctl enable-linger has it start at boot, and keeps them\n"
	uid := strconv.Itoa(os.Getuid())
	switch linger, err := run("loginctl", "show-user", uid, "--property=Linger", "--value"); {
	case err != nil:
		return "Cannot tell whether lingering is on (" + err.Error() + "): without it, " + without
	case linger == "yes":
		return "Lingering is on: your systemd starts at boot, and cld restore with it\n"
	}
	return "Lingering is off: " + without
}

// run runs the program name, found on the PATH, with args and no input, and returns what it
// printed on stdout, without the newlines at its end. What fails is an error naming the command
// and what it printed on stderr, or else how it failed.
func run(name string, args ...string) (string, error) {
	cmd, err := tool.Command(name, args...)
	if err != nil {
		return "", fmt.Errorf("%s is not installed", name)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &stdout, &stderr
	if err := cmd.Run(); err != nil {
		said := strings.TrimSpace(stderr.String())
		if said == "" {
			said = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), said)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}
