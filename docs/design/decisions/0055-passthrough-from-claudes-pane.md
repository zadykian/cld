# 55. Passthrough from claude's pane

Status: Accepted (#167).

## Context

claude wraps its notifications and OSC 52 copies in tmux passthrough
([decision 29](0029-notifications.md)). cld set `allow-passthrough on` server-wide, under which
tmux drops a pane's passthrough for a client with a redraw due
([tmux terminal findings](../findings/tmux-terminal.md#notifications-and-links)). Decision 29
took it that each terminal attached gets them.

A redraw is due after an attach, each answer to tmux's queries, a resize, and any option set,
cld's status hooks among them. It stays due while output to the terminal is pending, as on a slow
link or in a busy terminal. On tmux 3.7, and from 3.7b, it follows each frame claude draws, in
synchronized output. Under `on`, tmux also drops passthrough from a pane not shown: in a window the
terminal does not show, or beside a zoomed pane. tmux's default keys behind `C-q` make both.

A test froze the terminal, had tmux defer a redraw, and had claude notify. Under `on` the
notification never arrived ([testing](../testing.md#what-the-tests-found)). The wait of
[decision 54.5](0054-ghostty.md) sent its beacon again until it arrived, for the same reason.

This amends decisions [29](0029-notifications.md) and [54.5](0054-ghostty.md).

## Decision

### 55.1 `all` on claude's pane

claude's pane gets `allow-passthrough all`, beside the failure's options
([decision 5](0005-failures-stay-on-screen.md)). tmux then hands its sequences to each terminal on
the session, whatever window it shows, and while a redraw is due. It does so too while tmux holds
the terminal for a message, a prompt or `display-panes`.

The server keeps `on`, so that the other panes keep what they had. `all` server-wide was not taken:
a program in a hidden pane, an image previewer say, would draw over claude. claude's own
passthrough is copies and notifications, which draw nothing. Nor does a program in claude's pane
gain a power: `TMUX` already lets it run tmux on the server.

A program claude hands its pane to, the editor of `Ctrl+G` say, gets `all` too: an image it drew
could show over another window. That was accepted, as tmux sets the option per pane, not per
program, and such a program runs only while claude waits on it.

### 55.2 Running sessions keep theirs

cld sets these options as it makes a session, and `join` of a running one leaves them as they are.
A session made before keeps `on` until it ends and `join` makes it anew, as the guide's
[Upgrading](../../guide.md#upgrading) says of every release.

### 55.3 Tests

C5 freezes the terminal, has tmux defer a redraw, and checks that claude's notification arrives
once the terminal reads again. JediTerm's driver cannot freeze, and skips it. C5 also checks that a
terminal showing another window of the session gets it. `waitAnswered` sends its beacon once,
which still follows what tmux wrote at the answers.

## Consequences

A terminal gets claude's notifications while it shows another window of the session, or another
pane zoomed. Still lost, under any setting: what claude sends in passthrough while its pane is in a
mode, or while tmux discards output to a terminal that cannot keep up. The bell still arrives from
copy mode. None reaches a session with no terminal attached ([decision 29](0029-notifications.md)).

A pane that claude's agent teams split off keeps `on`
([claude findings](../findings/claude.md#inside-a-session)). A teammate's copies and notifications
still drop while a redraw is due, or while its pane is not shown. A hook that set `all` on each
split was not taken: it would set it on the user's panes too, the image previewer of 55.1.
