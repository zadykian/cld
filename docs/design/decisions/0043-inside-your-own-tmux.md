# 43. Inside your own tmux

Status: Accepted (#68). Amended by [50](0050-one-command-join.md) and
[51](0051-moving-between-sessions.md).

## Context

cld can run in a pane of the user's own tmux, one not cld's
([decision 34](0034-servers-are-marked.md)). That tmux takes its prefix before claude. Where its
extended keys are off, the default, Shift+Enter arrives as `\r`. It also holds back clipboard
copies, focus events and notifications (tmux 3.5a to 3.7c;
[tmux terminal findings](../findings/tmux-terminal.md)).

## Decision

`join` and the list's Enter name the keys that tmux keeps on cld's message line once attached, until
a key. For a default tmux:
`your tmux keeps C-b and Shift+Enter: see "Inside your own tmux" in cld's guide`. That section says
what each loss costs, and the `~/.tmux.conf` lines that bring back what they can. After a move to
another session, `TMUX` is empty and nothing is named ([51.3](0051-moving-between-sessions.md)).

### 43.1 What is read

cld asks that tmux for its prefixes and `extended-keys` for cld's pane, and for the features of the
terminal last active there. Kept are the prefixes but `None`, and Shift+Enter, unless
`extended-keys` is `always`, or `on` with cld's tmux 3.7 or newer and `extkeys` among the features.
With `on`, tmux passes modified keys only to a client that asks. cld's tmux asks a tmux for them
only from 3.7, and tmux asks a terminal only where it has `extkeys`.

The client's release counts, cld's own tmux, but the server's table of terminals decides `extkeys`.
So under cld's tmux 3.7, an outer server of 3.6 with `on` keeps a Shift+Enter that the line leaves
out. Empty features, as with no client attached, count as none. Nothing is said where that tmux does
not answer, or finds another pane. On a server with cld's mark, `join` moves the terminal instead
([51.5](0051-moving-between-sessions.md)). The read costs 6-20 ms natively, and 100-200 ms through
Ubuntu's snap ([38](0038-stale-sockets.md)).

### 43.2 Keys only

Clipboard copies, focus events, links ([30](0030-links.md)) and notifications take no key, so the
guide alone covers them. Its lines bring back all but the prefix and the notifications, of which
only the bell passes.

### 43.3 Where the line shows

The command that attaches shows the line, to that terminal alone. The `pane-died` hint of a claude
that exited takes its place. A warning before the exec would show only after detach. The line goes
unexpanded (`-l`), as [51.1](0051-moving-between-sessions.md)'s messages do; tmux hides the
terminal's cursor while it shows. `-C` keeps claude drawn meanwhile, where tmux would draw nothing
until the key. The line fits 80 columns, and names the guide alone where three keys leave no room
for more.

tmux 3.5a refuses the whole command with `-C`, new in 3.6. Under cld's tmux 3.5, cld names nothing,
and the guide's section stands alone. Newer clients attach to no 3.5a server, so `-C` reaches none
that refuses it. Rejected on 3.5: the line without `-C` for a few seconds, holding claude back as
long.

### 43.4 Drawn again

With `status off` the message is claude's last line, and claude's fullscreen renderer draws over it
as it starts. tmux draws the message again on a full redraw, so the command asks for two, a second
and three seconds after it attached. With the line they take some 180 bytes of tmux's limit
([41.5](0041-claude-options.md)). `C-q ~` lists the line later. Rejected: a pause before the exec; a
popup, which takes the key; a status or border line, which takes a line from claude.

### 43.5 Every attach

A tmux has a prefix unless set to `None`, so the line comes on every attach there. The prefix is a
key lost, and the line costs none. Rejected: naming the prefix only beside Shift+Enter, the line
only on creation, or until acknowledged once.

### 43.6 Tests

Two tests run cld in a pane of a tmux of their own, set up as a default tmux, with the guide's
lines, with two prefixes, and keeping nothing. They check the line, its going with the first key,
how the keys arrive, and an unmarked `cld-yours`. They run on 3.7c and 3.5a in CI, and fail without
the checks for 3.6 and 3.7 ([testing](../testing.md)).

## Consequences

Under tmux 3.5 nothing names the lost keys. A user who has set their tmux up still sees the prefix
named on each attach.
