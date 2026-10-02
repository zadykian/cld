# 39. What the title's hooks cost

Status: Accepted (#81).

## Context

Each hook of [decision 25](0025-title-follows-status.md) starts a tmux client that claude waits for
([claude findings](../findings/claude.md)). While the title is busy, each terminal on the session
starts one a second for the turning. A tmux client takes about 6 ms built from source, but 130-230
ms through Ubuntu's snap of 3.7c. There a turn of 40 tools waits 5 to 9 s more for `PostToolUse`
alone ([tmux terminal findings](../findings/tmux-terminal.md)).

## Decision

### 39.1 The status's hooks wait

They stay synchronous, `PostToolUse` and `ElicitationResult` too, since the order they land in is
the status's. claude runs the tools of one answer with no model call between them (claude 2.1.284).
So a `PostToolUse` in the background could land after the next tool's `PermissionRequest`. Through
the snap it did in up to half the runs, leaving the title turning while claude asks for the user.
`ElicitationResult`'s could land before an MCP server's next question, and `UserPromptSubmit`'s
after the idle of a `StopFailure` that comes at once. Rejected: those hooks in the background, as
the issue proposed. Keeping them costs the snap's tenth of a second after each tool. No guard was
found, as `PermissionRequest`'s input names no tool call to match.

### 39.2 A timeout of 5 s

Each of them, and `SessionStart`, has `"timeout": 5`. A tmux that hangs then holds claude up 5 s,
not claude's default of 600 s (30 s for `UserPromptSubmit`).

### 39.3 CwdChanged in the background

It runs with `async`, and no timeout, as claude ends no background hook at one. It asks git about
the directory claude started it in. Only two changes in one answer could land out of order, leaving
` [w]` behind until the next. `async` predates the claude release [decision 6](0006-versions.md)
requires, which stands.

### 39.4 The turning stays at a second

A tick every 2 s would halve the job's cost but turn at half claude's pace. A marker with no job
would cost nothing and not turn. The maintainer kept the turning. The [user guide](../../guide.md)
and the README say what the title costs.

### 39.5 Tests

The settings, word for word, have `async` and `timeout`. The probe waits for every hook, so tests
read the status each leaves ([testing](../testing.md)).

## Consequences

The record's hooks follow the same rules ([decision 40.2](0040-session-record.md)). Out of scope:
making tmux start faster, and fewer hooks.
