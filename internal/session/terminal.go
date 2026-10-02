package session

import (
	"os"

	"golang.org/x/term"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
)

// checkTerminal refuses a terminal tmux could not attach from, for command, join, once its other
// checks have passed. cld's stdin has to be a terminal, and TERM set, not empty and not dumb
// (decision 31.1). tmux would fail without a word of cld's, and leave the server's socket behind.
func checkTerminal(command string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fail.Runtime(command + " needs a terminal, and its input is not one")
	}
	name, set := os.LookupEnv("TERM")
	switch {
	case !set:
		return fail.Runtime(command + " needs a terminal, and TERM is not set")
	case name == "":
		return fail.Runtime(command + " needs a terminal, and TERM is empty")
	case name == "dumb":
		return fail.Runtime(command + " needs a terminal, and TERM is dumb")
	}
	return nil
}

// printTitle prints Title for join, where stdout is a terminal. tmux draws on the terminal of cld's
// stdin, and a pipe or a file would take the escape in as text (decision 31.4).
func printTitle(suffix string) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	return output.Print(Title(suffix))
}

// Title is what sets the terminal's title to the name of session cld-SUFFIX after the marker ✳.
// join and the session list print it before tmux starts, which then keeps the title (decision
// 25.6).
func Title(suffix string) string {
	return "\033]0;✳ cld-" + suffix + "\007"
}

// titles is the title tmux gives the terminals on session cld-SUFFIX (decisions 25 and 26). Its
// name comes after claude's marker, busyMarker while @cld-status is busy and ✳ otherwise, and
// before " [w]" while claude is in a linked git worktree. claude's own title stays out, as it
// never turns.
func titles(suffix string) string {
	return "#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} cld-" + suffix +
		"#{?@cld-worktree, [w],}"
}

// busyMarker is claude's marker while busy, ◐ and ◑ in turn by strftime's seconds. A job
// refreshes the terminal a second later to turn it, as status off leaves tmux no timer (decision
// 25.5). The job names tmux by @cld-tmux, quoted, as a path may hold "#", "%" or ")".
const busyMarker = "#{?#{m:*[02468],%S},◐,◑}" +
	"#((sleep 1; #{q:@cld-tmux} -S #{q:socket_path} refresh-client -S -t #{q:client_name})" +
	" >/dev/null 2>&1 &)"
