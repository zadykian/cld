---
paths:
  - "internal/completion/**"
  - "internal/configfile/**"
  - "internal/project/**"
  - "internal/restore/**"
  - "internal/update/**"
  - "install.sh"
---

# setup, update and install.sh

The invariants a change to the `setup` commands, `cld update` or `install.sh` keeps, each linking
the decision that gives its reasons. CLAUDE.md says where they may run.

## The files they write

- Every `setup` command writes through `internal/configfile`: it edits a file in place, and removes
  nothing (decisions [19](../../docs/design/decisions/0019-project-settings.md),
  [22](../../docs/design/decisions/0022-setting-completion-up.md) and
  [48](../../docs/design/decisions/0048-restore-after-reboot.md)).
- `setup project` never replaces a value the settings have, but replaces whole a server's entry in
  `.mcp.json` that differs ([decision 19](../../docs/design/decisions/0019-project-settings.md)).
- `setup project` only creates `.claude/settings.local.json`, and the settings it writes hold
  nothing of one person's, such as `theme`
  ([decision 28](../../docs/design/decisions/0028-shared-project-settings.md)).
- `setup project`'s `.gitignore` lines keep in git what a project shares in `.claude`. In a git work
  tree it checks that git ignores none of those files
  ([decision 28](../../docs/design/decisions/0028-shared-project-settings.md)).
- `setup completion` writes byte for byte what `cld completion SHELL` prints, where the shell reads
  it, and reads both files before it writes either
  ([decision 22](../../docs/design/decisions/0022-setting-completion-up.md)).
- The zsh lines in `.zshrc` run `compinit -i` only where nothing has before them
  ([decision 22](../../docs/design/decisions/0022-setting-completion-up.md)).

## Their checks

- `setup restore` runs on Linux with systemd alone, where `systemctl --user show-environment`
  answers ([decision 48](../../docs/design/decisions/0048-restore-after-reboot.md)).
- `setup restore` refuses a `CLD_IDLE_DAYS` that `restore` would refuse, and runs `daemon-reload`
  only where the unit changed
  ([decision 48](../../docs/design/decisions/0048-restore-after-reboot.md)).

## update and install.sh

- `update` runs none of the tmux and claude checks, and refuses a version that is no X.Y.Z, such as
  `dev` ([decision 21](../../docs/design/decisions/0021-self-update.md)).
- `update` replaces the file cld runs from by a rename, once the new binary matches its checksum and
  prints the release's version ([decision 21](../../docs/design/decisions/0021-self-update.md)).
- An interrupt before the rename leaves cld unchanged, and ends with 128 plus the signal's number
  ([decision 21](../../docs/design/decisions/0021-self-update.md)).
- A completion script `update` cannot write anew is a warning naming `cld setup completion SHELL`,
  never a failure ([decision 22](../../docs/design/decisions/0022-setting-completion-up.md)).
- `install.sh` is POSIX sh, piped from `curl`: everything stays in functions its last line calls, so
  that a download cut short runs nothing
  ([decision 20](../../docs/design/decisions/0020-install-script.md)).
