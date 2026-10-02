package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/cmd/cld/internal/cmdline"
	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/project"
)

// projectLong is setup project's help.
const projectLong = `set claude up in the project in the current directory: .claude/settings.json
holds the settings the project shares through git - what claude may do without
asking, which --permissions sets, and where claude keeps its plans - and
.claude/settings.local.json, holding its $schema alone, is for your own.
.gitignore gets /.claude/settings.local.json, /.claude/plans/ and
/.claude/worktrees/: git ignores those, and adds what the project shares in
.claude, such as its commands, agents and skills. With --mcp, cld adds MCP
servers to .mcp.json, and the settings enable them and allow their tools as
--permissions says.

Where the files exist, cld adds what they lack and keeps everything else, the
values of the settings it would set included, but for a server's entry in
.mcp.json that differs from cld's, which it replaces. Then, in a git work tree,
it checks that git does not ignore the settings, and warns of the other files a
project shares under .claude that git ignores. Review the changes before you
commit them: whoever trusts the project's folder gives claude what they allow.`

// setupProject is cld setup project (decision 19), named in its messages as typed.
func setupProject(typed string) *cobra.Command {
	command := &cobra.Command{
		Use:   "project [--mcp SERVER] [--permissions SET]",
		Short: "set claude up in the project in the current directory",
		Long:  projectLong,
		Args:  cmdline.NoArguments(typed),
	}
	command.SetFlagErrorFunc(cmdline.FlagError(typed))
	flags := command.Flags()
	mcp := flags.StringArray("mcp", nil, "an MCP `SERVER` for claude in the project: goland or\n"+
		"rider, the IDE's own server on 127.0.0.1, at the port\n"+
		"in GOLAND_MCP_PORT or RIDER_MCP_PORT where claude\n"+
		"runs, else the IDE's default, 64422 or 64482; or\n"+
		"jbcontext, JetBrains Context's code search. Give --mcp\n"+
		"again, or separate them with commas")
	permissions := flags.String("permissions", "",
		"what claude may do in the project without asking:\n"+
			"`SET` is read-only, the default, to read files, run\n"+
			"commands that only read, such as git status and ls,\n"+
			"and use the servers' tools that only read; cld, as\n"+
			"cld's own repository has it, to edit files, run git,\n"+
			"go, make, docker and more, use every server tool, but\n"+
			"not merge or push to main; or none, to add nothing")
	command.RunE = func(c *cobra.Command, _ []string) error {
		servers, err := mcpServers(*mcp)
		if err != nil {
			return err
		}
		set, err := permissionSet(c.Flags().Changed("permissions"), *permissions)
		if err != nil {
			return err
		}
		return project.Setup(servers, set)
	}
	cmdline.CompleteWith(command, "mcp", serverNames)
	cmdline.CompleteWith(command, "permissions", permissionSets)
	return command
}

// mcpServers is the servers --mcp names, given as lists, each option again or separated by
// commas: each once, in project.Servers' order, whatever the order given.
func mcpServers(lists []string) ([]project.Server, error) {
	chosen := map[string]bool{}
	for _, list := range lists {
		for name := range strings.SplitSeq(list, ",") {
			known := func(s project.Server) bool { return s.Name == name }
			if !slices.ContainsFunc(project.Servers, known) {
				return nil, fail.Usage(fmt.Sprintf("invalid MCP server '%s' for --mcp: goland, "+
					"jbcontext or rider (see cld help)", name))
			}
			chosen[name] = true
		}
	}
	var servers []project.Server
	for _, s := range project.Servers {
		if chosen[s.Name] {
			servers = append(servers, s)
		}
	}
	return servers, nil
}

// permissionSet is the set --permissions names, where given, and else the first of
// project.PermissionSets.
func permissionSet(given bool, name string) (project.Permissions, error) {
	if !given {
		return project.PermissionSets[0], nil
	}
	at := slices.IndexFunc(project.PermissionSets,
		func(p project.Permissions) bool { return p.Name == name })
	if at < 0 {
		return project.Permissions{}, fail.Usage(fmt.Sprintf("invalid permissions '%s' for "+
			"--permissions: read-only, cld or none (see cld help)", name))
	}
	return project.PermissionSets[at], nil
}

// serverNames completes the SERVER of setup project --mcp: the servers it takes that start with
// what was typed, each described. After a comma it completes the last of the list, offering the
// servers the list does not have yet after the ones it has.
func serverNames(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion,
	cobra.ShellCompDirective) {
	before, last := "", typed
	if comma := strings.LastIndexByte(typed, ','); comma >= 0 {
		before, last = typed[:comma+1], typed[comma+1:]
	}
	var names []cobra.Completion
	for _, s := range project.Servers {
		if strings.HasPrefix(s.Name, last) && !slices.Contains(strings.Split(before, ","), s.Name) {
			names = append(names, cobra.CompletionWithDesc(before+s.Name, s.Description))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// permissionSets completes the SET of setup project --permissions: the sets it takes that start
// with what was typed, each described.
func permissionSets(_ *cobra.Command, _ []string, typed string) ([]cobra.Completion,
	cobra.ShellCompDirective) {
	var names []cobra.Completion
	for _, p := range project.PermissionSets {
		if strings.HasPrefix(p.Name, typed) {
			names = append(names, cobra.CompletionWithDesc(p.Name, p.Description))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
