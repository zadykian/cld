# 44. Detaching without the key

Status: Accepted (#72). Amended by [50](0050-one-command-join.md) and
[51](0051-moving-between-sessions.md).

## Context

Some terminals keep `C-q` from tmux. In VS Code on macOS and Windows, and its remote windows,
`Ctrl+Q` is Quick Open View, which its terminal leaves to VS Code. In JetBrains IDEs with the Visual
Studio 2022 keymap, bundled with Rider, `Ctrl+Q` runs Find Action. Closing the tab still detaches,
as does `join --detach-others`, but cld named `C-q d` alone, a key that did nothing there.

## Decision

`cld detach [-n NAME] [-s SUFFIX]` detaches terminals from a session as `C-q d` does. tmux 3.5a and
3.7c behave alike here ([tmux terminal findings](../findings/tmux-terminal.md)).

### 44.1 With `-s`

`detach` looks the session up as `kill` does, connecting to the socket first
([38](0038-stale-sockets.md)). It refuses what `kill` refuses: no session, an ended one
([50.10](0050-one-command-join.md)), a server cld did not start ([34](0034-servers-are-marked.md)),
another repository's session ([37](0037-sessions-of-another-repository.md)). A server that runs
without its session it leaves to `kill` ([13](0013-a-server-per-session.md)). Every terminal on the
session goes. With none there, `detach` does nothing and exits 0: `detach-client -s` would fail with
`no current client` where no terminal is attached to the server at all.

### 44.2 In claude, `! cld detach`

Without `-n` and `-s`, where `TMUX` names one of cld's servers, `detach` has tmux detach the
terminal used last on the pane's session. That is the one the command was typed in, unless another
has since had a click, a focus event or the mouse moving over it. No format tells a key from these,
and the gap between Enter and the command is short, so cld leaves the choice to tmux. claude runs a
shell command without a terminal ([claude findings](../findings/claude.md)), so `detach` does not go
by `tty`.

`detach` runs its tmux command by the socket `TMUX` names, with cld's input and environment: tmux
finds the pane by the terminal, or else by `TMUX_PANE`. Unlike a lookup
([38.1](0038-stale-sockets.md)), it connects to nothing first; where that server has gone, tmux's
message ends `detach` with status 1.

It checks for a terminal on the session first: with none, tmux would detach a terminal of another
session on the server, one claude made. `-n` without `-s` is refused, rather than detach a terminal
of a session it does not name. On a server `cld-NAME` without cld's mark, `detach` refuses, pointing
at `-s`. `join` came to act in these panes too ([51.3](0051-moving-between-sessions.md)).

### 44.3 The hint names it

The `pane-died` hint ([5](0005-failures-stay-on-screen.md)) names `cld kill` and then
`C-q d or cld detach`, since a claude that exited runs no `!`. `kill` goes first, as ending the
session is what the hint is for, and a narrow pane drops the detach first. For `-s 1` the hint takes
105 columns, where it took 86. The root help names `! cld detach`. The guide says how to give `C-q`
back to tmux in those IDEs.

### 44.4 Completion and checks

`detach -n` and `-s` offer the sessions that run ([50.7](0050-one-command-join.md)). `detach` starts
no claude, and makes the checks `kill` makes.

### 44.5 Not taken

`--others`, to detach every terminal but this one: `join --detach-others` does as much.

### 44.6 Tests

On tmux 3.7c and 3.5a, `TestDetach` runs `detach` as claude runs it. The terminal a key was typed in
last goes, whichever attached first. Without either check of a terminal, or without cld's input
handed to tmux, the test fails. Others cover no terminal attached, a server gone or not cld's, and
the hint's widths ([testing](../testing.md)).

## Consequences

A bare `detach` can take the wrong terminal where another has just had the mouse over it, as tmux
offers nothing better to go by.
