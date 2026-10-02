package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld setup restore against the fake systemctl and loginctl (see probe), and completion, which
// runs neither (decision 48.9).

// The calls of systemctl and loginctl that setup restore makes.
var (
	showEnvironment = []string{"systemctl", "--user", "show-environment"}
	daemonReload    = []string{"systemctl", "--user", "daemon-reload"}
	enableUnit      = []string{"systemctl", "--user", "enable", "cld-restore.service"}
	showLinger      = []string{
		"loginctl", "show-user", strconv.Itoa(os.Getuid()), "--property=Linger", "--value",
	}
)

// What setup restore says of the unit it enabled, and of lingering.
const (
	unitEnabled  = "Enabled cld-restore.service: your systemd runs cld restore as it starts\n"
	lingeringOff = "Lingering is off: your systemd starts, and cld restore with it, " +
		"at your first login, and at your last logout ends the sessions cld restore brought " +
		"back; loginctl enable-linger has it start at boot, and keeps them\n"
	lingeringOn = "Lingering is on: your systemd starts at boot, and cld restore with it\n"
)

// systemdQuoted is value as setup restore writes it in the unit, in double quotes: "\" and "\""
// escaped, "%" doubled, and in ExecStart "$" doubled.
func systemdQuoted(value string, command bool) string {
	value = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(value)
	if command {
		value = strings.ReplaceAll(value, "$", "$$")
	}
	return `"` + value + `"`
}

// restoreUnit is the unit setup restore writes, where cld runs with the environment env: it runs
// cld, by the file it runs from, as cld restore, with the PATH, and the TMUX_TMPDIR,
// XDG_STATE_HOME and CLD_IDLE_DAYS that env sets.
func restoreUnit(t *testing.T, env map[string]string) string {
	t.Helper()
	cld, err := filepath.EvalSymlinks(sandbox.Cld)
	if err != nil {
		t.Fatal(err)
	}
	environment := ""
	for _, name := range []string{"PATH", "TMUX_TMPDIR", "XDG_STATE_HOME", "CLD_IDLE_DAYS"} {
		if value, set := env[name]; set {
			environment += "Environment=" + systemdQuoted(name+"="+value, false) + "\n"
		}
	}
	return "# Written by cld setup restore: at login, or at boot with lingering on, cld restore\n" +
		"# brings back the sessions of cld that ran when the machine stopped.\n" +
		"[Unit]\nDescription=Restore cld's sessions\n\n" +
		"[Service]\nType=oneshot\nRemainAfterExit=yes\nKillMode=process\n" + environment +
		"ExecStart=" + systemdQuoted(cld, true) + " restore\n\n" +
		"[Install]\nWantedBy=default.target\n"
}

// checkSetupRestore runs setup restore in s with extra, as when says, and checks its status, its
// output and the calls of systemctl and loginctl it makes.
func checkSetupRestore(
	t *testing.T, s *sandbox.Sandbox, when string, extra map[string]string,
	code int, stdout, stderr string, calls ...[]string,
) {
	t.Helper()
	before := len(s.SystemdCalls())
	checkExit(t, when, s.RunCld(extra, "setup", "restore"), code, stdout, stderr)
	if got := s.SystemdCalls()[before:]; !slices.EqualFunc(got, calls, slices.Equal) {
		t.Errorf("%s: calls %q, want %q", when, got, calls)
	}
}

// checkUnit checks that the unit at path is the one restoreUnit gives for env.
func checkUnit(t *testing.T, path string, env map[string]string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != restoreUnit(t, env) {
		t.Errorf("the unit: %v\n%s\nwant\n%s", err, data, restoreUnit(t, env))
	}
}

// setup restore writes the unit, quoted as systemd reads it, has the user's systemd read it,
// enables it, and says whether lingering is on. Run again it leaves the unit unchanged, but for a
// variable that changed. Without systemd it writes nothing, and a failure after the write says so.
func TestSetupRestore(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("setup restore works on Linux only; TestSetupRestoreLinuxOnly checks the refusal")
	}
	s := sandbox.New(t)
	unit := filepath.Join(s.Home, ".config", "systemd", "user", "cld-restore.service")
	checkSetupRestoreRefused(t, s, unit)

	state := filepath.Join(s.Root, `state 100% $HOME "q" \x`)
	env := map[string]string{"XDG_STATE_HOME": state, "CLD_IDLE_DAYS": "7.5"}
	checkSetupRestore(t, s, "the first time", env, 0,
		"Created "+unit+"\n"+unitEnabled+lingeringOff, "",
		showEnvironment, daemonReload, enableUnit, showLinger)
	all := map[string]string{
		"PATH": s.Env["PATH"], "TMUX_TMPDIR": s.Env["TMUX_TMPDIR"],
		"XDG_STATE_HOME": state, "CLD_IDLE_DAYS": "7.5",
	}
	checkUnit(t, unit, all)
	env["CLD_FAKE_LINGER"] = "yes"
	checkSetupRestore(t, s, "again", env, 0,
		"Left "+unit+" as it was\n"+unitEnabled+lingeringOn, "",
		showEnvironment, enableUnit, showLinger)
	delete(env, "XDG_STATE_HOME")
	delete(env, "CLD_IDLE_DAYS")
	checkSetupRestore(t, s, "without XDG_STATE_HOME and CLD_IDLE_DAYS", env, 0,
		"Updated "+unit+"\n"+unitEnabled+lingeringOn, "",
		showEnvironment, daemonReload, enableUnit, showLinger)
	delete(all, "XDG_STATE_HOME")
	delete(all, "CLD_IDLE_DAYS")
	checkUnit(t, unit, all)
	checkSetupRestoreFailed(t, s, unit, state)
	result := s.RunCld(nil, "setup", "restore", "x")
	want := "cld: setup restore: unexpected argument 'x' (see cld help)\n"
	if result.Code != 2 || result.Stderr != want {
		t.Errorf("an argument: exit %d, stderr %q, want exit 2, stderr %q",
			result.Code, result.Stderr, want)
	}
}

// checkSetupRestoreRefused checks that setup restore writes no unit at unit without systemctl,
// where systemctl --user fails, or with a CLD_IDLE_DAYS that restore would refuse.
func checkSetupRestoreRefused(t *testing.T, s *sandbox.Sandbox, unit string) {
	t.Helper()
	checkSetupRestore(t, s, "without systemctl", map[string]string{"PATH": s.Tools()}, 1, "",
		"cld: setup restore needs systemd, and systemctl is not installed\n")
	checkSetupRestore(t, s, "without a user manager",
		map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "show-environment"}, 1, "",
		"cld: setup restore needs your user's systemd: systemctl --user show-environment: "+
			"fake systemctl --user show-environment failed\n",
		showEnvironment)
	checkSetupRestore(t, s, "with CLD_IDLE_DAYS no number of days",
		map[string]string{"CLD_IDLE_DAYS": "7d"}, 1, "",
		"cld: CLD_IDLE_DAYS is not a number of days: '7d'\n")
	if exists(unit) {
		t.Fatal("setup restore wrote the unit without systemd")
	}
}

// checkSetupRestoreFailed checks setup restore where loginctl fails, which leaves lingering
// unknown, and where systemctl's enable fails, before or after a change to unit.
func checkSetupRestoreFailed(t *testing.T, s *sandbox.Sandbox, unit, state string) {
	t.Helper()
	linger := strings.Join(showLinger, " ")
	checkSetupRestore(t, s, "where loginctl fails",
		map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "show-user"}, 0,
		"Left "+unit+" as it was\n"+unitEnabled+"Cannot tell whether lingering is on ("+
			linger+": fake "+linger+" failed): without it, "+
			strings.TrimPrefix(lingeringOff, "Lingering is off: "), "",
		showEnvironment, enableUnit, showLinger)
	failed := "cld: systemctl --user enable cld-restore.service: " +
		"fake systemctl --user enable cld-restore.service failed"
	checkSetupRestore(t, s, "where enable fails",
		map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "enable"}, 1, "", failed+"\n",
		showEnvironment, enableUnit)
	checkSetupRestore(t, s, "where enable fails after a change",
		map[string]string{"CLD_FAKE_SYSTEMD_FAIL": "enable", "XDG_STATE_HOME": state}, 1, "",
		failed+" ("+unit+" written before it)\n",
		showEnvironment, daemonReload, enableUnit)
}

// On any system but Linux setup restore refuses to run, whatever its arguments, before it looks
// for systemd; -h still shows its help.
func TestSetupRestoreLinuxOnly(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "linux" {
		t.Skip("the refusal is for systems other than Linux")
	}
	s := sandbox.New(t)
	for _, args := range [][]string{{}, {"x"}, {"--bogus"}} {
		result := s.RunCld(nil, append([]string{"setup", "restore"}, args...)...)
		want := "cld: setup restore works on Linux only\n"
		if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("%q: exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
				args, result.Code, result.Stdout, result.Stderr, want)
		}
	}
	if calls := s.SystemdCalls(); len(calls) != 0 {
		t.Errorf("systemd ran: %q", calls)
	}
	result := s.RunCld(nil, "setup", "restore", "-h")
	if result.Code != 0 || !strings.HasPrefix(result.Stdout, "have your user's systemd") {
		t.Errorf("-h: exit %d, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr)
	}
}

// Completion runs none of the checks of setup restore, nor restore itself: whatever it completes
// after either, it runs no systemctl or loginctl, nor tmux, and offers nothing.
func TestCompletionRunsNoSystemd(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"__complete", "setup", "restore", ""},
		{"__complete", "setup", "restore", "-"},
		{"__complete", "restore", ""},
		{"__completeNoDesc", "restore", "-"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			result := s.RunCld(map[string]string{"PATH": s.Tools("systemctl", "loginctl")}, args...)
			directive := result.Stdout == ":4\n" || strings.HasSuffix(result.Stdout, "\n:4\n")
			stderr := "Completion ended with directive: ShellCompDirectiveNoFileComp\n"
			if result.Code != 0 || !directive || result.Stderr != stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout ending in :4, "+
					"stderr %q", result.Code, result.Stdout, result.Stderr, stderr)
			}
			if calls := s.SystemdCalls(); len(calls) != 0 {
				t.Errorf("systemd ran: %q", calls)
			}
		})
	}
}
