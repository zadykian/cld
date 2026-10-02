# 45. Resuming a copy

Status: Accepted (#75). Amended by [50](0050-one-command-join.md): `resume SESSION --fork` became
`join --resume SESSION --fork`.

## Context

`resume` renamed the conversation it resumed for good, and copied one only with `--fork-session`
after `--`, unchecked. claude's `--fork-session` resumes under a new ID and leaves the original's
transcript alone. The copy keeps neither its worktree nor its Remote Control session. claude finds a
conversation by its name lower-cased and trimmed (claude 2.1.283 and 2.1.284;
[claude findings](../findings/claude.md)).

## Decision

`--fork` gives claude `--fork-session` after `--resume SESSION` ([41.1](0041-claude-options.md)).
claude resumes a copy of SESSION, named after the session as any resumed conversation is
([16.2](0016-resume.md)), and SESSION keeps its transcript and name.

### 45.1 Not the session's own name

`--fork` needs `--resume` ([50.2](0050-one-command-join.md)). A copy of the session's own
conversation would take its name, and a resume by it would open claude's picker on two. The
maintainer chose to refuse it. A SESSION that is the session's own name, as claude compares names,
is refused too. That is status 2 where `-n` and `-s` make the name, and 1 where the index does
([24.2](0024-names-from-the-repository.md)). Passing over that index would not do, as an expired
session's name can come back at any index ([40.6](0040-session-record.md)). A SESSION naming that
conversation by its ID, or picked in claude's picker, cld cannot tell ([16.4](0016-resume.md)); nor
does it check a `--fork-session` after `--` ([41.4](0041-claude-options.md)).

### 45.2 Beside the original

A copy can run beside the original, where resuming in place would have two claudes write one
transcript ([16.6](0016-resume.md)). claude copies a conversation running in its background too,
which it will not resume in place ([16.10](0016-resume.md); read in its bundle, not run).

### 45.3 Where the copy starts, and its entry

The copy starts where `join` runs, with a Remote Control session of its own where that is on
([42](0042-remote-control-is-claudes.md)). The entry goes without an ID
([40.4](0040-session-record.md)), and the `SessionStart` hook, which claude runs as it forks, gives
it the copy's ([40.2](0040-session-record.md)). A later `join` brings the copy back, not SESSION.

### 45.4 claude's floor

[Decision 6](0006-versions.md) stays: the changelog names `--fork-session` at 2.0.73.

### 45.5 Names claude gives twice

They stay claude's to settle. claude makes a name unique only among the claudes running, so
`/clear`, or `join --new` of an ended name, leaves two conversations of one name. A resume by it
opens the picker, where `Ctrl+R` renames one; in the picker `Ctrl+W` shows every worktree's.
`join --resume ID` resumes one by its ID. cld reads no transcripts, so it neither warns of nor
prevents a duplicate.

### 45.6 Tests

They cover claude's arguments for `--fork` as the probe and tmux get them, each refusal, and the
copy brought back by the ID its hook gave it ([testing](../testing.md)).

## Consequences

A copy never takes its original's name or transcript, at the cost of a refusal where SESSION names
the session itself.
