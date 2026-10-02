# 9. Only cld's own sessions

Status: Accepted (0.2.1). Superseded by [13](0013-a-server-per-session.md).

## Context

Everything claude runs inherits `TMUX`, so a bare `tmux new-session` in claude's pane landed on
cld's shared server, where cld took it for one of its sessions
([tmux findings](../findings/tmux-sessions.md)).

## Decision

- `new` marked each session it made, in the same tmux command. `@cld` held the session's id,
  since tmux looks a user option up in other scopes before the session's.
- `list` showed marked sessions only, and the other commands refused a name an unmarked session
  held.
- What cld set for a failed claude ([decision 5](0005-failures-stay-on-screen.md)) went on
  claude's window, so an unmarked session closed as tmux would close it.

## Consequences

- Starting claude without `TMUX` was rejected: it would cut claude's tmux off cld's server, and
  its passthrough and copies with it.
- A server per session was rejected then, as it would still need a mark, and `list` would have
  to find the servers. [Decision 13](0013-a-server-per-session.md) answered both.
- The mark guards against mistakes, not intent: whatever reaches the socket can set it.
  [Decision 34](0034-servers-are-marked.md) marks servers instead.
