# 25. The tab's title follows claude's status

Status: Accepted. Amended by [26](0026-title-marks-worktree.md), [39](0039-hook-cost.md) and
[49](0049-status-in-the-list.md).

## Context

claude's own title turns between `◐` and `◑` while busy, but holds `✳` under tmux (claude
2.1.283; [claude findings](../findings/claude.md)). So cld's title, `✳ cld-S`, turns by hooks given
through `--settings`, which tell tmux the status; tmux sets the title of every terminal on the
session. How tmux sets titles under `status off` is in the
[tmux terminal findings](../findings/tmux-terminal.md).

## Decision

### 25.1 The status from claude's hooks

claude documents its hooks, and 2.1.232 has them, so the release of [decision 6](0006-versions.md)
stands. Rejected:

- `~/.claude/sessions/PID.json`, undocumented, and needing a process per session to watch it;
- the pane's output, which typing changes too;
- claude's title, which never turns under tmux;
- taking `TMUX` from claude, which needs it for its passthrough.

### 25.2 The events

`UserPromptSubmit`, `PostToolUse` and `ElicitationResult` make claude busy; `PostToolUse` also comes
once a tool allowed at a prompt has run ([decision 49.5](0049-status-in-the-list.md)).
`PermissionRequest` and `Elicitation` make it wait. `Stop`, `StopFailure`, `idle_prompt` and an
interrupted tool make it idle. An interrupt as claude writes has no event, so busy lasts until the
next prompt or `idle_prompt` a minute later.

### 25.3 What a hook runs

A hook runs tmux by the path cld checked, on claude's server by the socket's absolute path and for
the session by name. So it still works where claude runs hooks in the background, without `TMUX` and
`TMUX_PANE`; there the pane target of cld 0.8.0 reached the default server. Without a target tmux
would take the session used last, maybe one claude made. The hook sets the status only where it
changes, as any option set redraws every terminal. It prints nothing, since what a
`UserPromptSubmit` hook prints reaches the model.

### 25.4 Options on claude's session

The title's options go on claude's session, as those of
[decision 5](0005-failures-stay-on-screen.md) go on its pane, so a session claude makes keeps
tmux's. A claude that exited shows `✳`. The title leaves claude's own out, so C1 holds
([testing](../testing.md)).

### 25.5 The turning

The busy marker, expanded with strftime, is `◐` in even seconds and `◑` in odd ones. A `#()` job
after it redraws the title a second later, as nothing else would under `status off`. tmux runs a job
again only in another second, so a faster redraw would stop the turning. The job names tmux by a
quoted option, as a path may hold `#`, `%` or `)`.

### 25.6 Before tmux, and after

cld prints `✳ cld-S` before tmux starts, to a terminal only
([decision 31](0031-a-terminal-to-attach-from.md)), as the list does
([decision 14](0014-the-session-list.md)); tmux replaces it on attach. A terminal keeps its last
title after it detaches.

### 25.7 Tests

The probe runs the hooks of its `--settings` as claude would, and fails a test on a hook that fails
or prints. The tests cover each event, and C1 turning on each terminal ([testing](../testing.md)).

## Consequences

Each hook starts a tmux client that claude waits for, and the turning starts one a second for each
terminal ([decision 39](0039-hook-cost.md)). Out of scope: a marker for `waiting`, which claude's
title lacks too; more than the marker; the title after a detach.
