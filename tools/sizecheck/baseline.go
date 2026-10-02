package main

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
)

// A baseline holds the ceilings of the files over a cap, by their path from the repository root.
type baseline map[string]values

// baselineHeader opens the baseline file, which says what its keys mean.
const baselineHeader = "" +
	"# The files over sizecheck's caps, with ceilings that may only fall.\n" +
	"# lines: the file's lines (cap 300). comments: Go comment blocks over 4 lines (cap 0).\n" +
	"# package: the package comment's lines (cap 15).\n" +
	"# go run ./tools/sizecheck -update lowers the ceilings and drops stale entries.\n"

// readBaseline reads the baseline at path; an error wraps os.ErrNotExist where there is none.
func readBaseline(path string) (baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return baseline{}, err
	}
	base, err := parseBaseline(data)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", path, err)
	}
	return base, nil
}

// parseBaseline parses lines of a path and its ceilings as key=N, skipping blank and # lines.
// A path that starts with " is quoted as in Go.
func parseBaseline(data []byte) (baseline, error) {
	base := make(baseline)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, ceilings, err := parseEntry(line)
		if err == nil {
			if _, dup := base[path]; dup {
				err = fmt.Errorf("%s is listed twice", path)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%d: %w", i+1, err)
		}
		base[path] = ceilings
	}
	return base, nil
}

// parseEntry parses a line of the baseline.
func parseEntry(line string) (string, values, error) {
	var ceilings values
	path, rest := line, ""
	if strings.HasPrefix(line, `"`) {
		quoted, err := strconv.QuotedPrefix(line)
		if err != nil {
			return "", ceilings, fmt.Errorf("bad quoted path: %w", err)
		}
		rest = line[len(quoted):]
		if path, err = strconv.Unquote(quoted); err != nil {
			return "", ceilings, fmt.Errorf("bad quoted path: %w", err)
		}
	} else if i := strings.IndexAny(line, " \t"); i >= 0 {
		path, rest = line[:i], line[i:]
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", ceilings, fmt.Errorf("%s has no ceiling", path)
	}
	for _, field := range fields {
		c, n, err := parseCeiling(field)
		if err == nil && ceilings[c] != 0 {
			err = fmt.Errorf("%s is given twice", checks[c].key)
		}
		if err != nil {
			return "", ceilings, err
		}
		ceilings[c] = n
	}
	return path, ceilings, nil
}

// parseCeiling parses key=N, where N must be over the key's cap.
func parseCeiling(field string) (check, int, error) {
	key, value, _ := strings.Cut(field, "=")
	for c, def := range checks {
		if def.key != key {
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, 0, fmt.Errorf("%q is not key=N: %w", field, errors.Unwrap(err))
		}
		if n <= def.limit {
			return 0, 0, fmt.Errorf("%s is within the cap of %d", field, def.limit)
		}
		return check(c), n, nil
	}
	return 0, 0, fmt.Errorf("%q is not key=N with key lines, comments or package", field)
}

// formatBaseline writes base as the baseline file holds it, sorted by path.
func formatBaseline(base baseline) []byte {
	var b bytes.Buffer
	b.WriteString(baselineHeader)
	for _, path := range slices.Sorted(maps.Keys(base)) {
		if base[path] == (values{}) {
			continue
		}
		name := path
		if strings.ContainsAny(path, " \t\"") || strings.HasPrefix(path, "#") {
			name = strconv.Quote(path)
		}
		b.WriteString(name)
		for c, n := range base[path] {
			if n > 0 {
				fmt.Fprintf(&b, " %s=%d", checks[c].key, n)
			}
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// writeBaseline writes base to path, where that changes the file.
func writeBaseline(path string, base baseline) error {
	data := formatBaseline(base)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}
