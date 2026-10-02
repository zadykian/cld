package tests

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The pieces of the tmux commands cld hands over, as the tests expect them word for word, and the
// environment tmux gets.

// endText says how claude exited, how to end its session with cld kill and options, and how to
// detach. It keeps what fits the pane's width whole (decisions 5 and 44.3). The widths count the
// widest exit, signal vtalrm, the border line's spaces and 4 cells of border.
func endText(options string) string {
	exited := "claude exited with " +
		"#{?pane_dead_signal,signal #{pane_dead_signal},status #{pane_dead_status}}"
	width := func(text string) string {
		return strconv.Itoa(len("claude exited with signal vtalrm"+text) + 6)
	}
	kill := ": cld kill " + options + " ends the session"
	detach := " C-q d or cld detach " + options + " detaches"
	return "#{?#{e|<:#{pane_width}," + width(kill+","+detach) + "}," +
		"#{?#{e|<:#{pane_width}," + width(kill) + "}," + exited + "," + exited + kill + "}," +
		exited + kill + "#," + detach + "}"
}

// endHint is how the pane-died hook that join sets, and join attaching, show endText on the
// message line.
func endHint(options string) string {
	return "display-message -d 0 '" + endText(options) + "'"
}

// shellQuoted is path quoted for sh.
func shellQuoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// marksCommand is the run-shell join has tmux run once it has made session name (decision 48.1).
// It removes the session's busy and start marks and makes its run mark, beside its entry in s's
// record, printing nothing and exiting 0 whatever happens. Each "#" is doubled for the format.
func marksCommand(s *sandbox.Sandbox, name string) []string {
	base := strings.TrimSuffix(entryFile(s, name), ".json")
	sh := "{ rm -f " + shellQuoted(base+".busy") + " " + shellQuoted(base+".start") + "; touch " +
		shellQuoted(base+".run") + "; } 2>/dev/null || true"
	return []string{"run-shell", strings.ReplaceAll(sh, "#", "##")}
}

// unmarkCommand is how kill and the idle sweep remove session name's run mark from cld's record in
// s, as run-shell takes it.
func unmarkCommand(s *sandbox.Sandbox, name string) string {
	run := strings.TrimSuffix(entryFile(s, name), ".json") + ".run"
	return strings.ReplaceAll("rm -f "+shellQuoted(run)+" 2>/dev/null || true", "#", "##")
}

// endHook is the pane-died hook join sets (decision 48.2). For status 0 it removes the run mark,
// the file run, and closes the pane, as tmux would. Otherwise it keeps endText on the pane's border
// and shows endHint to a terminal on claude's window. The path goes quoted for sh and for tmux.
func endHook(options, run string) string {
	sh := "rm -f " + shellQuoted(run)
	tmux := "'" + strings.ReplaceAll(strings.ReplaceAll(sh, "#", "##"), "'", `'\''`) + "'"
	return "if -F '#{==:#{pane_dead_status},0}' { run-shell " + tmux + " ; kill-pane } { " +
		"set -w pane-border-status bottom ; " +
		"set -p pane-border-format ' " + endText(options) + " ' ; " +
		`if -F '#{window_active_clients}' "` + endHint(options) + `" }`
}

// switchKeys are the keys join binds on the server of session name, run by the tmux at path tmux,
// to move the terminal to another session (decision 51.1). Each run-shell runs in the background,
// printing nothing, and names cld by its file. The paths go quoted for sh, each "#" doubled.
func switchKeys(t *testing.T, s *sandbox.Sandbox, tmux, name string) []string {
	t.Helper()
	cld, err := filepath.EvalSymlinks(sandbox.Cld)
	if err != nil {
		t.Fatal(err)
	}
	quoted := func(path string) string {
		return strings.ReplaceAll(shellQuoted(path), "#", "##")
	}
	list := quoted(cld) + " list --switch #{q:client_name}"
	popup := quoted(tmux) + " -S " + quoted(filepath.Join(s.SocketDir(), "cld-"+name)) +
		" display-popup -c #{q:client_name} -B -w 100% -h 100% -E " + list
	const quiet = " >/dev/null 2>&1 || true"
	return []string{
		"bind", "s", "run-shell", "-b", popup + quiet, ";",
		"bind", "(", "run-shell", "-b", list + " --to previous" + quiet, ";",
		"bind", ")", "run-shell", "-b", list + " --to next" + quiet, ";",
		"bind", "L", "run-shell", "-b", list + " --to last" + quiet, ";",
	}
}

// passedOn is the environment cld runs in with extra, as cld hands it on to tmux. That leaves out
// the variables in dropped, and empties a TMUX that was set.
func passedOn(s *sandbox.Sandbox, extra map[string]string, dropped ...string) map[string]string {
	env := map[string]string{}
	for _, variable := range s.Environ(extra) {
		name, value, _ := strings.Cut(variable, "=")
		env[name] = value
	}
	for _, name := range dropped {
		delete(env, name)
	}
	if _, set := env["TMUX"]; set {
		env["TMUX"] = ""
	}
	return env
}

// checkEnv reports each variable of the environment tmux got that is not as in want.
func checkEnv(t *testing.T, got, want map[string]string) {
	t.Helper()
	for name, value := range want {
		if gotValue, found := got[name]; !found || gotValue != value {
			t.Errorf("tmux gets %s %q (set: %v), want %q", name, gotValue, found, value)
		}
	}
	for name, value := range got {
		if _, wanted := want[name]; !wanted {
			t.Errorf("tmux gets %s=%q, want it unset", name, value)
		}
	}
}
