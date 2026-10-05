package sandbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FakeTmuxRecord is what the fake tmux recorded about its invocation.
func (s *Sandbox) FakeTmuxRecord() Record {
	s.t.Helper()
	var record Record
	readJSON(s.t, filepath.Join(s.ProbeDir, "tmux.json"), &record)
	return record
}

// SystemdCalls are the calls of the fake systemctl and loginctl so far, oldest first, each the
// program's name and its arguments.
func (s *Sandbox) SystemdCalls() [][]string {
	s.t.Helper()
	return jsonLines[[]string](s.t, filepath.Join(s.ProbeDir, "systemd.jsonl"))
}

// jsonLines decodes the JSON lines a fake appends to the file path, nil before its first call.
func jsonLines[T any](tb testing.TB, path string) []T {
	tb.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		tb.Fatal(err)
	}
	var values []T
	for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
		var value T
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			tb.Fatalf("%s: %v", filepath.Base(path), err)
		}
		values = append(values, value)
	}
	return values
}
