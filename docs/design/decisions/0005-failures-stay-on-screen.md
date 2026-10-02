# 5. Failures stay on screen

Status: Accepted. Amended by #64, #78, [24](0024-names-from-the-repository.md),
[44](0044-detach-command.md), [48](0048-restore-after-reboot.md) and
[50](0050-one-command-join.md).

## Context

claude exits with status 0 unless it fails ([claude findings](../findings/claude.md)). What a
failed claude printed, a startup error above all, vanished with its session. tmux's
`remain-on-exit-format` scrolls a dead pane up a line. A `pane-died` hook's message reaches
another session's terminal where none is on the window
([tmux findings](../findings/tmux-sessions.md)).

## Decision

- A claude that exits with an error or a signal keeps its pane, with an empty format. One that
  exits with status 0 closes it, once the run mark is gone
  ([decision 48.2](0048-restore-after-reboot.md)).
- The `pane-died` hook says how to detach and how to end the session, named `-n NAME -s SUFFIX`.
  It says so on the message line, to a terminal on claude's window alone, and on a line of the
  pane's border (#64).
- Each text is what fits the pane's width whole.
- The options and the hook go on claude's pane (#78), and on its window only what tmux reads
  from the window alone.
- `join` of such a session attaches and shows the message; `list` shows the session as `exited`.

## Consequences

- A short error on the top line stays in view: the empty format scrolls nothing, and the border
  line takes the pane's last row.
- The message line goes at the first key, which left a claude that crashed mid-session looking
  hung. The border line stays through keys, detach and `join`.
- Set on claude's window, the options kept any other failed pane there dead with cld's hint,
  such as a teammate's that claude's agent teams split off.
- tmux would cut a longer text at the width, and a command cut short could name another session:
  `-s 1` of `-s 12`.
- The hook takes some 1 KB of tmux's command, leaving that much less for claude's words
  ([decision 41.5](0041-claude-options.md)).
