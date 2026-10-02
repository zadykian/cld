package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
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

// measure counts the lines of the file at path with contents src and, in Go, its comment blocks.
func measure(path string, src []byte) measurement {
	var m measurement
	m.values[checkLines] = bytes.Count(src, []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		m.values[checkLines]++
	}
	if !strings.HasSuffix(path, ".go") {
		return m
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		m.errLine, m.errMsg = 1, err.Error()
		if list, ok := errors.AsType[scanner.ErrorList](err); ok && len(list) > 0 {
			m.errLine, m.errMsg = list[0].Pos.Line, list[0].Msg
		}
		return m
	}
	for _, group := range file.Comments {
		b := block{line: fset.Position(group.Pos()).Line}
		b.length = fset.Position(group.End()).Line - b.line + 1
		switch {
		case group == file.Doc:
			m.pkg = b
			m.values[checkPackage] = b.length
		case b.length > maxComment:
			m.blocks = append(m.blocks, b)
		}
	}
	m.values[checkComments] = len(m.blocks)
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
