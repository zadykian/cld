package session

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/remote"
)

// moving is the command a terminal runs in its client's place to move: exec of the cld at path
// cld, as join --switched-from FROM, then words. FROM is the session C-q L goes back to (decision
// 51.4). Each word goes as the user's shell takes it (see shellArgument), and every later release
// keeps --switched-from and --moved (decision 51.2).
func moving(cld, from string, words []string) string {
	var command strings.Builder
	command.WriteString("exec " + shellArgument(cld) + " join --switched-from " + shellArgument(from))
	for _, word := range words {
		command.WriteString(" " + shellArgument(word))
	}
	return command.String()
}

// shellArgument is word as one word of the command a terminal runs as it moves: bare where plain,
// and otherwise within single quotes, with each "'" and "\" outside them after a "\". Every shell
// the move serves reads that alike, where fish reads sh's quoting otherwise (decision 51.3).
func shellArgument(word string) string {
	if remote.Plain(word) {
		return word
	}
	var quoted, run strings.Builder
	quote := func() {
		if run.Len() > 0 {
			quoted.WriteString("'" + run.String() + "'")
			run.Reset()
		}
	}
	for i := 0; i < len(word); i++ {
		if b := word[i]; b == '\'' || b == '\\' {
			quote()
			quoted.WriteByte('\\')
			quoted.WriteByte(b)
		} else {
			run.WriteByte(b)
		}
	}
	quote()
	if quoted.Len() == 0 {
		return "''"
	}
	return quoted.String()
}

// encodeMove is the value of join's --moved, by remote.Encode. It carries directory dir and words
// to the terminal a join in a pane moves (decision 51.3).
func encodeMove(dir string, words []string) string {
	return remote.Encode(append([]string{dir}, words...))
}

// Moved is join --moved, which a terminal runs as a join in a pane moves it. It enters the
// directory value carries, PWD naming it, and returns the words to join (decision 51.3). A value
// with no absolute directory is refused with status 2, and a directory it cannot enter with 1.
func Moved(value string) ([]string, error) {
	fields, ok := remote.Decode(value)
	if !ok || !filepath.IsAbs(fields[0]) {
		return nil, fail.Usage(fmt.Sprintf("invalid value '%s' for --moved (see cld help)", value))
	}
	dir := fields[0]
	if err := os.Chdir(dir); err != nil {
		if pathError, ok := errors.AsType[*fs.PathError](err); ok {
			err = pathError.Err
		}
		return nil, fail.Runtime("cannot enter " + dir + ", where cld join ran: " + err.Error())
	}
	if err := os.Setenv("PWD", dir); err != nil {
		return nil, fail.Runtime(err.Error())
	}
	return fields[1:], nil
}
