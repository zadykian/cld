package session

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/claudetemp"
)

// marked reports whether session cld-SUFFIX has its run mark in the record's directory dir.
func marked(dir, suffix string) bool {
	_, err := os.Stat(companion(dir, suffix, runMark))
	return err == nil
}

// leaveStartMark writes the start mark beside the entry in file of the session that cld, attached,
// is about to have tmux make. The mark is cld's process ID, which the tmux client it becomes keeps
// (decision 50.4). One cld cannot write is none, as the record serves the sessions (decision 40.7).
func leaveStartMark(file string) {
	mark := strings.TrimSuffix(file, ".json") + startMark
	_ = replace(mark, []byte(strconv.Itoa(os.Getpid())+"\n")) //nolint:errcheck // explained above
}

// starting reports whether another cld is starting session cld-SUFFIX (decision 50.4). Its start
// mark is then younger than lockWait and names a process that runs, as any answer of kill but ESRCH
// says. One older, as a failed new-session can leave, may name another process by now.
func starting(suffix string) bool {
	dir, err := StateDir()
	if err != nil {
		return false
	}
	file := companion(dir, suffix, startMark)
	info, err := os.Stat(file)
	if err != nil || time.Since(info.ModTime()) > lockWait {
		return false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	return !errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}

// setMarks is the run-shell, as tmux's words, that follows new-session to set the marks beside
// the entry in file (decision 48.1). It removes the busy and start marks, and makes the run mark
// run unless that is "", as restore passes to keep the mark's time (decision 48.8). It prints
// nothing and exits 0, as tmux would show anything else (docs/design/findings/tmux-commands.md).
func setMarks(file, run string) []string {
	base := strings.TrimSuffix(file, ".json")
	sh := `rm -f ` + shellWord(base+busyMark) + ` ` + shellWord(base+startMark)
	if run != "" {
		sh += `; touch ` + shellWord(run)
	}
	return []string{"run-shell", unexpanded(`{ ` + sh + `; } 2>/dev/null || true`)}
}

// unmark is the sh command, run-shell's argument, that removes session cld-SUFFIX's run mark in
// the command that ends the session, before kill-session (decision 48.1). Where cld has no
// record's directory, unmark is "". The command prints nothing and exits 0, as the session ends
// all the same, and has every "#" doubled, as run-shell expands a format.
func unmark(suffix string) string {
	dir, err := StateDir()
	if err != nil {
		return ""
	}
	return unexpanded(`rm -f ` + shellWord(companion(dir, suffix, runMark)) + ` 2>/dev/null || true`)
}

// leaving is unmark for a command that ends session cld-SUFFIX with kill-session, given to
// run-shell with -t claudePane. It first writes claude's directory beside the entry, as tmux has it
// in that pane (decision 56.3). tmux puts that in single quotes for sh, escaping each quote in it,
// so no character of a path runs. Where cld has no record's directory, leaving is "".
func leaving(suffix string) string {
	dir, err := StateDir()
	if err != nil {
		return ""
	}
	value := `'#{s/'/'\\''/:pane_current_path}'`
	return dirWriter(entryFile(dir, suffix), value, unexpanded) + `; ` + unmark(suffix)
}

// conversation is claude's ID of a conversation as a basic regular expression of sed: a UUID,
// which no character of a path can stretch.
const conversation = `[0-9a-fA-F]\{8\}-[0-9a-fA-F]\{4\}-[0-9a-fA-F]\{4\}-[0-9a-fA-F]\{4\}-` +
	`[0-9a-fA-F]\{12\}`

// removeTemp is the sh command that removes claude's temp directory, in each project of the temp
// root, as a session ends (decision 67.2). The directory is the conversation's that the entry in
// file names. sh reads the ID at the end of the entry's line, where the hook writes it (see
// entryWriter). It prints nothing and exits 0, as the session ends all the same.
func removeTemp(file string) string {
	return `{ id=$(sed -n 's/.*"conversation":"\(` + conversation + `\)"}$/\1/p' ` +
		shellWord(file) + `) && [ -n "$id" ] && rm -rf ` + claudetemp.RootWord() +
		`/*/"$id"; } 2>/dev/null || true`
}

// unmarkExited is the sh command of the pane-died hook for claude's exit with status 0. It removes
// the run mark run, and then claude's temp directory of the conversation that the entry beside run
// names (decisions 48.2 and 67.2).
func unmarkExited(run string) string {
	return `rm -f ` + shellWord(run) + `; ` + removeTemp(strings.TrimSuffix(run, runMark)+".json")
}

// tempGone is removeTemp for session cld-SUFFIX's entry, with every "#" doubled for run-shell's
// format. Where cld has no record's directory, tempGone is "".
func tempGone(suffix string) string {
	dir, err := StateDir()
	if err != nil {
		return ""
	}
	return unexpanded(removeTemp(entryFile(dir, suffix)))
}
