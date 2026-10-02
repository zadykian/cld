# 16. Resume

Status: Accepted (#26). Amended by [24](0024-names-from-the-repository.md),
[40](0040-session-record.md), [41](0041-claude-options.md), [45](0045-resuming-a-copy.md) and
[47](0047-agent-view-off.md). Superseded by [50](0050-one-command-join.md): `resume` is gone, `join`
brings back a session that has ended, and `join --resume SESSION` does what `resume SESSION` did.

## Context

A conversation outlives its session: `kill`, the list's Ctrl+X
([decision 15](0015-killing-from-the-list.md)), a reboot or a crash leave it in claude's history.
`claude --resume` takes a session ID, a name, or a search term for its picker. By name it looks
among the conversations of the directory's git worktrees, and by ID in any directory. A title given
with `--name` wins over the conversation's own. These were read in the bundles of claude 2.1.283 and
2.1.284 ([claude's findings](../findings/claude.md)).

## Decision

`resume` brought a conversation back in a new session, made as `new` made one. Only claude's
arguments differed, `--resume` among them. [Decision 50.1](0050-one-command-join.md) folded it into
`join`.

### 16.1 SESSION

SESSION is whatever `claude --resume` takes, passed on as one word. An empty one is refused, and one
starting with `-`, which claude would read as an option. Words after `--` go to claude since
[decision 41](0041-claude-options.md).

### 16.2 The session's name

`--name` with the session's name always goes with `--resume`. So the conversation takes the
session's name and the next resume finds it, at the cost of the name it had.

### 16.3 No worktree

claude takes a worktree conversation back to its worktree itself. Claude Code's docs do not say how
`--worktree` combines with `--resume`, so cld never passes both. The worktree base of
[decision 4](0004-worktrees.md) is left out: it governs the worktrees claude makes later, and cld
cannot tell a worktree conversation from another. Passing it would change conversations started
without `-w`.

### 16.4 No transcripts read

Claude Code's docs call the transcripts' format internal. claude resolves the conversation and
reports what it cannot find. cld's record ([decision 40](0040-session-record.md)) keeps the ID that
claude's `SessionStart` hook writes, and resuming passes it, in the session's directory.

### 16.5 A claude that exited

Such a session was refused, pointing at `kill`, rather than started again: a claude that failed at
startup has no conversation to resume. Since [decision 50.2](0050-one-command-join.md) `join`
attaches there, and refuses `--resume` as lost. A second pane in the session was rejected too:
`join`'s hint, the dead-pane check and `list` rely on the session's one pane.

### 16.6 A conversation open elsewhere

Not guarded: cld sees tmux sessions, not conversations. Two claudes on one conversation interleave
their messages in one transcript, as Claude Code's docs and the [guide](../../guide/resuming.md)
say.

### 16.7 The claude it needs

What resuming relies on came between claude 2.0.64 and 2.1.232, as Claude Code documents it. So the
oldest claude cld takes rose to 2.1.232 ([decision 6](0006-versions.md)), and the guide need not say
which part needs a newer one.

### 16.8 Words tmux would change

tmux ends a command at a word ending in `;`, and expands `-c` as a format, where `#(...)` runs a
command (tmux 3.3a to 3.7c; [tmux's findings](../findings/tmux-sessions.md)). So cld puts a `\`
before such a `;`, and doubles each `#` of the directory. Before, a directory ending in `;` made no
session, and one holding `#(...)` ran the command.

### 16.9 A resumed session is a session

`list`, the list and `kill` treat it as any other. Resuming is the way back from a kill by mistake.
Since [decision 40](0040-session-record.md) the killed session shows as `ended`.

### 16.10 Conversations moved to the background

Agent view's `/bg` and "Move to background and exit" exit claude with status 0, which ends the
session. After `←` claude stays, and `kill` leaves the copy that claude's daemon runs (claude
2.1.283 and 2.1.284). Resuming finds the copy, which claude refuses while it runs. cld neither stops
it nor forks it unasked, as it guards no conversation open elsewhere;
[decision 45](0045-resuming-a-copy.md) added `--fork`. Until the copy stops, its hooks reach any
running session of the name, a later one too. They set that session's status, title and `list` row,
and after a `/clear` they rewrite its record entry with the copy's conversation. cld leaves that
too. Agent view is off in cld's sessions ([decision 47.3](0047-agent-view-off.md)), so this holds
for sessions of cld 0.10.0 or earlier.

## Consequences

- The [guide](../../guide/resuming.md) brings a moved conversation back with `claude attach ID`, or
  with `claude stop ID`, then `kill` where the session stays, then `join`.
- `list` does not mark a moved conversation: that would take running claude, which `list` never
  does.
