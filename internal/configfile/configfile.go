// Package configfile reads and writes the files cld edits: claude's settings, a project's
// .mcp.json and .gitignore, completion scripts, .zshrc and a systemd unit. An edit keeps what cld
// does not change (decision 19.4).
//
// A JSON file is read with json.Decoder's tokens into the members of its top-level object, each
// value byte for byte as the file has it. They are written back in the file's order, one a line,
// in the file's indentation, so a file Claude Code wrote keeps its look. JSON.Object, JSON.Array
// and JSON.Value write what cld changes or adds in that indentation. claude reads its files with
// JSON.parse, which keeps the last of a key given twice, so cld edits the last (see Last).
//
// A write goes to a temporary file beside the file, with its mode, renamed over it: claude never
// reads it half written, and a symbolic link keeps pointing at it. A missing file and its
// directories are created as Claude Code creates them: 0644 and 0755, less the umask.
package configfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// File is a file cld edits, as read.
type File struct {
	// Path is the file as named; target the file written, where a symbolic link at Path leads.
	Path, target string
	// Exists is whether the file exists, and Data what it holds.
	Exists bool
	Data   []byte
	mode   fs.FileMode
}

// Read reads the file at path, which need not exist. A symbolic link that leads to no file is
// refused, since writing it would replace the link with a file.
func Read(path string) (*File, error) {
	f := &File{Path: path, target: path, mode: 0o644}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Lstat(path); err == nil {
			return nil, fail.Runtime(path + " is a symbolic link to a file that does not exist")
		}
		return f, nil
	}
	if err != nil {
		return nil, fail.Runtime("cannot read " + path + ": " + reason(err).Error())
	}
	if f.target, err = filepath.EvalSymlinks(path); err != nil {
		return nil, fail.Runtime("cannot read " + path + ": " + reason(err).Error())
	}
	info, err := os.Stat(f.target)
	if err != nil {
		return nil, fail.Runtime("cannot read " + path + ": " + reason(err).Error())
	}
	f.Exists, f.Data, f.mode = true, data, info.Mode().Perm()
	return f, nil
}

// Write replaces the file with data, atomically.
func (f *File) Write(data []byte) error {
	dir := filepath.Dir(f.target)
	if !f.Exists {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail.Runtime("cannot create " + dir + ": " + reason(err).Error())
		}
	}
	file, err := createTemp(dir, filepath.Base(f.target), f.mode)
	if err != nil {
		return fail.Runtime("cannot write " + f.Path + ": " + reason(err).Error())
	}
	_, err = file.Write(data)
	if err == nil && f.Exists {
		err = file.Chmod(f.mode) // Exactly the file's mode, whatever the umask took away.
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), f.target)
	}
	if err != nil {
		_ = os.Remove(file.Name()) //nolint:errcheck // best effort: the failure is returned
		return fail.Runtime("cannot write " + f.Path + ": " + reason(err).Error())
	}
	return nil
}

// createTemp creates a new file in dir, for the file named name, with the permissions perm less
// the umask.
func createTemp(dir, name string, perm fs.FileMode) (*os.File, error) {
	for try := 0; ; try++ {
		temp := filepath.Join(dir, fmt.Sprintf(".%s.cld-%d-%d", name, os.Getpid(), time.Now().UnixNano()))
		file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if !errors.Is(err, fs.ErrExist) || try == 9 {
			return file, err
		}
	}
}

// reason is err without the operation and path that *os.PathError and *os.SyscallError add to it:
// cld's messages name the file themselves.
func reason(err error) error {
	for {
		if e, ok := errors.AsType[*os.PathError](err); ok {
			err = e.Err
		} else if e, ok := errors.AsType[*os.SyscallError](err); ok {
			err = e.Err
		} else {
			return err
		}
	}
}
