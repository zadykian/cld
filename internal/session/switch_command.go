package session

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zadykian/cld/internal/fail"
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

// plain matches a word that sh, bash, zsh, fish, ksh and csh each read unchanged, unquoted. An "="
// may not start it, as zsh would take =word for the path of the program word.
var plain = regexp.MustCompile(`^[A-Za-z0-9_@+:,./-][A-Za-z0-9_@+=:,./-]*$`)

// shellArgument is word as one word of the command a terminal runs as it moves: bare where plain,
// and otherwise within single quotes, with each "'" and "\" outside them after a "\". Every shell
// the move serves reads that alike, where fish reads sh's quoting otherwise (decision 51.3).
func shellArgument(word string) string {
	if plain.MatchString(word) {
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

// encodeMove is the value of join's --moved, which carries directory dir and words to the terminal
// a join in a pane moves (decision 51.3). They go joined by NULs, which no argument has, in
// URL-safe base64 without padding, which every shell and tmux's parser read as it stands.
func encodeMove(dir string, words []string) string {
	joined := strings.Join(append([]string{dir}, words...), "\x00")
	return base64.RawURLEncoding.EncodeToString([]byte(joined))
}

// Moved is join --moved, which a terminal runs as a join in a pane moves it. It enters the
// directory value carries, PWD naming it, and returns the words to join (decision 51.3). A value
// with no absolute directory is refused with status 2, and a directory it cannot enter with 1.
func Moved(value string) ([]string, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	fields := strings.Split(string(data), "\x00")
	if err != nil || !filepath.IsAbs(fields[0]) {
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
