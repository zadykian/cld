# After a reboot

- A claude that the reboot cut off mid-turn gets the prompt
  `The machine restarted while you were working; continue where you left off.` A permission it
  needs waits until you join.
- A failed claude, whose session stays until you end it, counts as running. A session of an older
  cld records nothing, and stays ended until `cld join`.
- tmux's own `C-q x`, `C-q &` or `tmux kill-server` count as a crash. End a session with `/exit` or
  `cld kill` for it to stay ended.
- A session not started or prompted within `CLD_IDLE_DAYS` days stays ended, with a note. Joins and
  keys do not count here, since tmux's idle times start again at a restore.
- The environment comes back as the session started, stale where it belonged to the old login,
  such as `SSH_AUTH_SOCK` (see [claude's environment](sessions.md)). cld keeps it readable by you
  alone.
- `journalctl --user -u cld-restore` shows what `cld restore` said.
- `cld setup restore` needs `systemctl --user` to answer, which most containers lack. To undo it,
  run `systemctl --user disable cld-restore.service` and remove the unit.
- Not checked yet: a real reboot. The tests end each server instead, and a transient unit stands
  for your systemd's.
- Neither tmux-resurrect nor tmux-continuum brings a session of cld's back. tmux-continuum takes
  cld's servers for your own tmux's, and saves and restores nothing while one runs.
