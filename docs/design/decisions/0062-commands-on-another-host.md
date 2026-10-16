# 62. Commands on another host

Status: Accepted (#186). Amended by [59](0059-ghostty-tabs.md), under which `restore tabs` refuses
`--host`.

## Context

From a Mac, a session on a host such as ws took `ssh -t ws`, then cld typed there. A script quoted
its words for the host's shell, which may be fish ([decision 51.3](0051-moving-between-sessions.md)).
A plain `ssh ws cld list` finds no cld: ssh's shell reads no login profile, which puts
`~/.local/bin` on the `PATH` (#183). `install.sh` puts cld there, and claude's installer claude.

The line `cld restore tabs` is to type (#176) runs a login shell for that, and keeps ssh in front,
as the user types it. This decision gives the user an option to type instead.

What ssh does with a configured `RemoteCommand`, and with options after the host, is in the
[environment findings](../findings/environment.md#ssh).

This amends decisions [17](0017-shell-completion.md), [48](0048-restore-after-reboot.md) and
[51](0051-moving-between-sessions.md).

## Decision

Most commands take `--host HOST`. cld then becomes ssh to HOST, where a login shell runs cld with the
same words. The host's cld does the work, so the Mac needs cld and ssh alone.

### 62.1 The option

`join`, `list`, `kill`, `detach`, `update`, `restore sessions`, `setup config project`,
`setup config user` and `setup restore` take it. `completion`, `version`, `help` and
`setup completion` do not: they write or describe this machine's cld.

Each command takes it as its own option, as `-n` is: cld reads options per command, through pflag
([decision 11](0011-go-and-cobra.md)). It checks the first word before cobra
([overview](../overview.md#the-command-line-on-cobra)), so `cld --host ws list` is an unknown
command. A root option would also show in the help of the commands that refuse it.

Rejected: a `CLD_HOST` variable. The host's own environment could set it too, from a startup file
the machines share, and its cld would run ssh again, in a loop.

### 62.2 Checks here, before ssh

pflag's checks and each command's arguments come first, as without `--host`. Then, with status 2:

- A HOST other than `[A-Za-z0-9][A-Za-z0-9._@-]*`, of 255 characters at most, is refused. That is
  an ssh destination, such as a `Host` of `~/.ssh/config` or `user@host`, which neither ssh nor a
  shell reads as an option or splits.
- A hidden option, of the keys or a move ([decision 51.2](0051-moving-between-sessions.md)), makes
  `--host` an unexpected argument. Those name this machine's servers and terminals.
- `join` needs `-n` or `-C` ([decision 61](0061-running-in-another-directory.md)), `kill` and
  `detach` need `-n`, and `setup config project` needs `-C`. Without them, the host's cld would take
  NAME or the directory from the home that ssh starts it in.

In a pane of cld's servers, `join` and `list` refuse `--host` with status 1 where ssh would give
the host's cld a terminal (62.3). The host's tmux would run inside this one, both on `C-q`, and
this one would take the keys meant for the host's. Without a terminal, as in claude's `!`, they
run. Moving
the terminal to the host, as `join` moves it there ([decision 51.5](0051-moving-between-sessions.md)),
is not done.

Every other check is the host's cld's, so its messages are the same as there. `join`'s refusals
([decision 50.2](0050-one-command-join.md)) come after ssh, in 50.3's order.

### 62.3 ssh

cld execs ssh, as `join` execs tmux. Only `main` exits still ([decision 11](0011-go-and-cobra.md)): an
exec that fails returns its error. cld's status is the host's cld's, or ssh's own 255.

- `-t` gives the host's cld a terminal, for `join` and `list` where stdin and stdout are both
  terminals, as in decisions [31.1](0031-a-terminal-to-attach-from.md) and
  [14.1](0014-the-session-list.md). The host's cld checks the rest, `TERM` among them. ssh
  writes the host's tmux to stdout, where tmux here draws on stdin's terminal. So a `join` whose
  output is not a terminal gets no terminal, and the host's `join` refuses it.
  `-o LogLevel=ERROR` goes with it, which drops ssh's `Connection to HOST closed.` and keeps its
  errors.
- Otherwise `-T -n`: no terminal, and no input from this one. A pty would merge the host's stderr
  into stdout, and end each line in CRLF, which a pipe of `cld list` would take in.
- `-o RemoteCommand=none`: ssh refuses a command given beside a `RemoteCommand` of its config.
- `--` before HOST: ssh reads options after the host too, but none past `--`.

The user's ssh config does the rest: keys, `ProxyJump`, `ControlMaster`. cld passes no other option.

### 62.4 The words

The host's cld gets the command's path, `join` or `setup config project` say, then the options as
cobra read them. Each goes as `--NAME=VALUE`, a word per value of a list, but `--host` and `--help`.
Then come `--` and claude's words, where given.

ssh hands that command to the user's shell on the host, which may be fish. So where each word is
plain, as decision 51.3 has it, the words go as they are. Otherwise all go as `__host VALUE`: VALUE
is the words joined by NULs, in URL-safe base64 without padding, as `--moved` carries a move's
words. `__host` is a hidden first word, checked before cobra, like cobra's `__complete`. A VALUE
that does not decode is refused with status 2.

Plain words keep a host with an older cld working for most commands, where it has the options
given. `cld update --host HOST` is plain, so it brings an older cld up to one that reads `__host`.
`__host` and the plain words stay in every later release with these meanings, as `--moved` does
([decision 51.2](0051-moving-between-sessions.md)): another machine's cld sends them. This amends
51.3, whose words now also go to another host's cld.

### 62.5 The login shell

ssh hands over `exec $SHELL -lc 'exec cld WORDS'`. The single quotes hold plain words alone, which
`sh` and fish read alike.

A session made this way gets the login shell's environment, which cld records for a restore
([decision 48.4](0048-restore-after-reboot.md)). What `~/.bashrc` sets after its guard against
shells that are not interactive is missing.

`setup restore` is refused off Linux ([decision 48.9](0048-restore-after-reboot.md)), but not with
`--host`, as the host may run Linux. The host's cld refuses it there otherwise. pflag stops at the
first option it refuses, so only a `--host` before it counts. This amends 48.9.

### 62.6 Completion

After `--host`, `-n` and `-s` complete nothing: the sessions that `list` shows here are this
machine's, not the host's. `--host` completes nothing either: no host of ssh's config, and no file
names, which no argument offers ([decision 17.3](0017-shell-completion.md)). This amends 17.2 and
17.3.

### 62.7 Not done

- Forwarding the host's tmux sockets to a tmux client here: its protocol must match the server's,
  and cld's record and claude are on the host.
- A daemon on the host, which cld would have to install, start and keep in step with its releases.
- Completing names over ssh, a connection per TAB.
- Reconnecting a dropped connection, which ws's `ws-join` does (59.10).
- `-C` for `kill` and `detach`, which need `-n`: a session named by SUFFIX alone, made in `/`, is
  out of their reach. `join --host HOST -C /` reaches it, and the host's list kills it.
- Forwarding the terminal's variables: [decision 33](0033-terminal-variables.md) drops them from a
  session anyway.

## Consequences

- From a Mac, users type `cld join --host ws -n cld -s 3`. The
  [guide](../../guide/another-host.md) says what the host needs, and covers terminfo and ssh's
  connections.
- `internal/remote` makes ssh's command and the words. `__host` and the plain words stay in every
  later release (62.4).
- The tests run cld against the probe's fake ssh ([testing](../testing/harness.md#the-macs-fakes)):
  each command's ssh words, words in base64, the refusals, the statuses and completion.
