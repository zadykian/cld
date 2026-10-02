# tmux findings: servers and sessions

What tmux does with servers, sessions, clients, sockets, command lists, options, formats, hooks and
panes, as probed for cld. Probed on Linux unless an entry says otherwise, and on tmux 3.6 where an
entry names no release. The images are those `tests/Dockerfile` builds, one per tmux release, and
the snap is Ubuntu's tmux 3.7c snap. What tmux does at the terminal is in
[tmux-terminal.md](tmux-terminal.md).

## Releases

- **`tmux -V` beside an older server** (a 3.5a server, a 3.7c client; for #21 a 3.6 server too).
  `-V` reports the client, while the server keeps running the tmux that started it and serves the
  newer client. A version check passes after an upgrade while old servers run on.
- **Attaching to an older server** (#68: 3.5a and 3.6a servers; 3.5a, 3.6a and 3.7c clients).
  No 3.6a or 3.7c client attaches to a 3.5a server: `open terminal failed: not a terminal`. A 3.7c
  client attaches to a 3.6a server, which gives it no `extkeys`: the table of terminals is the
  server's.
- **What cld needs of tmux** (#74: the suite on 3.5, 3.5a and 3.6a in the images; `CHANGES` to 3.7c
  read). 3.5a, 3.6a and 3.6b pass once `paste-buffer -S` is kept to 3.7 and newer; 3.5 fails only
  the version checks, and 3.4 fails more. cld's options and commands date from 3.2 or 3.3, and
  extended keys' mode 2 from 3.5. See [decision 6](../decisions/0006-versions.md).

## Running a program

- **A command as one string, or as several words** (3.6; tmux(1)). One string runs through `sh -c`:
  `cld "a b"` gave claude `--name cld-a b`, and `b` became a prompt. Several words run directly,
  each arriving as one argument.
- **How tmux starts such a command** (3.7c, glibc 2.43). By `execvp` in `-c`'s directory, with the
  `PATH` of the client that ran `new-session`, relative entries included: `PATH=.:/abs` ran the
  directory's `claude`, not the checked `/abs/claude`. A file the system will not execute
  (`ENOEXEC`), a script without `#!` or an arm64 binary on x86_64 (glibc 2.41), runs with `/bin/sh`.
  See [decision 6](../decisions/0006-versions.md).
- **A `-c` directory its user cannot enter** (3.7c, glibc 2.41, in the image). tmux starts the
  command in the home directory, printing nothing, and exits 0.

## Parsing a command

- **An argv word ending in `;`** (the parser read in 3.3a and 3.7c; run on 3.3a, 3.4, 3.5a
  and 3.7c). It ends the tmux command, the text before the `;` staying an argument. A word ending in
  `\;` arrives with the `;`, and a `;` inside a word is left alone. See
  [decision 16.8](../decisions/0016-resume.md).
- **Other words of a program's command** (3.5a, 3.7c). Each arrives as given: an empty word as an
  empty argument, and `#{session_name}` unexpanded.
- **A `#` in `new-session -c`** (3.3a, 3.4, 3.5a, 3.7c). tmux expands `-c` as a format after
  splitting at `;`: `C#S` became `C`, and `x#(touch ran)` ran `touch ran`. A `-c` naming no
  directory starts the program in the home directory (3.3a: where the server started), and `##` is a
  `#`. See [decision 16.8](../decisions/0016-resume.md).
- **A value of several lines in an `if`'s command** (#112; 3.5a, 3.7c). Within quotes, tmux's parser
  drops the spaces and tabs after each newline, and takes a line starting with `#` for a comment. A
  command handed over as a word of its own keeps them. See
  [decision 51](../decisions/0051-moving-between-sessions.md).
- **The longest command a client sends** (3.7c, the snap's binary, and 3.5a in the image; `client.c`
  read). The words, each ended by a NUL, go in one message of at most 16384 bytes, which leaves
  16364 for them. One byte more fails with status 1, after starting a server that ends and leaves
  its socket. cld's own words measured 5967 bytes in the image, with tmux at a path of 97
  characters, and 5029 at the sandbox's paths. Counted, they came to about 8.1 KB in the image by
  decision 51, and some 7 KB at the sandbox's paths. See
  [decision 41.5](../decisions/0041-claude-options.md).

## Command lists

- **What follows `new-session` in one command** (3.3a to 3.7c). It takes effect before tmux sees the
  new pane's program exit, even at once, and is skipped where `new-session` fails. In a command that
  takes a pane, `=NAME` finds nothing: `=NAME:` names the session.
- **A `run-shell` after `new-session`** (#115; 3.5a, 3.7c in the images). It runs before the client
  returns, and not at all where a name taken or an unknown `TERM` fails `new-session`, which leaves
  no server. A failing one fails the client unless its command ends in `|| true`. A program that
  exits at once has its `pane-died` hook run after it. See
  [decision 48.1](../decisions/0048-restore-after-reboot.md).
- **A `run-shell` around the kill** (#115; 3.5a, 3.7c). Before `kill-session`, the session holds its
  name while the shell runs, so a `new-session` of the name fails. After it, the server serves other
  clients without the session until `kill-server`, which ends a session made meanwhile. An idle
  sweep that checks again after its `run-shell` keeps a session attached meanwhile.
- **The idle kill under `if -F`** (#79; 3.5a and 3.7c in the images, the snap).
  `if -F -t =S: COND 'kill-session -t =S ; kill-server'` ends both only where the condition holds;
  without session `S` the condition expands with no session and fails. A failing command in the
  string ends the rest. `e|<` compares whole numbers unless given `f`, an empty side as 0. See
  [decision 46.4](../decisions/0046-idle-sessions.md).

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
- **A server after its kill** (3.3a, 3.7c, 200 tries). Once `kill-server` has returned, the server
  answers no more. After `kill-session` of its last session, it still answered once or twice.
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

## Options and formats

- **A user option in a format** (`#{@cld}`; 3.3a, 3.7c). tmux looks it up in the server's, the
  pane's, the window's and the global window options before the session's. A server or window value
  counts for every session without one, and a window's hides the session's.
- **The servers' mark** (3.7c, the snap and the image). On a server started with `set -s @cld 1`,
  `#{@cld}` reads `1` for every session, and no window's or session's value hides it. `#{prefix}`, a
  session option, reads `C-q` on servers of cld 0.8.2 and earlier, and a session that sets its own
  reads that. `display-message -p` reads both on a server without a session. See
  [decision 34](../decisions/0034-servers-are-marked.md).
- **A user option's value** (`@cld-home`; 3.7c, the image; `format.c` and `options.c` read). `set`
  keeps it as given, tabs, an inner `;` and `#{...}` included; a `;` at its end ends the command
  unless written `\;`. `#{n:...}` is its length in bytes, `0` where unset. See
  [decision 37](../decisions/0037-sessions-of-another-repository.md).
- **Spaces in a `list-sessions -F` conditional** (#113; 3.5a, 3.7c). A space in a branch of
  `#{?...}` stays, in nested conditionals and an empty last branch too. cld's state field thus read
  `detached busy` and the like, `detached ` for an unknown status, and `exited` alone for a dead
  pane. See [decision 49](../decisions/0049-status-in-the-list.md).
- **`#{session_created}`, `#{session_id}` and `#{pane_pid}`** (3.3a, 3.4, 3.5a, 3.7c). The first
  counts whole seconds, and ids start again at `$0` on a new server, so neither tells a session made
  again from the killed one. A session's `pane_pid` is its active pane's, a dead pane's too;
  `#{W:#{P:#{pane_pid} }}` gives every pane's.
- **What moves `#{session_activity}` and `#{session_last_attached}`** (#79; 3.5a and 3.7c in the
  images, the snap). Activity moves as the session is made, on attach, with each key a client types,
  `C-q d` included, and as a suspended client wakes. A pane's output, and commands such as
  `send-keys`, `set` or `detach-client`, move neither. `session_last_attached` moves on attach and
  is empty where none has attached; both are whole seconds. See
  [decision 46.1](../decisions/0046-idle-sessions.md).
- **A format that tells a server that outlived session `cld-x`** (3.5, 3.5a, 3.6a and 3.7c in
  Docker; the snap). `#{&&:#{S:1},#{&&:#{==:#{N/s:cld-x},0},#{==:#{b:socket_path},cld-x}}}` gives 1
  on a server holding only other sessions. It gives 0 with `cld-x`, with no session, or on another
  server's socket reached through a link. `if -F` with it acts on the state it read, as the server
  runs one client's queued commands before another's. Before 3.6 there is no `#{!:}`, and `#{&&:}`
  takes two operands. See [decision 13](../decisions/0013-a-server-per-session.md).

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

## Sockets

- **The socket of `tmux -L NAME`** (3.3a, 3.4, 3.5a, 3.7c). tmux never removes it, whether the
  server exits, is killed or gets SIGKILL. On such a stale socket `list-sessions` says
  `no server running`, and `new-session` starts a fresh server. It lives in `tmux-UID` under
  `TMUX_TMPDIR`, or `/tmp` where that is unset, empty or missing. See
  [decision 38](../decisions/0038-stale-sockets.md).
- **The socket path's length** (3.3a, 3.4, 3.5a, 3.7c, as UID 0 and 1000). 107 bytes work on Linux,
  and 108 fail with `File name too long`. A NAME of 64 characters overflows from a `TMUX_TMPDIR` of
  32 characters as UID 0, or 29 with a four-digit UID. See
  [decision 13.2](../decisions/0013-a-server-per-session.md).
- **Connecting to the sockets from Go** (Go 1.27.1, Linux 7.0; the snap's 3.7c, 3.5a and 3.7c in the
  images). A stale socket or a plain file refuses the connection, and an unused name has no socket.
  tmux's client takes those alone for no server. A live server drops Go's connection as a client's
  and keeps its session; 61 sockets took 1.1 to 3.6 ms in all. tmux also refuses a socket directory
  others can use, a live server's too. See [decision 38](../decisions/0038-stale-sockets.md).
- **A socket directory that ignores case** (3.7c; a casefold tmpfs, Linux 7.0). `tmux -L cld-A`
  reaches server `cld-a`, whose `#{socket_path}` keeps the name it started with. Over a stale socket
  `cld-a`, `new-session` on `cld-A` starts a server named `cld-A`. macOS's APFS was not checked. See
  [decision 13.6](../decisions/0013-a-server-per-session.md).

## Servers ending

- **A terminal attached when its server is killed** (3.3a, 3.4, 3.5a, 3.7c; `server.c` read).
  `kill-server` alone ends the client with `[server exited]`, status 1; with `kill-session` before
  it in one command, `[exited]` and status 0. Until its clients have gone, it fails new ones with
  `server exited unexpectedly`, in 4 of 86 rounds on 3.7c. 3.3a showed none in 99. `cld kill` then
  `cld new` of a name still reached a fresh server in 100 of 100 rounds. See
  [decision 13](../decisions/0013-a-server-per-session.md).
- **A server that outlives its session, and one without a session** (3.7c, the snap; `server.c`
  read). A server runs on with a session claude's pane made once claude's own has ended. One with
  `exit-empty off` and no session answers `list-sessions` with nothing, status 0. So would a
  starting server, which listens before it runs the starting command (read, not seen).
- **A lookup while another cld's tmux starts the server** (#114; 3.7c in the image). tmux makes the
  server and its socket before `new-session` runs, so a lookup can find an empty server with cld's
  mark. Read before the lookup, the start mark let `join` look again and attach; read after, it came
  too late. See [decision 50.4](../decisions/0050-one-command-join.md).
- **`list-sessions` on a server that exits as it asks** (3.3a, 3.4, 3.5a, 3.7c; 15 s loops). The
  server closes the connection unanswered, and tmux fails with `server exited unexpectedly`,
  status 1. `cld list` failed so in 24 of 173 runs (3.3a), 46 of 294 (3.4), 19 of 310 (3.5a) and 6
  of 147 (3.7c). Passing over such a server, it failed in none. See
  [decision 13.1](../decisions/0013-a-server-per-session.md).
- **`list-sessions` right after a kill** (3.3a, 3.4, 3.5a, 3.7c, in Docker; 3.7c natively too). In
  single loops of 400, the last `kill-session` always gave no server. `kill-server` failed with
  `server exited unexpectedly` in 31 rounds on 3.3a and 44 on 3.4. Eight loops at once (2400 rounds)
  failed after `kill-session` in 40, 19, 0 and 10, printing nothing in 53, 17, 0 and 1. They failed
  after `kill-server` in 5 on 3.5a and 30 on 3.7c. The next `list-sessions` always said no server,
  and natively all 3200 rounds on 3.7c did at once. See
  [decision 15.5](../decisions/0015-killing-from-the-list.md).
- **What a kill leaves of the pane's program** (the snap's 3.7c; a script standing in for claude,
  sleeping 1.5 s on SIGHUP). tmux sends SIGHUP and does not wait: `cld kill` returned after 0.52 s,
  and the script ran on under PID 1, its last write failing with `Input/output error`. See
  [decision 32](../decisions/0032-what-a-kill-does.md).

## Costs

- **What a server per session costs** (3.3a, 3.4, 3.5a, 3.7c, in Docker on a busy host). A server
  holds 3.8 MB (3.3a) to 5.1 MB (3.5a), next to about 400 MB for claude. One `list-sessions` took 6
  to 12 ms. One per socket over 36 sockets, 16 of them stale, took 136 to 282 ms. On an idle
  host, #22's plan measured 3 to 6 and 115 to 170 ms (3.3a, 3.7c). See
  [decision 13.5](../decisions/0013-a-server-per-session.md).
- **What stale sockets cost, and asking servers at once** (the snap's 3.7c, 8 CPUs, busy; 50 stale
  sockets and 10 servers). One tmux took 100 to 180 ms, and the 10 servers 1.2 to 1.5 s in turn,
  or 0.24 to 0.26 s eight at a time. Connecting first and asking eight at a time cut `cld list`
  from 7.2 to 8.6 s to 0.35 to 0.44 s. It cut `cld new` over 31 stale sockets from 4.2 to 4.5 s
  to 0.28 s. See [decision 38](../decisions/0038-stale-sockets.md).
- **How long completion takes** (3.7c in the image, load about 2; natively the snap).
  `cld __complete join -n ''` took 3 ms with no socket, 22 ms with four sessions and 147 ms over 36
  sockets, close to `cld list`. On the snap, whose clients take about 150 ms to start, four sessions
  took 625 ms.

## Restoring sessions

- **tmux-resurrect and tmux-continuum** (resurrect cff343c, continuum 0698e8f; scripts read).
  resurrect saves the command of the pane program's child, what claude runs rather than claude, and
  restores it by typing into a shell. continuum saves and restores only where no other tmux server
  runs, cld's counted. See [decision 48.10](../decisions/0048-restore-after-reboot.md).
