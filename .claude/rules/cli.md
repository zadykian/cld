---
paths:
  - "cmd/cld/**"
  - "internal/picker/**"
---

# cmd/cld and internal/picker

The invariants a change to the command line keeps. Decision numbers are those of
[docs/design.md](../../docs/design.md#decisions), whose Implementation notes list how cld
overrides cobra.

## The command line

- Keep cobra's defaults overridden: the first argument and the one after `setup` checked before
  cobra, options read up to the first argument, the commands unsorted (decision 11).
- cld's output goes through `output.Print`, cobra's help and completion included, so that a
  failed write is an error (decision 11).
- `help` takes one of cld's commands, a command of `setup` or `completion` after it, and a shell
  after `setup completion`; it refuses anything else (decision 12).
- A usage error ends with status 2 before any tool runs; what `join` would lose ends with status 1
  (decision 50).
- `join` is the one command for a session; `new`, `resume` and any other word get
  `unknown command` (decision 50).
- `join` checks its own pane before any lookup, runs `claude --version` only where claude starts,
  and checks the terminal last, before the title (decisions 6, 31 and 50).
- `join` refuses a word after `--` that cld gives claude itself, that resumes a conversation, or
  with which claude leaves the session (`claudeOptions`; decision 41).
- The default NAME, from the repository or the directory, is the one name cld changes. Messages
  name a session back as `-n NAME -s SUFFIX` (decision 24).
- Without `-n`, `join`, `detach` and `kill` refuse a running session whose `@cld-home` is another
  directory (decision 37).
- The list's Enter takes `join`'s path on every row (decision 50). Its Ctrl+X, twice on an
  `ended` row, forgets the entry, which `kill` never does (decision 40).

## The help

- The help is cobra's default templates over each command's `Use`, `Short`, `Long` and option
  usages, with value names in backquotes (decision 12).
- cobra wraps nothing: break the texts by hand within 80 columns, which `TestHelpText` checks
  against `tests/testdata/help` (decision 12).
- The `Short`s are also what `cld <TAB>` shows (decision 17).

## Completion

- Completion runs none of the startup checks, `setup`'s included, never starts the interactive
  list, and never ends a session (decisions 17 and 46).
- Nothing completes a file name, `--collector-config`'s `FILE` neither; bash's script ends with
  cld's lines that turn them off under ble.sh too (decisions 17 and 27).
- The `setup` commands' checks stay in their `Args` and `RunE`, never in a root hook, which
  completion would run (decision 18).
