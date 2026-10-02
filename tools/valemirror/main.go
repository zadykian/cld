// Command valemirror lets Vale check every sentence of Go comments. tools/valecheck runs it:
//
//	go run ./tools/valemirror DIR FILE...
//	go run ./tools/valemirror -alerts DIR <ALERTS
//
// The first writes a Markdown mirror of each Go FILE into DIR, and prints the mirrors' paths. A
// mirror has its file's lines, but holds only the prose of the comments, without their markers.
// The second copies Vale's line output from stdin to stdout, with the Go FILEs' paths and columns
// for the mirrors' alerts. Vale's own reading of Go comments drops the alerts on a doc comment's
// first word (see docs/design/testing.md). The exit status is 2 for a usage or setup error.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	var stderr strings.Builder
	status := run(os.Args[1:], os.Stdin, os.Stdout, &stderr)
	fmt.Fprint(os.Stderr, stderr.String())
	os.Exit(status)
}

// run does what the command line args ask, and returns the exit status.
func run(args []string, stdin io.Reader, stdout io.Writer, stderr *strings.Builder) int {
	flags := flag.NewFlagSet("valemirror", flag.ContinueOnError)
	flags.SetOutput(stderr)
	alerts := flags.Bool("alerts", false, "map Vale's alerts on stdin from the mirrors in DIR")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: go run ./tools/valemirror DIR FILE...")
		fmt.Fprintln(stderr, "       go run ./tools/valemirror -alerts DIR <ALERTS")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() < 1 || *alerts && flags.NArg() > 1 {
		flags.Usage()
		return 2
	}
	dir := flags.Arg(0)
	var err error
	if *alerts {
		err = mapAlerts(dir, stdin, stdout)
	} else {
		err = writeMirrors(dir, flags.Args()[1:], stdout)
	}
	if err != nil {
		fmt.Fprintln(stderr, "valemirror:", err)
		return 2
	}
	return 0
}

// writeMirrors writes into dir the mirror of each of the Go files that has prose, with the index,
// and prints the mirrors' paths to out. A file gone from disk has none.
func writeMirrors(dir string, files []string, out io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	index := make(map[string]source)
	for i, file := range files {
		src, err := os.ReadFile(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		rows := mirror(src)
		markdown, lines := render(rows)
		if len(lines) == 0 {
			continue
		}
		// A directory per file keeps the mirrors of files of one name apart.
		name := strconv.Itoa(i) + "/" + filepath.Base(file) + ".md"
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, markdown, 0o644); err != nil {
			return err
		}
		index[name] = source{File: file, Lines: lines}
		if _, err := fmt.Fprintln(out, path); err != nil {
			return err
		}
	}
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, indexName), data, 0o644)
}

// render returns a mirror's Markdown, and the pieces of each line that has prose.
func render(rows []row) ([]byte, map[int][]piece) {
	var markdown []byte
	lines := make(map[int][]piece)
	for i, r := range rows {
		markdown = append(markdown, r.text...)
		markdown = append(markdown, '\n')
		if len(r.pieces) > 0 {
			lines[i+1] = r.pieces
		}
	}
	return markdown, lines
}

// mapAlerts copies Vale's alerts from in to out, those on the mirrors in dir mapped back.
func mapAlerts(dir string, in io.Reader, out io.Writer) error {
	data, err := os.ReadFile(filepath.Join(dir, indexName))
	if err != nil {
		return err
	}
	var index map[string]source
	if err := json.Unmarshal(data, &index); err != nil {
		return fmt.Errorf("%s: %w", indexName, err)
	}
	return translate(dir, index, in, out)
}
