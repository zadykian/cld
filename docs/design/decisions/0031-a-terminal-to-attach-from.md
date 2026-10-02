# 31. A terminal to attach from

Status: Accepted (#63). Amended by [41](0041-claude-options.md), [48](0048-restore-after-reboot.md),
[50](0050-one-command-join.md) and [51](0051-moving-between-sessions.md).

## Context

Run without a terminal - from cron, `ssh host cld ...` without `-t`, a script - cld printed the
title and became tmux, which failed with status 1. Where it created the session, its server had
started first and left its socket, which `list` and every TAB went on asking (tmux 3.7c;
[tmux session findings](../findings/tmux-sessions.md)).

## Decision

### 31.1 What counts as a terminal

cld's stdin is a terminal, and `TERM` is set, not empty and not `dumb`. These are the list's
conditions ([decision 14.1](0014-the-session-list.md)) without stdout, which tmux does not draw on,
and without the foreground. Otherwise `join` ends with status 1, naming what is missing
([decision 50.3](0050-one-command-join.md)). cld reads no terminfo: a `TERM` it does not know stays
tmux's to refuse.

### 31.2 Checked last

The check comes just before the title, once every other has passed, so that each of those says the
same without a terminal. The length of tmux's command ([decision 41.5](0041-claude-options.md))
comes before it. Writing the session's entry ([decision 40.1](0040-session-record.md)) comes after
it, so that a refused `join` leaves cld's record unchanged
([decision 48](0048-restore-after-reboot.md)).

### 31.3 The message

It says what is missing, and no more. Most ways into it involve no ssh, so `ssh -t` is in the
[user guide](../../guide.md)'s troubleshooting.

### 31.4 The title only to a terminal

cld prints the title only where stdout is a terminal. tmux draws on stdin's terminal, and a pipe or
a file took the escape in as text.

### 31.5 Not done

A detached start from the command line: only `restore` starts sessions detached
([decision 48](0048-restore-after-reboot.md)). The list's check of the foreground. In a pane of
cld's servers, `join` moves the terminal on that session instead, whose own `cld join` makes this
check ([decision 51](0051-moving-between-sessions.md)).

### 31.6 Tests

Each command runs without a terminal and with `TERM` `dumb`, empty or unset: status 1, the message,
nothing on stdout and no socket left. Refusals that come first still run without a terminal, which
pins the order of 31.2 ([testing](../testing.md)).

## Consequences

cron and plain ssh get a refusal that names the cause, and leave no socket behind.
