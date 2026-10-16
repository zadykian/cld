package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/mac"
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

// WriteProbeFile writes content as the file name of the probe directory, as mac.WriteFile does:
// a fake never reads it half written.
func (s *Sandbox) WriteProbeFile(name, content string) {
	s.t.Helper()
	if err := mac.WriteFile(s.ProbeDir, name, content); err != nil {
		s.t.Fatal(err)
	}
}

// OsascriptCalls are the calls of the fake osascript so far, oldest first.
func (s *Sandbox) OsascriptCalls() []mac.OsascriptCall {
	s.t.Helper()
	return jsonLines[mac.OsascriptCall](s.t, filepath.Join(s.ProbeDir, mac.OsascriptLog))
}

// PsCalls are the calls of the fake ps so far, oldest first.
func (s *Sandbox) PsCalls() []mac.PsCall {
	s.t.Helper()
	return jsonLines[mac.PsCall](s.t, filepath.Join(s.ProbeDir, mac.PsLog))
}

// SSHCalls are the calls of the fake ssh so far, oldest first.
func (s *Sandbox) SSHCalls() []mac.SSHCall {
	s.t.Helper()
	return jsonLines[mac.SSHCall](s.t, filepath.Join(s.ProbeDir, mac.SSHLog))
}

// jsonLines decodes the JSON lines a fake appends to the file path, nil before its first call.
// What follows the last newline is being written, as a watcher's fakes may append meanwhile.
func jsonLines[T any](tb testing.TB, path string) []T {
	tb.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		tb.Fatal(err)
	}
	ended := string(data[:bytes.LastIndexByte(data, '\n')+1])
	if ended == "" {
		return nil
	}
	var values []T
	for line := range strings.SplitSeq(strings.TrimSuffix(ended, "\n"), "\n") {
		var value T
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			tb.Fatalf("%s: %v", filepath.Base(path), err)
		}
		values = append(values, value)
	}
	return values
}
