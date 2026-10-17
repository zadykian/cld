// Package remote runs one of cld's commands on another host for --host HOST (decision 62). cld
// becomes ssh, which runs cld in the user's login shell on the host. There, the user's profile
// sets the PATH to cld, tmux and claude, and that cld does the work: here, cld needs ssh alone.
// Plain words go as they are, and else all go as __host VALUE (see Encode). A join in a pane
// moves its words to a terminal by the same rules (decision 51.3).
package remote

import (
	"encoding/base64"
	"os"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/tool"
)

// Command is the hidden command of cld's that runs words sent as Encode's VALUE.
const Command = "__host"

// hostPattern is a HOST that goes to ssh as it stands (decision 62.2).
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]*$`)

// ValidHost reports whether host is one --host takes, of 255 characters at most (decision 62.2).
func ValidHost(host string) bool { return len(host) <= 255 && hostPattern.MatchString(host) }

// plain matches a word that sh, bash, zsh, fish, ksh and csh each read unchanged, unquoted. An "="
// may not start it, as zsh would take =word for the path of the program word.
var plain = regexp.MustCompile(`^[A-Za-z0-9_@+:,./-][A-Za-z0-9_@+=:,./-]*$`)

// Plain reports whether every shell that a move or --host serves reads word unchanged, unquoted
// (decisions 51.3 and 62.4).
func Plain(word string) bool { return plain.MatchString(word) }

// argv is the ssh that runs cld with words on host. With terminal, ssh gives the host's cld a
// terminal, and leaves out its "Connection closed" line; otherwise none, nor this one's input.
// ssh's own RemoteCommand would refuse a command given (decision 62.3).
func argv(host string, words []string, terminal bool) []string {
	argv := []string{"ssh", "-T", "-n"}
	if terminal {
		argv = []string{"ssh", "-t", "-o", "LogLevel=ERROR"}
	}
	return append(argv, "-o", "RemoteCommand=none", "--", host, remoteCommand(words))
}

// remoteCommand is the command ssh hands the host's shell. A login shell runs cld with words, for
// the PATH, as restore tabs's line does (decisions 59.7 and 62.5).
func remoteCommand(words []string) string {
	if slices.ContainsFunc(words, func(w string) bool { return !Plain(w) }) {
		words = []string{Command, Encode(words)}
	}
	return "exec $SHELL -lc 'exec cld " + strings.Join(words, " ") + "'"
}

// Exec replaces cld with argv's ssh, as join replaces cld with tmux. The host's cld's exit status
// is then cld's, or ssh's own 255. It returns only where that fails.
func Exec(host string, words []string, terminal bool) error {
	path, err := tool.LookPath("ssh")
	if err != nil {
		return fail.Runtime("ssh is not installed")
	}
	return tool.CannotRun(path, syscall.Exec(path, argv(host, words, terminal), os.Environ()))
}

// Encode is words joined by NULs, which no argument has, in URL-safe base64 without padding: a
// plain word, which every shell and tmux's parser read as it stands. That makes __host's VALUE,
// and with a directory first, join --moved's (decision 51.3).
func Encode(words []string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(words, "\x00")))
}

// Decode is the words of a value Encode made, and whether value is one.
func Decode(value string) ([]string, bool) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return strings.Split(string(data), "\x00"), true
}
