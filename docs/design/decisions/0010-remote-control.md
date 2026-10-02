# 10. Remote Control

Status: Accepted. Superseded by [42](0042-remote-control-is-claudes.md).

## Context

Remote Control lets a session go on from claude.ai or the Claude app. A resumed conversation
does not keep its settings, and Claude Code's docs say to pass them again. Flag settings outrank
the user's ([claude findings](../findings/claude.md)).

## Decision

- `new` and `resume` passed `remoteControlAtStartup` as `true` in `--settings`. An org policy
  and a project's `false` could still keep it off.
- With `-w` the worktree's setting went into the same JSON, as claude does not document how two
  `--settings` merge.

## Consequences

cld's `true` overrode the `false` a user had saved with `/config`, which
[decision 42](0042-remote-control-is-claudes.md) ended. A project's `false` beats flag settings
from claude 2.1.222 on, which made that release the floor of [decision 6](0006-versions.md)
until decision 42.
