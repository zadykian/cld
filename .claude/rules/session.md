---
paths:
  - "internal/session/**"
---

# internal/session

The invariants a change to the tmux side keeps. Decision numbers are those of
[docs/design.md](../../docs/design.md#decisions); the package comment says how the code works.

## tmux

- Session `cld-NAME` runs on the server `cld-NAME`, and cld looks for that one session there, by
  its name and cld's mark (decision 13).
- A `set` target adds a `:` to `=cld-NAME`, as `set` takes a pane (decision 3).
- cld marks a server it starts, not its sessions. Every format that checks the mark reads `@cld`,
  or the prefix `C-q` of a server of cld 0.8.2 or older (decision 34).
- Connect to a socket before running tmux on it: a refused connection or no socket is no server.
  Stale sockets are passed over, never removed (decision 38).
- Hand tmux claude as separate argv words, which tmux runs without `sh -c`, by the path cld
  checked (decision 6).
- A word of cld's ending in `;` goes as `\;`, and a directory given with `-c` has each `#`
  doubled, since tmux expands it as a format (decision 16).
- The clients that attach, and the reads of the sessions, run `tmux -u`: under a locale without
  UTF-8, tmux writes `_` for claude's UI (Findings).
- Options go on claude's pane or session, never its window or the server, so that other panes
  and sessions behave as plain tmux. A failed claude's border line alone needs a window option
  (decisions 5 and 25).
- tmux takes a command of 16364 bytes at most: `commandLimit` counts the hooks and the keys with
  claude's words (decisions 41 and 51).
- Behaviour differs by tmux version only where decisions 43 and 51 say.

## The record

- The record lives in `$XDG_STATE_HOME/cld`: an entry per session, with its environment and its
  run, busy and start marks beside it (decision 40).
- The entry and the environment are written before tmux, and after the terminal check, so that a
  refusal writes nothing (decision 40).
- A `run-shell` after `new-session`, in the same command, makes the run mark and removes the busy
  and start marks; a failed `new-session` cuts it short (decision 48).
- `kill`, the list's Ctrl+X and the idle sweep remove the run mark in their tmux command, before
  `kill-session`, while the session holds its name (decision 48).
- `join` holds the record's lock from the lookup to tmux, `restore` for each session it brings
  back, and the list's forget too (decisions 40 and 50).
- A live start mark younger than 10 s, read before a lookup that finds no session, makes `join`
  wait and `restore` pass the session by. The list's forget refuses it (decision 50).
- A new session's index is above those of the running sessions, the entries and the indexes given
  (decisions 24 and 40).
- An entry or index older than 30 days goes, but for the entry of a session whose server runs
  (decision 40).
- A record cld cannot write is a warning, and the session is made all the same (decision 40).
- cld reads none of claude's transcripts (decision 40).
- `restore` brings a session back detached, with the claude and environment of `NAME.env` but
  cld's `TMUX_TMPDIR`; it never replays the words after `--` (decision 48).

## claude's settings and hooks

- No notification channel in `--settings`: it would override the user's, for whichever terminal
  joins (decision 29).
- Every hook event and setting cld passes, `async` and `timeout` among them, must exist in the
  oldest claude cld runs (decision 6).
- claude waits 5 s at most for a hook, but for `CwdChanged`'s, which runs `async`, and the
  record's `SessionEnd`, which claude ends at 1.5 s (decision 39).
- A status hook names the server's socket and the session, since a conversation in the
  background runs it without `TMUX`; it prints nothing (decision 25).
- A status hook sets `@cld-status` only where it changes: each option set redraws every terminal
  on the server (decision 25).
- The variables that name the terminal to claude stay out of the environment tmux starts with:
  claude trusts them over `TERM_PROGRAM=tmux` (decision 33).

## kill, detach and the idle sweep

- `kill` runs `kill-session` and `kill-server` in one tmux command, and does not wait for claude
  (decision 32).
- A server that outlived its session is killed only where its mark and socket path, checked
  before and again under `if -F`, make it cld's for that name (decision 34).
- `detach` runs under `if -F '#{session_attached}'`: with no terminal, tmux would fail or detach
  another session's (decision 44).
- A bare `detach` goes by the socket `TMUX` names, not by `tty`: claude runs shell commands
  without a terminal (decision 44).
- The idle sweep is one tmux command that checks the session again before the `rm` and before
  the kill, so that a terminal attaching meanwhile keeps it (decision 46).
- The sweep never ends the session whose server cld runs on, and completion never sweeps
  (decision 46).

## Moving a terminal

- The keys run the tmux cld checked, and cld by the file it runs from, symbolic links resolved;
  each ends in `>/dev/null 2>&1 || true` (decision 51).
- `list --switch`, `--to`, `--switched-from` and `--moved` stay in every later release, since a
  running server's keys name them (decision 51).
- `detach-client -E` runs `exec CLD join` in the session's `default-shell`; cld moves nothing
  where it cannot quote for that shell (decision 51).
- A `join` in a pane moves as `--moved=VALUE`, in URL-safe base64: as words, one of claude's
  could run as a command in fish (decision 51).
- `list --switch` ends no idle session, as it runs with the server's environment, not the
  terminal's; it reports on the message line (decision 51).
- Inside another tmux, the keys it keeps are named on cld's message line once attached, never
  with `output.Warn`, which shows only after a detach (decision 43).
