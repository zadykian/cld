// Command sizecheck keeps cld's files short, and its backlog shrinking.
//
// Run from the repository root:
//
//	go run ./tools/sizecheck [FILE...]
//
// Every .go and .md file is at most 300 lines. It checks the files git lists, or those of them a
// FILE names.
// baseline.txt holds today's overages as ceilings: a value may fall, never rise, and an entry the
// file no longer needs is stale. Findings go to stdout as PATH:LINE: message.
// The exit status is 1 for a finding and 2 for a usage or setup error, which go run reports as 1.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// baselinePath is the baseline's path from the repository root.
const baselinePath = "tools/sizecheck/baseline.txt"

func main() {
	var stdout, stderr strings.Builder
	status := run(".", os.Args[1:], &stdout, &stderr)
	if _, err := os.Stdout.WriteString(stdout.String()); err != nil {
		fmt.Fprintln(os.Stderr, "sizecheck:", err)
		status = 2
	}
	fmt.Fprint(os.Stderr, stderr.String())
	os.Exit(status)
}

// run checks the repository at dir as the command line args ask, and returns the exit status.
func run(dir string, args []string, stdout, stderr *strings.Builder) int {
	flags := flag.NewFlagSet("sizecheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: go run ./tools/sizecheck [FILE...]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	status, err := inspect(dir, flags.Args(), stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "sizecheck:", err)
		return 2
	}
	return status
}

// inspect measures the files in scope, compares them with the baseline, and reports the findings.
func inspect(dir string, args []string, stdout, stderr *strings.Builder) (int, error) {
	base, err := readBaseline(filepath.Join(dir, filepath.FromSlash(baselinePath)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	listed, err := listFiles(dir)
	if err != nil {
		return 0, err
	}
	inScope, err := scope(dir, args)
	if err != nil {
		return 0, err
	}
	files, err := measureFiles(dir, listed, inScope)
	if err != nil {
		return 0, err
	}
	findings, _ := compare(base, files, inScope, false)
	for _, f := range findings {
		fmt.Fprintf(stdout, "%s:%d: %s\n", f.path, f.line, f.msg)
	}
	if len(findings) == 0 {
		return 0, nil
	}
	if hasStale(findings) {
		fmt.Fprintln(stderr, "sizecheck: drop the stale entries from", baselinePath)
	}
	return 1, nil
}

// measureFiles measures the listed .go and .md files in scope, passing over those gone from disk.
func measureFiles(dir string, listed []string,
	inScope func(string) bool) (map[string]measurement, error) {
	files := make(map[string]measurement)
	for _, path := range listed {
		if !checked(path) || !inScope(path) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files[path] = measure(src)
	}
	return files, nil
}

// scope returns whether a path from the repository root is among the FILE arguments, all paths
// when there are none. An argument outside dir names nothing.
func scope(dir string, args []string) (func(string) bool, error) {
	if len(args) == 0 {
		return func(string) bool { return true }, nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]bool)
	for _, arg := range args {
		path := arg
		if filepath.IsAbs(arg) {
			if path, err = filepath.Rel(root, arg); err != nil {
				continue
			}
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if path != ".." && !strings.HasPrefix(path, "../") {
			paths[path] = true
		}
	}
	return func(path string) bool { return paths[path] }, nil
}

// checked returns whether sizecheck checks a file of path's kind.
func checked(path string) bool {
	return strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".md")
}
