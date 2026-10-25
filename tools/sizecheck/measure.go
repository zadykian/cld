package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// A check is one of sizecheck's caps, and indexes values.
type check int

const (
	checkLines check = iota
	checkComments
	checkPackage
	numChecks
)

// checks names each check, as the baseline's keys do, and gives its cap.
var checks = [numChecks]struct {
	key   string
	limit int
}{
	checkLines:    {"lines", 300},
	checkComments: {"comments", 0},
	checkPackage:  {"package", 15},
}

// maxComment is the cap on the lines of a comment block other than the package comment.
const maxComment = 4

// values holds a number per check: a file's measure, or the baseline's ceilings, 0 for none.
type values [numChecks]int

// A block is a comment block: the line it starts on and its length in lines.
type block struct {
	line, length int
}

// A measurement is what sizecheck finds in a file.
type measurement struct {
	values values
	blocks []block // the comment blocks over maxComment, the package comment aside
	pkg    block   // the package comment, of length 0 where there is none
	// Where a Go file fails to parse, and why; errMsg is empty where it parses.
	errLine int
	errMsg  string
}

// measure counts the lines of a file of contents src.
func measure(src []byte) measurement {
	var m measurement
	m.values[checkLines] = bytes.Count(src, []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		m.values[checkLines]++
	}
	return m
}

// listFiles returns the files git lists in dir, tracked or untracked but not ignored, sorted.
func listFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir, "ls-files", "-z", "--cached", "--others",
		"--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
			err = fmt.Errorf("git ls-files: %s", bytes.TrimSpace(exit.Stderr))
		}
		return nil, err
	}
	files := strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 })
	slices.Sort(files)
	return slices.Compact(files), nil
}
