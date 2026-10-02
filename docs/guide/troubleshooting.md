# Troubleshooting

- **claude too old.** Update claude the way you installed it: `claude update` for the native
  installer, or through Homebrew, npm or your package manager.
- **`needs a terminal`.** `cld join` attaches the terminal its input comes from, and has none from
  cron, `ssh host cld join` or a script, or with `TERM` unset or `dumb`. Use `ssh -t`.
- **`no terminal is attached to this session for join to move`.** You reached the session without
  a terminal, through Remote Control, say. Run `cld join` in a terminal.
- **`cannot move the terminal with tmux's default-shell`.** cld moves a terminal only from sh, bash,
  zsh, fish, ksh and csh. Detach with `C-q d` and run `cld join`, or start the session as
  `SHELL=/bin/bash cld join`.
- **`C-q s`, `C-q (`, `C-q )` or `C-q L` does nothing.** The keys run the cld file that started the
  session, which has moved or gone; `tmux -L cld-S list-keys | grep switch` shows it. `cld update`
  keeps that file in place. Otherwise `cld kill` the session and `cld join` it again.
- **`would be lost`.** Attach with `cld join -s SUFFIX` alone, `cld kill` the session first, or give
  another `-s`.
- **A server without its session.** What claude started through tmux keeps the server running after
  claude exits, or the session was renamed. `tmux -L cld-S ls` shows what runs there, and
  `cld kill` ends it.
- **A tmux server of your own named `cld-S`.** cld leaves it alone and refuses the name: use
  another. A server whose prefix is `C-q` counts as cld's, as those of cld 0.8.2 and earlier.
- **`C-q d` does nothing.** The terminal keeps `Ctrl+Q`: see [terminals that keep C-q](terminal.md).
- **Names that differ only in case.** Where the socket directory ignores case, as on macOS, `A` and
  `a` share a socket and a record: while one runs, cld refuses the other.
- **`cannot record session`.** The session runs, but once ended, `cld list` and `cld restore` do not
  know it. Point `XDG_STATE_HOME` at a directory you can write.
- **`File name too long`.** The socket's path, `$TMUX_TMPDIR/tmux-UID/cld-S` with links resolved,
  must fit in 103 bytes on macOS and 107 on Linux. Use a shorter name.
- **A slow cld.** Each run of tmux takes 6-20 ms built from source or most packages, and 100-200 ms
  from Ubuntu's snap.
  - `cld list`, a TAB and `cld join` without `-s` ask every running server, eight at a time: over
    10 sessions, under half a second through the snap. `CLD_IDLE_DAYS=0` spares `cld join` that.
  - Each hook of the title runs tmux, and claude waits for it (see [the tab's title](terminal.md)).
  - Inside your own tmux, from cld's tmux 3.6, joining runs one tmux more.
