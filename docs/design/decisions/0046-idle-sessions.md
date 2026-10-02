# 46. Idle sessions end

Status: Accepted (#79). Amended by [48](0048-restore-after-reboot.md),
[50](0050-one-command-join.md) and [51](0051-moving-between-sessions.md).

## Context

A claude runs until `kill`, holding 0.2 to 0.5 GB where its tmux server takes 4 to 5 MB (claude
2.1.284; [claude findings](../findings/claude.md)). Nothing showed which sessions were forgotten.
What moves tmux's activity times, and the path `TMUX` gives, are in the
[tmux session findings](../findings/tmux-sessions.md) (tmux 3.5a and 3.7c).

## Decision

`list`, and `join` without `-s`, end each session idle for longer than `CLD_IDLE_DAYS` days, 30 by
default, as `kill` ends it ([13](0013-a-server-per-session.md)), with a note on stderr. The table
and the list ([14](0014-the-session-list.md)) show `LAST ACTIVE`.

### 46.1 What idle means

Idle runs from the later of `#{session_activity}` and `#{session_last_attached}`, and never while a
terminal is attached. tmux moves these for an attach and each key typed, but not for output. So
claude working alone, or driven through Remote Control, counts as idle. A terminal closed without
`C-q d` leaves the session idle from its last key. Rejected: `#{window_activity}`, which claude's
every redraw moves; and an idle time from the `Stop` hook ([25](0025-title-follows-status.md)),
which sessions without the hooks lack.

### 46.2 Where the sweep runs

`list` sweeps first, as it reads the servers anyway. `join` without `-s` sweeps once it has taken
the index, under the record's lock ([50.5](0050-one-command-join.md)), so its session does not take
the name of one just ended. The sweep reads every server ([38](0038-stale-sockets.md)), some 0.25 s
more for 10 servers through the snap, and `CLD_IDLE_DAYS=0` reads no server. A failed read ends
`list`, and is a warning in `join`, for which the sweep is not what was asked. The sweep comes
before `join`'s checks of its own session, a terminal ([31](0031-a-terminal-to-attach-from.md)) and
a lingering server ([13](0013-a-server-per-session.md)), so a refused `join` has swept.

Neither ends the session whose server cld runs on, the socket `TMUX` names. Its claude running
`list` would end itself mid-turn, as tmux takes a session driven only through Remote Control for
idle. cld compares the socket's file, since tmux resolves symbolic links in that path. A move keeps
the session the terminal has just left, and the keys' `list --switch` sweeps none
([51.2](0051-moving-between-sessions.md), [51.5](0051-moving-between-sessions.md)). Completion,
`join -s`, `detach` and `kill` end none either, nor the list's reads after its own actions
([14.6](0014-the-session-list.md)). Rejected: a `kill --idle` alone, which would leave the forgotten
sessions to be remembered.

### 46.3 `CLD_IDLE_DAYS`

Decimal days, fractions too, and `0` for none; empty or unset means 30. Anything else, as `30d`, is
refused before anything runs, rather than read as another limit. The tests give it seconds.

### 46.4 One tmux command

The kill checks again that the session is idle, removes the run mark
([48.1](0048-restore-after-reboot.md)), checks once more, and ends the session and its server. A
terminal that attaches meanwhile, or a key, keeps the session, which
[15](0015-killing-from-the-list.md)'s check of pids would miss. A failed kill is a warning.

### 46.5 `LAST ACTIVE`

`now` under a minute or while attached, then `5m`, `2h` or `31d`, and `-` for an `ended` session
([40.3](0040-session-record.md)). Both times share one field, as fields split at runs of tabs and
the second is empty before any attach. A time cld cannot read counts as now, so nothing ends on it.
A session `restore` brought back starts again from the restore
([48.8](0048-restore-after-reboot.md)).

### 46.6 The record keeps the entry

The entry stays, without its run mark, so `list` shows the session `ended` and `join` brings it back
([40.4](0040-session-record.md)). claude, though, removes a transcript left longer than
`cleanupPeriodDays`, 30 days by default. A session idle 30 days is at that edge, and so is its entry
([40.2](0040-session-record.md)). The guide says to raise one or lower the other.

### 46.7 Tests

With the real tmux and a limit of 8.64 s, `list` and `join` end a detached session idle 10 s. They
keep one attached, one a key keeps, and the one their claude runs in. A terminal attaching while the
kill is held keeps the session. With the fake tmux: `LAST ACTIVE`, the limits, the kill's command
word for word, and `TMUX` through a symbolic link ([testing](../testing.md)).

## Consequences

A session that only claude or Remote Control kept busy for 30 days ends. Out of scope: a server that
outlived its session, which `kill` ends ([13](0013-a-server-per-session.md)).
