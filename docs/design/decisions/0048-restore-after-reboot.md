# 48. Sessions come back after a reboot

Status: Accepted (#115). Amended by [50](0050-one-command-join.md) and
[52](0052-telemetry-outside-cld.md).

## Context

A reboot ended every session, and `list` showed them `ended` beside those ended on purpose. Each
came back only by a resume of its own, idle mid-task. The tmux behaviour this rests on is in the
[tmux session findings](../findings/tmux-sessions.md) (tmux 3.5a and 3.7c). claude's is in the
[claude findings](../findings/claude.md), and systemd's and tmux-resurrect's in the
[environment findings](../findings/environment.md).

## Decision

`cld restore` brings back, detached, each session that ran when the machine stopped. claude resumes
its conversation, and continues a turn the stop cut off. `cld setup restore` has the user's systemd
run it at startup.

### 48.1 The run mark

`sessions/S.run`, beside the entry ([40.1](0040-session-record.md)), marks a session to bring back.
The tmux command that makes the session makes the mark once `new-session` has succeeded. tmux cuts a
command short where `new-session` fails, so a session never made has none.

`kill`, the list's Ctrl+X and the idle sweep ([46.4](0046-idle-sessions.md)) remove it before
`kill-session`, in one command. The session holds its name meanwhile, so one made again under the
name keeps its own mark. With the `rm` after `kill-session`, as first made, a `new-session` sent
meanwhile made a session that the kill then ended, and its mark stayed. The sweep checks after the
`rm` that the session is still idle: a terminal that attaches meanwhile keeps the session, but not
its mark. The kill of a server that outlived its session ([13](0013-a-server-per-session.md)) keeps
that gap, with no session to hold the name.

claude's exit with status 0 removes the mark (48.2), and so does the list's forget. claude's
`UserPromptSubmit` hook touches it, so its time is the last start or prompt (48.8). A reboot, a
crash or `tmux kill-server` leaves it. So do tmux's own keys that close claude's pane or window, and
a `kill-session`, as tmux runs no `pane-died` hook for a pane it kills. The guide says to end a
session with `/exit` or `kill`.

Rejected: the mark by `SessionEnd`'s reason. A kill and a shutdown both give `other`, and `/exit`'s
reason also covers a move to the background where agent view is on ([47](0047-agent-view-off.md)). A
hook that does not run, under `disableAllHooks`, would leave the mark of every `/exit`.

### 48.2 claude's exit with status 0

A pane's `pane-exited` hook never runs: tmux looks the hook up once it has closed the pane, and
finds neither pane nor session. So claude's pane has `remain-on-exit on`, where
[5](0005-failures-stay-on-screen.md) had `failed`, and the `pane-died` hook tells the two apart. For
status 0 it removes the run mark, then closes the pane as tmux would. Anything else keeps 5's hint.
The hook grows by some 100 bytes and the path ([41.5](0041-claude-options.md)). A session an older
cld made keeps `failed`, and has no mark.

Where cld has a record's directory, the hook removes the mark even where this `join` wrote no
`S.env`. A session that got no mark of its own can hold a reboot's (48.4). Without one, the branch
is `kill-pane` alone.

### 48.3 The busy mark

`sessions/S.busy` marks a turn. claude's `UserPromptSubmit` hook makes it, and the hooks that end a
turn remove it, `idle_prompt` too for an interrupt as claude writes
([25.2](0025-title-follows-status.md)). claude waits for each ([39](0039-hook-cost.md)), since their
order is the turn's. The command that makes the session removes it, as its claude is in no turn yet,
so a `restore` whose tmux fails keeps it for the next.

### 48.4 The environment

`sessions/S.env` holds the claude `join` checked and the environment it starts tmux with, without
the terminal's variables ([33](0033-terminal-variables.md)). The file is JSON, as a variable can
hold any byte but NUL, and only the user can read it, as an environment can hold secrets. cld writes
it before tmux makes the session, so any marked session has one. Where cld cannot write it, tmux
makes no mark, and a warning says why. It goes with the entry ([40.6](0040-session-record.md)). The
words after `--` are not kept ([41](0041-claude-options.md)). What names the login, as
`SSH_AUTH_SOCK` or `DISPLAY`, comes back stale, as after a reconnection.

### 48.5 `cld restore`

For each entry with a run mark and no server, in the order of the entries' file names, `restore`
does what `join` does for an ended session ([50.9](0050-one-command-join.md)). But the session
starts detached: no terminal check ([31](0031-a-terminal-to-attach-from.md)), no title, and cld
waits for tmux. The server starts with the recorded environment, but for `TMUX_TMPDIR`, where cld
and the title's hooks ([25](0025-title-follows-status.md)) look for the socket. A server that runs,
with or without the session, or one cld did not start ([34](0034-servers-are-marked.md)), it leaves
alone. It prints a line per session.

### 48.6 The lock

`restore` holds the record's lock ([40.6](0040-session-record.md)) for one session at a time, from
the lookup to tmux. Another `restore`, or a `join` of the session, waits, then finds it running:
without the lock both would make it, and tmux would fail the second. The list's forget takes the
lock too, so beside a `restore` it finds the session running rather than remove what `restore` wrote
for tmux. A `join` that goes first lets the lock go with its exec, and leaves a start mark, by which
`restore` leaves it the session ([50.4](0050-one-command-join.md)). A `kill`, taking no lock, is not
waited for: it removes the mark before the session has gone (48.1).

### 48.7 Continuing a turn

A session that was busy gets "The machine restarted while you were working; continue where you left
off." after `--resume ID`. claude runs a prompt given there as the first turn of the resumed
conversation (claude 2.1.232 and 2.1.285, read). Its permission prompts still apply. Rejected:
claude's hidden `--reply-on-resume` and `CLAUDE_CODE_RESUME_INTERRUPTED_TURN`, internal to its
background sessions.

### 48.8 Checks, failures and idle sessions

`restore` starts claude, so it checks the version ([6](0006-versions.md)) of `S.env`'s claude, in
the entry's directory and environment. The check runs where claude starts, so a version manager's
shim picks the same claude. A session it cannot bring back is a warning: no `S.env`, a directory
gone, a claude too old, a name `join` refuses, or tmux failing. The others come back, and `restore`
ends with status 1. With nothing to bring back it exits 0 silently.

`restore` leaves ended, with a note, a session whose run mark is older than `CLD_IDLE_DAYS`
([46](0046-idle-sessions.md)). A reboot takes tmux's times with the server. Without this, a session
nobody used would come back for good on a machine that restarts more often than the limit. The
mark's time is the record's nearest to 46.1's, though it misses an attach without a prompt. The
entry's time is not one: `restore` and `SessionStart` write it, and a `SessionEnd` at shutdown may
touch it. A session only watched that long stays ended, and `join` brings it back. `restore` keeps
the mark's time, and the prompt of 48.7 touches it. `restore` runs no sweep
([46](0046-idle-sessions.md)).

### 48.9 `cld setup restore`

It runs on Linux with systemd only, refused elsewhere as `setup telemetry` is
([18](0018-telemetry.md)), and completion makes none of its checks
([17.4](0017-shell-completion.md)). Where the user's systemd does not answer, it writes nothing. It
writes the user unit `cld-restore.service` where it differs:

- a oneshot unit that stays active once it has run;
- `KillMode=process`, as stopping the unit with the default would kill the servers in its cgroup;
- this cld by the file it runs from, which `update` replaces in place ([21](0021-self-update.md));
- the `PATH` it runs with, and `TMUX_TMPDIR`, `XDG_STATE_HOME` and `CLD_IDLE_DAYS` where set, the
  last refused where `restore` would refuse it.

The unit goes under `~/.config` always: the user's systemd reads `XDG_CONFIG_HOME` from its own
environment. A reload and an `enable` follow. `setup restore` restores nothing itself. Without
lingering, the user's systemd starts at the first login, so `restore` runs then and not at boot. It
also ends the sessions at the last logout, so the report names `loginctl enable-linger`, which may
ask for a password.

### 48.10 Rejected

tmux-resurrect restores the pane's child, where claude is the pane's own program, by typing it into
a shell. tmux-continuum saves from `status-right`, off in cld's servers, and only where one tmux
server runs. Also rejected: claude wrapped in `sh -c` to see its status, which decisions
[1](0001-naming.md) and [11](0011-go-and-cobra.md) keep claude out of. Out of scope: split panes,
and the words after `--`.

### 48.11 Help and completion

`restore` and `setup restore` take no argument and complete none. `kill`'s help says that `restore`
leaves a killed session ended.

### 48.12 Tests

`restore_marks_test.go` covers the marks and `S.env`, and no mark where tmux made no session.
`restore_unmark_test.go` holds the kill's `rm` while the session holds its name. `restore_test.go`
brings sessions back after `kill-server`, from elsewhere, and `restore_failures_test.go` has its
failures as warnings. `restore_race_test.go` and `restore_forget_test.go` race two `restore`, a
`restore` and a `join`, and the forget, which fail without the lock. On Linux the held cld goes
once two have the lock file open, elsewhere a second later. `restore_setup_test.go` runs
`setup restore` against a fake `systemctl` ([testing](../testing.md)).

## Consequences

A session ends for good only by `/exit` or `kill`, and what names the login comes back stale.
tmux-continuum in the user's own tmux counts cld's servers as others, which turns its saves off; the
guide says so. A real reboot is not tested yet ([testing](../testing.md)).
