# 53. A project's settings and the user's

Status: Accepted (#155).

## Context

`cld setup project` wrote a project's claude settings ([decision 19](0019-project-settings.md),
[decision 28](0028-shared-project-settings.md)). Nothing wrote the user's, which a new machine got
by hand. The author's playbook, `zadykian/ws`, had no command to run for them. They live in
`$CLAUDE_CONFIG_DIR/settings.json`, a person's own, which may hold what a project's leave out
([decision 28.3](0028-shared-project-settings.md)).

Claude Code 2.1.232 and 2.1.289 take the keys and values below in their settings. They read a
channel they do not know as unset, without a word ([claude findings](../findings/claude.md)).

This amends decisions [12](0012-help-from-cobra.md), [17](0017-shell-completion.md),
[19](0019-project-settings.md), [28](0028-shared-project-settings.md),
[29](0029-notifications.md) and [52](0052-telemetry-outside-cld.md).

## Decision

`cld setup config` holds the commands that write claude's settings: `project`, which is
`setup project` moved, and `user`, new, for claude's user settings.

### 53.1 `setup config`

`setup config project` takes the options `setup project` took, and does what it did. `setup project`
fails as an unknown command of `setup`, with status 2, as `new` and `resume` do
([decision 50.6](0050-one-command-join.md)).

The two write alike and share their permission sets, so they sit under one command. Their options
differ, so each is a command of its own. Not taken: one `setup config [--user]`, where `--mcp` would
mean nothing with `--user`. Nor a `setup user` beside `setup project`, which keeps the old name but
sets apart two commands that work alike.

The new command is `user`, claude's own name for the scope of these settings, as in
`claude mcp add --scope user` ([claude findings](../findings/claude.md)). Not taken: `global`,
which reads as the whole machine's, where the file is one user's.

### 53.2 The word after `setup config`

cld checks the argument after `setup config` before cobra, as after `setup`
([decision 52.3](0052-telemetry-outside-cld.md)). cobra would run `user` for
`setup config --permissions cld user`. The argument is `project`, `user`, `-h` or `--help`.
`help setup config user` shows the help of `user` ([decision 12.2](0012-help-from-cobra.md)).
Completion offers both commands, and the values of their options
([decision 17.3](0017-shell-completion.md)).

### 53.3 What `user` writes

`user` writes `$CLAUDE_CONFIG_DIR/settings.json`, by default `~/.claude/settings.json`, where
claude reads it. It writes each of these only where the file lacks it, in this order:

- `$schema`, first, as `project` writes it.
- `permissions.allow` and `permissions.deny` from `--permissions`, which takes the sets of
  [decision 28.4](0028-shared-project-settings.md), `read-only` by default. A set goes whole:
  `Bash(git:*)` of `cld` without its `deny` would allow a push to `main`.
- `model` `opus`, `effortLevel` `xhigh`, `theme` `dark`, `editorMode` `normal`,
  `autoCompactEnabled` `true` and `autoUpdatesChannel` `latest`. These are the author's choices,
  which a new machine then gets from one command.
- `preferredNotifChannel` from `--notifications` (53.4).

A value the file has stays, as in a project ([decision 19.4](0019-project-settings.md)). On a
machine set up by hand, `user` adds only what is missing. Not taken: an option per preference,
which a user changes in the file instead.

### 53.4 `--notifications`

`--notifications CHANNEL` writes `preferredNotifChannel`, with no default.
[Decision 29.1](0029-notifications.md) keeps a channel out of cld's `--settings`, which outrank the
user's. Here the user names the channel of their own terminal. cld takes claude's seven values and
refuses any other, which claude would drop without a word.

### 53.5 What `user` leaves alone

`user` leaves `env` alone, which holds claude's telemetry settings: the user or the machine's
setup writes them ([decision 52](0052-telemetry-outside-cld.md)). It does not touch
`~/.claude.json`, `CLAUDE.md`, agents, skills, hooks or plugins, which are claude's or the user's.

### 53.6 Writing the file

`user` writes as `project` and `setup restore` do, through `internal/configfile`. It reads the
file whole, and refuses one it cannot edit before it writes anything
([decision 19.6](0019-project-settings.md)). It writes only a change, and reports the file as
created, updated with the keys added, or unchanged.

Neither command runs tmux, claude or Docker, nor their checks, so both work on macOS. Completion
runs neither ([decision 17.4](0017-shell-completion.md)).

### 53.7 Breaking

`cld setup project` fails, and scripts move to `cld setup config project`. `setup telemetry` went
in the same release ([decision 52](0052-telemetry-outside-cld.md)), and the
[guide](../../guide/install-and-upgrade.md)'s upgrade notes name both.

### 53.8 Tests

The tests run `setup config user` against the sandbox's `HOME` and `CLAUDE_CONFIG_DIR`. They
cover no file, a file to edit, a symbolic link, each refusal, a failed write and each usage
mistake ([testing](../testing.md)). They pin `setup project` as an unknown command, and the help
and completion of `setup config`.

## Consequences

- A new machine gets the user's settings from one command, which the playbook of `zadykian/ws`
  runs after cld's `install.sh`.
- The hard rule on a scratch `HOME` covers `setup config user`, which writes the user's own
  settings.
- Out of scope: removing what cld wrote, `env`, and choosing other preferences.
