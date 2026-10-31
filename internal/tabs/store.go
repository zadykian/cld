package tabs

import (
	"bytes"
	"encoding/json"
	"errors"
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

// newRecord is the record before any observation.
func newRecord() *Record { return &Record{Format: Format} }

// save writes r to the record in dir, by a temporary file renamed over it. Where r's bytes are
// old, it writes nothing, so that a record that did not change keeps its file and time.
func save(dir string, r *Record, old []byte) error {
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
