# 28. Project settings a team can share

Status: Accepted. Amended by [53](0053-project-and-user-settings.md).

## Context

`cld setup project` wrote this repository's own settings into any project: a theme, an update
channel and an allow list that ran commands without a prompt. Its `.gitignore` hid every file in
`.claude` but the settings, so a new skill showed in no `git status`. Now it writes what a project
shares, amending [decision 19](0019-project-settings.md).

It rests on claude's own excludes, its settings' defaults and its permission rules (claude
2.1.284; [claude findings](../findings/claude.md)). It rests too on git's ignore rules (git 2.53.0
and 2.47.3) and the tools MCP servers mark read-only
([environment findings](../findings/environment.md)).

## Decision

### 28.1 Three lines in `.gitignore`

`.gitignore` gets `/.claude/settings.local.json`, `/.claude/plans/` and `/.claude/worktrees/`, each
unless git already reads it there. A line counts with or without the leading slash, and with a
carriage return or trailing spaces, but not with a trailing tab. What is missing goes at the end, in
the file's line endings ([decision 19.5](0019-project-settings.md)). git then adds the rest of
`.claude`, which Claude Code's docs have a project commit. claude excludes its worktrees itself, but
cld writes the line anyway, rather than rest on when claude does.

### 28.2 Old lines stay, and cld warns

cld removes no line. In a work tree it asks git about each path a project shares
([decision 19.7](0019-project-settings.md)), and warns once for each pattern that ignores some. An
ignored `.claude/settings.json` still ends with status 1. A directory goes with its slash, to count
before it exists, and without the index, so files added with `git add -f` hide no pattern. The index
then tells a submodule, whose files are beyond the project's patterns. A file goes with the index,
since one git tracks is shared whatever pattern matches it. A symbolic link goes as a file, without
the slash, as git refuses a slash beyond a link.

Rejected: rewriting the old lines, which a project may have made its own, and checking every file.

### 28.3 The settings

cld writes the schema, `permissions.allow` and `permissions.deny` (28.4), `plansDirectory` and,
with `--mcp`, the servers enabled. The theme, the update channel and the memory and compaction
switches are gone: the first two are a person's, and all four restate claude's defaults. cld never
replaces a value ([decision 19.4](0019-project-settings.md)).

### 28.4 Permission sets

`--permissions` picks one of three sets. `read-only`, the default, has `Read` and `Bash` prefix
rules for commands such as `ls`, `cat` and `git status`, whose options neither run a command nor
write a file. `cld` is this repository's list, one developer's, which lets claude run `git`, `go`
and `make` without a prompt. Its `permissions.deny` refuses `gh pr merge` and pushes to `main`.
`none` is empty.

A prefix rule admits every option, so `git diff`, `git log` and `git show` stay out. Their
`--output` can write `.git/config`, whose `core.fsmonitor` the next `git status` runs: a claude that
a prompt injected could run anything. claude allows bare read-only commands anyway, so what matters
is that the set is safe to share, as the folder's trust grants what the file allows.

### 28.5 MCP servers by set

A server follows the set: `cld` allows every tool, `read-only` the tools GoLand 2026.2.3's server
marks read-only and `jbcontext`'s search, and `none` nothing. Rider's server, the same platform's,
gets the same list without having been probed.

### 28.6 This repository's files

This repository's `.claude/settings.json` and `.mcp.json` are byte for byte what
`cld setup project --mcp goland --permissions cld` writes, but for the settings' `hooks`
([decision 19.1](0019-project-settings.md)).

### 28.7 Tests

The tests cover each set on new and existing files, a `permissions.deny` with entries or no array,
the lines through `git status`, and the warnings ([testing](../testing.md)).

## Consequences

A project keeps the old lines and keys until it removes them. Out of scope: other MCP servers
([decision 19](0019-project-settings.md)), removing old lines or keys, and a project's own sets.
