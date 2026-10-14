package session

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// Rename renames session cld-SUFFIX, which has ended, to cld-TO, for the list's Ctrl+R (decision
// 57). Both names are valid, and the caller holds the record's lock. It refuses what renamable and
// free refuse, and the same name renames nothing. The indexes are given first: a failure there
// moves nothing.
func (t *Tmux) Rename(ctx context.Context, suffix, to string) error {
	if to == suffix {
		return nil
	}
	r, err := t.renamable(ctx, suffix)
	if err != nil {
		return err
	}
	dir, _ := stateDir() //nolint:errcheck // recorded found the entry there
	if err := t.free(ctx, dir, to); err != nil {
		return err
	}
	// Once moved, the entry no longer counts the old name's index (see recordedNext), which is
	// given again as of now (decision 57.5).
	err = giveIndex(dir, suffix, true)
	if err == nil {
		err = giveIndex(dir, to, false)
	}
	if err == nil {
		err = move(dir, r, to)
	}
	if err != nil {
		return fail.Runtime("cannot rename session '" + suffix + "': " + reason(err).Error())
	}
	return nil
}

// renamable is the entry of session cld-SUFFIX, and else why Rename refuses it (decision 57.4). It
// refuses what Forget refuses, a server that still runs, and an entry with no conversation's ID.
func (t *Tmux) renamable(ctx context.Context, suffix string) (entry, error) {
	starts := starting(suffix)
	server, exists, _, _, err := t.lookup(ctx, suffix)
	switch {
	case err != nil:
		return entry{}, err
	case exists:
		return entry{}, runsAgain(suffix)
	case server:
		return entry{}, t.stillRuns(ctx, suffix)
	case starts:
		return entry{}, isStarting(suffix)
	}
	r, ok := recorded(suffix)
	if !ok {
		return entry{}, noSession(suffix)
	}
	if r.Conversation == "" {
		return entry{}, fail.Runtime(fmt.Sprintf("session '%s' has no conversation's ID to resume",
			suffix))
	}
	return r, nil
}

// free is nil where the name cld-TO is free, and else why not (decision 57.4). A session or a
// server may have it, or another cld may be starting it. Or the record's directory dir has a file
// of it, once what has expired is forgotten, as write forgets it.
func (t *Tmux) free(ctx context.Context, dir, to string) error {
	starts := starting(to)
	server, exists, _, _, err := t.lookup(ctx, to)
	switch {
	case err != nil:
		return err
	case exists:
		return fail.Runtime(fmt.Sprintf("session '%s' runs, and keeps its name", to))
	case server:
		return t.stillRuns(ctx, to)
	case starts:
		return isStarting(to)
	}
	forgetExpired(dir, t.runs)
	for _, file := range sessionFiles(dir, to) {
		_, err := os.Lstat(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fail.Runtime("cannot read cld's record: " + reason(err).Error())
		}
		if _, ok := recorded(to); ok {
			return fail.Runtime(fmt.Sprintf("session '%s' has ended, and keeps its name until "+
				"forgotten", to))
		}
		return fail.Runtime(fmt.Sprintf("session name '%s' is taken: cld's record has %s", to,
			filepath.Base(file)))
	}
	return nil
}

// move gives entry r and the files beside it in the record's directory dir the name to, keeping
// their times, and so the entry's expiry (see link). It then removes the old names. Where a link
// fails, it removes those made, and r stays whole. Once r's entry has gone, any file of r's that
// stays is left to forgetExpired.
func move(dir string, r entry, to string) error {
	from := entryFile(dir, r.Name)
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	made, err := link(dir, r, to, info.ModTime())
	if err == nil {
		err = os.Remove(from)
	}
	if err != nil {
		for _, file := range made {
			_ = os.Remove(file) //nolint:errcheck // best effort: the failure is returned
		}
		return err
	}
	for _, ext := range companions {
		_ = os.Remove(companion(dir, r.Name, ext)) //nolint:errcheck // left to forgetExpired, above
	}
	return nil
}

// link makes the files of session cld-TO in the record's directory dir: entry r, named to, with
// the time mtime, and a link to each file beside r. A link never replaces a file, as replace's
// rename would. It returns the files it made, up to the first failure.
func link(dir string, r entry, to string, mtime time.Time) ([]string, error) {
	file := entryFile(dir, to)
	moved := r
	moved.Name = to
	if err := create(file, entryLine(moved), mtime); err != nil {
		return nil, err
	}
	made := []string{file}
	for _, ext := range companions {
		file := companion(dir, to, ext)
		err := os.Link(companion(dir, r.Name, ext), file)
		switch {
		case err == nil:
			made = append(made, file)
		case !errors.Is(err, fs.ErrNotExist):
			return made, err
		}
	}
	return made, nil
}

// create makes file, which must not exist yet, holding data with the time mtime: a temporary file
// beside it (see writeTemp), linked as file.
func create(file string, data []byte, mtime time.Time) error {
	temp, err := writeTemp(file, data)
	if err != nil {
		return err
	}
	defer os.Remove(temp) //nolint:errcheck // a temporary file, which forgetExpired removes
	if err := os.Chtimes(temp, time.Time{}, mtime); err != nil {
		return err
	}
	return os.Link(temp, file)
}
