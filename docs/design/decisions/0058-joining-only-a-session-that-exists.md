# 58. Joining only a session that exists

Status: Accepted (#175).

## Context

`cld restore tabs` (#176) reopens Ghostty's tabs on a Mac, and types a join into each pane it
reopens: `ssh -t HOST 'exec $SHELL -lc "cld join -n N -s X; exec $SHELL -l"'`, or
`cd / && cld join -s S` for a name that does not split. That join names the session the pane showed
last, perhaps days before. Under [decision 50.1](0050-one-command-join.md), `join` then:

- creates a session where the name has gone: a fresh claude in the shell's directory, `$HOME` over
  ssh;
- resumes a session whatever ended it, one the user ended on purpose after the loss included, from
  this machine or another.

Neither is what a restore means. cld already tells a stop from an end by the run mark
([decision 48.1](0048-restore-after-reboot.md)). A reboot, a crash or `tmux kill-server` leaves it;
`cld kill`, the list's Ctrl+X, the idle sweep and claude's exit with status 0 remove it.
`cld restore` brings back only a session that has it.

This amends 50.1, under which `join` creates a session that neither runs nor has ended, and resumes
one that has ended, whatever ended it.

## Decision

`cld join --existing` joins only a session that runs, or that a stop ended, and never creates one.

### 58.1 By the session's state

- Where the session runs, `join --existing` attaches as `join` does. The same checks come first: a
  session of another repository without `-n`
  ([decision 37](0037-sessions-of-another-repository.md)), a server without its session
  ([decision 13](0013-a-server-per-session.md)), and the start mark's wait
  ([decision 50.4](0050-one-command-join.md)).
- Where it has ended, it resumes the conversation as `join` does, only while the run mark remains.
  That is `restore`'s rule, read under the record's lock with the lookup.
- Otherwise it fails with status 1, writing nothing, before claude's check and the terminal's:
  - `no session 'S'` where there is none, as the list's Enter says of a row gone
    ([decision 50.8](0050-one-command-join.md));
  - `session 'S' has ended: cld join -n N -s X resumes it` where the mark has gone. The advice names
    the session back as [decision 24.6](0024-names-from-the-repository.md) does.

Unlike `restore` ([decision 48.8](0048-restore-after-reboot.md)), it resumes a session whatever the
mark's age: `join -s` ends no idle session ([decision 46.2](0046-idle-sessions.md)).

### 58.2 The command line

`--existing` needs `-s`: without it, `join` takes the next index and creates the session. It
excludes `--new`, `--resume`, `--fork`, `-w` and the words after `--`. Each goes only to a session
`join` creates or makes anew, or would change the claude a stop ended, which `restore` brings back
without them too. Each is refused with status 2 before any tool runs, as 50.2's are.
`--detach-others` goes with it.

### 58.3 An older cld

A cld without the option refuses it with status 2 before any tool runs, as it refuses any unknown
option: `cld: join: unexpected argument '--existing' (see cld help)`. So an older cld on the host
creates nothing, and the pane's shell takes over.

### 58.4 Kept in every later release

Another machine's cld types `--existing`, as a running server's keys name the options of
[decision 51.2](0051-moving-between-sessions.md). So it stays in every later release with this
meaning, and in the help, as users type it too.

### 58.5 In a session's pane

There `join` moves the terminal ([decision 51.5](0051-moving-between-sessions.md)), and the terminal
runs `cld join` with the same words, `--existing` among them. The pane's `join` refuses first what
`--existing` refuses, as it refuses what would be lost. Refused by the terminal's `cld join`, the
terminal would be left in the shell it came from, outside any session. The terminal's `join` checks
again under the lock, for a session that ended meanwhile.

### 58.6 A name made again

`--existing` knows a name, not a conversation. It joins a session made again under the old name, by
`cld join -s OLD` or `-n N -s OLD`. It joins one that a bare `cld join` makes once the old index
has expired, at 30 days, too. The list's forget and a rename keep the old index given for those 30
days (decisions [40.6](0040-session-record.md) and [57.6](0057-renaming-from-the-list.md)), so a
bare `join` cannot give the name sooner. The machine that types the join cannot tell, and the
guide says so.

### 58.7 Not taken

- `--existing` resuming any ended session: it would bring back a session the user killed, ended or
  forgot after the loss, which the run mark already tells apart.
- `cld list --json`, asked before the restore: a round trip per host, and a gap between the check
  and the join, which a refusal at join time has not.
- Parsing `CLD_IDLE_DAYS=0 cld list`: the same gap, over output written for people.
- Accepting the silent create: a stale name would start a fresh claude in `$HOME`.
- The name `--no-create`: it names half the rule, and reads as resuming any ended session.

### 58.8 Tests

`record_existing_test.go` resumes a session that `kill-server` ended, its run mark older than
`CLD_IDLE_DAYS`. It refuses those that `kill`, claude's `/exit` and the list's Ctrl+X ended, and
missing ones, with nothing written. A row of `session_join_test.go`'s states attaches to a running
session with `--existing`. `cli_name_options_test.go` holds 58.2's refusals, and
`session_move_test.go` the refusals before a move ([testing](../testing.md)).

## Consequences

- A program on another machine joins a session by name without creating one or bringing back one
  ended on purpose, and its failure shows in the pane.
- A session that tmux's own keys or `tmux kill-server` ended comes back with `--existing`, as with
  `restore`.
- `--existing` stays in later releases with its meaning (58.4).
- A name made again is joined all the same (58.6).
