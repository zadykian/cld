package tabs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zadykian/cld/internal/session"
)

// The files in the record's directory (decision 59.4).
const recordFile = "tabs.json" // the record

// Dir is the record's directory, tabs/ in cld's state directory, made where missing. It sits
// outside sessions/, whose expiry removes the files it does not know.
func Dir() (string, error) {
	state, err := session.StateDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(state, "tabs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", reason(err)
	}
	return dir, nil
}

// load reads the record in dir, and old, its bytes for save. A missing record is a new one, with
// no bytes.
func load(dir string) (r *Record, old []byte, err error) {
	data, err := os.ReadFile(filepath.Join(dir, recordFile))
	if errors.Is(err, fs.ErrNotExist) {
		return newRecord(), nil, nil
	}
	if err != nil {
		return nil, nil, reason(err)
	}
	r = &Record{}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", filepath.Join(dir, recordFile), err)
	}
	normalize(r)
	return r, data, nil
}

// newRecord is the record before any observation.
func newRecord() *Record {
	r := &Record{Format: Format}
	normalize(r)
	return r
}

// normalize gives r an empty map or slice for a nil one, so that its bytes do not depend on what
// made it. It drops the nil panes that a record edited by hand may hold.
func normalize(r *Record) {
	if r.Panes == nil {
		r.Panes = map[string]*Pane{}
	}
	if r.Windows == nil {
		r.Windows = []Window{}
	}
	for id, p := range r.Panes {
		if p == nil {
			delete(r.Panes, id)
		}
	}
}

// save writes r to the record in dir, by a temporary file renamed over it. Where r's bytes are
// old, it writes nothing, so that a record that did not change keeps its file and time.
func save(dir string, r *Record, old []byte) error {
	normalize(r)
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", " ")
	if err := encoder.Encode(r); err != nil {
		return err
	}
	if bytes.Equal(data.Bytes(), old) {
		return nil
	}
	return replace(filepath.Join(dir, recordFile), data.Bytes())
}

// replace replaces file with data: written to a temporary file beside it, readable by the user
// alone, then renamed over it, so that no reader sees it half written.
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
