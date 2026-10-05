package tests

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What setup config project writes in .claude/settings.json: the permission rules of each set and
// MCP server. With --mcp goland --permissions cld, that is the repository's own file, but for its
// hooks.

// With --permissions cld, setup config project adds the entries permissions.deny lacks after those
// it has, keeping them. The settings' order and indentation stay, the new key permissions.allow
// last.
func TestSetupConfigProjectDeny(t *testing.T) {
	t.Parallel()
	deny := denied(t, "cld")
	if len(deny) < 3 {
		t.Fatalf("the repository's settings deny %q, want three entries or more", deny)
	}
	s := sandbox.New(t)
	gitInit(t, s)
	settings := projectWrite(t, s, ".claude/settings.json", `{
  "permissions": {
    "deny": [
      "Bash(rm:*)",
      `+strconv.Quote(deny[1])+`
    ]
  }
}
`)
	result := s.RunCld(nil, "setup", "config", "project", "--permissions", "cld")
	want := "Updated .claude/settings.json: $schema, permissions.allow, permissions.deny, " +
		"plansDirectory\n"
	if result.Code != 0 || !strings.HasPrefix(result.Stdout, want) || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout starting with %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	kept := append([]string{"Bash(rm:*)", deny[1], deny[0]}, deny[2:]...)
	checkSettings(t, settings, `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "deny": [
`+quoted("      ", kept)+`
    ],
    "allow": [
`+quoted("      ", allowed(t, "cld"))+`
    ]
  },
  "plansDirectory": ".claude/plans"
}
`)
}

// A permissions.deny that is no array: --permissions cld refuses it with status 1, writing
// nothing, and read-only, which denies nothing, leaves it as the file has it.
func TestSetupConfigProjectDenyNoArray(t *testing.T) {
	t.Parallel()
	const before = `{"permissions": {"allow": ["Read"], "deny": "Bash(rm:*)"}}`
	t.Run("cld", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		gitInit(t, s)
		settings := projectWrite(t, s, ".claude/settings.json", before)
		result := s.RunCld(nil, "setup", "config", "project", "--permissions", "cld")
		want := "cld: permissions.deny in .claude/settings.json is not a JSON array\n"
		if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
				result.Code, result.Stdout, result.Stderr, want)
		}
		checkSettings(t, settings, before)
	})
	t.Run("read-only", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		gitInit(t, s)
		settings := projectWrite(t, s, ".claude/settings.json", before)
		result := s.RunCld(nil, "setup", "config", "project")
		data, err := os.ReadFile(settings)
		if result.Code != 0 || result.Stderr != "" || err != nil ||
			!strings.Contains(string(data), `"deny": "Bash(rm:*)"`) {
			t.Errorf("exit %d, stderr %q, settings %q (%v), want exit 0 and the deny kept",
				result.Code, result.Stderr, data, err)
		}
	})
}
