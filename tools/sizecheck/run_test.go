package main

import (
	"strings"
	"testing"
)

// lines returns text of n lines.
func lines(n int) string {
	return strings.Repeat("x\n", n)
}

// longGo is a Go file of 301 lines.
var longGo = "package a\n" + strings.Repeat("\n", 300)

// runSteps is a session of sizecheck in one repository, each step on what those before it left.
// $DIR in args stands for the repository's directory.
var runSteps = []struct {
	name   string
	write  map[string]string // files to write before the step; "" removes one
	args   []string
	status int
	stdout string
}{
	{
		name: "every file", status: 1,
		stdout: "big.md:1: 303 lines, over the cap of 300\nold.md:1: 305 lines, over the cap of 300\n",
	},
	{
		name: "a file gone", write: map[string]string{"old.md": ""}, status: 1,
		stdout: "big.md:1: 303 lines, over the cap of 300\n",
	},
	{name: "a file at the cap", write: map[string]string{"big.md": lines(300)}},
	{
		name:  "a FILE alone",
		write: map[string]string{"big.md": lines(310), "new.go": longGo}, args: []string{"new.go"},
		status: 1, stdout: "new.go:1: 301 lines, over the cap of 300\n",
	},
	{
		name: "an absolute FILE", args: []string{"$DIR/new.go"},
		status: 1, stdout: "new.go:1: 301 lines, over the cap of 300\n",
	},
	{
		name: "FILEs out of the repository, ignored, of another kind or missing",
		args: []string{"../big.md", "ignored/big.md", ".gitignore", "missing.md"},
	},
	{
		name: "a file git does not track yet", status: 1,
		stdout: "big.md:1: 310 lines, over the cap of 300\nnew.go:1: 301 lines, over the cap of 300\n",
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
	}
}
