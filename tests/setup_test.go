package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What the tests of the setup commands share: the settings setup project writes, and setup
// completion's scripts and lines.

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

// repoSettings is the repository's .claude/settings.json without its last key, hooks, which setup
// project does not write.
func repoSettings(t *testing.T) string {
	t.Helper()
	settings := repoFile(t, ".claude/settings.json")
	if before, _, found := strings.Cut(settings, ",\n  \"hooks\": {\n"); found {
		return before + "\n}\n"
	}
	return settings
}

// cldRules are permissions.allow and permissions.deny with --permissions cld and no MCP server:
// the repository's own, without goland's entry.
func cldRules(t *testing.T) (allow, deny []string) {
	t.Helper()
	var settings struct {
		Permissions struct{ Allow, Deny []string }
	}
	if err := json.Unmarshal([]byte(repoSettings(t)), &settings); err != nil {
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
// --permissions set and the MCP servers named, in cld's order. That is repoSettings, with the
// set's and the servers' permissions in place of cld's and goland's, and none where they are empty.
func projectSettings(t *testing.T, set string, servers ...string) string {
	t.Helper()
	settings := repoSettings(t)
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

// replaceOnce is text with old, which it holds once, replaced by new.
func replaceOnce(t *testing.T, text, old, new string) string {
	t.Helper()
	if count := strings.Count(text, old); count != 1 {
		t.Fatalf("%q is %d times in\n%s", old, count, text)
	}
	return strings.Replace(text, old, new, 1)
}

// quoted is entries as JSON strings, each after prefix, separated by commas and newlines.
func quoted(prefix string, entries []string) string {
	var lines []string
	for _, entry := range entries {
		lines = append(lines, prefix+strconv.Quote(entry))
	}
	return strings.Join(lines, ",\n")
}

// projectWrite writes content to the file name of the sandbox's work directory, making its
// directory, and returns its path.
func projectWrite(t *testing.T, s *sandbox.Sandbox, name, content string) string {
	t.Helper()
	path := filepath.Join(s.Work, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s.WriteFile(path, content)
	return path
}

// zshLines are the lines setup completion zsh adds to .zshrc.
const zshLines = `# cld's completion, from cld setup completion zsh
if [[ -r ${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh/_cld ]]; then
  fpath=("${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh" $fpath)
  (( $+functions[compdef] )) || { autoload -U compinit && compinit -i; }
  autoload -Uz _cld && compdef _cld cld
fi
`

// scripts are where setup completion writes each shell's script, under the home directory, without
// the variables that move it.
var scripts = map[string]string{
	"bash": ".local/share/bash-completion/completions/cld",
	"zsh":  ".local/share/cld/zsh/_cld",
	"fish": ".config/fish/completions/cld.fish",
}
