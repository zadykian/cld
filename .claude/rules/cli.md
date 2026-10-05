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

- Keep cobra's defaults overridden: the first argument, and the one after `setup` and
  `setup config`, checked before cobra. Options are read up to the first argument, and the commands
  stay unsorted (decisions [11](../../docs/design/decisions/0011-go-and-cobra.md) and
  [53.2](../../docs/design/decisions/0053-project-and-user-settings.md)).
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
- `join` refuses a word after `--` that cld gives claude itself, that resumes a conversation, or
  with which claude leaves the session (`claudeOptions` in `cmdline`;
  [decision 41](../../docs/design/decisions/0041-claude-options.md)).
- The default NAME, from the repository or the directory, is the one name cld changes. Messages name
  a session back as `-n NAME -s SUFFIX`
  ([decision 24](../../docs/design/decisions/0024-names-from-the-repository.md)).
- Without `-n`, `join`, `detach` and `kill` refuse a running session whose `@cld-home` is another
  directory ([decision 37](../../docs/design/decisions/0037-sessions-of-another-repository.md)).
- The list's Enter takes `join`'s path on every row
  ([decision 50](../../docs/design/decisions/0050-one-command-join.md)). Its Ctrl+X, twice on an
  `ended` row, forgets the entry, which `kill` never does
  ([decision 40](../../docs/design/decisions/0040-session-record.md)).

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
