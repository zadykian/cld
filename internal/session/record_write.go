package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zadykian/cld/internal/output"
)

// remember writes r, the entry of the session create is about to make, and s, what its server
// starts with, under the lock the caller holds. It returns r's file and the run mark tmux makes
// (see setMarks). Both are "" where r failed, and the mark where the environment did, as restore
// could not start the session again. A failure is a warning (decisions 40.7 and 48.4).
func remember(r entry, s started, runs func(suffix string) bool) (file, run string) {
	file, run, err := write(r, s, runs)
	if err != nil {
		output.Warn("cannot record session '" + r.Name + "': " + err.Error())
	}
	return file, run
}

// write is remember's work, returning the first failure. It also forgets what has expired, but
// for the entries of the sessions whose servers run, as runs says, and gives the index r's name
// ends in. A failed environment leaves those to do all the same.
func write(r entry, s started, runs func(suffix string) bool) (file, run string, err error) {
	dir, err := stateDir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		return "", "", reason(err)
	}
	file = entryFile(dir, r.Name)
	if err := replace(file, entryLine(r)); err != nil {
		return "", "", err
	}
	// Before tmux makes the session, and so its run mark, as restore needs the environment of
	// any session it finds marked (decision 48.4).
	err = writeStarted(dir, r.Name, s)
	if err == nil {
		run = companion(dir, r.Name, runMark)
	}
	forgetExpired(dir, runs)
	indexErr := giveIndex(dir, r.Name)
	if err == nil {
		err = indexErr
	}
	return file, run, err
}

// writeStarted writes s as session cld-SUFFIX's NAME.env in the record's directory dir.
func writeStarted(dir, suffix string, s started) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return replace(companion(dir, suffix, environment), append(data, '\n'))
}

// forgetExpired removes the expired entries with the files beside them (decision 40.6). So go the
// files whose entry has gone, as a hook can mark a session forgotten meanwhile, and what a write
// cut short left, once as old. A file cld cannot remove stays, read as none.
func forgetExpired(dir string, runs func(suffix string) bool) {
	sessions := filepath.Join(dir, "sessions")
	files, _ := os.ReadDir(sessions) //nolint:errcheck // unlisted files stay
	stays := map[string]bool{}
	for _, file := range files {
		suffix, _, ok := sessionFile(file.Name())
		if ok && !entryStays(dir, suffix, runs, stays) || !ok && pastExpiry(file) {
			_ = os.Remove(filepath.Join(sessions, file.Name())) //nolint:errcheck // stays, as above
		}
	}
}

// entryStays reports whether the entry of session cld-SUFFIX stays, as stays remembers for its
// other files: one that has not expired, or whose server runs. Its claude's hooks touch the entry
// as the session is used, but make none that has gone.
func entryStays(dir, suffix string, runs func(suffix string) bool, stays map[string]bool) bool {
	if kept, known := stays[suffix]; known {
		return kept
	}
	info, err := os.Stat(entryFile(dir, suffix))
	kept := err == nil && (time.Since(info.ModTime()) <= expiry || runs(suffix))
	stays[suffix] = kept
	return kept
}

// pastExpiry reports whether file is older than expiry.
func pastExpiry(file fs.DirEntry) bool {
	info, err := file.Info()
	return err == nil && time.Since(info.ModTime()) > expiry
}

// giveIndex records the index that name ends in, if one does, as given in indexes.json, which
// keeps no index given longer ago than expiry.
func giveIndex(dir, name string) error {
	highest := indexes(dir)
	if prefix, index, ok := indexOf(name); ok {
		if g, found := highest[prefix]; !found || index >= g.Index {
			highest[prefix] = given{Index: index, Time: time.Now().UTC()}
		}
	}
	data, err := json.Marshal(highest)
	if err != nil {
		return err
	}
	return replace(filepath.Join(dir, "indexes.json"), append(data, '\n'))
}

// entryLine is r as sessions/NAME.json holds it: a line of JSON, without HTML's escapes, as the
// SessionStart hook writes it too (see entryWriter).
func entryLine(r entry) []byte {
	var line bytes.Buffer
	encoder := json.NewEncoder(&line)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(r) //nolint:errcheck,errchkjson // strings always encode, into memory
	return line.Bytes()
}

// replace replaces file with data: written to a temporary file beside it, readable by the user
// alone, then renamed over it. That way no reader sees it half written (decision 40.1).
func replace(file string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".*")
	if err != nil {
		return reason(err)
	}
	_, err = temp.Write(data)
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), file)
	}
	if err != nil {
		_ = os.Remove(temp.Name()) //nolint:errcheck // best effort: the failure is returned
		return reason(err)
	}
	return nil
}

// reason is err as the system says it, with the path it names.
func reason(err error) error {
	if pathError, ok := errors.AsType[*fs.PathError](err); ok {
		return errors.New(pathError.Path + ": " + pathError.Err.Error())
	}
	return err
}
