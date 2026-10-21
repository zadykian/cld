# tmux findings: commands

How tmux runs a session's program, and parses and runs a command, its options and formats, as probed
for cld. Probed on Linux unless an entry says otherwise, and on tmux 3.6 where an entry names no
release. The images are those `tests/Dockerfile` builds, one per tmux release, and the snap is
Ubuntu's tmux 3.7c snap. Servers are in [tmux-servers.md](tmux-servers.md), and sessions, clients
and panes in [tmux-sessions.md](tmux-sessions.md).

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
  decision 51. At the sandbox's paths they came to 7.8 KB, and 8.8 KB with decision 56's hooks.
  There, tmux at a path of 97 characters made it 9.8 KB. See decisions
  [41.5](../decisions/0041-claude-options.md) and [56.2](../decisions/0056-following-claudes-cd.md).

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
- **A directory in a `run-shell`'s command** (3.7c, run; 3.5a's `format.c` and `server-client.c`,
  read). `run-shell -t` expands the pane's `#{pane_current_path}` raw: 3.5a escapes control
  characters only as a client prints them. `#{q:}` escapes no newline, but
  `'#{s/'/'\\''/:pane_current_path}'` hands sh any path whole. See
  [decision 56.3](../decisions/0056-following-claudes-cd.md).
- **The idle kill under `if -F`** (#79; 3.5a and 3.7c in the images, the snap).
  `if -F -t =S: COND 'kill-session -t =S ; kill-server'` ends both only where the condition holds;
  without session `S` the condition expands with no session and fails. A failing command in the
  string ends the rest. `e|<` compares whole numbers unless given `f`, an empty side as 0. See
  [decision 46.4](../decisions/0046-idle-sessions.md).

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
  `detached busy` and the like, `detached` and a space for an unknown status, and `exited` alone
  for a dead pane. See [decision 49](../decisions/0049-status-in-the-list.md).
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
