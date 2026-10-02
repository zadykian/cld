// Package project is cld setup project: it sets Claude Code up in the project in the current
// directory, writing what claude reads there and what git needs to share it:
//
//   - .claude/settings.json, the settings the project shares through git: $schema, the permission
//     rules of the set given (see PermissionSets) and of each MCP server given (see Server),
//     plansDirectory, and the servers enabled. The file ranks above each developer's own
//     ~/.claude/settings.json, so it holds no setting of a person's, such as a theme. Where the
//     file exists, cld adds the keys it lacks and never replaces a value it has, adds the entries
//     that permissions.allow, permissions.deny and enabledMcpjsonServers lack - to the last of a
//     key given twice, which claude reads - and keeps everything else;
//   - .claude/settings.local.json, the settings of whoever works on the project, which git
//     ignores: made holding its $schema alone, and left as it is once anything is there, a
//     symbolic link that leads nowhere included, since they are someone's own;
//   - .mcp.json, with MCP servers only: each one's entry under mcpServers, replaced whole where it
//     differs - merging a stdio server's args, or a server whose type changed, would break it -
//     and the other servers kept;
//   - .gitignore: /.claude/settings.local.json, /.claude/plans/ and /.claude/worktrees/, so that
//     git ignores what claude keeps in .claude for one developer, and adds the rest - commands,
//     agents, skills, rules, hooks, CLAUDE.md - for the project to share. A line git reads the
//     same way (see ignoreLines) counts as there. claude keeps its other files in .claude out of
//     git itself, with lines of its own in .git/info/exclude.
//
// Setup reads every file first and refuses one it cannot edit - invalid JSON, or a permissions
// that is no object - before it writes any; it writes only a file that changes, as
// internal/configfile writes one. Then, in a git work tree, it asks git whether it ignores the
// files under .claude that a project shares (see shared) all the same: a pattern elsewhere can -
// .claude/ keeps git out of the directory, and /.claude/*, which cld wrote before, out of all but
// the exceptions after it - and so can git's own excludes. It warns of each path git ignores,
// and fails where git ignores .claude/settings.json, the file it writes to be shared.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/output"
	"github.com/zadykian/cld/internal/tool"
)

const (
	settingsFile = ".claude/settings.json"
	localFile    = ".claude/settings.local.json"
	mcpFile      = ".mcp.json"
	ignoreFile   = ".gitignore"
	// schema is the JSON schema of claude's settings, which editors read to check the files.
	schema = "https://json.schemastore.org/claude-code-settings.json"
)

// Server is an MCP server that setup project configures.
type Server struct {
	// Name is the server's name in .mcp.json and in the settings.
	Name string
	// Description is what completion shows for the name.
	Description string
	// config is the server's entry in .mcp.json; all is what the settings allow of it with
	// --permissions cld, every tool, and read what they allow with read-only, the tools that only
	// read.
	config    string
	all, read []string
}

// Servers are the MCP servers setup project takes, in the order it writes them: the servers
// built into GoLand and Rider (Settings | Tools | MCP Server), over streamable HTTP, and JetBrains
// Context's, which its CLI serves over stdio. jbcontext is allowed as a command too: its hooks and
// instructions have claude run jbcontext search.
//
// .mcp.json is shared through git, and each developer's IDE listens on a port of its own: 64342
// plus an offset per product by default - GoLand's 80, Rider's 140 in 2026.2 - or the one its
// MCP Server settings give. So an IDE's port is a variable that claude expands as it reads the
// file, GOLAND_MCP_PORT or RIDER_MCP_PORT, with the default after it for whoever sets none.
var Servers = []Server{
	{"goland", "GoLand's MCP server, port $GOLAND_MCP_PORT or 64422",
		`{"type": "http", "url": "http://127.0.0.1:${GOLAND_MCP_PORT:-64422}/stream"}`,
		[]string{"mcp__goland"}, ideTools("goland")},
	{"jbcontext", "JetBrains Context's semantic code search, jbcontext mcp",
		`{"type": "stdio", "command": "jbcontext", "args": ["mcp"]}`,
		[]string{"Bash(jbcontext:*)", "mcp__jbcontext"}, []string{"Bash(jbcontext search:*)", "mcp__jbcontext__code_search"}},
	{"rider", "Rider's MCP server, port $RIDER_MCP_PORT or 64482",
		`{"type": "http", "url": "http://127.0.0.1:${RIDER_MCP_PORT:-64482}/stream"}`,
		[]string{"mcp__rider"}, ideTools("rider")},
}

// ideTools are the tools of the IDE's MCP server name that only read, as allow entries: those
// GoLand 2026.2.3's server marks readOnlyHint, which Rider's, the same platform's, was taken to
// share. An entry for a tool that a server lacks allows nothing.
func ideTools(name string) []string {
	var entries []string
	for _, tool := range []string{
		"analyze_calls",
		"get_all_open_file_paths",
		"get_file_problems",
		"get_project_dependencies",
		"get_project_modules",
		"get_repositories",
		"get_run_configurations",
		"get_symbol_info",
		"git_status",
		"lint_files",
		"list_directory_tree",
		"read_file",
		"search_file",
		"search_regex",
		"search_symbol",
		"search_text",
	} {
		entries = append(entries, "mcp__"+name+"__"+tool)
	}
	return entries
}

// scalarSettings are the settings' other keys, after permissions, with their values as JSON, as
// this repository's own .claude/settings.json has them: where claude keeps its plans, which
// .gitignore keeps out of git.
var scalarSettings = []configfile.Member{
	{Key: "plansDirectory", Value: json.RawMessage(`".claude/plans"`)},
}

// ignoreLines are the lines .gitignore needs, each with the other way of writing it that git
// reads the same: a pattern with a slash before its end is anchored to the directory of the
// .gitignore, with or without a leading slash.
var ignoreLines = [][]string{
	{"/.claude/settings.local.json", ".claude/settings.local.json"},
	{"/.claude/plans/", ".claude/plans/"},
	{"/.claude/worktrees/", ".claude/worktrees/"},
}

// shared are the paths under .claude that Claude Code's docs have a project share through git,
// .claude/settings.json last. A directory has its slash, so that git takes it for one before it
// exists (see ignored).
var shared = []string{
	".claude/commands/",
	".claude/agents/",
	".claude/skills/",
	".claude/rules/",
	".claude/hooks/",
	".claude/CLAUDE.md",
	settingsFile,
}

// change is one file setup project writes: created, changed in what, or neither and left as it
// was; write writes it.
type change struct {
	file    string
	created bool
	what    []string
	write   func() error
}

// Setup sets Claude Code up in the project in the current directory, with the MCP servers
// servers and the permissions permissions, in the order the package comment gives, and reports
// what it changed.
func Setup(servers []Server, permissions Permissions) error {
	var changes []change

	settings, err := configfile.ReadJSON(settingsFile)
	if err != nil {
		return err
	}
	what, err := editSettings(settings, servers, permissions)
	if err != nil {
		return err
	}
	changes = append(changes, change{settingsFile, !settings.Exists, what, settings.Write})

	local := change{file: localFile}
	if _, err := os.Lstat(localFile); errors.Is(err, fs.ErrNotExist) {
		file, err := configfile.ReadJSON(localFile)
		if err != nil {
			return err
		}
		file.Members = []configfile.Member{{Key: "$schema", Value: configfile.String(schema)}}
		local.created, local.what, local.write = true, []string{"$schema"}, file.Write
	} else if err != nil {
		return fail.Runtime("cannot read " + localFile + ": " + reason(err))
	}
	changes = append(changes, local)

	if len(servers) > 0 {
		mcp, err := configfile.ReadJSON(mcpFile)
		if err != nil {
			return err
		}
		what, err := editMCP(mcp, servers)
		if err != nil {
			return err
		}
		changes = append(changes, change{mcpFile, !mcp.Exists, what, mcp.Write})
	}

	ignore, err := configfile.Read(ignoreFile)
	if err != nil {
		return err
	}
	data, added := editIgnore(ignore.Data)
	changes = append(changes, change{ignoreFile, !ignore.Exists, added, func() error { return ignore.Write(data) }})

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
	if err := output.Print(report(changes)); err != nil {
		return err
	}
	return checkIgnored()
}

// checkIgnored warns of the paths of shared that git ignores, in one warning those it ignores by
// the same pattern, and fails where it ignores .claude/settings.json.
func checkIgnored() error {
	var patterns []string
	var paths [][]string
	settings := ""
	for _, m := range ignored() {
		switch at := slices.Index(patterns, m.by); {
		case m.path == settingsFile:
			settings = m.by
		case at >= 0:
			paths[at] = append(paths[at], m.path)
		default:
			patterns, paths = append(patterns, m.by), append(paths, []string{m.path})
		}
	}
	for at, pattern := range patterns {
		output.Warn("git ignores " + and(paths[at]) + ", which a project shares through git, by the pattern " + pattern)
	}
	if settings != "" {
		return fail.Runtime("git ignores " + settingsFile + " all the same, by the pattern " + settings)
	}
	return nil
}

// editSettings edits the settings file holds, with the MCP servers servers and the permissions
// permissions (see the package comment), and returns the keys it changed, a nested one as
// permissions.allow.
func editSettings(file *configfile.JSON, servers []Server, permissions Permissions) ([]string, error) {
	top := &object{file: file, members: file.Members}
	top.setMissing("$schema", configfile.String(schema), true)
	entries := slices.Clone(permissions.allow)
	for _, s := range servers {
		entries = append(entries, permissions.server(s)...)
	}
	if len(entries) > 0 || len(permissions.deny) > 0 {
		nested, err := top.object("permissions")
		if err != nil {
			return nil, err
		}
		if err := nested.add("allow", entries); err != nil {
			return nil, err
		}
		if err := nested.add("deny", permissions.deny); err != nil {
			return nil, err
		}
		top.put("permissions", nested)
	}
	for _, m := range scalarSettings {
		top.setMissing(m.Key, m.Value, false)
	}
	if len(servers) > 0 {
		var names []string
		for _, s := range servers {
			names = append(names, s.Name)
		}
		if err := top.add("enabledMcpjsonServers", names); err != nil {
			return nil, err
		}
	}
	file.Members = top.members
	return top.changed, nil
}

// editMCP edits the servers of .mcp.json that file holds: those in servers get cld's entry.
func editMCP(file *configfile.JSON, servers []Server) ([]string, error) {
	top := &object{file: file, members: file.Members}
	entries, err := top.object("mcpServers")
	if err != nil {
		return nil, err
	}
	for _, s := range servers {
		entries.set(s.Name, json.RawMessage(s.config), false)
	}
	top.put("mcpServers", entries)
	file.Members = top.members
	return top.changed, nil
}

// object is a JSON object of a file that setup project edits: its members, its key in the file,
// such as permissions ("" for the file's own), and its depth; and the keys changed in it so far.
type object struct {
	file    *configfile.JSON
	members []configfile.Member
	path    string
	depth   int
	changed []string
}

// name is key as a key of the file: permissions.allow for allow in permissions.
func (o *object) name(key string) string {
	if o.path == "" {
		return key
	}
	return o.path + "." + key
}

// put puts nested, the object at key that object gave, back at key, as the file holds it, where
// it changed.
func (o *object) put(key string, nested *object) {
	if len(nested.changed) == 0 {
		return
	}
	o.putValue(key, o.file.Object(nested.members, nested.depth), false)
	o.changed = append(o.changed, nested.changed...)
}

// putValue sets key to value, as the file holds it, in place of the last of the key, which
// claude reads, where the object has it, else as a new member at the end, or at the start with
// first.
func (o *object) putValue(key string, value json.RawMessage, first bool) {
	if at := configfile.Last(o.members, key); at >= 0 {
		o.members[at].Value = value
		return
	}
	member := configfile.Member{Key: key, Value: value}
	if first {
		o.members = slices.Insert(o.members, 0, member)
	} else {
		o.members = append(o.members, member)
	}
}

// set sets key to value, JSON of cld's, where the object lacks it or it differs.
func (o *object) set(key string, value json.RawMessage, first bool) {
	if at := configfile.Last(o.members, key); at >= 0 && configfile.Same(o.members[at].Value, value) {
		return
	}
	o.putValue(key, o.file.Value(value, o.depth+1), first)
	o.changed = append(o.changed, o.name(key))
}

// setMissing sets key to value, JSON of cld's, where the object lacks it: a value the object has
// is the project's, and stays.
func (o *object) setMissing(key string, value json.RawMessage, first bool) {
	if configfile.Last(o.members, key) < 0 {
		o.set(key, value, first)
	}
}

// add adds to the array at key the entries, strings, it lacks, at its end; an object without the
// key gets an array of them. Without entries it reads nothing.
func (o *object) add(key string, entries []string) error {
	var elements []json.RawMessage
	if at := configfile.Last(o.members, key); at >= 0 && len(entries) > 0 {
		var err error
		if elements, err = configfile.Array(o.members[at].Value); err != nil {
			return fail.Runtime(o.name(key) + " in " + o.file.Path + " is not a JSON array")
		}
	}
	added := false
	for _, entry := range entries {
		value := configfile.String(entry)
		if !slices.ContainsFunc(elements, func(e json.RawMessage) bool { return configfile.Same(e, value) }) {
			elements, added = append(elements, value), true
		}
	}
	if added {
		o.putValue(key, o.file.Array(elements, o.depth+1), false)
		o.changed = append(o.changed, o.name(key))
	}
	return nil
}

// object is the object at key, to be edited and then put back with put; an empty one where the
// object lacks the key.
func (o *object) object(key string) (*object, error) {
	nested := &object{file: o.file, path: o.name(key), depth: o.depth + 1}
	if at := configfile.Last(o.members, key); at >= 0 {
		members, err := configfile.Object(o.members[at].Value)
		if err != nil {
			return nil, fail.Runtime(nested.path + " in " + o.file.Path + " is not a JSON object")
		}
		nested.members = members
	}
	return nested, nil
}

// editIgnore is .gitignore, holding data, with the lines it lacks of ignoreLines added at its
// end, in its line endings, and those lines. git reads a line without a carriage return at its
// end, and without trailing spaces - but not tabs.
func editIgnore(data []byte) ([]byte, []string) {
	there := make([]bool, len(ignoreLines))
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), " ")
		for at, forms := range ignoreLines {
			there[at] = there[at] || slices.Contains(forms, line)
		}
	}
	var added []string
	for at, forms := range ignoreLines {
		if !there[at] {
			added = append(added, forms[0])
		}
	}
	if len(added) == 0 {
		return data, nil
	}
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	edited := slices.Clone(data)
	if len(edited) > 0 && !bytes.HasSuffix(edited, []byte("\n")) {
		edited = append(edited, newline...)
	}
	for _, line := range added {
		edited = append(edited, line+newline...)
	}
	return edited, added
}

// match is a path of shared that git ignores, as git was asked about it, and by what: the
// pattern and where it is, as git check-ignore names them, "PATTERN (FILE, line LINE)".
type match struct {
	path, by string
}

// ignored is what git ignores of shared once .gitignore is written, in shared's order; nothing
// where git cannot tell: without git, outside a work tree. A directory is asked about with its
// slash, so that git takes it for one before it exists, and with --no-index, so that git answers
// for it where the project added files in it all the same (git add -f): a new one there is still
// ignored. But not a submodule, which git's index holds as one entry, checked out or not: what
// is in it belongs to the submodule's own repository, which the project's patterns never reach.
// A path there as something else, such as a symbolic link to the directory, which git keeps as a
// link and with the slash refuses to look beyond, failing for every path, is asked about as the
// files are: without the slash, and with the index, since a file git tracks is shared whatever
// pattern matches it. For each path git names the last pattern that matches, which keeps the
// path where it starts with "!".
func ignored() []match {
	git, err := tool.LookPath("git")
	if err != nil {
		return nil
	}
	var asked, dirs, files []string
	for _, path := range shared {
		name, dir := strings.CutSuffix(path, "/")
		if info, err := os.Lstat(name); dir && (err != nil || info.IsDir()) {
			dirs = append(dirs, path)
		} else {
			path = name
			files = append(files, path)
		}
		asked = append(asked, path)
	}
	by := checkIgnore(git, dirs, "--no-index")
	for _, path := range submodules(git, slices.Collect(maps.Keys(by))) {
		delete(by, path)
	}
	maps.Copy(by, checkIgnore(git, files))
	var matches []match
	for _, path := range asked {
		if pattern, ok := by[path]; ok {
			matches = append(matches, match{path, pattern})
		}
	}
	return matches
}

// checkIgnore asks git, the program at the path git, which of paths it ignores, with options
// such as --no-index, and returns the pattern of each, as match has it, by path: none where git
// ignores none of them, or cannot tell.
func checkIgnore(git string, paths []string, options ...string) map[string]string {
	by := map[string]string{}
	if len(paths) == 0 {
		return by
	}
	cmd := exec.Command(git, append(append([]string{"check-ignore"}, options...), "--verbose", "-z", "--stdin")...)
	cmd.Args[0] = "git"
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	if err != nil {
		return by
	}
	fields := strings.Split(string(out), "\x00")
	for at := 0; at+4 <= len(fields); at += 4 {
		source, line, pattern, path := fields[at], fields[at+1], fields[at+2], fields[at+3]
		if !strings.HasPrefix(pattern, "!") {
			by[path] = pattern + " (" + source + ", line " + line + ")"
		}
	}
	return by
}

// submodules are those of dirs, directories as ignored asks about them, with their slash, that
// git's index holds as submodules: an entry of mode 160000 by the directory's name. git, the
// program at that path, is asked only where dirs has any, and so only where a pattern matches.
func submodules(git string, dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	var names []string
	for _, dir := range dirs {
		names = append(names, strings.TrimSuffix(dir, "/"))
	}
	cmd := exec.Command(git, append([]string{"ls-files", "--stage", "-z", "--"}, names...)...)
	cmd.Args[0] = "git"
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var found []string
	for entry := range strings.SplitSeq(string(out), "\x00") {
		// An entry is "MODE OBJECT STAGE\tPATH", PATH from the current directory, as names are.
		info, name, _ := strings.Cut(entry, "\t")
		if strings.HasPrefix(info, "160000 ") && slices.Contains(names, name) {
			found = append(found, name+"/")
		}
	}
	return found
}

// report is what Setup prints once it is done: a line for each file.
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
