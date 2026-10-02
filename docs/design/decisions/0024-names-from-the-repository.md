# 24. Names from the repository

Status: Accepted. Amended by [37](0037-sessions-of-another-repository.md),
[38](0038-stale-sockets.md), [40](0040-session-record.md), [49](0049-status-in-the-list.md) and
[50](0050-one-command-join.md).

## Context

`-n` named a session whole and defaulted to `main` ([decision 3](0003-commands.md)). Every session
past the first needed a name typed, which said nothing of where it belonged. Worktrees took that
name, and nothing told them from claude's own.

A session is now `NAME-SUFFIX`, from `-n NAME` and `-s SUFFIX`. Below, `S` is the whole name tmux
sees, which earlier decisions and the code call `NAME`. What git gives for worktrees, bare
repositories, submodules and symbolic links (git 2.47.3 and 2.53.0) is in the
[environment findings](../findings/environment.md).

## Decision

### 24.1 The next index

Without `-s`, `join` takes the index above the highest in use, or 0. The maintainer chose this over
the lowest free index, so that the names keep their order.

In use are running sessions, servers that outlived theirs
([decision 13](0013-a-server-per-session.md)), and for 30 days the record's entries and given
indexes ([decision 40.6](0040-session-record.md)). So a name comes back only once claude has dropped
its conversation. `NAME` matches ignoring case, as a socket directory may
([decision 13.6](0013-a-server-per-session.md)). Servers are asked from the highest index down
([decision 38.3](0038-stale-sockets.md)), and the record's lock gives two joins at once two names.
Counting worktrees would tie names to directories cld does not manage.

### 24.2 `-n` and `-s` together

The two go together, in either order, each defaulting where it can; the maintainer preferred this
to the first version, where they excluded each other. Each is checked as a whole name
([decision 1](0001-naming.md)), since `SUFFIX` is the whole name where `NAME` leaves nothing. A
name over 64 characters is refused: with status 2, before any tool, where the command line made it,
and with 1 where a default did.

### 24.3 `NAME`'s default

`NAME` defaults to the name of the directory that holds the repository's common `.git`, so that
subdirectories and linked worktrees share it. A bare repository's worktree or a submodule takes its
git directory's name. The remote's name was rejected: a repository may have none, and two clones
would share names. Outside a work tree, or without git, the default is the current directory's
name, as `pwd` shows it. The maintainer asked for it over the index alone, so that the names say
where their sessions started.

### 24.4 Made a `NAME`

A default that is no valid `NAME` is made one, not refused: each run of other characters becomes
`-`, trimmed at either end, so `my.site` becomes `my-site`. Refusing would fail every session in a
repository such as `user.github.io`. Where nothing is left, as in `/`, `S` is `SUFFIX` alone.

### 24.5 Where `-s` is needed

`kill` needs `-s`, rather than defaulting to `main`, to the only session of `NAME` or to its highest
index, as the maintainer chose: `cld list` picks a session by its row
([decision 14](0014-the-session-list.md)). `join` without `-s` creates a session
([decision 50.1](0050-one-command-join.md)). From another directory a session needs `-n` as well.

### 24.6 Messages name the options

Messages and the hint of a failed claude ([decision 5](0005-failures-stay-on-screen.md)) name a
session as `-n NAME -s SUFFIX`, split at its last `-`. cld writes them into the hook as it makes
the session, rather than have tmux split the window's name. A message names a session's home too
([decision 37](0037-sessions-of-another-repository.md)).

### 24.7 The worktree's name

`-w` gives claude `--worktree cld-S`, so that the worktree and its branch carry the session's name
([decision 4](0004-worktrees.md)), apart from those claude names itself, as the maintainer asked.

### 24.8 Completion

`join -n` offers each `NAME` of the sessions `list` shows. `join -s` offers the suffixes under
`NAME` that `join` takes ([decision 37.5](0037-sessions-of-another-repository.md)), ended ones too
([decision 50.7](0050-one-command-join.md)), with their states and claude's status
([decision 49.6](0049-status-in-the-list.md)). `kill` offers nothing
([decision 17.3](0017-shell-completion.md)).

### 24.9 Tests

The tests cover the defaults in and out of repositories, gaps, other repositories, each kind of git
directory, names made a `NAME`, the errors, messages and completion ([testing](../testing.md)). The
sandbox's work directory, `_`, leaves nothing, so other tests name sessions with `-s` alone.

## Consequences

A gap in the indexes stays. Out of scope: filling gaps; counting a session's worktree, or its
conversation past the record's 30 days; completing the options of `kill`.
