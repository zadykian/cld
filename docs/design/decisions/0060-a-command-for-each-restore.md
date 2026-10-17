# 60. A command for each restore

Status: Accepted (#185).

## Context

`cld restore` brought back the sessions a reboot ended, on the host that ran them
([decision 48](0048-restore-after-reboot.md)). [Decision 59](0059-ghostty-tabs.md) gave it a
command, `tabs`, which reopens a Mac's Ghostty tabs, and left plain `restore` unchanged (59.1). One
word then named two restores, each on its own machine. `help restore` was the sessions' help, with
`tabs` beside it, and completion offered `tabs` alone after `restore`. `setup`, by contrast, runs
nothing itself, and names each of its commands ([decision 52.3](0052-telemetry-outside-cld.md)).

This amends 48.5, 48.9 and 48.11, and 59.1.

## Decision

`cld restore sessions` brings back the sessions, as `cld restore` did. `restore` runs nothing itself:
its commands are `sessions` and `tabs`.

### 60.1 `restore` takes a command

cld checks the word after `restore` before cobra: `sessions`, `tabs`, `-h` or `--help`. Without a
word, or with an empty one, `restore` fails as a missing command with status 2, as `setup` does.
Another option is an unexpected argument and another word an unknown command, as under 59.1. The
help of `restore` lists its two commands, and has no usage line of its own.

Not taken:

- Plain `restore` as a hidden name for `sessions`, which keeps two names for one command for good.
- A release in which plain `restore` warns first, as decision [52.1](0052-telemetry-outside-cld.md)
  did not take one either.

### 60.2 The unit

`setup restore` writes `ExecStart=CLD restore sessions`. The command keeps its name, as does the
unit, `cld-restore.service`: a new name would leave the old unit enabled beside the new one.

A unit an older cld wrote runs plain `restore`, which fails with status 2 as the user's systemd
starts. `systemctl --user status cld-restore` and the journal show cld's message. The sessions keep
their run marks ([48.1](0048-restore-after-reboot.md)), so `cld restore sessions` by hand still
brings them back. `setup restore` writes the unit anew, as it does wherever the unit differs.

### 60.3 Help and completion

`restore sessions` takes the help that `restore` had, and no argument. Completion offers `sessions`
and `tabs` after `restore` and after `help restore`. This amends 48.11 and 59.1.

## Consequences

- Users type `cld restore sessions`. On Linux, they run `cld setup restore` once after the upgrade,
  as the [guide](../../guide/install-and-upgrade.md)'s upgrade notes say.
- A script that ran `cld restore` fails with status 2 and the message naming both commands.
- The playbook of `zadykian/ws` runs `setup restore` at every run, so the unit there follows cld.
  Its dry run compares the unit's `ExecStart` with the command the installed cld takes.
- `cli_restore_test.go` pins plain `restore` as a missing command. The `restore_*` tests run
  `restore sessions`, and `restore_setup_test.go` checks the unit's `ExecStart`.
