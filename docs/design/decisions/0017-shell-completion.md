# 17. Shell completion

Status: Accepted (#25). Amended by [18](0018-telemetry.md), [19](0019-project-settings.md),
[22](0022-setting-completion-up.md), [24](0024-names-from-the-repository.md),
[27](0027-completion-with-blesh.md), [28](0028-shared-project-settings.md),
[37](0037-sessions-of-another-repository.md), [40](0040-session-record.md),
[44](0044-detach-command.md), [49](0049-status-in-the-list.md), [50](0050-one-command-join.md),
[52](0052-telemetry-outside-cld.md) and [53](0053-project-and-user-settings.md).

## Context

Joining a session took typing its name. cobra 1.10.2 generates completion scripts for bash, zsh,
fish and PowerShell, which ask `cld __complete` on every TAB. cobra takes `-nNAME` for an option
being typed. bash 3.2 lacks `compopt`, so it offers file names wherever cld offers none
([the environment's findings](../findings/environment.md)).

## Decision

`cld completion SHELL` prints cobra's script for bash, zsh or fish. With it `join -n` and `-s` offer
the NAMEs and SUFFIXes that `cld list` shows ([decision 24.8](0024-names-from-the-repository.md)).

### 17.1 cobra's scripts

The scripts know nothing of cld's commands, and need no copy per shell to keep in step. `__complete`
is tested from Go, without a shell.

### 17.2 The sessions `join` offers

`join` offers the sessions `list` shows that `join` takes
([decision 37.5](0037-sessions-of-another-repository.md)), read as `list` reads them
([decision 13.1](0013-a-server-per-session.md)). Each is described by its state, which says what
`join` will do. Ended ones count since [decision 50.7](0050-one-command-join.md). Completion keeps
the names that start with what was typed, so every shell offers the same.

### 17.3 What else completes

`help` offers its commands, `setup project --mcp` its servers
([decision 19.8](0019-project-settings.md)), and `--permissions` its sets
([decision 28](0028-shared-project-settings.md)). `detach -n` and `-s` offer the sessions that run
([decision 44](0044-detach-command.md)). `kill -n` offers none, as the request named `join` only.
`join --resume` offers nothing, since that would take reading claude's transcripts
([decision 16.4](0016-resume.md)). No argument offers file names, not even `--collector-config`'s
`FILE` ([decision 18.9](0018-telemetry.md)). bash with ble.sh is
[decision 27](0027-completion-with-blesh.md).

### 17.4 No startup checks

The checks of [decision 6](0006-versions.md) live in the commands, not in a root hook. So
`cld completion` works without tmux or claude, and a TAB costs no `tmux -V`. It costs a read of the
socket directory and one `list-sessions` a server ([decision 38](0038-stale-sockets.md)). For four
sessions in the test image that took about 22 ms. Completion never runs claude, starts no server and
never opens the list. Where anything fails it offers nothing and exits 0.

### 17.5 Arguments and help

The completion commands read their arguments as cld's other commands do, with status 2 for a
mistake. With cobra's own handling, `cld completion tcsh > FILE` would fill FILE with the help and
succeed. Their help is cobra's; their `Short`s are cld's ([decision 12](0012-help-from-cobra.md)).

### 17.6 Output through cld

What cobra prints goes through cld's output, so a write that fails ends cld with status 1, where
cobra exits 0.

### 17.7 Where the scripts ship

Only through `cld completion SHELL`, not as release assets. `cld setup completion` writes a script
where the shell reads it ([decision 22](0022-setting-completion-up.md)).

### 17.8 `-V` and `--version`

They are not offered: as the root's options, they would show in the root's help beside `version`.

## Consequences

- Out of scope: a positional `cld join NAME`, claude's conversation names, and hints under the
  prompt (cobra's ActiveHelp).
