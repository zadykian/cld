// Package project is cld setup project: it sets Claude Code up in the project in the current
// directory, writing what claude reads there and what git needs to share it.
//
//   - .claude/settings.json holds the settings the project shares. They rank above each
//     developer's own, so they hold no setting of a person's, such as a theme.
//   - .claude/settings.local.json, each developer's own and ignored by git, is only created.
//   - .mcp.json gets each MCP server's entry, replaced whole where it differs, since a merge
//     would break it.
//   - .gitignore gets the lines that keep one developer's files in .claude out of git.
//
// Setup never replaces a value the settings have. It reads every file, refusing one it cannot
// edit, before it writes only those that change. Then, in a git work tree, it warns of the shared
// files git ignores, and fails where git ignores .claude/settings.json. Decisions 19 and 28 give
// the reasons.
package project

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
)

const (
	settingsFile = ".claude/settings.json"
	localFile    = ".claude/settings.local.json"
	mcpFile      = ".mcp.json"
	ignoreFile   = ".gitignore"
	// schema is the JSON schema of claude's settings, which editors read to check the files.
	schema = "https://json.schemastore.org/claude-code-settings.json"
)

// change is one file setup project writes: created, changed in what, or neither and left as it
// was; write writes it.
type change struct {
	file    string
	created bool
	what    []string
	write   func() error
}

// Setup sets Claude Code up in the project in the current directory, with the MCP servers and
// the permission set given, in the package comment's order. It reports what it changed.
func Setup(servers []Server, permissions Permissions) error {
	changes, err := plan(servers, permissions)
	if err != nil {
		return err
	}
	if err := write(changes); err != nil {
		return err
	}
	if err := output.Print(report(changes)); err != nil {
		return err
	}
	return checkIgnored()
}

// plan reads and edits every file, so that one it cannot edit stops Setup before it writes any
// (decision 19.6).
func plan(servers []Server, permissions Permissions) ([]change, error) {
	settings, err := settingsChange(servers, permissions)
	if err != nil {
		return nil, err
	}
	local, err := localChange()
	if err != nil {
		return nil, err
	}
	changes := []change{settings, local}
	if len(servers) > 0 {
		mcp, err := mcpChange(servers)
		if err != nil {
			return nil, err
		}
		changes = append(changes, mcp)
	}
	ignore, err := ignoreChange()
	if err != nil {
		return nil, err
	}
	return append(changes, ignore), nil
}

// write writes each file that changes, in order, and names those written before one that fails.
func write(changes []change) error {
	var written []string
	for _, c := range changes {
		if len(c.what) == 0 {
			continue
		}
		if err := c.write(); err != nil {
			if len(written) > 0 {
				err = fail.Runtime(err.Error() + " (" + and(written) + " written before it)")
			}
			return err
		}
		written = append(written, c.file)
	}
	return nil
}

// report is what Setup prints at its end: a line for each file.
func report(changes []change) string {
	var b strings.Builder
	for _, c := range changes {
		switch {
		case len(c.what) == 0:
			b.WriteString("Left " + c.file + " as it was\n")
		case c.created:
			b.WriteString("Created " + c.file + "\n")
		default:
			b.WriteString("Updated " + c.file + ": " + strings.Join(c.what, ", ") + "\n")
		}
	}
	return b.String()
}

// and joins names as a sentence does: "a", "a and b", "a, b and c".
func and(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// reason is err without the operation and path that *fs.PathError adds to it.
func reason(err error) string {
	if pathError, ok := errors.AsType[*fs.PathError](err); ok {
		return pathError.Err.Error()
	}
	return err.Error()
}
