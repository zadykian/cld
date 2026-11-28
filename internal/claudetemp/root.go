// Package claudetemp is claude's temp root, where claude keeps a directory for each conversation,
// and the removal of the directories that no claude needs any more (decision 67).
package claudetemp

import (
	"os"
	"path/filepath"
	"strconv"
)

// RootWord is claude's temp root as a word of sh, which takes the variables from the environment
// it runs in. A tmux server's run-shell has the server's, as claude in it has. claude keeps
// claude-UID in CLAUDE_CODE_TMPDIR, or else in the system's temporary directory, as Node's
// os.tmpdir takes it from TMPDIR, TMP or TEMP, by default /tmp. An empty variable counts as unset
// (decision 67.1).
func RootWord() string {
	return `"${CLAUDE_CODE_TMPDIR:-${TMPDIR:-${TMP:-${TEMP:-/tmp}}}}"/` + rootName()
}

// Root is claude's temp root for cld's own environment, as RootWord has it for sh.
func Root() string {
	for _, name := range []string{"CLAUDE_CODE_TMPDIR", "TMPDIR", "TMP", "TEMP"} {
		if dir := os.Getenv(name); dir != "" {
			return filepath.Join(dir, rootName())
		}
	}
	return filepath.Join("/tmp", rootName())
}

// rootName is the name of the temp root in its directory: claude-UID.
func rootName() string {
	return "claude-" + strconv.Itoa(os.Getuid())
}
