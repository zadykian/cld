# 34. cld's servers are marked

Status: Accepted (#71).

## Context

[Decision 13](0013-a-server-per-session.md) told cld's servers by the socket's name alone, so the
user's own `tmux -L cld-outer` counted as cld's. In its pane `join` refused, pointing at a `C-q d`
not bound there. Elsewhere cld pointed at `kill-server` or `kill`, which ended the user's sessions.
A session there named as cld names them was listed, offered and killed (cld 0.8.2, tmux 3.7c;
[tmux session findings](../findings/tmux-sessions.md)).

## Decision

### 34.1 The server option @cld

cld sets `@cld` to `1` on the server in the tmux command that starts it, before its other options. A
format looks a user option up in the server's options first, so nothing claude's tmux sets on a
session or a window hides it.

### 34.2 Where cld reads it

The mark is `#{||:#{@cld},#{==:#{prefix},C-q}}`, read in formats cld runs anyway, so it costs no
tmux command. The filter for session `cld-NAME` requires it, so `list`, completion, `join` and
`kill` pass over an unmarked server. cld refuses an unmarked server's name as not its own, pointing
at another name, never at `kill`. `kill`'s `if -F` checks the mark again, so a server that loses it
after the read is left. The check for cld's own pane looks at marked servers only, so `join` nests
in any other. The next index ([decision 24](0024-names-from-the-repository.md)) still counts an
unmarked server, whose name `join` would refuse. Decisions [43](0043-inside-your-own-tmux.md),
[44](0044-detach-command.md) and [51](0051-moving-between-sessions.md) read the mark in their
formats too.

### 34.3 Servers of cld 0.8.2 and earlier

They have no `@cld`, but the prefix `C-q` they set marks them, so an upgrade keeps every session.
tmux's default prefix is `C-b`, so a user's own server `cld-NAME` with prefix `C-q` counts as cld's.
The prefix is a session option: a session that sets its own reads that, where cld's keep the global
one ([tmux session findings](../findings/tmux-sessions.md)).

### 34.4 A guard against mistakes

Like [decision 9](0009-only-clds-own-sessions.md)'s, the mark guards against mistakes, not intent:
whatever reaches the socket can set it. 9's told cld's sessions from others on a shared server; this
one tells cld's servers from others.

### 34.5 Tests

Unmarked servers are left out by `list` and refused by `join` and `kill`, their sessions left
running. A server as cld 0.8.2 started it works as before. A server that loses its mark before
`kill-server` keeps running ([testing](../testing.md)).

## Consequences

Tests that make cld's servers by hand mark them with `set -s @cld 1`.
