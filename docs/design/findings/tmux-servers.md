# tmux findings: servers

What tmux does with its servers, as probed for cld: the release a server runs, its socket, how it
ends, and what servers cost. Probed on Linux unless an entry says otherwise, and on tmux 3.6 where
an entry names no release. The images are those `tests/Dockerfile` builds, one per tmux release, and
the snap is Ubuntu's tmux 3.7c snap. Sessions, clients and panes are in
[tmux-sessions.md](tmux-sessions.md), and commands, options and formats in
[tmux-commands.md](tmux-commands.md).

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
- **Moving a running server's socket** (#172; the snap's 3.7c; `server.c` and `tmux.c` of 3.5a and
  3.7c read). tmux sets `socket_path` once, as the server starts. After a `mv`, `-S` on the new path
  reaches the server, but `#{socket_path}` and a new pane's `TMUX` keep the old one. SIGUSR1 makes
  the socket again at the old path, the new one left stale. A hard link reaches the server until
  then. See [decision 57](../decisions/0057-renaming-from-the-list.md).

## Servers ending

- **A terminal attached when its server is killed** (3.3a, 3.4, 3.5a, 3.7c; `server.c` read).
  `kill-server` alone ends the client with `[server exited]`, status 1; with `kill-session` before
  it in one command, `[exited]` and status 0. Until its clients have gone, it fails new ones with
  `server exited unexpectedly`, in 4 of 86 rounds on 3.7c. 3.3a showed none in 99. `cld kill` then
  `cld new` of a name still reached a fresh server in 100 of 100 rounds. See
  [decision 13](../decisions/0013-a-server-per-session.md).
- **A server after its kill** (3.3a, 3.7c, 200 tries). Once `kill-server` has returned, the server
  answers no more. After `kill-session` of its last session, it still answered once or twice.
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
