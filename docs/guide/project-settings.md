# Project settings

- A person's settings, such as a theme, belong in `~/.claude/settings.json`: the project's would
  override everyone's.
- `read-only` allows `Read`, and `ls`, `cat`, `grep`, `git status` and the like with any options,
  none of which runs a command or writes a file. It leaves out `git diff`, `git log` and
  `git show`, whose `--output` writes any file, `.git/config` too: claude still runs them with the
  options it knows to be safe.
- `cld` allows editing files, web access, and commands such as `git`, `make` and `docker run`:
  enough for a prompt-injected claude to run anything. It denies `gh pr merge` and pushes to
  `main`.
- With `--mcp`, `read-only` allows the IDE's tools that only read, as GoLand 2026.2.3 marks them.
  `cld` allows every tool, GoLand's `execute_terminal_command` among them.
- claude keeps its other files in `.claude` out of git itself, in `.git/info/exclude`.
- `.mcp.json` is shared, so each developer whose IDE port differs sets `GOLAND_MCP_PORT` or
  `RIDER_MCP_PORT`. Set it in your shell's profile or `~/.claude/settings.json`: claude 2.1.283 does
  not expand `.mcp.json` from the project's local settings.
- cld removes nothing, so what an earlier cld wrote stays (see [upgrading](install-and-upgrade.md)).
  A file it cannot edit, such as one of invalid JSON, stops it with nothing changed.
- cld fails where git ignores `.claude/settings.json`, naming the pattern, and warns of the other
  shared files git ignores. A directory counts even with files added by `git add -f`, as git
  ignores a new one there.
