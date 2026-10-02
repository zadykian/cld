# 19. Project settings

Status: Accepted. Amended by [28](0028-shared-project-settings.md), which writes only what a project
shares.

## Context

Setting claude up in a project took copying this repository's files by hand. The facts below are in
[claude's findings](../findings/claude.md) and
[the environment's findings](../findings/environment.md).

- claude asks before it starts a project's MCP server that `enabledMcpjsonServers` does not name
  (claude 2.1.283).
- A JetBrains IDE's MCP server listens on 64342 plus an offset per product: 64422 for GoLand and
  64482 for Rider in 2026.2.
- claude expands `${VAR:-DEFAULT}` in a server's URL from its environment or the user's settings,
  not from the project's local settings (claude 2.1.283).
- git does not look into a directory it ignores (git 2.53.0 and 2.47.3).

## Decision

`cld setup project` writes claude's project settings, an empty local settings file and lines in
`.gitignore`, as cld's own repository has them. `--mcp` adds MCP servers. What the settings and the
lines hold is [decision 28](0028-shared-project-settings.md)'s.

### 19.1 This repository's settings

`--mcp goland --permissions cld` writes this repository's `.claude/settings.json` and `.mcp.json`
byte for byte, which the tests check. So the files and the code change together. The settings are Go
data rather than an embedded copy, so each server's entries go where the file has goland's.

### 19.2 The MCP servers

`--mcp` takes `goland`, `jbcontext` or `rider`, given again or with commas; anything else is a usage
error. Each server is written once, in the order goland, jbcontext, rider, whatever the order given.
A server is its entry in `.mcp.json`, its name in `enabledMcpjsonServers`, and what `--permissions`
allows of it ([decision 28.5](0028-shared-project-settings.md)). Since `.mcp.json` is shared and
each IDE may listen elsewhere, its port is a variable with a default, as in
`${GOLAND_MCP_PORT:-64422}`.

Not taken: ports written out, which fit only whoever ran cld. An option for them would write one
developer's port for all. Servers in the local scope would leave each developer to set them up. Nor
servers as arguments (`cld setup project goland`), which cld's other commands take as options.

### 19.3 The project

The project is the current directory, where `cld join` starts claude. It need not be a git
repository. In a subdirectory of one the `.gitignore` is that directory's, its patterns anchored
there.

### 19.4 Files edited in place

cld adds what a file lacks ([decision 28.3](0028-shared-project-settings.md)), and replaces a
server's entry in `.mcp.json` that differs, whole: a merge would keep stale fields. Everything else
stays byte for byte, and nothing is removed. The local settings file is only created. Running setup
again changes nothing.

### 19.5 `.gitignore`

cld adds its lines at the end, where git does not read them already. They were `/.claude/*` and
`!/.claude/settings.json`, since an ignored `.claude/` would have hidden the settings.
[Decision 28.1](0028-shared-project-settings.md) replaced them.

### 19.6 The order of the steps

Every file is read and edited before any is written, so a file cld cannot parse changes nothing. A
write that fails names the files written before it.

### 19.7 Asking git

In a work tree cld then asks git whether a pattern cld did not write still ignores
`.claude/settings.json`. It names the pattern and exits with status 1, leaving the pattern alone.
[Decision 28.2](0028-shared-project-settings.md) also warns of the other shared files git ignores.

### 19.8 No tmux or claude

The command runs neither, so the checks of [decision 6](0006-versions.md) do not apply, and it works
on macOS. `setup` takes it as it takes `telemetry` ([decision 18.8](0018-telemetry.md)). Completion
offers the servers, each described by its URL or command.

## Consequences

- An IDE on another port costs its developer a variable, not a change to a shared file.
- Out of scope: removing what cld wrote, other servers, finding an IDE's port, and settings per kind
  of project.
