# 37. Sessions of another repository

Status: Accepted (#80). Amended by [44](0044-detach-command.md) and [50](0050-one-command-join.md).

## Context

Repositories of one name share `NAME` and its indexes
([decision 24](0024-names-from-the-repository.md)): two clones, a fork beside its upstream, generic
names such as `api`. So do directories of one name outside a repository. With cld 0.8.0 and tmux
3.7c, from `scratch/api`, `cld join -s 0` attached to `work/api`'s `api-0`, and `cld kill -s 0`
ended it, printing nothing.

## Decision

### 37.1 The session's home

`join` records where it made the session, its home, as `@cld-home` on claude's session. The home is
the directory `NAME` defaults to: the one holding the repository's common `.git`, or else the
current directory. A session belongs where cld made it, so the home goes with `-n` too. tmux's `set`
keeps the value unexpanded, and a `;` at its end goes escaped
([tmux session findings](../findings/tmux-sessions.md)).

### 37.2 Another home is refused

Without `-n`, where `NAME`'s default is not empty, `join`, `detach` and `kill` refuse a session
whose home is another directory, with status 1. Homes are compared as files, since git names one
repository by two paths through a symbolic link
([environment findings](../findings/environment.md)). A file system that ignores case also finds one
directory by paths in other letters (worked out, not checked on macOS). `-n`, `-s` where `NAME`
leaves nothing, and the list take a session from anywhere. A session without a home, of cld 0.8.2 or
earlier, is taken as before. The maintainer chose a refusal over a warning, which tmux would hide as
it attaches.

### 37.3 No tmux command more

The lookup reads the home in the `list-sessions` it runs anyway, with `-u`: tmux writes a tab and
other characters as `_` to a client without UTF-8
([tmux terminal findings](../findings/tmux-terminal.md)).

### 37.4 Messages name the home

A refusal of a taken name names the session's home. `join` attaches to a running session, so only a
race past the record's lock gets that refusal ([decision 50.10](0050-one-command-join.md)).

### 37.5 Completion

Without `-n`, `join -s` offers only sessions whose home is here or unset. `list` shows no home.
`list`'s read ends each line with the home's length, the home and the directory, so a tab in either
cannot shift the fields.

### 37.6 Names stay as they are

Rejected: a `NAME` made unique on a collision, such as `scratch-api`. A name would then hang on what
else runs, and change once the other's sessions ended. The maintainer kept names stable.

### 37.7 Tests

Two repositories `api`, one through a link, a directory `api` whose path holds a tab, and an older
cld's session. The refusals and completion run from each place, and again without UTF-8
([testing](../testing.md)).

## Consequences

Out of scope: the home in `cld list`, a home for older clds' sessions, and the index, which
repositories of one name still share.
