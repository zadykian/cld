// Package restore is cld setup restore: it has the user's systemd run cld restore as it starts, so
// that the sessions a reboot ended come back (see Tmux.Restore in internal/session). Setup runs
// these steps in this order, so that nothing is written where it would not run:
//
//   - checks: systemctl on the PATH (see tool.LookPath), then the user's systemd answering
//     systemctl --user show-environment: without it - a container, a system without systemd, a
//     shell with no user manager of its own - nothing is written. Linux alone has systemd (see
//     Supported), which cld refuses to go past elsewhere
//   - the unit: ~/.config/systemd/user/cld-restore.service, where the user's systemd reads units -
//     from $XDG_CONFIG_HOME/systemd/user only where its own environment sets that, which the
//     shell's does not tell - written as internal/configfile writes a file, where it differs.
//     Type=oneshot, with RemainAfterExit=yes: the unit is done once cld restore returns, and
//     stays active. KillMode=process: stopping the unit ends cld restore, if it still runs, and
//     not the tmux servers it started, which stay in the unit's cgroup - a snap's tmux moves its
//     server to a scope of its own (see docs/design/findings/environment.md).
//     WantedBy=default.target, which the user's systemd starts as it starts: at boot where
//     lingering is on, and otherwise at the first login. ExecStart names this cld by the file it
//     runs from, which cld update replaces in place, and Environment gives cld restore what the
//     user's systemd has none of: the PATH cld runs with now, where cld restore finds tmux, and
//     TMUX_TMPDIR and XDG_STATE_HOME where set, where the sessions' sockets and cld's record are,
//     and CLD_IDLE_DAYS where set, past which cld restore leaves a session ended. Each goes quoted,
//     as systemd reads C's escapes, specifiers (%) and, in ExecStart, variables ($)
//   - enable: systemctl --user daemon-reload where the unit changed, so that the user's systemd
//     reads it, then systemctl --user enable cld-restore.service, which links it into
//     default.target.wants: once more changes nothing
//   - lingering: loginctl show-user UID --property=Linger --value. Without it the user's systemd
//     starts at the first login, not at boot, and stops at the last logout, which ends what runs in
//     its units, the sessions cld restore started among them. The report names loginctl
//     enable-linger, which cld does not run: logind may ask for a password
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
// (see the package comment); PATH goes whether set or not.
var variables = []string{"PATH", "TMUX_TMPDIR", "XDG_STATE_HOME", "CLD_IDLE_DAYS"}

// unit is the unit file that runs cld, the file at that path, as cld restore.
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

// quoted is value as a word of a unit file, in double quotes: systemd reads C's escapes in it,
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

// Setup has the user's systemd run cld, the file at that path, as cld restore as it starts (see
// the package comment), and returns what it did.
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
	const without = "your systemd starts, and cld restore with it, at your first login, and at your last " +
		"logout ends the sessions cld restore brought back; loginctl enable-linger has it start at boot, and keeps them\n"
	switch linger, err := run("loginctl", "show-user", strconv.Itoa(os.Getuid()), "--property=Linger", "--value"); {
	case err != nil:
		report.WriteString("Cannot tell whether lingering is on (" + err.Error() + "): without it, " + without)
	case linger == "yes":
		report.WriteString("Lingering is on: your systemd starts at boot, and cld restore with it\n")
	default:
		report.WriteString("Lingering is off: " + without)
	}
	return report.String(), nil
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
