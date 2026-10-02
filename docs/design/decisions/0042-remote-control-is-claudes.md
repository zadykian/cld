# 42. Remote Control is claude's own setting

Status: Accepted (#62). Amended by [47](0047-agent-view-off.md).

## Context

[Decision 10](0010-remote-control.md), which this replaces, passed `remoteControlAtStartup: true` in
`--settings`. Flag settings outrank the user's, so it overrode a `false` saved with `/config`. While
Remote Control is connected, the session's transcript is stored on Anthropic's servers, as Claude
Code's docs say. Accounts that cannot have it went without it silently, and some orgs got a policy
notice in every session (claude 2.1.282 to 2.1.284; [claude findings](../findings/claude.md)).

## Decision

### 42.1 The user's choice

Whether a conversation goes to Anthropic's servers is the user's to choose, in claude's settings.
cld passes no `remoteControlAtStartup`, so a session connects where a claude started without cld
would.

### 42.2 What `--settings` carries

It carries what cld needs, and nothing that is the user's to choose. That is the title's hooks
([25](0025-title-follows-status.md), [26](0026-title-marks-worktree.md)), the record's
([40.2](0040-session-record.md)) and the worktree's base with `-w` ([4](0004-worktrees.md)). Agent
view goes off too ([47.2](0047-agent-view-off.md)). The notification channel stays the user's
([29](0029-notifications.md)). They go again with every resume, as a resumed conversation does not
keep them ([decision 10](0010-remote-control.md)).

### 42.3 One session

`/remote-control` connects a session, and so does claude's `--remote-control` after `--`
([41](0041-claude-options.md)). An option of cld's would copy claude's.

### 42.4 The docs

The README and the user guide say where a connected session's transcript goes. The guide's Upgrading
says how to connect sessions as before. A session an older cld started keeps Remote Control until it
ends.

### 42.5 claude's floor

[Decision 6](0006-versions.md) stands: 2.1.222 mattered only for a project's `false` to beat cld's
`true`, and resuming still needs 2.1.232.

### 42.6 Tests

The settings word for word ([25.7](0025-title-follows-status.md)) hold no key, and the root's help
no longer says "with Remote Control on".

## Consequences

Who relied on cld's `true` now sets it in `/config`, where it reaches every claude.
