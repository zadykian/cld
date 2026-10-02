package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lines returns text of n lines.
func lines(n int) string {
	return strings.Repeat("x\n", n)
}

// entries returns the baseline's lines but its comments, or "none" where there is no baseline.
func entries(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(baselinePath)))
	if os.IsNotExist(err) {
		return "none"
	}
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for line := range strings.Lines(string(data)) {
		if !strings.HasPrefix(line, "#") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

// badGo is a Go file with a comment block of 5 lines on line 3.
var badGo = "package a\n\n" + strings.Repeat("// x\n", 5) + "var x int\n"

// runSteps is a session of sizecheck in one repository, each step on what those before it left.
// $DIR in args stands for the repository's directory.
var runSteps = []struct {
	name    string
	write   map[string]string // files to write before the step; "" removes one
	args    []string
	status  int
	stdout  string
	entries string // the baseline's entries after the step
}{
	{
		name: "no baseline", status: 1, entries: "none",
		stdout: "big.md:1: 303 lines, over the cap of 300\nold.md:1: 305 lines, over the cap of 300\n",
	},
	{
		name: "-update creates it", args: []string{"-update"},
		entries: "big.md lines=303\nold.md lines=305\n",
	},
	{name: "it holds", entries: "big.md lines=303\nold.md lines=305\n"},
	{
		name: "a file gone", write: map[string]string{"old.md": ""}, status: 1,
		stdout:  "old.md:1: stale baseline entry: the file is gone or git ignores it\n",
		entries: "big.md lines=303\nold.md lines=305\n",
	},
	{name: "-update drops its entry", args: []string{"-update"}, entries: "big.md lines=303\n"},
	{
		name: "above the ceiling", write: map[string]string{"big.md": lines(304)}, status: 1,
		stdout:  "big.md:1: 304 lines, above the baseline's ceiling of 303\n",
		entries: "big.md lines=303\n",
	},
	{
		name: "-update raises nothing", args: []string{"-update"}, status: 1,
		stdout:  "big.md:1: 304 lines, above the baseline's ceiling of 303\n",
		entries: "big.md lines=303\n",
	},
	{
		name: "below the ceiling", write: map[string]string{"big.md": lines(302)},
		entries: "big.md lines=303\n",
	},
	{name: "-update lowers it", args: []string{"-update"}, entries: "big.md lines=302\n"},
	{
		name: "stale", write: map[string]string{"big.md": lines(290)}, status: 1,
		stdout:  "big.md:1: stale baseline entry lines=302: now 290, within the cap of 300\n",
		entries: "big.md lines=302\n",
	},
	{
		name: "a FILE alone", write: map[string]string{"new.go": badGo}, args: []string{"new.go"},
		status: 1, stdout: "new.go:3: comment block of 5 lines, over the cap of 4\n",
		entries: "big.md lines=302\n",
	},
	{
		name: "an absolute FILE", args: []string{"$DIR/new.go"},
		status: 1, stdout: "new.go:3: comment block of 5 lines, over the cap of 4\n",
		entries: "big.md lines=302\n",
	},
	{
		name:    "FILEs out of the repository, ignored, of another kind or missing",
		args:    []string{"../big.md", "ignored/big.md", ".gitignore", "missing.md"},
		entries: "big.md lines=302\n",
	},
	{
		name: "-update drops the stale entry and adds none", args: []string{"-update"}, status: 1,
		stdout: "new.go:3: comment block of 5 lines, over the cap of 4\n", entries: "",
	},
	{name: "a file fixed", write: map[string]string{"new.go": "package a\n"}, entries: ""},
	{
		name: "-update on an empty baseline adds none", args: []string{"-update"}, status: 1,
		write:  map[string]string{"big.md": lines(310)},
		stdout: "big.md:1: 310 lines, over the cap of 300\n", entries: "",
	},
}

func TestRun(t *testing.T) {
	t.Parallel()
	dir := repository(t, map[string]string{
		".gitignore":     "/ignored/\n",
		"small.go":       "// Package a is small.\npackage a\n",
		"big.md":         lines(303),
		"old.md":         lines(305),
		"ignored/big.md": lines(400),
	})
	for _, step := range runSteps {
		write(t, dir, step.write)
		args := make([]string, len(step.args))
		for i, arg := range step.args {
			args[i] = strings.ReplaceAll(arg, "$DIR", dir)
		}
		var stdout, stderr strings.Builder
		status := run(dir, args, &stdout, &stderr)
		if status != step.status || stdout.String() != step.stdout {
			t.Errorf("%s: status %d, stdout %q, stderr %q; want %d and %q", step.name, status,
				stdout.String(), stderr.String(), step.status, step.stdout)
		}
		if got := entries(t, dir); got != step.entries {
			t.Errorf("%s: the baseline's entries are %q, want %q", step.name, got, step.entries)
		}
	}
}
