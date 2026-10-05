package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup config project on files that exist, which it edits in place (decisions 19.4 and 28.3).

// Files that exist: cld adds what each lacks after what it has, and replaces rider's entry, which
// differs, whole. It keeps the order, indentation and mode, the values it leaves, theme's and those
// with <, > and &, and jbcontext's entry, written otherwise but the same. settings.local.json, a
// symbolic link to no file, stays unchanged.
func TestSetupConfigProjectEditsFiles(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	settings := writeFilesToEdit(t, s)
	result := s.RunCld(nil, "setup", "config", "project", "--mcp", "rider,jbcontext")
	want := "Updated .claude/settings.json: " +
		"$schema, permissions.allow, plansDirectory, enabledMcpjsonServers\n" +
		"Left .claude/settings.local.json as it was\n" +
		"Updated .mcp.json: mcpServers.rider\n" +
		"Updated .gitignore: " + addedLines + "\n"
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	checkEditedSettings(t, settings)
	checkEditedFiles(t, s)

	t.Run("a key given twice", editsKeyGivenTwice)
	t.Run("no servers", editsNoServers)
	t.Run("no permissions", editsNoPermissions)
}

// writeFilesToEdit writes TestSetupConfigProjectEditsFiles' files, and returns the settings' path.
func writeFilesToEdit(t *testing.T, s *sandbox.Sandbox) string {
	t.Helper()
	settings := projectWrite(t, s, ".claude/settings.json", `{
    "permissions": {
        "deny": ["Bash(rm:*)"],
        "allow": ["Bash(make test)", "Read", "mcp__rider", "Read(<&>)"],
        "defaultMode": "acceptEdits"
    },
    "theme": "light",
    "model": "opus",
    "autoCompactEnabled": false,
    "enabledMcpjsonServers": ["github"],
    "hooks": {"Stop": []}
}
`)
	if err := os.Chmod(settings, 0o666); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.Work, ".claude", "settings.local.json")
	if err := os.Symlink(filepath.Join(s.Root, "dotfiles", "settings.local.json"), link); err != nil {
		t.Fatal(err)
	}
	projectWrite(t, s, ".mcp.json", `{
  "mcpServers": {
    "github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/"},
    "rider": {"type": "http", "url": "http://127.0.0.1:63342/stream", `+
		`"headers": {"X-Token": "<&>"}},
    "jbcontext": {"args": ["mcp"], "command": "jbcontext", "type": "stdio"}
  },
  "other": true
}
`)
	projectWrite(t, s, ".gitignore", "node_modules/\n")
	return settings
}

// checkEditedSettings reports TestSetupConfigProjectEditsFiles' settings at path, unless they hold
// the entries and keys cld adds after those the file has.
func checkEditedSettings(t *testing.T, path string) {
	t.Helper()
	allow := []string{"Bash(make test)", "Read", "mcp__rider", "Read(<&>)"}
	for _, entry := range allowed(t, "read-only", "jbcontext", "rider") {
		if !slices.Contains(allow, entry) {
			allow = append(allow, entry)
		}
	}
	checkSettings(t, path, `{
    "$schema": "https://json.schemastore.org/claude-code-settings.json",
    "permissions": {
        "deny": ["Bash(rm:*)"],
        "allow": [
`+quoted("            ", allow)+`
        ],
        "defaultMode": "acceptEdits"
    },
    "theme": "light",
    "model": "opus",
    "autoCompactEnabled": false,
    "enabledMcpjsonServers": [
        "github",
        "jbcontext",
        "rider"
    ],
    "hooks": {"Stop": []},
    "plansDirectory": ".claude/plans"
}
`)
	checkMode(t, path, 0o666)
}

// checkEditedFiles reports TestSetupConfigProjectEditsFiles' other files, unless they hold what cld
// adds, and a temporary file cld leaves.
func checkEditedFiles(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	checkFile(t, s, ".mcp.json", `{
  "mcpServers": {
    "github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/"},
`+mcpEntries["rider"]+`,
    "jbcontext": {"args": ["mcp"], "command": "jbcontext", "type": "stdio"}
  },
  "other": true
}
`)
	checkFile(t, s, ".gitignore", "node_modules/\n"+ignoreLines)
	target := filepath.Join(s.Root, "dotfiles", "settings.local.json")
	link, err := os.Readlink(filepath.Join(s.Work, ".claude", "settings.local.json"))
	if err != nil || link != target {
		t.Errorf("settings.local.json leads to %q (%v)", link, err)
	}
	if leftovers, err := filepath.Glob(filepath.Join(s.Work, ".claude", ".*.cld-*")); err != nil {
		t.Error(err)
	} else if len(leftovers) != 0 {
		t.Errorf("left %q", leftovers)
	}
}

// A key given twice: cld adds entries to the last array and leaves the others. It replaces
// neither value of a setting, nor the file's own $schema.
func editsKeyGivenTwice(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	settings := projectWrite(t, s, ".claude/settings.json",
		`{"$schema": "https://example.com/settings.json", "plansDirectory": "plans", `+
			`"enabledMcpjsonServers": ["goland"], "plansDirectory": "notes", `+
			`"enabledMcpjsonServers": []}`)
	projectWrite(t, s, ".mcp.json", `{"mcpServers": {"goland": {}}, `+
		`"mcpServers": {"goland": {}, "goland": {"type": "sse"}}}`)
	result := s.RunCld(nil, "setup", "config", "project", "--mcp", "goland", "--permissions", "cld")
	want := "Updated .claude/settings.json: permissions.allow, permissions.deny, " +
		"enabledMcpjsonServers\n" +
		"Created .claude/settings.local.json\n" +
		"Updated .mcp.json: mcpServers.goland\n" +
		"Created .gitignore\n"
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	checkSettings(t, settings, `{
  "$schema": "https://example.com/settings.json",
  "plansDirectory": "plans",
  "enabledMcpjsonServers": ["goland"],
  "plansDirectory": "notes",
  "enabledMcpjsonServers": [
    "goland"
  ],
  "permissions": {
    "allow": [
`+quoted("      ", allowed(t, "cld", "goland"))+`
    ],
    "deny": [
`+quoted("      ", denied(t, "cld"))+`
    ]
  }
}
`)
	checkFile(t, s, ".mcp.json", `{
  "mcpServers": {"goland": {}},
  "mcpServers": {
    "goland": {},
`+mcpEntries["goland"]+`
  }
}
`)
}

// Without --mcp, cld reads no .mcp.json: this one is not even valid.
func editsNoServers(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	projectWrite(t, s, ".mcp.json", "{")
	result := s.RunCld(nil, "setup", "config", "project")
	if result.Code != 0 || strings.Contains(result.Stdout, ".mcp.json") || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, nothing of .mcp.json",
			result.Code, result.Stdout, result.Stderr)
	}
	checkFile(t, s, ".mcp.json", "{")
}

// With --permissions none, cld allows nothing, and reads no permissions: these are no object.
func editsNoPermissions(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	settings := projectWrite(t, s, ".claude/settings.json", `{"permissions": "ask"}`)
	result := s.RunCld(nil, "setup", "config", "project", "--permissions", "none")
	want := "Updated .claude/settings.json: $schema, plansDirectory\n"
	if result.Code != 0 || !strings.HasPrefix(result.Stdout, want) || result.Stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout starting with %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	checkSettings(t, settings, `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": "ask",
  "plansDirectory": ".claude/plans"
}
`)
}
