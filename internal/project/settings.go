package project

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"slices"

	"github.com/zadykian/cld/internal/configfile"
	"github.com/zadykian/cld/internal/fail"
)

// scalarSettings are the settings' keys after permissions, with their values as JSON, as this
// repository's own .claude/settings.json has them. They say where claude keeps its plans, which
// .gitignore keeps out of git.
var scalarSettings = []configfile.Member{
	{Key: "plansDirectory", Value: json.RawMessage(`".claude/plans"`)},
}

// settingsChange reads .claude/settings.json and edits it for the servers and permission set.
func settingsChange(servers []Server, permissions Permissions) (change, error) {
	settings, err := configfile.ReadJSON(settingsFile)
	if err != nil {
		return change{}, err
	}
	what, err := editSettings(settings, servers, permissions)
	if err != nil {
		return change{}, err
	}
	return change{settingsFile, !settings.Exists, what, settings.Write}, nil
}

// localChange creates .claude/settings.local.json holding its $schema alone where nothing is
// there. Anything there, a symbolic link that leads nowhere included, is someone's own, and stays.
func localChange() (change, error) {
	_, err := os.Lstat(localFile)
	if err == nil {
		return change{file: localFile}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return change{}, fail.Runtime("cannot read " + localFile + ": " + reason(err))
	}
	file, err := configfile.ReadJSON(localFile)
	if err != nil {
		return change{}, err
	}
	file.Members = []configfile.Member{{Key: "$schema", Value: configfile.String(schema)}}
	return change{localFile, true, []string{"$schema"}, file.Write}, nil
}

// mcpChange reads .mcp.json and gives each of the servers cld's entry.
func mcpChange(servers []Server) (change, error) {
	mcp, err := configfile.ReadJSON(mcpFile)
	if err != nil {
		return change{}, err
	}
	what, err := editMCP(mcp, servers)
	if err != nil {
		return change{}, err
	}
	return change{mcpFile, !mcp.Exists, what, mcp.Write}, nil
}

// editSettings adds to the settings that file holds the keys and entries they lack for the
// servers and permission set (decision 28.3). It returns the keys it changed, a nested one as
// permissions.allow.
func editSettings(
	file *configfile.JSON, servers []Server, permissions Permissions,
) ([]string, error) {
	top := &object{file: file, members: file.Members}
	top.setMissing("$schema", configfile.String(schema), true)
	if err := addPermissions(top, servers, permissions); err != nil {
		return nil, err
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

// addPermissions adds the set's entries, and what it allows of each server, to permissions.allow
// and permissions.deny in top, the settings, where it has any.
func addPermissions(top *object, servers []Server, permissions Permissions) error {
	entries := slices.Clone(permissions.allow)
	for _, s := range servers {
		entries = append(entries, permissions.server(s)...)
	}
	if len(entries) == 0 && len(permissions.deny) == 0 {
		return nil
	}
	nested, err := top.object("permissions")
	if err != nil {
		return err
	}
	if err := nested.add("allow", entries); err != nil {
		return err
	}
	if err := nested.add("deny", permissions.deny); err != nil {
		return err
	}
	top.put("permissions", nested)
	return nil
}

// editMCP gives each of the servers cld's entry under mcpServers in the .mcp.json that file holds,
// keeping the others, and returns the keys it changed.
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
