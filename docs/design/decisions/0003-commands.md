# 3. Commands

Status: Accepted (0.2.0). Amended by [13](0013-a-server-per-session.md),
[17](0017-shell-completion.md), [18](0018-telemetry.md), [24](0024-names-from-the-repository.md) and
[40](0040-session-record.md). Superseded by [50](0050-one-command-join.md) in its split of `new` and
`join`.

## Context

tmux matches a session target exactly, then as a prefix: `kill-session -t cld-rev` beside
`cld-review` kills `cld-review` ([tmux findings](../findings/tmux-sessions.md)).

## Decision

- `new` created a session and failed where it existed; `join` attached and failed where none
  did. The name moved to `-n NAME`, and `cld NAME` failed, naming both.
- Commands address a session as `=cld-NAME`, an exact match.
- `list` shows the directory claude is in now, and nothing where no session runs or has ended
  ([decision 40.3](0040-session-record.md)).
- `kill` ends a session as a closing terminal would: claude gets SIGHUP.

## Consequences

[Decision 50](0050-one-command-join.md) made `join` the one command, acting by the session's
state. `cld NAME`, `cld new` and `cld resume` now fail as any unknown command.
