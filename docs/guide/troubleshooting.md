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
- **`cannot record session`.** The session runs, but once ended, `cld list` and
  `cld restore sessions` do not know it. Point `XDG_STATE_HOME` at a directory you can write.
- **`restore: missing command` in `journalctl --user -u cld-restore`.** An older cld wrote the
  unit, which runs plain `cld restore`. Run `cld setup restore` again
  ([upgrade notes](install-and-upgrade.md#from-cld-0130-and-earlier)).
- **`File name too long`.** The socket's path, `$TMUX_TMPDIR/tmux-UID/cld-S` with links resolved,
  must fit in 103 bytes on macOS and 107 on Linux. Use a shorter name.
- **`macOS denies control of Ghostty`.** `cld restore tabs` needs the grant of the app it runs in:
  allow Ghostty under that app in System Settings › Privacy & Security › Automation. In Ghostty's
  panes, that app is Ghostty itself. As a last resort there, `tccutil reset AppleEvents
  com.mitchellh.ghostty` asks again, but resets every Automation grant Ghostty holds.
- **`Ghostty says: AppleScript is disabled by the macos-applescript configuration`.** Set
  `macos-applescript = true` in Ghostty's config, and reload it.
- **A pane's session missing from `cld restore tabs --dry-run`.** A `title` in Ghostty's config, or
  a title set with Change Terminal Title…, hides cld's title. Remove the `title` line. In Change
  Terminal Title…, an empty name hands the title back to the pane's programs.
- **`restore tabs needs Ghostty 1.4 or newer`.** Update Ghostty: 1.3.1's AppleScript does not tell
  what runs in a pane ([Ghostty's tabs](ghostty-tabs.md)).
- **`tabs.json has format N, which this cld cannot read`.** A newer cld wrote the record of
  Ghostty's tabs. Run `cld update`.
- **`missing or unsuitable terminal: xterm-ghostty`** from tmux on a host, or another program
  there that does not know `xterm-ghostty`. The host lacks Ghostty's terminfo. Add `ssh-terminfo`
  to `shell-integration-features` in Ghostty's config, then ssh to the host once and leave with
  `exit`. Where `ghostty +ssh-cache` lists the host all the same, as after a rebuild, first remove
  its entry with `ghostty +ssh-cache --remove=ENTRY` ([Ghostty's tabs](ghostty-tabs.md#terminfo)).
  With `--host`, see [another machine](another-host.md#terminfo).
- **`ssh ws` refused for 10 minutes, after a restore or failed logins.** ws's fail2ban has banned
  the address the Mac has on ws's tunnel. `ssh ws-ssh` comes from another address: use it until the
  ban ends, or lift the ban there, as root, with `fail2ban-client set sshd unbanip 10.99.0.2`.
- **A slow cld.** Each run of tmux takes 6-20 ms built from source or most packages, and 100-200 ms
  from Ubuntu's snap.
  - `cld list`, a TAB and `cld join` without `-s` ask every running server, eight at a time: over
    10 sessions, under half a second through the snap. `CLD_IDLE_DAYS=0` spares `cld join` that.
  - Each hook of the title runs tmux, and claude waits for it (see [the tab's title](terminal.md)).
  - Inside your own tmux, from cld's tmux 3.6, joining runs one tmux more.
