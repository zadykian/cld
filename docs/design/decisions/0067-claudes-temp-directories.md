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

cld removes a conversation's temp directory as its session ends. On Linux, `list` and `join`
remove those that no process holds.

### 67.1 The temp root

cld takes the temp root as claude does: `claude-UID` in `CLAUDE_CODE_TMPDIR`, or else in `TMPDIR`,
`TMP` or `TEMP`, by default `/tmp`, an empty variable counting as unset. As a session ends, sh reads
them from the server's environment, which claude's pane shares. The sweep reads cld's own.

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
there is silent, and the sweep finds what is left (67.3).

Rejected: a removal by cld once tmux returns, which a claude exiting by itself has no cld for.

### 67.3 The sweep of what is left

`list`, and `join` without `-s`, remove each `ROOT/PROJECT/ID` that no process holds, once both it
and the conversation's transcript were last written over a day ago. The transcript is
`projects/*/ID.jsonl` in `CLAUDE_CONFIG_DIR`, by default `~/.claude`, and one that is gone counts
as old.

- A process holds a directory where its working directory, or a file it has open, is under it, as
  `/proc` shows for cld's user. A running claude keeps its `tasks` open.
- The root must be a directory the user owns, not a symbolic link. Only its parent is resolved.
  claude refuses such a root too, and another user can plant a link in a sticky `/tmp`.
- Where `/proc` does not show cld its own files, the sweep removes none.
- It runs whatever `CLD_IDLE_DAYS` says, as it ends no session. `join` runs it before the
  record's lock, as it reads no record.
- It removes quietly. One directory it cannot remove is a warning, never an exit.

It finds the directories of sessions ended before this release, those of conversations that
`/clear` left, and those of claudes run outside cld. Rejected: a sweep without the day, which could
take the directory of a claude starting.

### 67.4 None on a Mac

On a Mac the sweep removes nothing, and 67.2 holds. darwin has no `/proc`. `lsof` reads every
process's open files, a second or more at each `list` and `join`, and `libproc` needs cgo. macOS's
`TMPDIR` is a directory of the user's on disk, so a directory left costs disk, not memory.

## Consequences

- A conversation resumed after its session ended has an empty scratchpad.
- The entry's line keeps ending in `"conversation":"ID"}`, which 67.2's sh reads
  (`entryWriter` in `internal/session`).
- `internal/claudetemp` holds the temp root and the sweep. The tests' sandbox sets
  `CLAUDE_CODE_TMPDIR` and `TMPDIR` to a directory of its own, so no test reaches the user's. A
  sweep there would remove the directories of the user's ended sessions.
- The guide's [sessions](../../guide/sessions.md#ending-a-session) and
  [idle sessions](../../guide/idle-sessions.md) pages say what goes.
