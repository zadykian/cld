package session

import (
	"strconv"
	"strings"
)

// how says how claude exited, and widest is how many cells it takes at most. tmux numbers a status
// up to 255 and a signal up to 64. It names the signal where the C library has sys_signame, as
// macOS does, vtalrm the longest.
const (
	how    = "#{?pane_dead_signal,signal #{pane_dead_signal},status #{pane_dead_status}}"
	widest = len("signal vtalrm")
)

// ending says, as a tmux format, how claude exited and how to end session cld-SUFFIX or detach
// from it, naming it as cld kill takes it (see Options). It says what fits the pane's width whole,
// dropping the detach first (decisions 5 and 44.3). It holds nothing that tmux's quotes or formats
// would change.
func ending(suffix string) string {
	options := Options(suffix)
	exited := "claude exited with " + how
	kill := exited + ": cld kill " + options + " ends the session"
	all := kill + "#, C-q d or cld detach " + options + " detaches"
	return fits(all, fits(kill, exited))
}

// fits is the format text where the pane is wide enough for it, and instead elsewhere. The
// border line, the narrower place ending goes, gets the pane's width less 4 cells (tmux 3.5a; 3.7c
// less 2), and how takes its widest. In text, "#," is a comma that does not end the branch.
func fits(text, instead string) string {
	width := len(strings.NewReplacer(how, strings.Repeat(" ", widest), "#,", ",").Replace(text)) + 6
	return "#{?#{e|<:#{pane_width}," + strconv.Itoa(width) + "}," + instead + "," + text + "}"
}

// hint shows ending on the message line until a key is pressed (decision 5): the pane-died hook
// shows it to a terminal attached then, join to one attaching later.
func hint(suffix string) string {
	return "display-message -d 0 '" + ending(suffix) + "'"
}

// died is the pane-died hook of claude's pane in session cld-SUFFIX. For claude's exit with status
// 0, it removes the run mark, run, and closes the pane, as no pane-exited hook would run (decision
// 48.2). Otherwise it keeps ending on the pane's border line, and shows the hint (decision 5).
func died(suffix, run string) string {
	exited := "kill-pane"
	if run != "" {
		// The path goes through tmux's parser, run-shell's format, where "##" is a "#", and sh.
		exited = "run-shell " + shellWord(unexpanded("rm -f "+shellWord(run))) + " ; kill-pane"
	}
	return "if -F '#{==:#{pane_dead_status},0}' { " + exited + " } { " +
		"set -w pane-border-status bottom ; set -p pane-border-format ' " + ending(suffix) + " ' ; " +
		"if -F '#{window_active_clients}' \"" + hint(suffix) + "\" }"
}
