package tabs

import "github.com/zadykian/cld/internal/ghostty"

// Report is what a restore did or would do, for cmd to print: Lines on stdout, then Warnings and
// Notes on stderr. Failed is a step that failed, which makes the status 1 (decision 48.8).
type Report struct {
	Lines    []string
	Warnings []string
	Notes    []string
	Failed   bool
}

// ghosttyLine is the report's first line: whether Ghostty runs.
func ghosttyLine(snap *ghostty.Snapshot) string {
	if !snap.Running {
		return "Ghostty is not running"
	}
	return "Ghostty is running"
}
