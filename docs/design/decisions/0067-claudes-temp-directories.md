# 67. Claude's temp directories

Status: Accepted (#240).

## Context

claude keeps a directory for each conversation in its temp root, and leaves it there once the
conversation ends ([claude's files](../findings/claude-files.md)). Where `/tmp` is tmpfs, that is
memory held for good. One host held 12 GB there, 9.6 GB of it in six ended sessions.

cld ends a session in four places, each with a `run-shell` that removes the run mark
([48.1](0048-restore-after-reboot.md)):

- `kill`, and the list's kill ([13](0013-a-server-per-session.md));
- the end of a server that has outlived its session ([15.3](0015-killing-from-the-list.md));
- claude's exit with status 0, in the `pane-died` hook ([48.2](0048-restore-after-reboot.md));
- the sweep of idle sessions ([46.4](0046-idle-sessions.md)).

The session's entry names its conversation ([40](0040-session-record.md)). cld builds with
`CGO_ENABLED=0`, so no C library of the system is at hand.

## Decision

cld removes a conversation's temp directory as its session ends.

### 67.1 The temp root

cld takes the temp root as claude does: `claude-UID` in `CLAUDE_CODE_TMPDIR`, or else in `TMPDIR`,
`TMP` or `TEMP`, by default `/tmp`, an empty variable counting as unset. As a session ends, sh reads
them from the server's environment, which claude's pane shares.

Rejected: setting claude's `TMPDIR` to a directory of cld's, which would hide the files from
claude's own cleanup.

### 67.2 Removal as a session ends

The `run-shell` that removes the run mark also removes `ROOT/*/ID`: every project's directory for
the conversation, as `/cd` and worktrees change the project. sh reads the ID from the end of the
entry's line, as a UUID or nothing, and removes nothing without one.

- `kill` and an outlived server remove it before `kill-session`, as the run mark goes.
- The `pane-died` hook removes it once claude has exited with status 0.
- The idle sweep removes it after `kill-session`, within the check made again, so a session kept
  keeps its directory.

A conversation resumed later starts with an empty one: its scratchpad served the run that ended.
The command prints nothing and exits 0, as tmux would show anything else. A removal that fails
there is silent.

Rejected: a removal by cld once tmux returns, which a claude exiting by itself has no cld for.

## Consequences

- A conversation resumed after its session ended has an empty scratchpad.
- The entry's line keeps ending in `"conversation":"ID"}`, which 67.2's sh reads
  (`entryWriter` in `internal/session`).
- `internal/claudetemp` holds the temp root. The tests' sandbox sets `CLAUDE_CODE_TMPDIR` and
  `TMPDIR` to a directory of its own, so no test reaches the user's.
- The guide's [sessions](../../guide/sessions.md#ending-a-session) page says what goes.
