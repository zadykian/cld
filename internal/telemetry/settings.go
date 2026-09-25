package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// Claude Code's user settings, whose env Setup edits. The edit keeps what cld does not manage:
// the members of the top-level object and of env are read with json.Decoder's tokens into lists
// in the file's order, their values as the file has them, byte for byte, and written back in
// that order, in the file's indentation, which a file Claude Code writes keeps as it was. A key
// cld sets keeps its place, a new one goes at the end of env. The file is written to a
// temporary file beside it, with its mode, and renamed over it, so that claude never reads it
// half written; a symbolic link keeps pointing at it. A missing file, and its directory, are
// created, as Claude Code would: 0644 and 0755, less the umask.

// setting is an env key Setup manages: set to value, or removed.
type setting struct {
	key, value string
	remove     bool
}

// member is a key of a JSON object with its value, as the file has it.
type member struct {
	key   string
	value json.RawMessage
}

// settings is claude's settings file, read to be edited.
type settings struct {
	// path is the settings file; target the file written, where a symbolic link at path leads.
	path, target string
	exists       bool
	mode         fs.FileMode
	// members are the members of the top-level object; env those of its env object, the member
	// at envAt, or -1 for none.
	members []member
	env     []member
	envAt   int
	// indent is one level of the file's indentation.
	indent string
}

// settingsFile is the path of claude's user settings: settings.json in $CLAUDE_CONFIG_DIR, by
// default ~/.claude.
func settingsFile() (string, error) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fail.Runtime("cannot find claude's settings: " + err.Error())
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "settings.json"), nil
}

// readSettings reads the settings at path, which need not exist, into what they are made of.
func readSettings(path string) (*settings, error) {
	s := &settings{path: path, target: path, mode: 0o644, envAt: -1, indent: "  "}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Lstat(path); err == nil {
			return nil, fail.Runtime(path + " is a symbolic link to a file that does not exist")
		}
		return s, nil
	}
	if err != nil {
		return nil, fail.Runtime("cannot read " + path + ": " + reason(err).Error())
	}
	if s.target, err = filepath.EvalSymlinks(path); err != nil {
		return nil, fail.Runtime("cannot read " + path + ": " + reason(err).Error())
	}
	info, err := os.Stat(s.target)
	if err != nil {
		return nil, fail.Runtime("cannot read " + path + ": " + reason(err).Error())
	}
	s.exists, s.mode, s.indent = true, info.Mode().Perm(), indentation(data)
	if s.members, err = object(data); errors.Is(err, errNotObject) {
		return nil, fail.Runtime(path + " holds no JSON object")
	} else if err != nil {
		return nil, fail.Runtime(path + " is not valid JSON: " + invalid(data, err))
	}
	// A key given twice: JSON.parse, which claude reads the file with, keeps the last.
	for i, m := range s.members {
		if m.key == "env" {
			s.envAt = i
		}
	}
	if s.envAt >= 0 {
		if s.env, err = object(s.members[s.envAt].value); err != nil {
			return nil, fail.Runtime("env in " + path + " is not a JSON object")
		}
	}
	return s, nil
}

// errNotObject is what object says of JSON that is some other value.
var errNotObject = errors.New("it is not an object")

// object reads data, a JSON object and nothing more, into its members in order.
func object(data []byte) ([]member, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil {
		return nil, err
	} else if token != json.Delim('{') {
		return nil, errNotObject
	}
	var members []member
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		members = append(members, member{key: token.(string), value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("more follows the object")
	}
	return members, nil
}

// invalid says what is wrong with data, which object refused with err, and where.
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

// apply edits env as wanted says and returns what it changed, "KEY=VALUE" for a key set to a new
// value and "KEY removed", in wanted's order. A key given twice keeps its first place, and the
// value set, once.
func (s *settings) apply(wanted []setting) []string {
	var changes []string
	for _, w := range wanted {
		var env []member
		var old json.RawMessage // The last, which claude reads.
		at := -1
		for _, m := range s.env {
			if m.key != w.key {
				env = append(env, m)
				continue
			}
			old = m.value
			if at < 0 && !w.remove {
				at = len(env)
				env = append(env, m)
			}
		}
		s.env = env
		if w.remove {
			if old != nil {
				changes = append(changes, w.key+" removed")
			}
			continue
		}
		if at < 0 {
			s.env = append(s.env, member{key: w.key})
			at = len(s.env) - 1
		}
		s.env[at].value = jsonString(w.value)
		var value string
		if old == nil || json.Unmarshal(old, &value) != nil || value != w.value {
			changes = append(changes, w.key+"="+w.value)
		}
	}
	return changes
}

// encode is the settings as the file holds them: every member as it was, env as apply left it.
func (s *settings) encode() []byte {
	var env bytes.Buffer
	writeObject(&env, s.env, s.indent, s.indent)
	members, at := append([]member(nil), s.members...), s.envAt
	if at < 0 {
		members, at = append(members, member{key: "env"}), len(members)
	}
	members[at].value = env.Bytes()
	var out bytes.Buffer
	writeObject(&out, members, s.indent, "")
	out.WriteByte('\n')
	return out.Bytes()
}

// writeObject writes a JSON object of members to b, a member a line, indented by indent after
// prefix, the indentation of the object's own line.
func writeObject(b *bytes.Buffer, members []member, indent, prefix string) {
	if len(members) == 0 {
		b.WriteString("{}")
		return
	}
	b.WriteString("{\n")
	for i, m := range members {
		b.WriteString(prefix + indent)
		b.Write(jsonString(m.key))
		b.WriteString(": ")
		b.Write(m.value)
		if i < len(members)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(prefix + "}")
}

// jsonString is s as a JSON string, with <, > and & as they are.
func jsonString(s string) json.RawMessage {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n")
}

// write replaces the settings file with the settings, atomically.
func (s *settings) write() error {
	dir := filepath.Dir(s.target)
	if !s.exists {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail.Runtime("cannot create " + dir + ": " + reason(err).Error())
		}
	}
	file, err := createTemp(dir, s.mode)
	if err != nil {
		return fail.Runtime("cannot write " + s.path + ": " + reason(err).Error())
	}
	_, err = file.Write(s.encode())
	if err == nil && s.exists {
		err = file.Chmod(s.mode) // Exactly the file's mode, whatever the umask took away.
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), s.target)
	}
	if err != nil {
		_ = os.Remove(file.Name())
		return fail.Runtime("cannot write " + s.path + ": " + reason(err).Error())
	}
	return nil
}

// createTemp creates a new file in dir, for the settings, with the permissions perm less the
// umask.
func createTemp(dir string, perm fs.FileMode) (*os.File, error) {
	for try := 0; ; try++ {
		name := filepath.Join(dir, fmt.Sprintf(".settings.json.cld-%d-%d", os.Getpid(), time.Now().UnixNano()))
		file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if !errors.Is(err, fs.ErrExist) || try == 9 {
			return file, err
		}
	}
}
