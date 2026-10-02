# Sessions

What `cld help join` and the [README](../../README.md) leave out. Below, `S` is a session's whole
name, `NAME-SUFFIX`.

## Names

- `NAME` comes from the directory that holds the repository's `.git`, so its worktrees and
  subdirectories share it. A submodule, or a worktree of a bare repository, takes the git
  directory's name without `.git`.
- Outside a repository, `NAME` is the directory's name as `pwd` shows it, a symbolic link's own.
- Characters a name cannot have become `-`, and `-` and `_` go from either end: `my.site` gives
  `my-site-0`. Where nothing is left, as in `/`, the session is `SUFFIX` alone.
- cld's messages split `S` at its last `-` into `-n NAME -s SUFFIX`. A name without a `-` takes
  `-s S` in `/`, as in `cd / && cld kill -s S`.
- The next index also counts forgotten sessions, and servers that outlive their session (see
  [Troubleshooting](troubleshooting.md)). So a name comes back only once claude has removed its old
  conversation, by default after 30 days.

## Repositories of one name

Repositories, or directories, of one name share `NAME` and its indexes: two clones, or a fork
beside its upstream. Without `-n`, cld refuses a running session that another of them made, and
says where.

- The repository's worktrees and subdirectories count as its own.
- cld takes a session from anywhere with `-n`, and a session of cld 0.8.2 or earlier, which
  recorded no directory, without it.
- An ended session comes back from any repository of its name, in the directory it ran in.

## claude's environment

- claude keeps the environment of the shell that ran the `cld join` that created or brought back
  its session, for its life. After an ssh reconnect, claude's `git push` finds no agent at the old
  `SSH_AUTH_SOCK`.
- The way around is an agent socket at a fixed path, such as a link that `~/.ssh/rc` points at each
  login's socket. Or `cld kill`, then `cld join`.
- cld leaves out the variables that name the shell's terminal, which would have claude take
  Shift+Enter for Enter in another, and VS Code's askpass and git editor. Your own `GIT_ASKPASS`
  or `GIT_EDITOR` stays.
- cld takes the terminal for UTF-8 whatever the locale says, as claude does.

## Ending a session

- `cld kill` ends claude as a closing terminal does: claude kills its shell commands, runs its
  `SessionEnd` hooks and exits. What it prints then, its resume hint among it, is lost.
- claude gives those hooks 1.5 s, or a hook's longer `timeout` up to 60 s;
  `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS` sets the time. `cld kill` returns once tmux has ended
  the session, without waiting for those hooks, so `cld kill -s 1 && cld join -s 1` can briefly run
  the old claude beside the new one.
- A failed claude's line takes the screen's last row, which can cost a line of claude's output.
  A narrow terminal drops how to detach from it, then how to end the session.
- Only claude's own pane stays when it fails. A pane split off beside it closes when its program
  ends, as in plain tmux.

## Remote Control and agent view

- A session connects to Remote Control as claude's own settings say
  ([decision 42](../design/decisions/0042-remote-control-is-claudes.md)). Left at `default`, your
  organisation's default or Claude Code's decides. A `false` in the project's settings keeps it off
  there.
- `cld join -- --remote-control` connects one session as it starts.
- Agent view stays off in every session, and no option turns it on
  ([decision 47](../design/decisions/0047-agent-view-off.md)).
- Not checked yet with claude running: what `/bg`, `←`, `/exit`'s dialog and `/fork` do with agent
  view off. Nor are `ListAgents` and `SendMessage` between sessions, or what else needs claude's
  daemon.
