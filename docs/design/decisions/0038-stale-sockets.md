# 38. Stale sockets cost no tmux

Status: Accepted (#65). Amended by [46](0046-idle-sessions.md).

## Context

Neither tmux nor cld removes a socket ([decision 13.1](0013-a-server-per-session.md)), so `tmux-UID`
keeps one for every name used since `/tmp` was last cleaned. cld ran a `list-sessions` on each, one
after another: 6-20 ms each natively, 100-200 ms through Ubuntu's snap of tmux 3.7. There 50 stale
sockets made `cld list` take 7-8 s ([tmux session findings](../findings/tmux-sessions.md)).

## Decision

### 38.1 Connect first

Before cld runs tmux on a socket, it connects to it, as tmux's client does first. A refused
connection, or no socket, is no server, and runs no tmux. cld does so only where tmux would get that
far: in a `tmux-UID` directory of the user's, no link, closed to others, on a path that fits
`sun_path`. Anywhere else, tmux runs and says what is wrong
([decision 13.2](0013-a-server-per-session.md)). tmux 3.5a checks as 3.7c does, so nothing goes by
tmux's version.

Not on the socket `TMUX` names, the server cld runs in. The check of cld's own pane, 43's read, a
bare `detach` ([decision 44.2](0044-detach-command.md)) and a move
([decision 51.3](0051-moving-between-sessions.md)) ask it at once. Where that server has gone,
tmux's own message ends the command with status 1.

### 38.2 Eight servers at once

`list` asks the servers that take the connection eight at a time, and shows their sessions in the
order of their names. Through the snap, 10 servers took 1.2-1.5 s one after another and 0.24-0.26 s
eight at a time. All ten at once took 0.21-0.24 s. Once one fails no more are asked, as an unsafe
directory fails each alike. Of those that failed, the first in name order gives the error.

### 38.3 The index asks one server

`join` without `-s` looks up its candidates' servers from the highest index down, and stops at the
first that runs, as the highest alone counts ([decision 24.1](0024-names-from-the-repository.md)).
The idle sweep then reads every server that runs ([decision 46.2](0046-idle-sessions.md)). On macOS,
a file there that is no socket fails tmux, but only where cld asks it.

### 38.4 Stale sockets stay

Removing them under tmux's own lock, `flock` on `cld-NAME.lock`, which a starting tmux takes, lost
none of 3000 live sockets in #65's trial; the lock file itself would never go. Removal on every
`list`, in `kill` alone or behind a `list --prune` was left out of this change. A stale socket now
costs a connection, some microseconds.

### 38.5 Tests

A tmux that writes down what it runs shows none run for a stale socket, and one server asked for the
index. Held asks show eight at once. A `tmux-UID` open to others ends each command with tmux's
message ([testing](../testing.md)).

## Consequences

Out of scope: the `tmux -V` every command but completion runs ([decision 6](0006-versions.md)), and
the tmux each of the title's hooks runs ([decision 39](0039-hook-cost.md)).
