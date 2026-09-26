// Package project is cld setup project: it sets Claude Code up in the project in the current
// directory, writing what claude reads there and what git needs to share it:
//
//   - .claude/settings.json, the settings the project shares through git: cld's permissions and
//     settings, those of this repository's own .claude/settings.json (see allow and
//     scalarSettings), and what each MCP server given adds (see Server). Where the file exists,
//     cld sets the keys it has - the last of a key given twice, which claude reads - adds the
//     entries that permissions.allow and enabledMcpjsonServers lack, and keeps everything else;
//   - .claude/settings.local.json, the settings of whoever works on the project, which git
//     ignores: made holding its $schema alone, and left as it is once anything is there, a
//     symbolic link that leads nowhere included, since they are someone's own;
//   - .mcp.json, with MCP servers only: each one's entry under mcpServers, replaced whole where it
//     differs - merging a stdio server's args, or a server whose type changed, would break it -
//     and the other servers kept;
//   - .gitignore: /.claude/* and then !/.claude/settings.json, so that git ignores what claude
//     keeps in .claude - settings.local.json, plans, worktrees - but for the shared settings. A
//     line git reads the same way (see ignoreLines) counts as there; the exception counts only
//     after the last line that ignores .claude/*, where it wins.
//
// Setup reads every file first and refuses one it cannot edit - invalid JSON, or a permissions
// that is no object - before it writes any; it writes only a file that changes, as
// internal/configfile writes one. Then, in a git work tree, it asks git whether it ignores
// .claude/settings.json all the same: a pattern elsewhere can - .claude/ keeps git out of the
// directory, where no exception reaches - and so can git's own excludes.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
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
	// config is the server's entry in .mcp.json, and allow what the settings allow of it.
	config string
	allow  []string
}

// Servers are the MCP servers setup project takes, in the order it writes them: the servers
// built into GoLand and Rider (2025.2 and newer, Settings | Tools | MCP Server), over streamable
// HTTP at the ports they have in the maintainer's projects - an IDE takes the first free port from
// 64342, so these are that setup's, not defaults - and JetBrains Context's, which its CLI serves
// over stdio. jbcontext is allowed as a command too: its hooks and instructions have claude run
// jbcontext search.
var Servers = []Server{
	{"goland", "GoLand's MCP server, http://127.0.0.1:64422/stream",
		`{"type": "http", "url": "http://127.0.0.1:64422/stream"}`, []string{"mcp__goland"}},
	{"jbcontext", "JetBrains Context's semantic code search, jbcontext mcp",
		`{"type": "stdio", "command": "jbcontext", "args": ["mcp"]}`, []string{"Bash(jbcontext:*)", "mcp__jbcontext"}},
	{"rider", "Rider's MCP server, http://127.0.0.1:64482/stream",
		`{"type": "http", "url": "http://127.0.0.1:64482/stream"}`, []string{"mcp__rider"}},
}

// allow is what the settings let claude do without asking, as this repository's own
// .claude/settings.json has it, before what the MCP servers add.
var allow = []string{
	"Read",
	"Edit",
	"Write",
	"WebSearch",
	"WebFetch",
	"Bash(ls:*)",
	"Bash(dir:*)",
	"Bash(pwd:*)",
	"Bash(find:*)",
	"Bash(grep:*)",
	"Bash(grep -E:*)",
	"Bash(rg:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(tail:*)",
	"Bash(wc:*)",
	"Bash(sort:*)",
	"Bash(uniq:*)",
	"Bash(printf:*)",
	"Bash(echo:*)",
	"Bash(tree:*)",
	"Bash(stat:*)",
	"Bash(du:*)",
	"Bash(which:*)",
	"Bash(where:*)",
	"Bash(mkdir:*)",
	"Bash(touch:*)",
	"Bash(chmod:*)",
	"Bash(sed:*)",
	"Bash(nl -ba)",
	"Bash(git:*)",
	"Bash(dotnet:*)",
	"Bash(jq:*)",
	"Bash(go:*)",
	"Bash(gofmt:*)",
	"Bash(make:*)",
	"Bash(shellcheck:*)",
	"Bash(shfmt:*)",
	"Bash(docker build:*)",
	"Bash(docker run:*)",
	"Bash(docker images:*)",
	"Bash(gh issue view:*)",
	"Bash(gh issue list:*)",
	"Bash(gh pr create:*)",
	"Bash(gh pr view:*)",
	"Bash(gh pr list:*)",
	"Bash(gh pr checks:*)",
	"Bash(gh pr diff:*)",
	"Bash(gh pr merge:*)",
	"Bash(gh run list:*)",
	"Bash(gh run view:*)",
	"Workflow(code-review)",
}

// scalarSettings are the settings' other keys, after permissions, with their values as JSON, as
// this repository's own .claude/settings.json has them.
var scalarSettings = []configfile.Member{
	{Key: "autoUpdatesChannel", Value: json.RawMessage(`"latest"`)},
	{Key: "plansDirectory", Value: json.RawMessage(`".claude/plans"`)},
	{Key: "autoMemoryEnabled", Value: json.RawMessage(`true`)},
	{Key: "theme", Value: json.RawMessage(`"dark"`)},
	{Key: "autoCompactEnabled", Value: json.RawMessage(`true`)},
}

// ignoreLines are the lines .gitignore needs, in order, each with the other way of writing it
// that git reads the same: a pattern with a slash before its end is anchored to the directory of
// the .gitignore, with or without a leading slash.
var ignoreLines = [][]string{{"/.claude/*", ".claude/*"}, {"!/.claude/settings.json", "!.claude/settings.json"}}

// change is one file setup project writes: created, changed in what, or neither and left as it
// was; write writes it.
type change struct {
	file    string
	created bool
	what    []string
	write   func() error
}

// Setup sets Claude Code up in the project in the current directory, with the MCP servers
// servers, in the order the package comment gives, and reports what it changed.
func Setup(servers []Server) error {
	var changes []change

	settings, err := configfile.ReadJSON(settingsFile)
	if err != nil {
		return err
	}
	what, err := editSettings(settings, servers)
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
	if err := fail.Print(report(changes)); err != nil {
		return err
	}
	if pattern, source, line := ignoredBy(); pattern != "" {
		return fail.Runtime("git ignores " + settingsFile + " all the same, by the pattern " + pattern + " (" + source + ", line " + line + ")")
	}
	return nil
}

// editSettings edits the settings file holds, with the MCP servers servers (see the package
// comment), and returns the keys it changed, a nested one as permissions.allow.
func editSettings(file *configfile.JSON, servers []Server) ([]string, error) {
	top := &object{file: file, members: file.Members}
	top.set("$schema", configfile.String(schema), true)
	permissions, err := top.object("permissions")
	if err != nil {
		return nil, err
	}
	entries := slices.Clone(allow)
	for _, s := range servers {
		entries = append(entries, s.allow...)
	}
	if err := permissions.add("allow", entries); err != nil {
		return nil, err
	}
	top.put("permissions", permissions)
	for _, m := range scalarSettings {
		top.set(m.Key, m.Value, false)
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

// add adds to the array at key the entries, strings, it lacks, at its end; an object without the
// key gets an array of them.
func (o *object) add(key string, entries []string) error {
	var elements []json.RawMessage
	if at := configfile.Last(o.members, key); at >= 0 {
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
	ignored, excepted := false, false
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), " ")
		switch {
		case slices.Contains(ignoreLines[0], line):
			ignored, excepted = true, false
		case ignored && slices.Contains(ignoreLines[1], line):
			excepted = true
		}
	}
	var added []string
	if !ignored {
		added = append(added, ignoreLines[0][0])
	}
	if !excepted {
		added = append(added, ignoreLines[1][0])
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

// ignoredBy is the pattern by which git ignores .claude/settings.json once .gitignore is written,
// and where it is, a file and a line, as git check-ignore names them; "" where git does not
// ignore it, or cannot tell: without git, outside a work tree. git names the last pattern that
// matches, which keeps the file where it starts with "!".
func ignoredBy() (pattern, source, line string) {
	path, err := tool.LookPath("git")
	if err != nil {
		return "", "", ""
	}
	cmd := exec.Command(path, "check-ignore", "--verbose", "-z", "--stdin")
	cmd.Args[0] = "git"
	cmd.Stdin = strings.NewReader(settingsFile + "\x00")
	out, err := cmd.Output()
	fields := strings.Split(string(out), "\x00")
	if err != nil || len(fields) < 4 || strings.HasPrefix(fields[2], "!") {
		return "", "", ""
	}
	return fields[2], fields[0], fields[1]
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
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		return pathError.Err.Error()
	}
	return err.Error()
}
