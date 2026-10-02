package configfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zadykian/cld/internal/fail"
)

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

// indentation is one level of the indentation of data, a JSON object: the white space that
// starts the line of its first member, or two spaces where that member shares the object's line.
func indentation(data []byte) string {
	_, rest, ok := bytes.Cut(data, []byte{'{'})
	if !ok {
		return "  "
	}
	space := rest[:len(rest)-len(bytes.TrimLeft(rest, " \t\r\n"))]
	if line := bytes.LastIndexByte(space, '\n'); line >= 0 && line+1 < len(space) {
		return string(space[line+1:])
	}
	return "  "
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

// Array is an array of elements as the file holds it, depth levels deep, as for Object.
func (j *JSON) Array(elements []json.RawMessage, depth int) json.RawMessage {
	if len(elements) == 0 {
		return json.RawMessage("[]")
	}
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, element := range elements {
		b.WriteString(strings.Repeat(j.indent, depth+1))
		b.Write(element)
		if i < len(elements)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat(j.indent, depth) + "]")
	return b.Bytes()
}

// Value is value, valid JSON of cld's, as the file holds it depth levels deep, as for Object:
// every object and array in it has a member or element a line.
func (j *JSON) Value(value []byte, depth int) json.RawMessage {
	var b bytes.Buffer
	if err := json.Indent(&b, value, strings.Repeat(j.indent, depth), j.indent); err != nil {
		panic("cld's own JSON is not valid: " + string(value))
	}
	return b.Bytes()
}
