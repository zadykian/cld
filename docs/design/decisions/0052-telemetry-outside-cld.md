# 52. Telemetry outside cld

Status: Accepted (#158). Replaces [18](0018-telemetry.md).

## Context

`cld setup telemetry` ran an OpenTelemetry Collector in Docker and pointed claude's user settings at
it ([decision 18](0018-telemetry.md)). No other command of cld's runs Docker. This one ran on Linux
alone, pinned an image to bump, and its tests needed a fake docker.

The author's machines now get claude's telemetry from the playbook of `zadykian/ws`. There a
collector on the host takes the journal and the host's metrics too. It sends everything to a local
SigNoz, and claude's metrics to a remote collector as well. The playbook writes claude's `env` keys
itself, which `setup telemetry` would rewrite. A collector for the whole host is the machine's
setup, not a session manager's.

This amends decisions [12](0012-help-from-cobra.md), [17](0017-shell-completion.md),
[19](0019-project-settings.md), [22](0022-setting-completion-up.md) and
[48](0048-restore-after-reboot.md), which name `setup telemetry` or 18.8.

## Decision

cld drops `cld setup telemetry`. claude's own settings decide where it sends its telemetry
([Claude Code's docs](https://code.claude.com/docs/en/monitoring-usage)), and the user or the
machine's setup writes them.

### 52.1 An unknown command

`cld setup telemetry` fails as an unknown command of `setup`, with status 2, as `new` and `resume`
do ([decision 50.6](0050-one-command-join.md)). `help` and completion no longer name it. `setup`
keeps `project`, `completion` and `restore`. Not taken: a release that only warns, as decision 50
dropped two commands at once too.

### 52.2 What a machine keeps

cld removes nothing that `setup telemetry` set up. The `cld-telemetry` container keeps running,
Docker starts it after a reboot, and the `env` keys stay, so claude keeps sending to it. The
[guide](../../guide/install-and-upgrade.md)'s upgrade notes say how to remove both. Not taken: a
command that removes them, which cld would keep for this alone.

### 52.3 `setup` checks its word before cobra

This was 18.8, which `setup`'s other commands rely on. cld checks the argument after `setup` before
cobra, which would run `setup project --mcp goland` for `setup --mcp goland project`. The shell
after `setup completion` is checked the same way ([decision 22.1](0022-setting-completion-up.md)).
`help setup project` works as cobra's help does ([decision 12.2](0012-help-from-cobra.md)).

### 52.4 No Docker

cld runs no `docker`, and the README's requirements drop it. The tests drop their fake docker, and
no test needs one. The findings on the collector and Docker stay in the
[environment findings](../findings/environment.md), and claude's on its telemetry in
[claude's findings](../findings/claude.md), as records of what was probed. The `cld` permission set
of `setup project` keeps `docker`, which this repository's own checks run.

## Consequences

- A user of `setup telemetry` keeps a working collector, and edits or removes it and the keys by
  hand.
- The tests pin `setup telemetry` and `help setup telemetry` as unknown commands.
- The hard rule on a scratch `HOME` covers `setup completion` and `setup restore`.
- Out of scope: other ways to send claude's telemetry, which claude's settings decide.
