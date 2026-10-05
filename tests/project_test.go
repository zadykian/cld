package tests

import (
	"maps"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// setup config project in the sandbox's work directory, a git work tree unless a test says
// otherwise, against the real git. Where nothing was, cld writes the repository's own .mcp.json and
// .claude/settings.json, less its hooks, with --mcp goland --permissions cld (decisions 19.1 and
// 28.6). The tests read both from the repository, so a change to either goes with a change to cld.

const (
	// localSettings is the .claude/settings.local.json setup config project writes where there is
	// none.
	localSettings = "{\n  \"$schema\": \"https://json.schemastore.org/claude-code-settings.json\"\n}\n"
	// ignoreLines are the lines setup config project adds to .gitignore, and addedLines how it
	// reports them.
	ignoreLines = "/.claude/settings.local.json\n/.claude/plans/\n/.claude/worktrees/\n"
	addedLines  = "/.claude/settings.local.json, /.claude/plans/, /.claude/worktrees/"
)

// mcpEntries are each MCP server's entry in .mcp.json's mcpServers, as cld writes it there.
var mcpEntries = map[string]string{
	"goland": `    "goland": {
      "type": "http",
      "url": "http://127.0.0.1:${GOLAND_MCP_PORT:-64422}/stream"
    }`,
	"jbcontext": `    "jbcontext": {
      "type": "stdio",
      "command": "jbcontext",
      "args": [
        "mcp"
      ]
    }`,
	"rider": `    "rider": {
      "type": "http",
      "url": "http://127.0.0.1:${RIDER_MCP_PORT:-64482}/stream"
    }`,
}

// mcpFile is the .mcp.json setup config project writes where there is none, with the MCP servers
// named.
func mcpFile(servers ...string) string {
	var entries []string
	for _, name := range servers {
		entries = append(entries, mcpEntries[name])
	}
	return "{\n  \"mcpServers\": {\n" + strings.Join(entries, ",\n") + "\n  }\n}\n"
}

// checkFile reports a file name of the sandbox's work directory that does not hold want.
func checkFile(t *testing.T, s *sandbox.Sandbox, name, want string) {
	t.Helper()
	checkSettings(t, filepath.Join(s.Work, name), want)
}

// gitStatus is what git status says of the files of the sandbox's work directory, one a line,
// sorted: "?? PATH" for one that git add would add, "!! PATH" for one it ignores.
func gitStatus(t *testing.T, s *sandbox.Sandbox) []string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all", "--ignored")
	cmd.Dir, cmd.Env = s.Work, s.Environ(nil)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	slices.Sort(lines)
	return lines
}

// projectFresh is a run of setup config project where there is nothing: its options, the permission
// set expected and the MCP servers.
type projectFresh struct {
	args    []string
	set     string
	servers []string
}

// projectFreshCases are TestSetupConfigProject's: read-only's entries by default and none with
// none, the servers in cld's order whatever the order given, each once.
var projectFreshCases = []projectFresh{
	{nil, "read-only", nil},
	{[]string{"--mcp", "goland"}, "read-only", []string{"goland"}},
	{
		[]string{"--permissions", "read-only", "--mcp=jbcontext,rider"},
		"read-only", []string{"jbcontext", "rider"},
	},
	{[]string{"--mcp", "goland", "--permissions", "cld"}, "cld", []string{"goland"}},
	{
		[]string{"--permissions=cld", "--mcp", "rider,goland", "--mcp", "jbcontext,rider"},
		"cld", []string{"goland", "jbcontext", "rider"},
	},
	{[]string{"--permissions", "cld"}, "cld", nil},
	{[]string{"--permissions", "none"}, "none", nil},
	{[]string{"--mcp=jbcontext", "--permissions", "none"}, "none", []string{"jbcontext"}},
	{[]string{"--permissions", "none", "--permissions", "cld"}, "cld", nil},
}

// Where there is nothing, setup config project writes each file as the repository has it, with
// --mcp goland --permissions cld. git adds the settings and .mcp.json, and ignores
// settings.local.json. Run again, setup config project changes no file.
func TestSetupConfigProject(t *testing.T) {
	t.Parallel()
	if want, got := repoFile(t, ".mcp.json"), mcpFile("goland"); got != want {
		t.Fatalf("the repository's .mcp.json\n%s\nwant, as the tests expect of --mcp goland\n%s",
			want, got)
	}
	if got := projectSettings(t, "cld", "goland"); got != repoSettings(t) {
		t.Fatalf("the settings the tests expect of --mcp goland --permissions cld\n%s\n"+
			"are not the repository's", got)
	}
	for _, test := range projectFreshCases {
		t.Run(strings.Join(test.command(), " "), test.run)
	}
}

// command is cld's arguments.
func (test projectFresh) command() []string {
	return append([]string{"setup", "config", "project"}, test.args...)
}

// report is what setup config project prints: a line for each file it writes, verb before it and
// suffix after it.
func (test projectFresh) report(verb, suffix string) string {
	names := []string{".claude/settings.json", ".claude/settings.local.json"}
	if test.servers != nil {
		names = append(names, ".mcp.json")
	}
	report := ""
	for _, name := range append(names, ".gitignore") {
		report += verb + " " + name + suffix + "\n"
	}
	return report
}

// files is what the work directory holds afterwards, "/" for a directory, and status what git
// status says of it.
func (test projectFresh) files(t *testing.T) (files map[string]string, status []string) {
	t.Helper()
	files = map[string]string{
		".claude":                     "/",
		".claude/settings.json":       projectSettings(t, test.set, test.servers...),
		".claude/settings.local.json": localSettings,
		".gitignore":                  ignoreLines,
	}
	status = []string{"!! .claude/settings.local.json", "?? .claude/settings.json", "?? .gitignore"}
	if test.servers != nil {
		files[".mcp.json"] = mcpFile(test.servers...)
		status = append(status, "?? .mcp.json")
	}
	return files, status
}

// run runs setup config project in a new git work tree, then again.
func (test projectFresh) run(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s)
	result := s.RunCld(nil, test.command()...)
	want := test.report("Created", "")
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			result.Code, result.Stdout, result.Stderr, want)
	}
	files, status := test.files(t)
	for name, content := range files {
		if content != "/" {
			checkFile(t, s, name, content)
		}
	}
	names := slices.Sorted(maps.Keys(files))
	if got := slices.Sorted(maps.Keys(tree(t, s.Work))); !slices.Equal(got, names) {
		t.Errorf("the work directory holds %q, want %q", got, names)
	}
	if runtime.GOOS == "linux" {
		mask := umask(t)
		checkMode(t, filepath.Join(s.Work, ".claude"), 0o755&^mask)
		checkMode(t, filepath.Join(s.Work, ".claude", "settings.json"), 0o644&^mask)
	}
	if got := gitStatus(t, s); !slices.Equal(got, status) {
		t.Errorf("git status %q, want %q", got, status)
	}
	test.again(t, s)
}

// again runs setup config project once more, which changes no file.
func (test projectFresh) again(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	before := tree(t, s.Work)
	again := s.RunCld(nil, test.command()...)
	want := test.report("Left", " as it was")
	if again.Code != 0 || again.Stdout != want || again.Stderr != "" {
		t.Errorf("again: exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			again.Code, again.Stdout, again.Stderr, want)
	}
	if after := tree(t, s.Work); !maps.Equal(before, after) {
		t.Errorf("again, the files changed: %q, were %q", after, before)
	}
}
