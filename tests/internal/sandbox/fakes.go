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

// DockerCall is a call of the fake docker, as it records it.
type DockerCall struct {
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env"`
	// PortFree is, for run -d, whether the port of its label cld.port was free on 127.0.0.1.
	PortFree *bool `json:"portFree,omitempty"`
}

// DockerCalls are the calls of the fake docker so far, oldest first.
func (s *Sandbox) DockerCalls() []DockerCall {
	s.t.Helper()
	return jsonLines[DockerCall](s.t, filepath.Join(s.ProbeDir, "docker.jsonl"))
}

// DockerContainer is the fake docker's container, as a call left it: "STATUS PORT [RESTARTS]",
// or "" for none. "untouched" means no call has changed the one the test gave the fake.
func (s *Sandbox) DockerContainer() string {
	s.t.Helper()
	data, err := os.ReadFile(filepath.Join(s.ProbeDir, "docker.container"))
	if errors.Is(err, os.ErrNotExist) {
		return "untouched"
	}
	if err != nil {
		s.t.Fatal(err)
	}
	return string(data)
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
