package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gateWords make a sentence of 31 words, one past Vale's 30, with the word before them.
var gateWords = strings.Repeat(" word", 30)

// TestValecheck runs tools/valecheck, and Vale with it, on a Go file. The doc comment's first
// sentence of 31 words, its name counted, fails at its line and column there. A directive and a
// code block as long pass.
func TestValecheck(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.go")
	src := "// Package a is short.\npackage a\n\n//go:generate" + gateWords + "\n\n" +
		"// F" + gateWords + ".\n//\n//\tF" + gateWords + "\nfunc F() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "tools/valecheck", path)
	cmd.Dir = filepath.Join("..", "..")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
		t.Fatalf("tools/valecheck: %v, want exit status 1; stderr:\n%s", err, stderr.String())
	}
	want := path + ":6:4:Microsoft.SentenceLength:"
	if got := stdout.String(); strings.Count(got, "\n") != 1 || !strings.HasPrefix(got, want) {
		t.Errorf("tools/valecheck printed\n%s\nwant one alert, %s...", got, want)
	}
}
