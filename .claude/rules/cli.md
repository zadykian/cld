---
paths:
  - "cmd/cld/**"
  - "internal/picker/**"
---

# cmd/cld and internal/picker

The invariants a change to the command line keeps, each linking the decision that gives its
reasons. The [overview](../../docs/design/overview.md#the-command-line-on-cobra) lists how cld
overrides cobra.

## The packages

- Package `main` in `cmd/cld` holds `main`, the root command and a file per command.
- What the commands share goes in a package of `cmd/cld/internal`, which exports only what a
  command uses. The [overview](../../docs/design/overview.md#the-packages) says what each holds.

## The command line

- Keep cobra's defaults overridden: the first argument, and the one after `setup`, `setup config`
  and `restore`, checked before cobra. Options are read up to the first argument, and the commands
  stay unsorted (decisions [11](../../docs/design/decisions/0011-go-and-cobra.md),
  [53.2](../../docs/design/decisions/0053-project-and-user-settings.md),
  [59.1](../../docs/design/decisions/0059-ghostty-tabs.md) and
  [60.1](../../docs/design/decisions/0060-a-command-for-each-restore.md)).
- cld's output goes through `output.Print`, cobra's help and completion included, so that a failed
  write is an error ([decision 11](../../docs/design/decisions/0011-go-and-cobra.md)).
- `help` takes one of cld's commands, a command of `setup` or `completion` after it, a command
  after `setup config` and a shell after `setup completion`; it refuses anything else
  ([decision 12](../../docs/design/decisions/0012-help-from-cobra.md)).
- A usage error ends with status 2 before any tool runs; what `join` would lose ends with status 1
  ([decision 50](../../docs/design/decisions/0050-one-command-join.md)).
- `join` is the one command for a session; `new`, `resume` and any other word get `unknown command`
  ([decision 50](../../docs/design/decisions/0050-one-command-join.md)).
- `join` checks its own pane before any lookup, runs `claude --version` only where claude starts,
  and checks the terminal last, before the title (decisions
  [6](../../docs/design/decisions/0006-versions.md),
  [31](../../docs/design/decisions/0031-a-terminal-to-attach-from.md) and
  [50](../../docs/design/decisions/0050-one-command-join.md)).
- `join --existing` stays in every later release with its meaning, as another machine's cld types it
  ([decision 58](../../docs/design/decisions/0058-joining-only-a-session-that-exists.md)).
- `restore tabs --watch` and its hidden `--watching` stay in every later release with their
  meanings, as ssh's config and the watcher's own exec name them
  ([decision 59.11](../../docs/design/decisions/0059-ghostty-tabs.md)).
- `join` refuses a word after `--` that cld gives claude itself, that resumes a conversation, or
  with which claude leaves the session (`claudeOptions` in `cmdline`;
  [decision 41](../../docs/design/decisions/0041-claude-options.md)).
- `join` and `setup config project` enter `-C`'s DIR as `cd` does, after their own checks and
  before anything reads the current directory; a move carries the directory before `-C`
  ([decision 61](../../docs/design/decisions/0061-running-in-another-directory.md)).
- `--host` checks only HOST, the hidden options, what the host would take from its home and a pane
  of cld's servers. cld then execs ssh, and the host's cld makes every other check. `__host` and
  the plain words `--host` sends stay in every later release
  ([decision 62](../../docs/design/decisions/0062-commands-on-another-host.md)).
- The default NAME, from the repository or the directory, is the one name cld changes. Messages name
  a session back as `-n NAME -s SUFFIX`
  ([decision 24](../../docs/design/decisions/0024-names-from-the-repository.md)).
- Without `-n`, `join`, `detach` and `kill` refuse a running session whose `@cld-home` is another
  directory ([decision 37](../../docs/design/decisions/0037-sessions-of-another-repository.md)).
- The list's Enter takes `join`'s path on every row
  ([decision 50](../../docs/design/decisions/0050-one-command-join.md)). Its Ctrl+X, twice on an
  `ended` row, forgets the entry, which `kill` never does
  ([decision 40](../../docs/design/decisions/0040-session-record.md)).
- The list's Ctrl+R renames an `ended` row only; no command renames. Its editor keeps each byte
  that is none of its keys, control characters included, and the rename refuses an invalid name
  whole ([decision 57](../../docs/design/decisions/0057-renaming-from-the-list.md)).

## The help

- The help is cobra's default templates over each command's `Use`, `Short`, `Long` and option
  usages, with value names in backquotes
  ([decision 12](../../docs/design/decisions/0012-help-from-cobra.md)).
- cobra wraps nothing: break the texts by hand within 80 columns, which `TestHelpText` checks
  against `tests/testdata/help`
  ([decision 12](../../docs/design/decisions/0012-help-from-cobra.md)).
- The `Short`s are also what `cld <TAB>` shows
  ([decision 17](../../docs/design/decisions/0017-shell-completion.md)).

## Completion

- Completion runs none of the startup checks, `setup`'s included, never starts the interactive list,
  and never ends a session (decisions [17](../../docs/design/decisions/0017-shell-completion.md) and
  [46](../../docs/design/decisions/0046-idle-sessions.md)).
- Nothing completes a file name; bash's script ends with cld's lines that turn them off under
  ble.sh too (decisions [17](../../docs/design/decisions/0017-shell-completion.md) and
  [27](../../docs/design/decisions/0027-completion-with-blesh.md)).
- The `setup` commands' checks stay in their `Args` and `RunE`, never in a root hook, which
  completion would run (decisions [17.4](../../docs/design/decisions/0017-shell-completion.md) and
  [48.9](../../docs/design/decisions/0048-restore-after-reboot.md)).
