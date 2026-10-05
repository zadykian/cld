package project

// Server is an MCP server that setup config project configures.
type Server struct {
	// Name is the server's name in .mcp.json and in the settings.
	Name string
	// Description is what completion shows for the name.
	Description string
	// config is the server's entry in .mcp.json. all is what --permissions cld allows of it,
	// every tool, and read what read-only allows, the tools that only read.
	config    string
	all, read []string
}

// Servers are the MCP servers setup config project takes, in the order it writes them. GoLand's and
// Rider's, under Settings | Tools | MCP Server, go over streamable HTTP, and JetBrains Context's
// over stdio. An IDE's port is a variable with a default, since each developer's IDE may listen
// on another (decision 19.2).
var Servers = []Server{
	{"goland", "GoLand's MCP server, port $GOLAND_MCP_PORT or 64422",
		`{"type": "http", "url": "http://127.0.0.1:${GOLAND_MCP_PORT:-64422}/stream"}`,
		[]string{"mcp__goland"}, ideTools("goland")},
	// jbcontext is allowed as a command too: its hooks and instructions have claude run
	// jbcontext search.
	{"jbcontext", "JetBrains Context's semantic code search, jbcontext mcp",
		`{"type": "stdio", "command": "jbcontext", "args": ["mcp"]}`,
		[]string{"Bash(jbcontext:*)", "mcp__jbcontext"},
		[]string{"Bash(jbcontext search:*)", "mcp__jbcontext__code_search"}},
	{"rider", "Rider's MCP server, port $RIDER_MCP_PORT or 64482",
		`{"type": "http", "url": "http://127.0.0.1:${RIDER_MCP_PORT:-64482}/stream"}`,
		[]string{"mcp__rider"}, ideTools("rider")},
}

// ideTools are the allow entries for the tools of the IDE's MCP server name that only read. They
// are those GoLand 2026.2.3 marks readOnlyHint, which Rider's server was taken to share
// (decision 28.5). An entry for a tool that a server lacks allows nothing.
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
