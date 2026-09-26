// Package configfile reads and writes the files cld edits for claude: its user settings, whose env
// setup telemetry edits. An edit keeps what cld does not change.
//
// A JSON file is read with json.Decoder's tokens into the members of its top-level object, in the
// file's order, their values as the file has them, byte for byte, and written back in that order,
// one member a line, in the file's indentation, which a file Claude Code writes keeps as it was.
// An object cld changes inside it is written the same way (see JSON.Object). A key given twice:
// claude reads its files with JSON.parse, which keeps the last, so cld edits the last (see Last).
//
// A file is written to a temporary file beside it, with its mode, and renamed over it, so that
// claude never reads it half written; a symbolic link keeps pointing at it. A missing file, and
// its directories, are created, as Claude Code would: 0644 and 0755, less the umask.
package configfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// Read reads the file at path, which need not exist, unless it is a symbolic link: one that leads
// to no file is refused, since writing it would replace the link with a file.
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
		_ = os.Remove(file.Name())
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
		switch e := err.(type) {
		case *os.PathError:
			err = e.Err
		case *os.SyscallError:
			err = e.Err
		default:
			return err
		}
	}
}

// Member is a key of a JSON object with its value, as the file has it.
type Member struct {
	Key   string
	Value json.RawMessage
}

// JSON is a file holding a JSON object, read into its members.
type JSON struct {
	*File
	// Members are the members of the file's object; none for a file that does not exist.
	Members []Member
	// indent is one level of the file's indentation.
	indent string
}

// ReadJSON reads the file at path as Read does, and the JSON object it holds. A missing file
// holds none, and is written indented by two spaces, as Claude Code writes its files.
func ReadJSON(path string) (*JSON, error) {
	f, err := Read(path)
	if err != nil {
		return nil, err
	}
	j := &JSON{File: f, indent: "  "}
	if !f.Exists {
		return j, nil
	}
	j.indent = indentation(f.Data)
	if j.Members, err = Object(f.Data); errors.Is(err, ErrNotObject) {
		return nil, fail.Runtime(path + " holds no JSON object")
	} else if err != nil {
		return nil, fail.Runtime(path + " is not valid JSON: " + invalid(f.Data, err))
	}
	return j, nil
}

// ErrNotObject is what Object says of JSON that is some other value.
var ErrNotObject = errors.New("it is not an object")

// Object reads data, a JSON object and nothing more, into its members in order.
func Object(data []byte) ([]Member, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil {
		return nil, err
	} else if token != json.Delim('{') {
		return nil, ErrNotObject
	}
	var members []Member
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		members = append(members, Member{Key: token.(string), Value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("more follows the object")
	}
	return members, nil
}

// invalid says what is wrong with data, which Object refused with err, and where.
func invalid(data []byte, err error) string {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Sprintf("line %d: %v", bytes.Count(data[:syntax.Offset], []byte("\n"))+1, err)
	}
	if errors.Is(err, io.EOF) {
		return "it is empty"
	}
	return err.Error()
}

// indentation is one level of the indentation of data, a JSON object: the white space that
// starts the line of its first member, or two spaces where it is on the object's line.
func indentation(data []byte) string {
	start := bytes.IndexByte(data, '{')
	if start < 0 {
		return "  "
	}
	rest := data[start+1:]
	space := rest[:len(rest)-len(bytes.TrimLeft(rest, " \t\r\n"))]
	if line := bytes.LastIndexByte(space, '\n'); line >= 0 && line+1 < len(space) {
		return string(space[line+1:])
	}
	return "  "
}

// Last is the index of the last of members with key, which claude reads; -1 for none.
func Last(members []Member, key string) int {
	for i := len(members) - 1; i >= 0; i-- {
		if members[i].Key == key {
			return i
		}
	}
	return -1
}

// String is s as a JSON string, with <, > and & as they are.
func String(s string) json.RawMessage {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n")
}

// Encode is the file's members as the file holds them, ending with a newline.
func (j *JSON) Encode() []byte {
	return append(j.Object(j.Members, 0), '\n')
}

// Write replaces the file with its members, atomically.
func (j *JSON) Write() error {
	return j.File.Write(j.Encode())
}

// Object is an object of members as the file holds it, depth levels deep: the object's line is
// indented depth levels, its members, one a line, a level more.
func (j *JSON) Object(members []Member, depth int) json.RawMessage {
	if len(members) == 0 {
		return json.RawMessage("{}")
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, m := range members {
		b.WriteString(strings.Repeat(j.indent, depth+1))
		b.Write(String(m.Key))
		b.WriteString(": ")
		b.Write(m.Value)
		if i < len(members)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat(j.indent, depth) + "}")
	return b.Bytes()
}
