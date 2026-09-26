package tests

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// cld setup project in the sandbox's work directory, a git work tree unless a test says otherwise,
// against the real git, which checks the .gitignore cld writes. What cld writes where there was
// nothing is this repository's own .claude/settings.json and .mcp.json with --mcp goland, which
// the tests read from the repository (they run in its tests directory): a change to either shows
// here, and goes with a change to cld.

const (
	// localSettings is the .claude/settings.local.json setup project writes where there is none.
	localSettings = "{\n  \"$schema\": \"https://json.schemastore.org/claude-code-settings.json\"\n}\n"
	// ignoreLines are the lines setup project adds to .gitignore.
	ignoreLines = "/.claude/*\n!/.claude/settings.json\n"
)

// serverAllow are the entries each MCP server adds to permissions.allow, and mcpEntries its entry
// in .mcp.json's mcpServers, as cld writes it there.
var (
	serverAllow = map[string][]string{
		"goland":    {"mcp__goland"},
		"jbcontext": {"Bash(jbcontext:*)", "mcp__jbcontext"},
		"rider":     {"mcp__rider"},
	}
	mcpEntries = map[string]string{
		"goland":    "    \"goland\": {\n      \"type\": \"http\",\n      \"url\": \"http://127.0.0.1:64422/stream\"\n    }",
		"jbcontext": "    \"jbcontext\": {\n      \"type\": \"stdio\",\n      \"command\": \"jbcontext\",\n      \"args\": [\n        \"mcp\"\n      ]\n    }",
		"rider":     "    \"rider\": {\n      \"type\": \"http\",\n      \"url\": \"http://127.0.0.1:64482/stream\"\n    }",
	}
)

// repoFile is what the file name of cld's own repository holds.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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

// projectSettings is the .claude/settings.json setup project writes where there is none, with the
// MCP servers named, in cld's order: the repository's own, whose server is goland, with theirs
// in goland's place.
func projectSettings(t *testing.T, servers ...string) string {
	t.Helper()
	settings := repoFile(t, ".claude/settings.json")
	var allow []string
	for _, name := range servers {
		allow = append(allow, serverAllow[name]...)
	}
	added := ""
	if len(allow) > 0 {
		added = ",\n" + quoted("      ", allow)
	}
	settings = replaceOnce(t, settings, ",\n      \"mcp__goland\"\n    ]", added+"\n    ]")
	enabled := ""
	if len(servers) > 0 {
		enabled = ",\n  \"enabledMcpjsonServers\": [\n" + quoted("    ", servers) + "\n  ]"
	}
	return replaceOnce(t, settings, ",\n  \"enabledMcpjsonServers\": [\n    \"goland\"\n  ]", enabled)
}

// baseAllow is permissions.allow of the settings setup project writes without MCP servers: the
// repository's own, but for goland's entry.
func baseAllow(t *testing.T) []string {
	t.Helper()
	var settings struct{ Permissions struct{ Allow []string } }
	if err := json.Unmarshal([]byte(repoFile(t, ".claude/settings.json")), &settings); err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(settings.Permissions.Allow, func(entry string) bool { return entry == "mcp__goland" })
}

// mcpFile is the .mcp.json setup project writes where there is none, with the MCP servers named.
func mcpFile(servers ...string) string {
	var entries []string
	for _, name := range servers {
		entries = append(entries, mcpEntries[name])
	}
	return "{\n  \"mcpServers\": {\n" + strings.Join(entries, ",\n") + "\n  }\n}\n"
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

// tree is what dir holds, but for .git: each directory, file and symbolic link by its path, with
// a file's content and a link's target.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		name, _ := filepath.Rel(dir, path)
		switch {
		case entry.Name() == ".git":
			return filepath.SkipDir
		case entry.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			entries[name] = "-> " + target
			return err
		case entry.IsDir():
			entries[name] = "/"
		default:
			data, err := os.ReadFile(path)
			entries[name] = string(data)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// setup project where there is nothing: each file as the repository has it with --mcp goland, the
// servers in cld's order whatever the order given, each once, and git adding the settings and
// .mcp.json but ignoring settings.local.json. Run again, it leaves every file as it is.
func TestSetupProject(t *testing.T) {
	t.Parallel()
	if want, got := repoFile(t, ".mcp.json"), mcpFile("goland"); got != want {
		t.Fatalf("the repository's .mcp.json\n%s\nwant, as the tests expect of --mcp goland\n%s", want, got)
	}
	if got := projectSettings(t, "goland"); got != repoFile(t, ".claude/settings.json") {
		t.Fatalf("the settings the tests expect of --mcp goland\n%s\nare not the repository's", got)
	}
	for _, test := range []struct {
		args    []string
		servers []string
	}{
		{nil, nil},
		{[]string{"--mcp", "goland"}, []string{"goland"}},
		{[]string{"--mcp=jbcontext"}, []string{"jbcontext"}},
		{[]string{"--mcp", "rider,goland", "--mcp", "jbcontext,rider"}, []string{"goland", "jbcontext", "rider"}},
	} {
		args := append([]string{"setup", "project"}, test.args...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			result := s.RunCld(nil, args...)
			want := "Created .claude/settings.json\nCreated .claude/settings.local.json\n"
			if test.servers != nil {
				want += "Created .mcp.json\n"
			}
			want += "Created .gitignore\n"
			if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, want)
			}
			files := map[string]string{
				".claude":                     "/",
				".claude/settings.json":       projectSettings(t, test.servers...),
				".claude/settings.local.json": localSettings,
				".gitignore":                  ignoreLines,
			}
			status := []string{"!! .claude/settings.local.json", "?? .claude/settings.json", "?? .gitignore"}
			if test.servers != nil {
				files[".mcp.json"] = mcpFile(test.servers...)
				status = append(status, "?? .mcp.json")
			}
			for name, content := range files {
				if content != "/" {
					checkFile(t, s, name, content)
				}
			}
			if got := slices.Sorted(maps.Keys(tree(t, s.Work))); !slices.Equal(got, slices.Sorted(maps.Keys(files))) {
				t.Errorf("the work directory holds %q, want %q", got, slices.Sorted(maps.Keys(files)))
			}
			if runtime.GOOS == "linux" {
				mask := umask(t)
				checkMode(t, filepath.Join(s.Work, ".claude"), 0o755&^mask)
				checkMode(t, filepath.Join(s.Work, ".claude", "settings.json"), 0o644&^mask)
			}
			if got := gitStatus(t, s); !slices.Equal(got, status) {
				t.Errorf("git status %q, want %q", got, status)
			}

			before := tree(t, s.Work)
			again := s.RunCld(nil, args...)
			want = "Left .claude/settings.json as it was\nLeft .claude/settings.local.json as it was\n"
			if test.servers != nil {
				want += "Left .mcp.json as it was\n"
			}
			want += "Left .gitignore as it was\n"
			if again.Code != 0 || again.Stdout != want || again.Stderr != "" {
				t.Errorf("again: exit %d, stdout %q, stderr %q, want exit 0, stdout %q", again.Code, again.Stdout, again.Stderr, want)
			}
			if after := tree(t, s.Work); !maps.Equal(before, after) {
				t.Errorf("again, the files changed: %q, were %q", after, before)
			}
		})
	}
}

// Files that exist: cld sets the keys it manages - the last of a key given twice, which claude
// reads - adds the entries that permissions.allow and enabledMcpjsonServers lack after theirs,
// replaces a server's entry that differs from its own, whole, and keeps everything else as the
// file has it: its order, indentation and mode, the values it leaves - <, > and & in them - and
// the other servers, a server written otherwise but the same included. settings.local.json is
// left as it is, a symbolic link to no file included, and .gitignore gets the lines it lacks.
func TestSetupProjectEditsFiles(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	gitInit(t, s.Work)
	settings := projectWrite(t, s, ".claude/settings.json", `{
    "permissions": {
        "deny": ["Bash(rm:*)"],
        "allow": ["Bash(make test)", "Read", "mcp__rider", "Read(<&>)"],
        "defaultMode": "acceptEdits"
    },
    "theme": "light",
    "model": "opus",
    "autoCompactEnabled": true,
    "enabledMcpjsonServers": ["github"],
    "hooks": {"Stop": []}
}
`)
	if err := os.Chmod(settings, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(s.Root, "dotfiles", "settings.local.json"), filepath.Join(s.Work, ".claude", "settings.local.json")); err != nil {
		t.Fatal(err)
	}
	projectWrite(t, s, ".mcp.json", `{
  "mcpServers": {
    "github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/"},
    "rider": {"type": "http", "url": "http://127.0.0.1:63342/stream", "headers": {"X-Token": "<&>"}},
    "jbcontext": {"args": ["mcp"], "command": "jbcontext", "type": "stdio"}
  },
  "other": true
}
`)
	projectWrite(t, s, ".gitignore", "node_modules/\n")
	result := s.RunCld(nil, "setup", "project", "--mcp", "rider,jbcontext")
	want := "Updated .claude/settings.json: $schema, permissions.allow, autoUpdatesChannel, plansDirectory, autoMemoryEnabled, theme, enabledMcpjsonServers\n" +
		"Left .claude/settings.local.json as it was\n" +
		"Updated .mcp.json: mcpServers.rider\n" +
		"Updated .gitignore: /.claude/*, !/.claude/settings.json\n"
	if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, want)
	}
	allow := []string{"Bash(make test)", "Read", "mcp__rider", "Read(<&>)"}
	for _, entry := range append(baseAllow(t), "Bash(jbcontext:*)", "mcp__jbcontext", "mcp__rider") {
		if !slices.Contains(allow, entry) {
			allow = append(allow, entry)
		}
	}
	checkSettings(t, settings, `{
    "$schema": "https://json.schemastore.org/claude-code-settings.json",
    "permissions": {
        "deny": ["Bash(rm:*)"],
        "allow": [
`+quoted("            ", allow)+`
        ],
        "defaultMode": "acceptEdits"
    },
    "theme": "dark",
    "model": "opus",
    "autoCompactEnabled": true,
    "enabledMcpjsonServers": [
        "github",
        "jbcontext",
        "rider"
    ],
    "hooks": {"Stop": []},
    "autoUpdatesChannel": "latest",
    "plansDirectory": ".claude/plans",
    "autoMemoryEnabled": true
}
`)
	checkMode(t, settings, 0o666)
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
	if link, err := os.Readlink(filepath.Join(s.Work, ".claude", "settings.local.json")); err != nil || link != filepath.Join(s.Root, "dotfiles", "settings.local.json") {
		t.Errorf("settings.local.json leads to %q (%v)", link, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(s.Work, ".claude", ".*.cld-*")); len(leftovers) != 0 {
		t.Errorf("left %q", leftovers)
	}

	// A key given twice: cld edits the last, and leaves the others as they are.
	t.Run("a key given twice", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		gitInit(t, s.Work)
		settings := projectWrite(t, s, ".claude/settings.json", `{"theme": "light", "enabledMcpjsonServers": [], "theme": "light", "enabledMcpjsonServers": ["goland"]}`)
		projectWrite(t, s, ".mcp.json", `{"mcpServers": {"goland": {}}, "mcpServers": {"goland": {}, "goland": {"type": "sse"}}}`)
		result := s.RunCld(nil, "setup", "project", "--mcp", "goland")
		want := "Updated .claude/settings.json: $schema, permissions.allow, autoUpdatesChannel, plansDirectory, autoMemoryEnabled, theme, autoCompactEnabled\n" +
			"Created .claude/settings.local.json\n" +
			"Updated .mcp.json: mcpServers.goland\n" +
			"Created .gitignore\n"
		if result.Code != 0 || result.Stdout != want || result.Stderr != "" {
			t.Fatalf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, want)
		}
		checkSettings(t, settings, `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "theme": "light",
  "enabledMcpjsonServers": [],
  "theme": "dark",
  "enabledMcpjsonServers": ["goland"],
  "permissions": {
    "allow": [
`+quoted("      ", append(baseAllow(t), "mcp__goland"))+`
    ]
  },
  "autoUpdatesChannel": "latest",
  "plansDirectory": ".claude/plans",
  "autoMemoryEnabled": true,
  "autoCompactEnabled": true
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
	})

	// Without --mcp, cld reads no .mcp.json: this one is not even valid.
	t.Run("no servers", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		gitInit(t, s.Work)
		projectWrite(t, s, ".mcp.json", "{")
		if result := s.RunCld(nil, "setup", "project"); result.Code != 0 || strings.Contains(result.Stdout, ".mcp.json") || result.Stderr != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, nothing of .mcp.json", result.Code, result.Stdout, result.Stderr)
		}
		checkFile(t, s, ".mcp.json", "{")
	})
}

// .gitignore gets /.claude/* and then !/.claude/settings.json where it lacks them, in its line
// endings, after a newline where its last line has none. A line counts as there as git reads it:
// with its leading slash or without, and without a carriage return or spaces at its end - but a
// tab stays - and the exception only after the last line that ignores .claude/*. Each time git
// adds the settings and ignores settings.local.json.
func TestSetupProjectGitignore(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, before, after, added string
	}{
		{"empty", "", ignoreLines, "/.claude/*, !/.claude/settings.json"},
		{"no newline at the end", "dist/", "dist/\n" + ignoreLines, "/.claude/*, !/.claude/settings.json"},
		{"carriage returns", "dist/\r\n", "dist/\r\n/.claude/*\r\n!/.claude/settings.json\r\n", "/.claude/*, !/.claude/settings.json"},
		{"there", "dist/\n" + ignoreLines, "", ""},
		{"without slashes", ".claude/*\n!.claude/settings.json\n", "", ""},
		{"with carriage returns", "/.claude/*\r\n!/.claude/settings.json\r\n", "", ""},
		{"with spaces", "/.claude/*  \n!/.claude/settings.json \n", "", ""},
		{"with a tab", "/.claude/*\n!/.claude/settings.json\t\n", "/.claude/*\n!/.claude/settings.json\t\n!/.claude/settings.json\n", "!/.claude/settings.json"},
		{"ignored only", "/.claude/*\n", ignoreLines, "!/.claude/settings.json"},
		{"excepted only", "!/.claude/settings.json\n", "!/.claude/settings.json\n" + ignoreLines, "/.claude/*, !/.claude/settings.json"},
		{"excepted first", "!/.claude/settings.json\n/.claude/*\n", "!/.claude/settings.json\n" + ignoreLines, "!/.claude/settings.json"},
		{"ignored again", ignoreLines + ".claude/*\n", ignoreLines + ".claude/*\n!/.claude/settings.json\n", "!/.claude/settings.json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			projectWrite(t, s, ".gitignore", test.before)
			result := s.RunCld(nil, "setup", "project")
			line, after := "Left .gitignore as it was\n", test.before
			if test.added != "" {
				line, after = "Updated .gitignore: "+test.added+"\n", test.after
			}
			if result.Code != 0 || !strings.HasSuffix(result.Stdout, line) || result.Stderr != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout ending in %q", result.Code, result.Stdout, result.Stderr, line)
			}
			checkFile(t, s, ".gitignore", after)
			if got, want := gitStatus(t, s), []string{"!! .claude/settings.local.json", "?? .claude/settings.json", "?? .gitignore"}; !slices.Equal(got, want) {
				t.Errorf("git status %q, want %q", got, want)
			}
		})
	}
}

// Once the files are written, cld asks git whether it ignores .claude/settings.json all the same -
// a pattern that ignores .claude/, which keeps git out of the directory, another after cld's
// lines, git's own excludes - and says by what, with status 1, after the report. It does not ask
// outside a git work tree, or without git.
func TestSetupProjectIgnoredAllTheSame(t *testing.T) {
	t.Parallel()
	const created = "Created .claude/settings.json\nCreated .claude/settings.local.json\n"
	for _, test := range []struct {
		name, gitignore, exclude string
		// git is whether the work directory is a git work tree, with git on the PATH.
		git            bool
		stdout, stderr string
	}{
		{"the directory", ".claude/\n", "", true, created + "Updated .gitignore: /.claude/*, !/.claude/settings.json\n",
			"cld: git ignores .claude/settings.json all the same, by the pattern .claude/ (.gitignore, line 1)\n"},
		{"after cld's lines", ignoreLines + "*.json\n", "", true, created + "Left .gitignore as it was\n",
			"cld: git ignores .claude/settings.json all the same, by the pattern *.json (.gitignore, line 3)\n"},
		{"git's excludes", "", ".claude\n", true, created + "Created .gitignore\n",
			"cld: git ignores .claude/settings.json all the same, by the pattern .claude (.git/info/exclude, line 1)\n"},
		{"no work tree", ".claude/\n", "", false, created + "Updated .gitignore: /.claude/*, !/.claude/settings.json\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			// git looks no further up than the sandbox for a work tree.
			extra := map[string]string{"GIT_CEILING_DIRECTORIES": s.Root}
			if test.git {
				gitInit(t, s.Work)
			}
			if test.gitignore != "" {
				projectWrite(t, s, ".gitignore", test.gitignore)
			}
			if test.exclude != "" {
				projectWrite(t, s, ".git/info/exclude", test.exclude)
			}
			result := s.RunCld(extra, "setup", "project")
			code := 0
			if test.stderr != "" {
				code = 1
			}
			if result.Code != code || result.Stdout != test.stdout || result.Stderr != test.stderr {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit %d, stdout %q, stderr %q", result.Code, result.Stdout, result.Stderr, code, test.stdout, test.stderr)
			}
			checkFile(t, s, ".claude/settings.json", projectSettings(t))
		})
	}
	t.Run("no git", func(t *testing.T) {
		t.Parallel()
		s := sandbox.New(t)
		gitInit(t, s.Work)
		projectWrite(t, s, ".gitignore", ".claude/\n")
		result := s.RunCld(map[string]string{"PATH": s.Tools()}, "setup", "project")
		if want := created + "Updated .gitignore: /.claude/*, !/.claude/settings.json\n"; result.Code != 0 || result.Stdout != want || result.Stderr != "" {
			t.Errorf("exit %d, stdout %q, stderr %q, want exit 0, stdout %q", result.Code, result.Stdout, result.Stderr, want)
		}
	})
}

// cld reads every file before it writes any: one it cannot edit ends it with status 1 and
// nothing written.
func TestSetupProjectRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		files map[string]string
		// link is where a symbolic link .claude/settings.json leads, "" for none.
		link, want string
	}{
		{"settings not valid", map[string]string{".claude/settings.json": "{"}, "",
			".claude/settings.json is not valid JSON: line 1: unexpected end of JSON input"},
		{"settings empty", map[string]string{".claude/settings.json": ""}, "", ".claude/settings.json is not valid JSON: it is empty"},
		{"settings with a syntax error", map[string]string{".claude/settings.json": "{\n  \"permissions\": {\n    \"allow\": [],\n  }\n}\n"}, "",
			".claude/settings.json is not valid JSON: line 4: invalid character '}' looking for beginning of object key string"},
		{"settings no object", map[string]string{".claude/settings.json": "[]"}, "", ".claude/settings.json holds no JSON object"},
		{"permissions no object", map[string]string{".claude/settings.json": `{"permissions": ["Read"]}`}, "",
			"permissions in .claude/settings.json is not a JSON object"},
		{"allow no array", map[string]string{".claude/settings.json": `{"permissions": {"allow": "Read"}}`}, "",
			"permissions.allow in .claude/settings.json is not a JSON array"},
		{"enabledMcpjsonServers no array", map[string]string{".claude/settings.json": `{"enabledMcpjsonServers": null}`}, "",
			"enabledMcpjsonServers in .claude/settings.json is not a JSON array"},
		{"settings a link to no file", nil, "../elsewhere/settings.json",
			".claude/settings.json is a symbolic link to a file that does not exist"},
		{".mcp.json not valid", map[string]string{".mcp.json": "{} {}"}, "", ".mcp.json is not valid JSON: more follows the object"},
		{"mcpServers no object", map[string]string{".mcp.json": `{"mcpServers": []}`}, "", "mcpServers in .mcp.json is not a JSON object"},
		{".gitignore a directory", map[string]string{".gitignore/x": ""}, "", "cannot read .gitignore: is a directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			gitInit(t, s.Work)
			for name, content := range test.files {
				projectWrite(t, s, name, content)
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
			if want := "cld: " + test.want + "\n"; result.Code != 1 || result.Stderr != want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
			}
			if after := tree(t, s.Work); !maps.Equal(before, after) {
				t.Errorf("the files changed: %q, were %q", after, before)
			}
		})
	}
}

// A file that cannot be written ends cld, which says what it wrote before it: here .gitignore, a
// symbolic link to a file whose path is 4095 bytes long, the most Linux takes, where the temporary
// file beside it has a longer one.
func TestSetupProjectCannotWrite(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the longest path is Linux's")
	}
	s := sandbox.New(t)
	gitInit(t, s.Work)
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
	want := "cld: cannot write .gitignore: file name too long (.claude/settings.json and .claude/settings.local.json written before it)\n"
	if result.Code != 1 || result.Stderr != want || result.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1, stderr %q", result.Code, result.Stdout, result.Stderr, want)
	}
	checkFile(t, s, ".claude/settings.json", projectSettings(t))
	checkSettings(t, target, "")
}

// setup project's mistakes are usage errors, read left to right as other commands' are: a server
// that --mcp does not take, the empty one included, an argument, an option cld does not know.
// Nothing is written.
func TestSetupProjectRejectsArguments(t *testing.T) {
	t.Parallel()
	const servers = ": goland, jbcontext or rider (see cld help)\n"
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--mcp", "idea"}, "cld: invalid MCP server 'idea' for --mcp" + servers},
		{[]string{"--mcp", "GoLand"}, "cld: invalid MCP server 'GoLand' for --mcp" + servers},
		{[]string{"--mcp", "goland", "--mcp", "rider,idea,x"}, "cld: invalid MCP server 'idea' for --mcp" + servers},
		{[]string{"--mcp", ""}, "cld: invalid MCP server '' for --mcp" + servers},
		{[]string{"--mcp=goland,"}, "cld: invalid MCP server '' for --mcp" + servers},
		{[]string{"--mcp", "goland rider"}, "cld: invalid MCP server 'goland rider' for --mcp" + servers},
		{[]string{"--mcp"}, "cld: option '--mcp' needs a value (see cld help)\n"},
		{[]string{"x"}, "cld: setup project: unexpected argument 'x' (see cld help)\n"},
		{[]string{"--mcp", "goland", "rider"}, "cld: setup project: unexpected argument 'rider' (see cld help)\n"},
		{[]string{"x", "--mcp", "goland"}, "cld: setup project: unexpected argument 'x' (see cld help)\n"},
		{[]string{"--"}, "cld: setup project: unexpected argument '--' (see cld help)\n"},
		{[]string{"--bogus"}, "cld: setup project: unexpected argument '--bogus' (see cld help)\n"},
		{[]string{"-n", "x"}, "cld: setup project: unexpected argument '-n' (see cld help)\n"},
	} {
		args := append([]string{"setup", "project"}, test.args...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			s := sandbox.New(t)
			if result := s.RunCld(nil, args...); result.Code != 2 || result.Stderr != test.want || result.Stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q, want exit 2, stderr %q", result.Code, result.Stdout, result.Stderr, test.want)
			}
			if entries, err := os.ReadDir(s.Work); err != nil || len(entries) != 0 {
				t.Errorf("the work directory holds %v (%v), want nothing", entries, err)
			}
		})
	}
}
