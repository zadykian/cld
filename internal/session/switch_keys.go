package session

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// switchKeys are the keys create binds in a session's server, as tmux's words, for cld at path cld
// and tmux at path tmux on socket (decision 51.1). C-q s shows cld list --switch in a popup, and
// C-q (, C-q ) and C-q L add --to (decision 51.2). Where cld is "", as cld cannot find its file,
// tmux's keys stay.
func switchKeys(tmux, socket, cld string) []string {
	if cld == "" {
		return nil
	}
	// run-shell expands its command as a format: #{q:client_name} names the terminal, and a path's
	// "#" goes doubled (see unexpanded). tmux reads the popup's cld as a word of its command, which a
	// ";" would end (see literal).
	client := "#{q:client_name}"
	quiet := func(sh string) string { return sh + " >/dev/null 2>&1 || true" }
	list := unexpanded(shellWord(cld)) + " list --switch " + client
	popup := unexpanded(shellWord(tmux)) + " -S " + unexpanded(shellWord(socket)) +
		" display-popup -c " + client + " -B -w 100% -h 100% -E " +
		unexpanded(shellWord(literal(cld))) + " list --switch " + client
	return []string{
		"bind", "s", "run-shell", "-b", quiet(popup), ";",
		"bind", "(", "run-shell", "-b", quiet(list + " --to previous"), ";",
		"bind", ")", "run-shell", "-b", quiet(list + " --to next"), ";",
		"bind", "L", "run-shell", "-b", quiet(list + " --to last"), ";",
	}
}

// self is the file cld runs from, symbolic links resolved, by which the keys and a move name cld,
// as cld update replaces it in place (decision 51.1). A path with a control character, which tmux's
// parser and the shells do not all read alike, is refused (decision 51.3).
func self() (string, error) {
	file, err := os.Executable()
	if err == nil {
		file, err = filepath.EvalSymlinks(file)
	}
	if pathError, ok := errors.AsType[*os.PathError](err); ok {
		err = pathError.Err
	}
	if err == nil && strings.ContainsFunc(file, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return "", errors.New("its path, " + strconv.Quote(file) + ", has a control character")
	}
	return file, err
}
