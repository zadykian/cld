package tests

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// cld's scripts themselves: their syntax, and their logic against a mock Ghostty, where macOS's
// osascript runs them.

// scriptsDir holds the scripts cld embeds.
var scriptsDir = filepath.Join("..", "internal", "ghostty", "scripts")

// Each script compiles as JavaScript for Open Scripting Architecture, where macOS has
// osacompile. JXA finds Ghostty's terms only as a script runs.
func TestTabsScriptSyntax(t *testing.T) {
	t.Parallel()
	osacompile, err := exec.LookPath("osacompile")
	if err != nil {
		t.Skip("no osacompile: the macos job runs it")
	}
	scripts, err := filepath.Glob(filepath.Join(scriptsDir, "*.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range scripts {
		compiled := filepath.Join(t.TempDir(), filepath.Base(path)+".scpt")
		out, err := exec.Command(osacompile, "-l", "JavaScript", "-o", compiled, path).
			CombinedOutput()
		if err != nil {
			t.Errorf("osacompile %s: %v\n%s", path, err, out)
		}
	}
}
