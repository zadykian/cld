# tmux findings: sessions and panes

What tmux does with sessions, their names and clients, and the panes they run, as probed for cld.
Probed on Linux unless an entry says otherwise, and on tmux 3.6 where an entry names no release. The
images are those `tests/Dockerfile` builds, one per tmux release, and the snap is Ubuntu's tmux 3.7c
snap. Servers are in [tmux-servers.md](tmux-servers.md), commands, options and formats in
[tmux-commands.md](tmux-commands.md), and what tmux does at the terminal in
[tmux-terminal.md](tmux-terminal.md).

## Targets and names

- **A `.` or `:` in a session name** (3.6). tmux turns each into `_`, as both separate a target's
  parts: `cld foo.bar` made `cld-foo_bar`, while claude and the title kept `cld-foo.bar`. See
  [decision 1](../decisions/0001-naming.md).
- **A target beside a longer name** (3.3a, 3.7c). `-t cld-rev` beside `cld-review` finds
  `cld-review`: tmux matches a session target exactly, then as a prefix, then as a pattern.
  `=cld-rev` matches exactly alone.
- **A bare `tmux new-session` in a pane of cld's server** (3.3a, 3.4, 3.5a, 3.7c). The pane's `TMUX`
  names that server, so the session lands beside cld's, which on one shared server cld took for its
  own ([decision 9](../decisions/0009-only-clds-own-sessions.md)). `kill-session` of cld's session
  leaves the server running, and `kill-server` ends both, giving the panes' programs SIGHUP. See
  [decision 13](../decisions/0013-a-server-per-session.md).
- **`TMUX` where `TMUX_TMPDIR` is a symbolic link** (#79; 3.5a and 3.7c in the images, the snap). A
  pane's `TMUX` names the socket with the link resolved, so cld compares socket paths as files. See
  [decision 46.2](../decisions/0046-idle-sessions.md).
- **A server named `cld-NAME` that cld did not start** (3.7c; cld 0.8.2). That cld took each for its
  own. It refused `new` in its panes, pointed at `kill-server` for its sessions, and its `kill`
  ended the server. Hence the servers' mark, [decision 34](../decisions/0034-servers-are-marked.md).

## Clients

- **`new-session` and `attach-session` without a terminal** (the snap's 3.7c; stdin `/dev/null`, or
  `TERM` `dumb`, empty, unset or unknown). Each fails with status 1 and tmux's message alone, such
  as `open terminal failed: not a terminal`. `new-session` starts the server first and leaves its
  socket behind. With stdout a pipe, tmux draws on stdin's terminal and writes only `[exited]` to
  the pipe. See [decision 31](../decisions/0031-a-terminal-to-attach-from.md).
- **Several terminals on one session** (3.7c). All stay attached, and under the default
  `window-size latest` the window takes the size of the terminal typed in last. A larger terminal
  shows it within a border of `·`. `C-q d` detaches only the terminal typed in, and the hook's
  message reaches the one used last. See
  [decision 23](../decisions/0023-joining-beside-other-terminals.md).
- **cld inside another tmux** (3.6). Nesting works, since cld's socket is another server. tmux
  refuses a client with `TMUX` set only where its tty has the name of one of that server's panes.
- **A dead pane's pty** (3.3a to 3.7c). tmux keeps its name in `#{pane_tty}`, and the system hands
  the name to the next pty opened. A client with `TMUX` set on that pty is refused,
  `sessions should be nested with care`. An empty `TMUX` skips the check, yet still makes the client
  take the terminal for UTF-8. See [decision 2](../decisions/0002-inside-another-tmux.md).

## A pane's environment

- **On a shared server and on a server per session** (3.3a, 3.4, 3.5a, 3.7c). On a shared server a
  second session's pane got the environment of the shell that started the server, but for `PATH` and
  the `update-environment` variables. On servers of their own, each pane got exactly that of the
  shell that made it. See [decision 13](../decisions/0013-a-server-per-session.md).
- **What tmux changes in it** (3.7c). It sets `TERM` to its `default-terminal`, `tmux-256color`, and
  `TERM_PROGRAM` to `tmux`; every other variable, `TERMINAL_EMULATOR` included, reaches the pane as
  it came. An attach updates `SSH_AUTH_SOCK` and `DISPLAY` for later windows only. See
  [decision 33](../decisions/0033-terminal-variables.md).
- **The `PATH` of a new pane** (3.7c, Ubuntu 26.04). The client that ran `new-session` or
  `new-window` gives it, not the server, nor `-e`.

## Dead panes

- **`remain-on-exit-format`** (3.3a, 3.7c). A non-empty format scrolls the dead pane up a line to
  write itself at the bottom, hiding a short error on the top line; an empty one does nothing.
  `#{pane_dead_signal}` is a number on Linux and a name, `term`, on macOS (3.7c). See
  [decision 5](../decisions/0005-failures-stay-on-screen.md).
- **The hook and its options on the window or on the pane** (#78; 3.7c, the image). Set on the
  window, they reach every pane, and the hook reported a pane split off as claude's. Set with `-p`
  on claude's pane, they stay there, and a new pane takes the window's defaults. An `attach-session`
  followed by `if -F '#{pane_dead}'` shows the hint again.
- **Which hook runs as a pane's program exits** (3.5a, 3.7c in the images). With
  `remain-on-exit failed`, status 1 runs `pane-died` wherever set. Status 0 runs neither the pane's
  nor the window's `pane-exited`, as tmux closes the pane first, and a global one only while another
  session keeps the server up.
- **A `pane-died` hook that branches on the status** (3.5a, 3.7c in the images;
  `remain-on-exit on`). Status 0 can remove a file and close the pane, ending a lone session, server
  and client as tmux's own close would. Status 1 and SIGTERM take the other branch. `kill-pane`,
  `kill-window` and `kill-session` run no `pane-died` hook. See
  [decision 48.2](../decisions/0048-restore-after-reboot.md).
- **`display-message` from the `pane-died` hook** (3.5a, 3.7c). With no terminal on the dead pane's
  window, it goes to another session's terminal. With none at all, tmux shows it over the next
  session attached, whose claude gets no keys until `q`. Under `if -F '#{window_active_clients}'` it
  reaches only a terminal on that window.
- **A line of the pane's border from the hook** (3.5a, Debian's and built from source, and 3.7c, in
  Docker). `pane-border-status bottom` draws the pane's `pane-border-format` in its last row, which
  tmux deletes unless the cursor is on it; the line stays through keys and reattaching. Without
  `-t`, the hook's `set` takes the dead pane and its window. Set on the window in a split window,
  the format also shows below the live pane. The text gets the width less 2 cells (3.7c) or 4
  (3.5a): at 80 columns `-s 12` was cut to `-s 1`, another session's name. See
  [decision 5](../decisions/0005-failures-stay-on-screen.md).
- **A format that fits the pane's width** (3.5a, 3.7c in Docker; 120 to 40 columns).
  `#{?#{e|<:#{pane_width},N},SHORTER,TEXT}` picks its branch by the dead pane's width, where the
  line is drawn and the message shown, and again on resize. `#,` in a branch shows as `,`.

## History and width

- **`history-limit`** (3.5, 3.6a and 3.7c in the images; the snap). The default is 2000 lines, and
  tmux drops a tenth on reaching it. Set before `new-session` in one command, 50000 holds on all
  three; set after it, only 3.7c applies it to the existing pane. See
  [decision 36](../decisions/0036-scrollback.md).
- **What a full history costs** (the snap's 3.7c; 50000 lines of 100 characters). The server grew
  from 3.6 MB to 35 MB for plain ASCII, and to 156 MB with an RGB colour every ten characters. Those
  lines took 10 MB under the default 2000.
- **A pane that narrows** (3.3a, 3.7c). tmux reflows the main screen's rows, pushing the top ones
  into the history and moving the cursor with its row. A program that redraws in place relative to
  the cursor then draws over the wrong lines, and recovers only by clearing the screen. See
  [decision 14](../decisions/0014-the-session-list.md).

## Restoring sessions

- **tmux-resurrect and tmux-continuum** (resurrect cff343c, continuum 0698e8f; scripts read).
  resurrect saves the command of the pane program's child, what claude runs rather than claude, and
  restores it by typing into a shell. continuum saves and restores only where no other tmux server
  runs, cld's counted. See [decision 48.10](../decisions/0048-restore-after-reboot.md).
