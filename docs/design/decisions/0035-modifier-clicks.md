# 35. Clicks with a modifier reach claude

Status: Accepted (#73).

## Context

Of tmux's mouse bindings on a pane, only `C-MouseDown1Pane` (`swap-pane -s @`) and
`M-MouseDown3Pane` (the pane menu) take the press without asking whether the pane's program takes
the mouse. A Ctrl+click reached claude as its release alone, an Alt+right-click not at all (tmux
3.7c; [tmux terminal findings](../findings/tmux-terminal.md)). claude opens a link only on the
release of a click whose press it saw, so under cld no Ctrl+click opened one (claude 2.1.284;
[claude findings](../findings/claude.md)).

## Decision

### 35.1 Two keys unbound

cld's server unbinds both with its other options, after `mouse on`. tmux hands a mouse key with no
binding to the pane, which is claude's where claude takes the mouse. Unbinding a key that is not
bound is no error, so the server's options can be set again.

### 35.2 Panes claude makes

Key tables are the server's, so the panes claude's agent teams split off lose the two bindings too.
Accepted: `C-q {` and `C-q }` still swap panes, and `C-q >` opens the pane menu. Rejected: bindings
that ask, as tmux's others do, at the cost of a copy of tmux's pane menu in cld.

### 35.3 What stays

tmux's own handling stays over a program that does not take the mouse, such as a claude that failed.
There a drag selects, a middle-click pastes and a right-click opens the menu. Under tmux, claude
opens a link on a Ctrl or Alt click alone, as `TERM_PROGRAM` and XTVERSION are tmux's there. The
terminal's own click on links is the terminal's ([decision 30](0030-links.md)). The
[user guide](../../guide.md) says so, and how to select with the terminal instead.

### 35.4 Tests

The root table lacks both keys and keeps `MouseDown1Pane`. C4 types a Ctrl+click and an
Alt+right-click and waits for each press and release in claude's input, on each terminal
([testing](../testing.md)).

## Consequences

A session an older cld started keeps tmux's two bindings until it ends.
