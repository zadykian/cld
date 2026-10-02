package session

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/fail"
)

// switchTerminal has the terminal of sw run cld join with words in its client's place (see
// moving). checkShell first passes the shell tmux runs it with (decision 51.3). A key names the
// terminal by its client; otherwise tmux takes the one a bare detach takes (see DetachTerminal).
func (t *Tmux) switchTerminal(sw *Switch, words ...string) error {
	cld, err := self()
	if err != nil {
		return fail.Runtime("cannot find the file cld runs from: " + err.Error())
	}
	command := moving(cld, sw.suffix, words)
	if err := t.checkShell(sw); err != nil {
		return err
	}
	if sw.client != "" {
		return t.detachClient(sw, command)
	}
	return t.detachFromPane(sw, command)
}

// detachClient has the client of sw, which a key named, run command in its place. The popup cld
// may run in closes with the detach, and tmux can end cld before this returns, so nothing follows
// (decision 51.3).
func (t *Tmux) detachClient(sw *Switch, command string) error {
	detach := t.command("-S", sw.socket, "detach-client", "-t", sw.client, "-E", literal(command))
	if err := detach.Run(); err != nil {
		return t.exitStatus(err)
	}
	return nil
}

// detachFromPane has the terminal used last on the pane's session run command in its place
// (decision 51.3). An if keeps it to a marked server and that session's terminals; command goes in
// it quoted for sh, which tmux's parser reads alike. With no terminal to move, cld refuses, as it
// refuses words that make the command too long.
func (t *Tmux) detachFromPane(sw *Switch, command string) error {
	args := []string{"display-message", "-p", mark + " #{session_attached}", ";",
		"if", "-F", "#{&&:" + mark + ",#{session_attached}}", "detach-client -E " + shellWord(command)}
	if size := commandSize(args); size > commandLimit {
		return fail.Usage(fmt.Sprintf("join's words make tmux's command %d bytes, and tmux takes "+
			"%d at most: give claude long text in a file, as with --append-system-prompt-file",
			size, commandLimit))
	}
	detach := t.command(append([]string{"-S", sw.socket}, args...)...)
	out, err := detach.Output()
	if err != nil {
		return t.exitStatus(err)
	}
	marked, attached, _ := strings.Cut(strings.TrimSuffix(string(out), "\n"), " ")
	switch {
	case marked != "1":
		return &fail.Error{Status: 1,
			Message: "tmux server cld-" + sw.suffix + " is not one of cld's",
			Advice:  "; run cld join in a terminal (see cld help)"}
	case attached == "0":
		return &fail.Error{Status: 1,
			Message: "no terminal is attached to this session for join to move",
			Advice:  "; run cld join in a terminal (see cld help join)"}
	}
	return nil
}

// shells are the base names of the default-shells the move's command is written for: sh, bash,
// zsh, fish, ksh, csh and theirs (decision 51.3). Each has exec, and reads a word bare or quoted
// alike.
var shells = []string{"sh", "ash", "dash", "bash", "ksh", "mksh", "oksh", "yash", "zsh", "fish",
	"csh", "tcsh"}

// checkShell refuses to move the terminal of sw where tmux would run the command with another
// default-shell, nu or pwsh say (decision 51.3). Such a shell could leave it out of any session.
// tmux shows the session's default-shell for the terminal's client, or else for cld's pane.
func (t *Tmux) checkShell(sw *Switch) error {
	args := []string{"-S", sw.socket, "display-message", "-p"}
	if sw.client != "" {
		args = append(args, "-c", sw.client)
	}
	out, err := t.command(append(args, "#{default-shell}")...).Output()
	if err != nil {
		return t.exitStatus(err)
	}
	shell := strings.TrimSuffix(string(out), "\n")
	if slices.Contains(shells, filepath.Base(shell)) {
		return nil
	}
	return &fail.Error{Status: 1,
		Message: "cannot move the terminal with tmux's default-shell '" + shell +
			"': cld writes the move for sh, bash, zsh, fish, ksh and csh alone",
		Advice: "; detach with C-q d and run cld join (see cld help join)"}
}
