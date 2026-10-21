# 56. Following claude's /cd

Status: Accepted (#171).

## Context

claude's `/cd` moves a conversation to another directory: claude changes its own directory, and the
transcript moves to that directory's project. Every claude cld runs has it
([claude findings](../findings/claude.md#moving-a-conversation)). A running session's row in
`cld list` followed, as tmux reads claude's directory. cld's record kept the directory `join`
started claude in ([decision 40.1](0040-session-record.md)).

Once the session ended, the list showed that directory, and `join` and `restore` resumed the
conversation there ([40.4](0040-session-record.md)). claude then worked in the old tree while the
conversation named the new one. A session on the maintainer's host moved so, from one repository to
another.

`/cd` fires neither `SessionStart` nor `CwdChanged`, and starts no turn, so no `Stop` follows it.
The hooks' `cwd` follows `/cd`, and also a shell `cd` into a subdirectory.

This amends decisions [40.1, 40.2 and 40.4](0040-session-record.md), and
[41.5](0041-claude-options.md)'s count.

## Decision

### 56.1 claude's last directory beside the entry

`sessions/S.dir`, beside the entry, is one line: the directory claude was last in. cld and the hooks
write it whole, by a rename, and only with a directory that exists, while the entry does. It goes
with the entry at expiry and with the list's Ctrl+X ([40.6](0040-session-record.md)). `join` writes
where it starts claude, replacing what an earlier session of the name left.

The entry keeps the directory `join` made the session in. A field in the entry was rejected:
`SessionStart` rewrites the whole line ([40.2](0040-session-record.md)), and two hooks would write
one file without a lock.

### 56.2 The hooks

A record hook reads `#{pane_current_path}` with the tmux cld checked, through the session's socket
([decision 25](0025-title-follows-status.md)). It reads `%0`, claude's pane: the first pane of the
session's own server ([decision 13](0013-a-server-per-session.md)), which no move changes. The
session's active pane, `=cld-S:`, can be another, of a window split by hand. `TMUX_PANE` can be
another too: a teammate of claude's agent teams runs cld's hooks in a pane of its own
([claude findings](../findings/claude.md#inside-a-session)).

`#{pane_current_path}` is claude's process directory, which `/cd` and claude's worktree moves
change and a shell `cd` does not ([claude findings](../findings/claude.md#worktrees)).

The hook runs on `Stop` in the background, and on `SessionEnd` beside the touch, with no timeout.
So the record's hooks now start tmux: one client a turn, which claude does not wait for
([decision 39](0039-hook-cost.md)), and one within the 1.5 s claude gives `SessionEnd`.

tmux names a directory removed under claude `DIR (deleted)`, and tmux 3.4 and 3.5a print a control
character in a path as an escape ([tmux terminal findings](../findings/tmux-terminal.md)). The hook
writes neither, as neither is a directory. After a kill, `SessionEnd` finds no server, and writes
nothing.

`Stop`, `SessionEnd` and `async` exist in claude 2.1.232, so [decision 6](0006-versions.md) stands.
The hooks add 1 KB to tmux's command, counted against its limit ([41.5](0041-claude-options.md)).
cld's own words now take 8.8 KB at the sandbox's paths, and 9.8 KB with tmux at a path of 97
characters ([tmux session findings](../findings/tmux-commands.md)).

A failed turn, which ends in `StopFailure`, writes nothing. A third copy of the hook would take
0.5 KB more of that limit, and the next `Stop`, `SessionEnd` or kill writes the directory.

### 56.3 The kill

`kill`, the list's Ctrl+X and the idle sweep write it too, in the `run-shell` that removes the run
mark before `kill-session` ([48.1](0048-restore-after-reboot.md)). That costs no tmux client, and
covers a `/cd` with no turn after it. The sweep checks the session again around it, as before
([46.4](0046-idle-sessions.md)).

`run-shell -t %0` expands claude's `#{pane_current_path}` into its command, so tmux puts it in
single quotes, each quote in it escaped for sh. tmux's `#{q:}` leaves a newline, which would end
sh's command there ([tmux session findings](../findings/tmux-commands.md)).

### 56.4 Where a session comes back

An ended row, `join`, `restore`, the list's Enter and `C-q s` take the directory in `S.dir`. Where
that cannot be entered, they take the entry's, where cld resumed before, and refuse only where
neither can be entered, as 40.4 refuses. An exited row takes it too, else where claude started, as
before. claude finds a conversation by its ID from any directory
([claude findings](../findings/claude.md#moving-a-conversation)).

The entry, and the `SessionStart` hook, keep the directory `join` made the session in. The
session's home ([37.1](0037-sessions-of-another-repository.md)) comes from it while it can be
entered, else from where claude starts. So `@cld-home` and the names stay where `join` made them.

tmux names claude's directory by its target, through every symbolic link. Where `S.dir` names the
entry's directory so, the entry's name stands, and `PWD` with it. A directory claude moved to keeps
tmux's name.

### 56.5 claude's worktrees

A directory under a `.claude/worktrees` directory counts as none: a `-w` session's own worktree, or
one `EnterWorktree` made. claude goes back to it itself as it resumes a conversation that recorded
one ([claude findings](../findings/claude.md#worktrees)). So such a session comes back as before,
where `join` made it. A worktree that a `WorktreeCreate` hook makes elsewhere is followed as any
directory.

### 56.6 Rejected

- The hooks' `cwd`, and `CwdChanged`'s `new_cwd`, which follow the shell's `cd`
  ([decision 26](0026-title-marks-worktree.md)). `/cd` fires no `CwdChanged`.
- The transcript's `relocated` line: cld reads none of claude's transcripts
  ([40.9](0040-session-record.md)).
- Moving `@cld-home` with `/cd`, which would move the session's name and what `kill` and `join` take
  from where ([decision 37](0037-sessions-of-another-repository.md)).
- A `UserPromptSubmit` hook, a tmux client more a turn. The kill covers a `/cd` with no turn after
  it.

### 56.7 Tests

`record_moved_test.go` drives the hooks and the kill through the probe's `cd`, as `/cd` moves
claude, with a teammate's probe in a pane of its own. It runs `SessionEnd`'s commands after
`kill-server`. `record_moved_join_test.go` covers each fallback, a worktree, a symbolic link, an
older record and `restore`, and `record_list_test.go` the list's Enter. The list's kill of a window
split by hand keeps claude's directory, not the other pane's. The idle sweep keeps one whose name
holds what sh and tmux's formats take for their own ([testing](../testing.md)).

## Consequences

Still uncovered: a `/cd`, then a reboot or a crash with no turn between, or only failed ones. The
session then comes back where claude was before the `/cd`. A session an older cld made keeps its
hooks until it ends, so only the kill writes its directory, as
[55.2](0055-passthrough-from-claudes-pane.md) says of options. An older cld ignores `S.dir`, and
leaves it alone.
