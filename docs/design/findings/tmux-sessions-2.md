# tmux findings: servers and sessions, continued

The findings of [tmux-sessions.md](tmux-sessions.md) continue here, under its sections and on its
terms: Linux, and tmux 3.6 where an entry names no release. The snap is Ubuntu's tmux 3.7c snap.

## Sockets

- **Moving a running server's socket** (#172; the snap's 3.7c; `server.c` and `tmux.c` of 3.5a and
  3.7c read). tmux sets `socket_path` once, as the server starts. After a `mv`, `-S` on the new path
  reaches the server, but `#{socket_path}` and a new pane's `TMUX` keep the old one. SIGUSR1 makes
  the socket again at the old path, the new one left stale. A hard link reaches the server until
  then. See [decision 57](../decisions/0057-renaming-from-the-list.md).
