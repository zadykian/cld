# 2. Inside another tmux

Status: Accepted. Amended by [13](0013-a-server-per-session.md),
[34](0034-servers-are-marked.md), [43](0043-inside-your-own-tmux.md),
[44](0044-detach-command.md), [50](0050-one-command-join.md) and
[51](0051-moving-between-sessions.md).

## Context

cld's private socket is a server of its own, so cld nests inside any tmux. tmux refuses a client
whose tty has the name of one of its panes, dead ones included. The system hands a dead pane's
name to the next pty ([tmux findings](../findings/tmux-sessions.md)).

## Decision

- Inside a tmux that is not one of cld's, cld nests without a check.
- In a live pane of one of cld's servers, both sessions would take `C-q`. There `join` and the
  list's Enter move the terminal on that session instead
  ([decision 51.5](0051-moving-between-sessions.md)).
- cld looks among the live panes itself, asking the server `cld-NAME` that `TMUX` names, where
  it has cld's mark ([decision 34](0034-servers-are-marked.md)). It gives its tmux client an
  empty `TMUX`, which skips tmux's own check.

## Consequences

- A terminal that took a dead pane's tty name is not refused.
- A default outer tmux keeps `C-b`, claude's key to background a task, and turns Shift+Enter into
  Enter. It drops clipboard copies, focus events and, at times, links
  ([tmux terminal findings](../findings/tmux-terminal.md)). No tmux passes claude's
  notifications on but the bell.
- cld names the keys the outer tmux keeps ([decision 43](0043-inside-your-own-tmux.md)), and the
  [user guide](../../guide.md) says what brings back the rest.
