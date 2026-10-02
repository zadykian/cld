# 51. Moving between sessions

Status: Accepted (#112).

## Context

Going from one session to another took leaving it: `C-q d`, `cld list` in the shell, `Enter`. With
agent view off ([decision 47](0047-agent-view-off.md)), claude's `←` opens no list either. tmux's
own `C-q s`, `C-q (`, `C-q )` and `C-q L` switch among the sessions of one server, and each session
of cld's has a server of its own ([decision 13](0013-a-server-per-session.md)).

tmux hands a terminal to another server through `detach-client -E`. The terminal's client then runs
a shell command in its place, here `cld join` of the other session (tmux 3.5a and 3.7c;
[tmux terminal findings](../findings/tmux-terminal.md)).

This amends decisions [2](0002-inside-another-tmux.md), [13](0013-a-server-per-session.md),
[14](0014-the-session-list.md), [15](0015-killing-from-the-list.md),
[31](0031-a-terminal-to-attach-from.md), [41](0041-claude-options.md),
[43](0043-inside-your-own-tmux.md), [44](0044-detach-command.md), [46](0046-idle-sessions.md) and
[50](0050-one-command-join.md).

## Decision

`C-q s` shows `cld list` in a popup over the session: `Enter` moves the terminal to the session
picked, and `Esc` closes it. `C-q (` and `C-q )` move it to the previous and the next session that
runs, and `C-q L` back to the one it came from. `cld join` in a pane of cld's servers, `! cld join`
in claude say, moves the terminal on that session, where it refused
([decision 50.3](0050-one-command-join.md)).

### 51.1 The keys

`C-q s` opens a popup with no border, filling the terminal, that runs `cld list --switch` for the
terminal that pressed it. The other three keys add `--to`. Each runs in the background, so the key
does not wait, and drops its output and status, which tmux would show over claude's pane.

- They name the tmux cld checked, and cld by the file it runs from, which `update` replaces in place
  ([decision 21](0021-self-update.md)). Each path is quoted for `sh` and for tmux's formats.
- `run-shell` expands its command as a format, so `#{client_name}` there names the terminal that
  pressed the key. A popup's command expands none, and given several words runs them with no shell
  (tmux 3.5a and 3.7c; [tmux terminal findings](../findings/tmux-terminal.md)).
- The popup takes its program's output away as it closes. So `list --switch` says what goes wrong on
  the terminal's message line, as tmux shows its own, and on stderr.
- It shows for three seconds or until a key, which reaches claude, as tmux shows its own. Rejected:
  `-d 0`, as the `pane-died` hint has it, which held claude's screen until a key. That key, `Esc`
  say, then reached claude and interrupted a turn.
- It goes unexpanded (`-l`). As a format, tmux would hand it to `strftime` first, and
  `/opt/50%done/nu` would come out as `/opt/5001one/nu` (tmux 3.5a and 3.7c).
- From tmux 3.6 it goes with `-C`, as [decision 43](0043-inside-your-own-tmux.md)'s line does:
  without it tmux draws nothing more of claude's pane until the message goes. tmux 3.5, which has no
  `-C`, holds the pane for those three seconds at most.

The keys take some 420 bytes of tmux's command ([decision 41.5](0041-claude-options.md)). A server
an older cld started keeps tmux's keys until its session ends, and the [guide](../../guide.md)'s
Upgrading says so. Where the file the keys name has gone, they do nothing and say nothing, which the
guide's Troubleshooting says.

### 51.2 The keys' own options

`list --switch` and `--to previous|next|last` are hidden from the help and from completion.
`--switch` outside cld's servers is refused. These options, `join --switched-from` (51.4) and
`--moved` (51.3) stay in every later release with these meanings, as `completion SHELL` does
([decision 22.5](0022-setting-completion-up.md)). A running server's keys name cld by its file, and
a release that dropped one would leave them silently doing nothing.

With `--to`, `list` moves the terminal to the session before or after the server's own among those
that run, by name and going round, or back (51.4). Where there is none, it says so on the message
line, and the terminal stays.

Neither the keys nor the popup sweep idle sessions ([decision 46.2](0046-idle-sessions.md)). tmux
runs them with the server's environment, not the terminal's (tmux 3.5a and 3.7c;
[tmux terminal findings](../findings/tmux-terminal.md)). So the sweep would go by a `CLD_IDLE_DAYS`
the terminal no longer has, and the popup would take its notes away. In the popup `Enter` moves the
terminal and `Ctrl+X` kills as ever. The list sets no title there and asks the terminal nothing, as
a popup does not answer.

### 51.3 The move

For a key, cld detaches the terminal that pressed it with `detach-client -E`. The popup closes with
the detach, and tmux may end `list` before that returns, so nothing follows it.

In a pane, cld runs a bare `detach-client -E` by the socket `TMUX` names. tmux takes the terminal
used last on the pane's session, as a bare `cld detach` does
([decision 44.2](0044-detach-command.md)). An `if` on the session's terminals keeps it from taking
one of another session. With no terminal there, `join` refuses: a `join` that moves nothing has not
done what the user asked.

The command, `exec CLD join --switched-from S WORDS`, runs with the session's `default-shell`, which
tmux also puts in `SHELL`. fish reads a `\` before a `\` or a `'` within single quotes as an escape
(fish 4.0.2; [environment findings](../findings/environment.md)). A word quoted for `sh` can thus
end its quotes early in fish, and the rest run in the terminal, outside claude. So cld writes the
command for every shell alike:

- cld moves nothing unless the shell is sh, bash, zsh, fish, ksh, csh or one of theirs, each of
  which has `exec` and reads CLD alike. nu, pwsh or elvish would leave the terminal out of any
  session.
- CLD is the one word that can need quoting. It goes bare where plain, and otherwise in single
  quotes with each `'` and `\` outside them after a `\`. dash, bash, zsh and fish read that alike
  (dash 0.5.12, bash 5.2.37, zsh 5.9); ksh and csh by their manuals, not tried.
- WORDS name the session (51.7), or carry the directory and words of a `join` typed in a pane in
  `--moved`, in URL-safe base64. As plain words, a newline in one of claude's would come out changed
  from tmux's parser within the `if` (tmux 3.5a and 3.7c;
  [tmux findings](../findings/tmux-sessions.md)).

The terminal's `cld join --moved` enters the directory the `join` ran in, `PWD` naming it, and runs
it anew. A cld path with a control character binds no keys: they stay tmux's. Past some 12 KB of
words the move's command exceeds tmux's limit ([decision 41.5](0041-claude-options.md)), and `join`
refuses them in the pane.

The client runs the command in the terminal's environment, with an empty `TMUX`
([decision 33](0033-terminal-variables.md)). So a session the move creates gets the terminal's
variables, and none of claude's. After a move, [decision 43](0043-inside-your-own-tmux.md)'s line on
keys an outer tmux keeps is not shown again. Carrying that tmux's socket along would put it in the
environment of a server the move makes.

### 51.4 Back with C-q L

tmux's `L` goes back on one server only: a client that moves to another server is a new one there.
So the session a move reaches records the one the terminal left, as `@cld-last` on claude's session,
through `join --switched-from`. `--to last` reads it there. `--switched-from` sets nothing where S
is the session itself, and where it attaches, only once the attach has succeeded. The record is the
session's, not the terminal's: of two terminals on one session, `C-q L` takes each where the last to
arrive came from. Rejected: keeping it per terminal by its tty's name, which comes back with the
next terminal opened ([decision 2](0002-inside-another-tmux.md)), so one that never moved could be
sent anywhere.

### 51.5 `join` in a pane

As the maintainer chose, `join` in a pane of one of cld's servers moves the terminal. That is where
cld's stdin is a live pane of the marked server `TMUX` names, or no terminal at all, as under
claude's `!` ([decision 34](0034-servers-are-marked.md)). A pane of any other tmux attaches there as
before ([decision 43](0043-inside-your-own-tmux.md)).

Without `-s` it moves at once, and the terminal's `cld join` takes the index. Only `-w` outside a
git work tree ([decision 4](0004-worktrees.md)) is refused first. With `-s` it looks the session up
first and refuses what `join` would refuse before claude starts
([decision 50.2](0050-one-command-join.md)). The terminal then stays, and claude shows why.

- That lookup takes no lock, and leaves a session another cld is starting to the terminal's
  `cld join`, which waits for it ([decision 50.4](0050-one-command-join.md)). Waiting in the pane
  held claude's `!` for 10 s, after which the terminal's `join` took the mark for stale: two moves
  into one session made it twice.
- `claude --version` is left to the terminal's `cld join`, as the claude that starts is the one on
  the terminal's `PATH` ([decision 6](0006-versions.md)).
- The words go on with the directory the `join` ran in, in `--moved` (51.3), so `NAME`'s default
  comes from there. The issue's bare `-s 1` would name another repository's session in the
  terminal's own directory.
- What the terminal's `cld join` refuses then, a race or a claude too old, it prints in the
  terminal.

The terminal's `cld join` without `-s` keeps the session it has just left from its sweep. Its claude
may have moved the terminal from the Bash tool, driven through Remote Control say, with no key typed
for longer than `CLD_IDLE_DAYS`. The sweep would end that claude, as
[decision 46.2](0046-idle-sessions.md) keeps the session cld runs in.

### 51.6 `list` in a pane

In a live pane of cld's servers `list` is interactive, where it printed its table
([decision 14.3](0014-the-session-list.md)). `Enter` moves the terminal as `join` does there.
Without a terminal, as under `!`, it prints the table.

### 51.7 Naming the session

A key's and the list's command names the session by `-n NAME -s SUFFIX`, split at its last `-`
([decision 24](0024-names-from-the-repository.md)), and a NAME with no such split by `-s S` in `/`.
The terminal's `cld join` makes a session gone since the list read it, which the list's own Enter
refuses ([decision 50.8](0050-one-command-join.md)). Picking the terminal's own session closes the
popup and moves nothing; a `join` of it in a pane moves the terminal there again.

### 51.8 Not taken

- `switch-client` for a session on the same server, which only sessions claude makes share.
- A hint for the popup in the list's footer.
- A line in the keys showing `cannot run CLD`. tmux fails a popup's program it cannot start with
  status 1, as any failure of `list`, so the line would serve the other three keys alone. It would
  also add paths to each key, where claude's words share tmux's 16 KB
  ([decision 41.5](0041-claude-options.md)).
- A `default-shell` of cld's own for the move, `/bin/sh`. tmux puts it in `SHELL`, and a session the
  move made would give claude's windows `sh`.

### 51.9 Tests

The tests run on tmux 3.7c, 3.5a and macOS ([testing](../testing.md)). They cover the popup in C10
on both terminals, the keys over three sessions, and quoting under sh, bash, zsh and fish. They also
cover a `nu` shell refused, `! cld join` and its refusals, and two moves into one session making it
once.

## Consequences

A terminal moves among sessions without leaving claude, at the cost of hidden options that every
later release keeps (51.2). Moving goes through the user's shell, so cld refuses shells it has not
written the move for (51.3).
