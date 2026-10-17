# From another machine

`--host HOST` runs a command on HOST through ssh: `cld help COMMAND` shows whether one takes it.
[Decision 62](../design/decisions/0062-commands-on-another-host.md) gives the reasons.

```sh
cld list --host ws                     # the sessions on ws
cld join --host ws -n cld -s 3         # attach to cld-3 there
cld join --host ws -C repository/cld   # a new session in ~/repository/cld there
cld update --host ws                   # update the cld on ws
```

## What each machine needs

- The host needs cld, tmux and claude on the `PATH` of your login shell there, as `install.sh` and
  claude's installer leave them. cld runs there through `$SHELL -lc`.
- This machine needs cld and ssh alone, and runs neither tmux nor claude. `install.sh` installs cld
  on macOS too.
- HOST is an ssh destination, such as a `Host` of `~/.ssh/config` or `user@host`. Your ssh config
  does the rest: keys, `ProxyJump`, the agent.
- A host's cld older than this one's may refuse an option, or say `unknown command '__host'` where
  a word needs quoting. `cld update --host HOST` updates it.

## Names and directories

- `join` needs `-n` or `-C`, `kill` and `detach` need `-n`, and `setup config project` needs `-C`.
  The host's cld would otherwise take the name or the project from your home there.
- A relative `-C DIR` starts from your home on the host. Your shell here expands `~` to this
  machine's home, so write `-C repository/cld`, not `-C ~/repository/cld`.
- TAB offers no session names after `--host`: those it knows are this machine's.
- A session that `join --host` creates gets your login shell's environment on the host. What
  `~/.bashrc` sets after its check for an interactive shell is missing.

## Terminfo

ssh hands your `TERM` to the host. In Ghostty that is `xterm-ghostty`, and tmux there fails with
`missing or unsuitable terminal: xterm-ghostty` where the host lacks its terminfo. Ghostty's
`ssh-terminfo` installs it only through an `ssh` you type in Ghostty's shell, not one cld runs.

Type `ssh HOST` once in a Ghostty pane with `ssh-terminfo` on, and leave with `exit`
([Ghostty's tabs](ghostty-tabs.md#terminfo)). Or run `TERM=xterm-256color cld join --host HOST …`.

## ssh's connections

- Each command opens a connection. ssh can share one, kept open for 10 minutes after the last
  command, with these lines under the host in `~/.ssh/config`:

  ```text
  ControlMaster auto
  ControlPath ~/.ssh/cm-%C
  ControlPersist 10m
  ```

- A `LocalCommand` runs as each connection opens, for `--host` too. ws's playbook writes one that
  starts the watcher of `cld restore tabs`.
- A dropped connection leaves the session running, as a detach does: run the same `join` again.
  cld does not reconnect.
- In a session of cld's terminal, `join` and `list` refuse `--host`: the host's tmux would run
  inside this one, both on `C-q`. Detach first. Without a terminal, as in claude's `!`, they run.
- With its output piped, `cld list --host` prints the table, and `cld join --host` is refused for
  want of a terminal.
- cld ends with the status of the host's cld, or 255 where ssh fails.
