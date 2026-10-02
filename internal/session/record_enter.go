package session

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"

	"github.com/zadykian/cld/internal/fail"
)

// enter makes the directory the session of entry r ran in the current one, PWD naming it too.
// join and restore start claude there to resume the session's conversation (decision 40.4). A
// directory that cannot be entered is refused (see enterError).
func enter(r entry) error {
	if err := enterable(r); err != nil {
		return err
	}
	if err := os.Chdir(r.Directory); err != nil {
		return enterError(r, err)
	}
	if err := os.Setenv("PWD", r.Directory); err != nil {
		return fail.Runtime(err.Error())
	}
	return nil
}

// enterable is nil where the directory of entry r can be entered, and otherwise why join refuses
// to resume the session's conversation there.
func enterable(r entry) error {
	if _, err := os.Stat(r.Directory); err != nil {
		return enterError(r, err)
	}
	if err := unix.Access(r.Directory, unix.X_OK); err != nil {
		return enterError(r, err)
	}
	return nil
}

// enterError refuses the directory of entry r, which err says cannot be entered. Its advice is the
// join that resumes the conversation in the current directory instead, by the entry's ID or else
// by the session's name.
func enterError(r entry, err error) error {
	conversation := r.Conversation
	if conversation == "" {
		conversation = "cld-" + r.Name
	}
	advice := "; resume it from here with cld join " + Options(r.Name) + " --resume " + conversation
	if errors.Is(err, fs.ErrNotExist) {
		return &fail.Error{Status: 1, Advice: advice,
			Message: "session '" + r.Name + "' ran in " + r.Directory + ", which no longer exists"}
	}
	if errno, ok := errors.AsType[unix.Errno](err); ok {
		err = errno
	}
	return &fail.Error{Status: 1, Advice: advice, Message: "cannot enter " + r.Directory +
		", where session '" + r.Name + "' ran: " + err.Error()}
}
