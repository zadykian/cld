# 23. Joining beside other terminals

Status: Accepted. Amended by [50](0050-one-command-join.md).

## Context

`cld join` detached every other terminal. Leaving them attached lets two terminals, a laptop's and
a desktop's, show one claude, and a join typed in the wrong tab takes nothing from anyone. With tmux
3.7c both stay attached, and the window takes the size of the terminal used last; a larger one shows
the rest dotted ([tmux session findings](../findings/tmux-sessions.md)).

## Decision

### 23.1 `--detach-others`, long only

`join` attaches beside the others, and `--detach-others`, as the maintainer named it, detaches them.
Long only, it leaves tmux's `-d` free.

### 23.2 The window's size

cld keeps tmux's default `window-size latest`, so claude redraws as the terminals take turns.

### 23.3 The list joins alike

The list's Enter joins alike ([decision 14.5](0014-the-session-list.md)), and warns only of the
armed kill ([decision 15.1](0015-killing-from-the-list.md)). It has no key for `--detach-others`:
`C-q d` in the other terminal takes a session over.

### 23.4 Detach, kill and the hint

`C-q d` detaches only its own terminal, and `cld kill` ends them all with status 0. The hint of a
failed claude reaches the terminal used last ([decision 5](0005-failures-stay-on-screen.md)).
`list` says `attached` whatever the count.

### 23.5 Tests

The tests attach two terminals, and detach them one at a time and with `--detach-others`, from
`join` and the list ([testing](../testing.md)).

## Consequences

`join` also brings back or creates a session, and `--detach-others` is never refused as lost
([decision 50.1](0050-one-command-join.md), [50.2](0050-one-command-join.md)). Out of scope: a key
in the list for `--detach-others`, a short option, and counting terminals in `list`.
