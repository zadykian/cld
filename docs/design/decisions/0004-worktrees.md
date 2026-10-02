# 4. Worktrees

Status: Accepted. Amended by [24](0024-names-from-the-repository.md),
[42](0042-remote-control-is-claudes.md) and [50](0050-one-command-join.md).

## Context

claude makes a worktree itself with `--worktree NAME`, in `.claude/worktrees/NAME` on the branch
`worktree-NAME`, and reopens one that exists ([claude findings](../findings/claude.md)).

## Decision

- `join -w` passes claude `--worktree cld-NAME`
  ([decision 24.7](0024-names-from-the-repository.md)) rather than running `git worktree add`.
- cld's `--settings` set `worktree.baseRef` to `head`
  ([decision 42](0042-remote-control-is-claudes.md)), which outranks the user's and the
  project's settings.
- cld checks for a git work tree first. claude reports workspace trust, which lives in its own
  state.
- `kill` leaves the worktree.

## Consequences

- Each worktree gets what claude applies to its own: `.worktreeinclude`, `worktree.baseRef`,
  `WorktreeCreate` hooks, one layout per repository.
- A new worktree carries the work at hand, not the remote's default branch.
- claude offers to remove a worktree only when it exits on its own. A killed session's worktree
  stays, and the next `-w` of that name reopens it.
