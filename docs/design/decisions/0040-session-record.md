# 40. A record of the sessions

Status: Accepted (#57). Amended by [44](0044-detach-command.md), [48](0048-restore-after-reboot.md)
and [50](0050-one-command-join.md).

## Context

cld kept no state, so after a reboot `list` printed nothing. A session came back only by its
conversation's name, which claude resumes only where one conversation has it, in its repository. An
index came back once its session ended, giving the name to a second conversation.

claude hands each hook the conversation's ID on a line of JSON. It removes a conversation not
written for `cleanupPeriodDays`, 30 by default (claude 2.1.232 to 2.1.284;
[claude findings](../findings/claude.md)).

## Decision

### 40.1 The record's files

The record is `cld` in `$XDG_STATE_HOME`, or else in `~/.local/state`; a relative `XDG_STATE_HOME`
is ignored, as the XDG Base Directory Specification says. A session's entry, `sessions/S.json`, is a
line of JSON: its name, the directory claude started in and its conversation's ID. The file's time
is the entry's. A file a session, replaced whole by a rename, lets claude's hook write it without
cld, jq or a lock. Beside them are `indexes.json`, the highest index given after each `NAME-`, and
`lock`.

`join` writes the entry before tmux, once nothing is left to refuse it, the terminal included
([decision 31.2](0031-a-terminal-to-attach-from.md)). [Decision 48](0048-restore-after-reboot.md)
adds the marks and the environment beside the entry, and [decision 50.4](0050-one-command-join.md)
the start mark.

### 40.2 The ID from claude's hooks

A `SessionStart` hook in `--settings` takes `session_id` from its input, where it has only letters,
digits and `-`, and writes the entry anew. So the entry names the session's last conversation, after
`/clear`, `/resume` or `/rename` too. `Stop` and `SessionEnd` hooks touch it, making no entry that
was forgotten. That keeps its time near the conversation's last write, which claude's cleanup counts
from, even after a crash. The events exist in claude 2.1.232, so [decision 6](0006-versions.md)
stands.

The hooks print nothing, which would reach the model, and start no tmux. claude waits for
`SessionStart`'s and `Stop`'s 5 s at most ([decision 39](0039-hook-cost.md)). `SessionEnd`'s has
none: claude gives those hooks 1.5 s together, which a timeout of 5 s would stretch.
`SessionStart`'s stays in order: a write in the background after `/clear` could land after a
`/resume`'s right after it. `Stop`'s touch runs beside the status's `Stop` hook, which claude waits
for anyway ([39.1](0039-hook-cost.md)).

### 40.3 Ended sessions in the list

`list` shows an entry whose session is not on its server as `ended`, with its directory, in the
order of the names. A session killed from the list keeps its row
([decision 15](0015-killing-from-the-list.md)). There Enter brings it back, and Ctrl+X twice forgets
it, as agent view deletes after its stop. Enter checks claude's version only once the list has
handed the terminal over, since the list can abandon a lookup but not `claude --version`.

### 40.4 Bringing a session back

`join` of an ended session passes `--resume ID`, or `--resume cld-S` without an ID
([decision 50.1](0050-one-command-join.md)). claude starts in the entry's directory, wherever `join`
runs: the conversation's project, where its trust was accepted. The maintainer chose this over
refusing another directory or leaving it to claude. cld enters that directory, `PWD` too, after
`NAME`'s default is taken from where `join` runs. A directory gone is refused, naming the
`--resume ID` that resumes from here.

The session's home ([decision 37.1](0037-sessions-of-another-repository.md)) is that directory's, as
`join` would record it there. The entry records no home, so without `-n`, `join -s S` in a
repository of the same name brings another's ended session back. Refusing that is left open.

### 40.5 kill keeps the entry

`kill` and `detach` refuse an ended session, pointing at `cld join`. `kill` leaves the entry, so a
kill by mistake is an Enter away: the maintainer chose to forget one only at expiry and with the
list's Ctrl+X. `kill` removes the run mark, so `restore` leaves the session ended
([decision 48.1](0048-restore-after-reboot.md)).

### 40.6 Indexes, expiry and the lock

`join` without `-s` gives the index above the highest of the running sessions
([decision 24.1](0024-names-from-the-repository.md)), of the entries and of the indexes given. An
index given with `-s` counts as given too. An entry or index older than 30 days, claude's default,
counts no more and goes, but for the entry of a session whose server runs. So a name, and `-w`'s
worktree, comes back only once claude no longer keeps its conversation. A user's own
`cleanupPeriodDays` is not read.

`join`, `restore` and the list's forget hold `lock` (`flock`) up to tmux, so that two `join` at once
take two names. A lock another cld holds is waited for ten seconds, then gone without.

### 40.7 The record serves the sessions

Where cld cannot write it, `join` warns and makes the session all the same, without the record's
hooks. An entry or index cld cannot read is none. So is an entry whose name is not its file's, as on
a socket directory that ignores case ([decision 13.6](0013-a-server-per-session.md)).

### 40.8 Completion

`join -n` and `-s` offer ended sessions beside those that run
([decision 50.7](0050-one-command-join.md)); `detach`'s offer only those that run.

### 40.9 Rejected

The ID from claude's transcripts or `~/.claude/sessions` ([decision 16.4](0016-resume.md)). The name
alone, which `/rename` and `/clear` defeat. One file for all entries, which the hook could not edit
alone. A hook that runs cld, which would tie the session to cld's path. Forgetting at `kill`.

### 40.10 Tests

`record_test.go` covers the entry, its hooks, the record's place and a record cld cannot write.
`record_ended_test.go` and `record_list_test.go` cover ended sessions, `record_expiry_test.go` the
indexes and expiry, and `record_lock_test.go` the lock and the start mark
([testing](../testing.md)).

## Consequences

Out of scope: counting worktrees, a column for when a session last ran, forgetting from the command
line, and a user's own `cleanupPeriodDays`.
