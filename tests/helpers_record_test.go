package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld's record of its sessions in the sandbox's home directory: the entries, and the files and
// marks beside them, as the tests write and read them.

// Conversations' IDs, as claude gives them.
const (
	firstID  = "0f4c1d7e-5a2b-4c3d-9e8f-1a2b3c4d5e6f"
	secondID = "7e6d5c4b-3a29-4817-a6f5-e4d3c2b1a098"
)

// entryFile is the file of session name's entry in cld's record, in s's home directory.
func entryFile(s *sandbox.Sandbox, name string) string {
	return filepath.Join(s.Home, ".local", "state", "cld", "sessions", name+".json")
}

// entry is session name's entry as cld writes it, and its SessionStart hook: the session started
// in dir, with the conversation of that ID, "" for none yet.
func entry(name, dir, conversation string) string {
	return `{"name":` + jsonText(name) + `,"directory":` + jsonText(dir) +
		`,"conversation":"` + conversation + "\"}\n"
}

// jsonText is text as a JSON string, quotes included, without HTML's escapes, as cld writes the
// settings and the entries of its record.
func jsonText(text string) string {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(encoded.String(), "\n")
}

// writeEntry writes session name's entry in cld's record in s, as join would have written it for a
// session in dir, with the conversation of that ID, "" for none yet.
func writeEntry(t *testing.T, s *sandbox.Sandbox, name, dir, conversation string) {
	t.Helper()
	file := entryFile(s, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(file, entry(name, dir, conversation))
}

// readEntry is session name's entry in cld's record in s, "" where there is none.
func readEntry(s *sandbox.Sandbox, name string) string {
	data, err := os.ReadFile(entryFile(s, name))
	if err != nil {
		return ""
	}
	return string(data)
}

// forget removes the entries of the sessions names from cld's record in s, as the list's forget
// does: once their sessions end, they are gone rather than ended.
func forget(t *testing.T, s *sandbox.Sandbox, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.Remove(entryFile(s, name)); err != nil {
			t.Fatal(err)
		}
	}
}

// age makes the files paths as old as ago, as their time goes.
func age(t *testing.T, ago time.Duration, paths ...string) {
	t.Helper()
	then := time.Now().Add(-ago)
	for _, path := range paths {
		if err := os.Chtimes(path, then, then); err != nil {
			t.Fatal(err)
		}
	}
}

// companionFile is the file beside session name's entry in cld's record in s with the extension
// ext: .run, .busy or .env.
func companionFile(s *sandbox.Sandbox, name, ext string) string {
	return strings.TrimSuffix(entryFile(s, name), ".json") + ext
}

// exists reports whether there is a file at path.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// checkMarks reports the run marks of the sessions in s that are not as want has them, by name:
// true for a mark there. It waits for a mark that should be there, which tmux makes in a run-shell
// once it has made the session. That can end after the terminal has attached and claude started.
func checkMarks(t *testing.T, s *sandbox.Sandbox, when string, want map[string]bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for name, marked := range want {
		got := exists(companionFile(s, name, ".run"))
		for marked && !got && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			got = exists(companionFile(s, name, ".run"))
		}
		if got != marked {
			t.Errorf("%s: session %s's run mark there: %v, want %v", when, name, got, marked)
		}
	}
}

// recorded is the environment file cld writes beside an entry: the claude it started, by its
// path, and the environment of its server.
type recorded struct {
	Claude      string   `json:"claude"`
	Environment []string `json:"environment"`
}

// writeRestorable writes session name's entry in s - claude started in dir, in the conversation
// of that ID - with its run mark and the environment file naming claude and env, as join would
// have left them for a session a reboot ended.
func writeRestorable(
	t *testing.T, s *sandbox.Sandbox, name, dir, conversation, claude string, env []string,
) {
	t.Helper()
	writeEntry(t, s, name, dir, conversation)
	data, err := json.Marshal(recorded{Claude: claude, Environment: env})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(companionFile(s, name, ".env"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(companionFile(s, name, ".run"), "")
}
