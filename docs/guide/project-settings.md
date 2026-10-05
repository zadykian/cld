# claude's settings

`cld setup config project` writes a project's settings, and `cld setup config user` your own.
Each adds what the settings lack, and keeps the values they have.

## A project's

- A person's settings, such as a theme, belong in your own (see below): a project's would override
  everyone's.
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

## Your own: claude's user settings

- `user` writes `~/.claude/settings.json`, or `settings.json` in `$CLAUDE_CONFIG_DIR` where that
  is set. Every claude reads it, in cld or not, in every project, and a project's settings rank
  above it.
- `--permissions` takes the sets above, which then hold in every project. There `cld` lets claude
  edit files and run `git` anywhere, short of a merge or a push to `main`.
- `--notifications` names your terminal's channel, as [notifications](terminal.md#notifications)
  lists them. claude reads a channel it does not know as unset, so cld refuses one.
- A value you have stays, the channel's too: to take cld's, remove the key and run `user` again.
  The preferences, such as `model` `opus` and `theme` `dark`, are the author's: change them in the
  file.
- `user` leaves `env` alone: claude's telemetry is yours to set, as
  [Claude Code's docs](https://code.claude.com/docs/en/monitoring-usage) say. It writes nothing else
  of claude's, such as `~/.claude.json`, `CLAUDE.md` or hooks.
