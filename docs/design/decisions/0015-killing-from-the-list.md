# 15. Killing from the session list

Status: Accepted (#24). Amended by [23](0023-joining-beside-other-terminals.md),
[40](0040-session-record.md) and [51](0051-moving-between-sessions.md).

## Context

Ending a session took leaving the list ([decision 14](0014-the-session-list.md)) for `cld kill`. In
Claude Code's agent view, Ctrl+X stops a session, a second press within two seconds deletes it, and
Esc keeps it. The delete also removes a worktree Claude created. That comes from agent view's docs,
read 2026-09-25, and claude 2.1.282's hints ([claude's findings](../findings/claude.md)).

## Decision

In the list, Ctrl+X arms the kill of the selected session, and a second Ctrl+X within two seconds
kills it as `cld kill` does. Esc keeps it. Agent view's two steps are one here: the kill ends
claude, and the row stays as `ended` ([decision 40](0040-session-record.md)), which Ctrl+X twice
then forgets.

### 15.1 Confirming

Once armed, the footer reads `ctrl+x again to kill · esc to keep`, with `and detach its terminal`
where one is attached. Esc, or the two seconds running out, disarms it; another key disarms it and
then does its own work. A key that came in time counts as such even when cld reads it late. Before
that rule, a late Esc now kept the session, now closed the list.

After a kill, Ctrl+X does nothing until none has come for a second. A terminal repeats a key held
down, and a second Ctrl+X held too long killed the rows that took the killed one's place.

Rejected: one press, which lets a stray key end a session and detach whoever is on it, through
Remote Control too. A `y/n` question is not how agent view asks.

### 15.2 The session the list runs in

Ctrl+X twice on the terminal's own row ends that session, and with it the popup or window the list
runs in. `cld kill` run there does the same. The list was not interactive there before
[decision 51.1 and 51.6](0051-moving-between-sessions.md).

### 15.3 The same session

The list kills a row's session only while it holds a pane's pid that the list read. A session made
again has a new claude, and a dead pane keeps its pid. `#{session_created}` counts whole seconds and
`#{session_id}` starts again on a new server, so neither tells a session made again (tmux 3.3a to
3.7c; [tmux's findings](../findings/tmux-sessions.md)). A server that outlived its session
([decision 13](0013-a-server-per-session.md)) has no pane to check, and ends as `cld kill` ends it.

### 15.4 One kill step

The list and `cld kill` share the code: the lookup, then one tmux command that ends the session and
its server ([decision 13](0013-a-server-per-session.md)). The list shows errors in its footer rather
than letting tmux write over it.

### 15.5 After the kill

The list reads the sessions again, as after any read (14.6). It passes over the killed server as
that exits. tmux then reports no server or, now and then, a server that exited unexpectedly (tmux
3.3a to 3.7c; [tmux's findings](../findings/tmux-sessions.md)). The kill runs beside the list, as
Enter's lookup does.

### 15.6 What it leaves

The kill leaves a worktree of `-w` in place, where agent view's delete removes it. Killing several
sessions, a stopped state and removing worktrees are not part of it.

## Consequences

- A kill by mistake costs the session, not the conversation: `join` brings it back
  ([decision 16.9](0016-resume.md)).
- A key held down no longer walks the list, killing sessions nobody selected.
