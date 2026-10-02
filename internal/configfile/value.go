package configfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
)

// Member is a key of a JSON object with its value, as the file has it.
type Member struct {
	Key   string
	Value json.RawMessage
}

// ErrNotObject and ErrNotArray are what Object and Array say of JSON that is some other value.
var (
	ErrNotObject = errors.New("it is not an object")
	ErrNotArray  = errors.New("it is not an array")
)

// Object reads data, a JSON object and nothing more, into its members in order.
func Object(data []byte) ([]Member, error) {
	var members []Member
	err := container(data, '{', "object", ErrNotObject, func(decoder *json.Decoder) error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		members = append(members, Member{Key: token.(string), Value: value})
		return nil
	})
	return members, err
}

// Array reads data, a JSON array and nothing more, into its elements in order, as data has them.
func Array(data []byte) ([]json.RawMessage, error) {
	var elements []json.RawMessage
	err := container(data, '[', "array", ErrNotArray, func(decoder *json.Decoder) error {
		var element json.RawMessage
		if err := decoder.Decode(&element); err != nil {
			return err
		}
		elements = append(elements, element)
		return nil
	})
	return elements, err
}

// container reads data, an object or array (what) that opens with open, and nothing more. It
// calls each for each member or element; other JSON is notOpen.
func container(
	data []byte, open json.Delim, what string, notOpen error, each func(*json.Decoder) error,
) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil {
		return err
	} else if token != open {
		return notOpen
	}
	for decoder.More() {
		if err := each(decoder); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("more follows the " + what)
	}
	return nil
}

// invalid says what is wrong with data, which Object refused with err, and where. A syntax error
// is told as json.Unmarshal tells it, alike in Go 1.26 and 1.27: see "encoding/json on invalid
// JSON" in docs/design/findings/environment.md.
func invalid(data []byte, err error) string {
	if len(bytes.TrimSpace(data)) == 0 {
		return "it is empty"
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		var value any
		if errors.As(json.Unmarshal(data, &value), &syntax) {
			return fmt.Sprintf("line %d: %v", bytes.Count(data[:syntax.Offset], []byte("\n"))+1, syntax)
		}
	}
	return err.Error()
}

// Last is the index of the last of members with key, which claude reads; -1 for none.
func Last(members []Member, key string) int {
	for i, member := range slices.Backward(members) {
		if member.Key == key {
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
	_ = encoder.Encode(s) //nolint:errcheck,errchkjson // a string always encodes, into memory
	return bytes.TrimRight(b.Bytes(), "\n")
}

// Same is whether a and b are the same JSON value, as Go decodes them: 1.0 is 1, and the members
// of an object may come in any order.
func Same(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}
