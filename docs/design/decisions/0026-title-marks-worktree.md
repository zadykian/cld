# 26. The tab marks a worktree

Status: Accepted. Amended by [39](0039-hook-cost.md).

## Context

The title ends in ` [w]` while claude works in a linked git worktree, whichever way it got there,
following claude as the status of [decision 25](0025-title-follows-status.md) does. claude runs a
hook in its own directory, and fires `CwdChanged` as `--worktree`, a worktree's entry or exit, or a
resume moves it. A shell `cd` fires it too, and claude then moves the shell back without an event
(claude 2.1.283; [claude findings](../findings/claude.md)).

## Decision

### 26.1 `SessionStart` and `CwdChanged`

These two hooks keep a worktree option on claude's session. Whether `--worktree` moves claude before
`SessionStart` or after it, one of the two runs in the worktree.

### 26.2 Asking git

A hook asks git, in its own directory, whether the git directory differs from the common one. The
absolute paths need git 2.31. `CwdChanged`'s new directory is not taken: it names where the shell
went, which claude undoes.

### 26.3 git by an absolute path

git runs by an absolute `PATH` entry, as a relative one, read in claude's directory, could find the
project's own. Without git the hooks are left out. Their output goes to `/dev/null`, as what
`SessionStart` prints reaches the model. The settings skip HTML's escapes, so that the redirection
reads as written.

### 26.4 Following claude, not `-w`

Marking the `-w` a session started with would need no hooks, but miss `EnterWorktree`, a session
started in a worktree and a resumed one. The maintainer chose to follow claude.

### 26.5 Tests

The tests move claude among worktrees, a subdirectory, the outside and `.git`, and check C1 on each
terminal ([testing](../testing.md)).

## Consequences

`CwdChanged` runs in the background, so two moves in one answer may land out of order
([decision 39.3](0039-hook-cost.md)). Out of scope: a shell `cd` that the session does not follow,
and naming the worktree.
