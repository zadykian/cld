package tests

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// tests/ghostty/deps.txt pins the Ghostty commit that go.mitchellh.com/libghostty in go.mod binds.
// The bindings' CMakeLists.txt names it, and the two move together (decision 54.2).
func TestGhosttyPin(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("go", "mod", "download", "-json", "go.mitchellh.com/libghostty").Output()
	// A failed download still writes its JSON, with the reason in Error.
	var module struct{ Dir, Error string }
	if jsonErr := json.Unmarshal(out, &module); err == nil && jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if err != nil {
		stderr := ""
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = strings.TrimSpace(string(exit.Stderr))
		}
		t.Fatalf("go mod download: %v: %s %s", err, module.Error, stderr)
	}
	cmake, err := os.ReadFile(filepath.Join(module.Dir, "CMakeLists.txt"))
	if err != nil {
		t.Fatal(err)
	}
	deps, err := os.ReadFile(filepath.Join("ghostty", "deps.txt"))
	if err != nil {
		t.Fatal(err)
	}
	bound := regexp.MustCompile(`GIT_TAG\s+([0-9a-f]{40})\b`).FindSubmatch(cmake)
	pinned := regexp.MustCompile(`(?m)^([0-9a-f]{40})\s+\S+/ghostty-org/ghostty`).FindSubmatch(deps)
	if bound == nil || pinned == nil {
		t.Fatalf("no commit: %q in the bindings' CMakeLists.txt, %q in tests/ghostty/deps.txt",
			bound, pinned)
	}
	if string(bound[1]) != string(pinned[1]) {
		t.Errorf("tests/ghostty/deps.txt pins Ghostty %s, the bindings in go.mod bind %s",
			pinned[1], bound[1])
	}
}
