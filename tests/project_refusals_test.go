package tests

import (
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// What setup project refuses: files it cannot edit or write (decision 19.6), and usage errors.

// projectRefusal is a file setup project cannot edit: the file name holding content, or a
// symbolic link .claude/settings.json leading to link, and what cld says.
type projectRefusal struct {
	name, file, content, link, want string
}

// projectRefusals are TestSetupProjectRefuses' cases.
var projectRefusals = []projectRefusal{
	{"settings not valid", ".claude/settings.json", "{", "",
		".claude/settings.json is not valid JSON: line 1: unexpected end of JSON input"},
	{"settings empty", ".claude/settings.json", "", "",
		".claude/settings.json is not valid JSON: it is empty"},
	{"settings with a syntax error", ".claude/settings.json",
		"{\n  \"permissions\": {\n    \"allow\": [],\n  }\n}\n", "",
		".claude/settings.json is not valid JSON: line 4: " +
			"invalid character '}' looking for beginning of object key string"},
	{"settings no object", ".claude/settings.json", "[]", "",
		".claude/settings.json holds no JSON object"},
	{"permissions no object", ".claude/settings.json", `{"permissions": ["Read"]}`, "",
		"permissions in .claude/settings.json is not a JSON object"},
	{"allow no array", ".claude/settings.json", `{"permissions": {"allow": "Read"}}`, "",
		"permissions.allow in .claude/settings.json is not a JSON array"},
	{"enabledMcpjsonServers no array", ".claude/settings.json", `{"enabledMcpjsonServers": null}`,
		"", "enabledMcpjsonServers in .claude/settings.json is not a JSON array"},
	{"settings a link to no file", "", "", "../elsewhere/settings.json",
		".claude/settings.json is a symbolic link to a file that does not exist"},
	{".mcp.json not valid", ".mcp.json", "{} {}", "",
		".mcp.json is not valid JSON: more follows the object"},
	{"mcpServers no object", ".mcp.json", `{"mcpServers": []}`, "",
		"mcpServers in .mcp.json is not a JSON object"},
	{".gitignore a directory", ".gitignore/x", "", "", "cannot read .gitignore: is a directory"},
}

// cld reads every file before it writes any: one it cannot edit ends it with status 1 and
// nothing written.
func TestSetupProjectRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range projectRefusals {
		t.Run(test.name, test.run)
	}
}

// run runs setup project on the file that cld cannot edit.
func (test projectRefusal) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	if test.file != "" {
		projectWrite(t, s, test.file, test.content)
	}
	if test.link != "" {
		if err := os.Mkdir(filepath.Join(s.Work, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(test.link, filepath.Join(s.Work, ".claude", "settings.json")); err != nil {
			t.Fatal(err)
		}
	}
	before := tree(t, s.Work)
	result := s.RunCld(nil, "setup", "project", "--mcp", "goland")
	want := "cld: " + test.want + "\n"
	if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	if after := tree(t, s.Work); !maps.Equal(before, after) {
		t.Errorf("the files changed: %q, were %q", after, before)
	}
}

// A write that fails ends cld, which names the files it wrote before (decision 19.6). Here
// .gitignore is a symbolic link to a path of 4095 bytes, the most Linux takes, so the temporary
// file beside it has a longer one.
func TestSetupProjectCannotWrite(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the longest path is Linux's")
	}
	s := sandbox.New(t)
	gitInit(t, s)
	length := 4095 - len("/gitignore")
	dir := filepath.Join(s.Root, "long")
	for len(dir) < length-202 {
		dir += "/" + strings.Repeat("d", 200)
	}
	dir += "/" + strings.Repeat("d", length-len(dir)-1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "gitignore")
	s.WriteFile(target, "")
	if err := os.Symlink(target, filepath.Join(s.Work, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	result := s.RunCld(nil, "setup", "project")
	want := "cld: cannot write .gitignore: file name too long " +
		"(.claude/settings.json and .claude/settings.local.json written before it)\n"
	if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	checkFile(t, s, ".claude/settings.json", projectSettings(t, "read-only"))
	checkSettings(t, target, "")
}

// projectUsage is setup project's options and the usage error cld gives for them.
type projectUsage struct {
	args []string
	want string
}

const (
	// mcpUsage and permissionsUsage end the errors of a value --mcp or --permissions does not take.
	mcpUsage         = ": goland, jbcontext or rider (see cld help)\n"
	permissionsUsage = ": read-only, cld or none (see cld help)\n"
)

// projectUsages are TestSetupProjectRejectsArguments' cases.
var projectUsages = []projectUsage{
	{[]string{"--mcp", "idea"}, "cld: invalid MCP server 'idea' for --mcp" + mcpUsage},
	{[]string{"--mcp", "GoLand"}, "cld: invalid MCP server 'GoLand' for --mcp" + mcpUsage},
	{[]string{"--mcp", "goland", "--mcp", "rider,idea,x"},
		"cld: invalid MCP server 'idea' for --mcp" + mcpUsage},
	{[]string{"--mcp", ""}, "cld: invalid MCP server '' for --mcp" + mcpUsage},
	{[]string{"--mcp=goland,"}, "cld: invalid MCP server '' for --mcp" + mcpUsage},
	{[]string{"--mcp", "goland rider"},
		"cld: invalid MCP server 'goland rider' for --mcp" + mcpUsage},
	{[]string{"--mcp"}, "cld: option '--mcp' needs a value (see cld help)\n"},
	{[]string{"--permissions", "all"},
		"cld: invalid permissions 'all' for --permissions" + permissionsUsage},
	{[]string{"--permissions", "Cld"},
		"cld: invalid permissions 'Cld' for --permissions" + permissionsUsage},
	{[]string{"--permissions", ""},
		"cld: invalid permissions '' for --permissions" + permissionsUsage},
	{[]string{"--permissions=read-only,cld"},
		"cld: invalid permissions 'read-only,cld' for --permissions" + permissionsUsage},
	{[]string{"--permissions", "all", "--mcp", "idea"},
		"cld: invalid MCP server 'idea' for --mcp" + mcpUsage},
	{[]string{"--permissions"}, "cld: option '--permissions' needs a value (see cld help)\n"},
	{[]string{"--permissions", "cld", "none"},
		"cld: setup project: unexpected argument 'none' (see cld help)\n"},
	{[]string{"x"}, "cld: setup project: unexpected argument 'x' (see cld help)\n"},
	{[]string{"--mcp", "goland", "rider"},
		"cld: setup project: unexpected argument 'rider' (see cld help)\n"},
	{[]string{"x", "--mcp", "goland"},
		"cld: setup project: unexpected argument 'x' (see cld help)\n"},
	{[]string{"--"}, "cld: setup project: unexpected argument '--' (see cld help)\n"},
	{[]string{"--bogus"}, "cld: setup project: unexpected argument '--bogus' (see cld help)\n"},
	{[]string{"-n", "x"}, "cld: setup project: unexpected argument '-n' (see cld help)\n"},
}

// setup project's mistakes are usage errors, read left to right as other commands' are. A server
// --mcp does not take comes first, the empty one included, then a set --permissions does not
// take, an argument, or an option cld does not know. Nothing is written.
func TestSetupProjectRejectsArguments(t *testing.T) {
	t.Parallel()
	for _, test := range projectUsages {
		t.Run(strings.Join(test.command(), " "), test.run)
	}
}

// command is cld's arguments.
func (test projectUsage) command() []string {
	return append([]string{"setup", "project"}, test.args...)
}

// run runs setup project with the options, in an empty work directory.
func (test projectUsage) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	result := s.RunCld(nil, test.command()...)
	if result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q",
			result.Code, result.Stdout, result.Stderr, test.want)
	}
	if entries, err := os.ReadDir(s.Work); err != nil || len(entries) != 0 {
		t.Errorf("the work directory holds %v (%v), want nothing", entries, err)
	}
}
