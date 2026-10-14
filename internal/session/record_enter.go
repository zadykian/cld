package session

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
)

// enter makes the directory the session of entry r comes back in (see backIn) the current one, PWD
// naming it too, and returns it. join and restore start claude there to resume the session's
// conversation (decisions 40.4 and 56.4). One that cannot be entered is refused (see enterError).
func enter(r entry) (string, error) {
	dir, err := enterable(r)
	if err != nil {
		return "", err
	}
	if err := os.Chdir(dir); err != nil {
		return "", enterError(r, dir, err)
	}
	if err := os.Setenv("PWD", dir); err != nil {
		return "", fail.Runtime(err.Error())
	}
	return dir, nil
}

// enterable is the directory the session of entry r comes back in, where it can be entered, and
// otherwise why join refuses to resume the session's conversation there.
func enterable(r entry) (string, error) {
	dir := backIn(r)
	if err := canEnter(dir); err != nil {
		return "", enterError(r, dir, err)
	}
	return dir, nil
}

// backIn is the directory the session of entry r comes back in, and shows once ended: claude's
// last, or else the entry's (see lastDirectory and decision 56.4).
func backIn(r entry) string {
	return lastDirectory(r.Name, r.Directory)
}

// lastDirectory is the directory session cld-SUFFIX's claude was last in, as NAME.dir has it. It
// takes only a whole path that can be entered, in no worktree claude made (decision 56.5). Else it
// gives dir, as for a cld before decision 56, and where NAME.dir names dir by another path.
func lastDirectory(suffix, dir string) string {
	state, err := stateDir()
	if err != nil {
		return dir
	}
	data, err := os.ReadFile(companion(state, suffix, lastDir))
	if err != nil {
		return dir
	}
	last := strings.TrimSuffix(string(data), "\n")
	if !filepath.IsAbs(last) || strings.Contains(last, "/.claude/worktrees/") ||
		canEnter(last) != nil || sameDirectory(last, dir) {
		return dir
	}
	return last
}

// sameDirectory reports whether a and b name one directory, as a symbolic link and its target do.
func sameDirectory(a, b string) bool {
	here, err := os.Stat(a)
	if err != nil {
		return false
	}
	there, err := os.Stat(b)
	return err == nil && os.SameFile(here, there)
}

// canEnter is nil where dir is a directory that can be entered, and otherwise why not.
func canEnter(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return unix.ENOTDIR
	}
	return unix.Access(dir, unix.X_OK)
}

// enterError refuses dir, where the session of entry r would come back, which err says cannot be
// entered. Its advice is the join that resumes the conversation in the current directory instead,
// by the entry's ID or else by the session's name.
func enterError(r entry, dir string, err error) error {
	conversation := r.Conversation
	if conversation == "" {
		conversation = "cld-" + r.Name
	}
	advice := "; resume it from here with cld join " + Options(r.Name) + " --resume " + conversation
	if errors.Is(err, fs.ErrNotExist) {
		return &fail.Error{Status: 1, Advice: advice,
			Message: "session '" + r.Name + "' ran in " + dir + ", which no longer exists"}
	}
	if errno, ok := errors.AsType[unix.Errno](err); ok {
		err = errno
	}
	return &fail.Error{Status: 1, Advice: advice, Message: "cannot enter " + dir +
		", where session '" + r.Name + "' ran: " + err.Error()}
}
