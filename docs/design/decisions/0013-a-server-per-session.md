# 13. A server per session

Status: Accepted (#22). Replaces [9](0009-only-clds-own-sessions.md). Amended by #69,
[15](0015-killing-from-the-list.md), [24](0024-names-from-the-repository.md),
[34](0034-servers-are-marked.md), [38](0038-stale-sockets.md), [40](0040-session-record.md)
and [51](0051-moving-between-sessions.md).

## Context

cld's sessions shared one server, so each fact below reached every session
([tmux findings](../findings/tmux-sessions.md)):

- A bare `tmux new-session` from anything claude runs lands on claude's server, through `TMUX`.
- A pane starts with the environment of the client that started the server, but for `PATH` and
  `update-environment`. Every claude got the first session's `CLAUDE_CONFIG_DIR` or `VIRTUAL_ENV`.
- A crash, or a stray `kill-server` or `set -g` from anything claude ran, reached every session.

## Decision

Session `cld-NAME` runs on a tmux server of its own, `tmux -L cld-NAME`, with the same options.
cld looks for that one session there, by its whole name. A session claude makes there has
another name, so sessions need no mark; servers have one
([decision 34](0034-servers-are-marked.md)).

- `kill` runs `kill-session` and then `kill-server`, in one tmux command.
- A server that outlives its session is not reused, and `join` refuses its name. `kill` ends such
  a server where it has cld's mark and sessions, none of them `cld-NAME`, and its socket path
  names `cld-NAME` (#69). The kill's own command checks this again.

### 13.1 list reads the socket directory

`list` reads the sockets `cld-NAME` in tmux's socket directory, and asks each server for its
session, eight at a time ([decision 38](0038-stale-sockets.md)). It passes over a stale socket, and
a server that exits as it asks. cld removes no socket. An unlocked `rm` could race the server of a
new session, which tmux starts on a stale socket under a lock (read in tmux's client code, not
tested).

### 13.2 Names of 64 characters at most

NAME has at most 64 characters, so that the socket path fits in `sun_path`: 107 bytes on Linux, 103
on macOS (not checked). A long `TMUX_TMPDIR` leaves less, and tmux then fails with
`File name too long`. cld reports that rather than take the name for a free one. A `TMUX_TMPDIR`
under macOS's `$TMPDIR` would leave about 33 characters (worked out, not checked).

### 13.3 No switching between servers

tmux's `C-q s`, `C-q (` and `C-q )` switch only within one server, and paste buffers are not
shared. [Decision 51](0051-moving-between-sessions.md) brought the keys back as cld's own.

### 13.4 Sessions of 0.3.0 and earlier

The sessions of cld 0.3.0 and earlier stay on the server `tmux -L cld`, where cld does not look,
and the [user guide](../../guide.md) says to end them. `list` shows no session whose name differs
from its server's, such as one renamed by hand.

### 13.5 A tmux process per session

Each session costs one more tmux process, 4 to 5 MB, next to about 400 MB for claude.

### 13.6 Names that differ only in case

Where the socket directory ignores case, as macOS's APFS does (not checked), `a` and `A` share one
socket. There cld reads the socket path the server started on, and refuses a name that clashes with
another session's, pointing at no kill. Without that check, `join` and `kill` took server `a` for
one that outlived session `A`, and pointed at, or in `kill` ran, `kill-server`, ending `a`. Names
stay case-sensitive, as on Linux.

## Consequences

- Each claude gets the environment of the shell that made its session, as no server is reused,
  and a crash or a stray command reaches one session alone.
- claude gets SIGHUP, and what it started through tmux ends with the server. The session goes
  first, so an attached terminal ends with `[exited]` and status 0, where `kill-server` alone
  gives status 1.
- `kill-server` only signals the server, which refuses new connections until its clients have
  gone. cld counts that as no server, so a session made right after a kill gets a fresh server.
- `kill` leaves a server without a session, one being started or exiting, and one whose
  `cld-NAME` was made after the lookup.
- A session renamed by a `rename-session` claude runs is not the one its server is named after,
  and `kill` ends its server, claude with it.
