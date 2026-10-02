# 29. Notifications

Status: Accepted (#59).

## Context

claude notifies only on the channel its setting `preferredNotifChannel` names. Its default, `auto`,
sends nothing where `TERM_PROGRAM` is `tmux`, as in claude's pane (claude 2.1.284;
[claude findings](../findings/claude.md)). With `allow-passthrough on`, tmux passes a channel's
sequences, and the bell, to every terminal attached, and drops them while none is (tmux 3.7c;
[tmux terminal findings](../findings/tmux-terminal.md)).

## Decision

### 29.1 cld sets no channel

cld could put a channel for the terminal attached in its `--settings`. But those outrank the user's
settings, and a terminal that joins later, or beside the first
([decision 23](0023-joining-beside-other-terminals.md)), may take another sequence.

### 29.2 The docs name the channels

The README points at the [user guide](../../guide/terminal.md#notifications), which names the
channel each terminal takes.

### 29.3 Tests

C5 has the probe notify on four channels as claude 2.1.284 writes them under tmux. Each terminal
must get the sequences and the bell. Which channel `auto` picks is claude's
([testing](../testing.md)).

## Consequences

A user who sets no channel gets no notifications in cld's sessions. Out of scope: a notification
while no terminal is attached.
