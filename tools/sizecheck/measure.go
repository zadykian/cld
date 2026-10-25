package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// maxLines is the cap on a file's lines.
const maxLines = 300

// measure counts the lines of a file of contents src.
func measure(src []byte) int {
	n := bytes.Count(src, []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		n++
	}
	return n
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
