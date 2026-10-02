package tests

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What setup project writes in .claude/settings.json: the permission rules of each set and MCP
// server. With --mcp goland --permissions cld, that is the repository's own file.

// readOnlyAllow is permissions.allow with --permissions read-only, the default, before the
// servers' entries; serverAllow are the entries each MCP server adds with read-only and with cld.
var (
	readOnlyAllow = []string{
		"Read",
		"Bash(ls:*)",
		"Bash(pwd:*)",
		"Bash(cat:*)",
		"Bash(head:*)",
		"Bash(tail:*)",
		"Bash(wc:*)",
		"Bash(grep:*)",
		"Bash(stat:*)",
		"Bash(du:*)",
		"Bash(which:*)",
		"Bash(git status:*)",
	}
	serverAllow = map[string]map[string][]string{
		"read-only": {
			"goland":    ideTools("goland"),
			"jbcontext": {"Bash(jbcontext search:*)", "mcp__jbcontext__code_search"},
			"rider":     ideTools("rider"),
		},
		"cld": {
			"goland":    {"mcp__goland"},
			"jbcontext": {"Bash(jbcontext:*)", "mcp__jbcontext"},
			"rider":     {"mcp__rider"},
		},
	}
)

// ideTools are the entries for the tools of the IDE's MCP server name that --permissions
// read-only allows: those GoLand 2026.2.3's server marks readOnlyHint.
func ideTools(name string) []string {
	var entries []string
	for _, tool := range []string{
		"analyze_calls", "get_all_open_file_paths", "get_file_problems", "get_project_dependencies",
		"get_project_modules", "get_repositories", "get_run_configurations", "get_symbol_info",
		"git_status", "lint_files", "list_directory_tree", "read_file", "search_file",
		"search_regex", "search_symbol", "search_text",
	} {
		entries = append(entries, "mcp__"+name+"__"+tool)
	}
	return entries
}

// cldRules are permissions.allow and permissions.deny with --permissions cld and no MCP server:
// the repository's own, without goland's entry.
func cldRules(t *testing.T) (allow, deny []string) {
	t.Helper()
	var settings struct {
		Permissions struct{ Allow, Deny []string }
	}
	if err := json.Unmarshal([]byte(repoFile(t, ".claude/settings.json")), &settings); err != nil {
		t.Fatal(err)
	}
	allow = slices.DeleteFunc(settings.Permissions.Allow,
		func(entry string) bool { return entry == "mcp__goland" })
	return allow, settings.Permissions.Deny
}

// allowed is permissions.allow with --permissions set and the MCP servers named: the set's
// entries, then the servers' in cld's order.
func allowed(t *testing.T, set string, servers ...string) []string {
	t.Helper()
	var allow []string
	switch set {
	case "read-only":
		allow = slices.Clone(readOnlyAllow)
	case "cld":
		allow, _ = cldRules(t)
	}
	for _, name := range servers {
		allow = append(allow, serverAllow[set][name]...)
	}
	return allow
}

// denied is permissions.deny with --permissions set: the repository's own with cld, else none.
func denied(t *testing.T, set string) []string {
	t.Helper()
	if set != "cld" {
		return nil
	}
	_, deny := cldRules(t)
	return deny
}

// projectSettings is the .claude/settings.json setup project writes where there is none, with
// --permissions set and the MCP servers named, in cld's order: the repository's own, with the
// set's and the servers' permissions in place of cld's and goland's, and none where they are empty.
func projectSettings(t *testing.T, set string, servers ...string) string {
	t.Helper()
	settings := repoFile(t, ".claude/settings.json")
	start, end := "  \"permissions\": {\n", "\n    ]\n  },\n"
	from, to := strings.Index(settings, start), strings.Index(settings, end)
	if from < 0 || to < from {
		t.Fatalf("no permissions in the repository's settings\n%s", settings)
	}
	permissions := ""
	if allow := allowed(t, set, servers...); len(allow) > 0 {
		permissions = start + "    \"allow\": [\n" + quoted("      ", allow)
		if deny := denied(t, set); len(deny) > 0 {
			permissions += "\n    ],\n    \"deny\": [\n" + quoted("      ", deny)
		}
		permissions += end
	}
	settings = settings[:from] + permissions + settings[to+len(end):]
	enabled := ""
	if len(servers) > 0 {
		enabled = ",\n  \"enabledMcpjsonServers\": [\n" + quoted("    ", servers) + "\n  ]"
	}
	goland := ",\n  \"enabledMcpjsonServers\": [\n    \"goland\"\n  ]"
	return replaceOnce(t, settings, goland, enabled)
}

// With --permissions cld, setup project adds the entries permissions.deny lacks after those it
// has, keeping them. The settings' order and indentation stay, the new key permissions.allow last.
func TestSetupProjectDeny(t *testing.T) {
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
	result := s.RunCld(nil, "setup", "project", "--permissions", "cld")
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
func TestSetupProjectDenyNoArray(t *testing.T) {
	t.Parallel()
	const before = `{"permissions": {"allow": ["Read"], "deny": "Bash(rm:*)"}}`
	t.Run("cld", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		gitInit(t, s)
		settings := projectWrite(t, s, ".claude/settings.json", before)
		result := s.RunCld(nil, "setup", "project", "--permissions", "cld")
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
		result := s.RunCld(nil, "setup", "project")
		data, err := os.ReadFile(settings)
		if result.Code != 0 || result.Stderr != "" || err != nil ||
			!strings.Contains(string(data), `"deny": "Bash(rm:*)"`) {
			t.Errorf("exit %d, stderr %q, settings %q (%v), want exit 0 and the deny kept",
				result.Code, result.Stderr, data, err)
		}
	})
}
