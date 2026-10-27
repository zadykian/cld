# 66. Merges left to the ruleset

Status: Accepted.

## Context

[Decision 28.4](0028-shared-project-settings.md) has `--permissions cld` deny `gh pr merge`. This
repository's pull requests then landed on `main` by fast-forward alone, through a `/ff` comment,
and the ruleset on `main` refused every merge method.

[zadykian/toolkit](https://github.com/zadykian/toolkit) 5.0.0 lands pull requests by GitHub's
merge commits instead ([zadykian/toolkit#36](https://github.com/zadykian/toolkit/issues/36)). Its
`land` skill merges a stack with `gh stack merge TOP --merge --yes`, and a pull request of no stack
with `gh pr merge N --merge`, which the deny refuses. A pull request's own signed commits land as
they are, under a merge commit that GitHub signs. This amends decision 28.4.

## Decision

`--permissions cld` denies no merge. 28.4 now reads: the set's `permissions.deny` refuses pushes to
`main`.

### 66.1 The ruleset judges each merge

The ruleset on `main` allows the merge method alone, and requires signed commits and branches up to
date, so a merge's tree is the tree CI checked. It requires the checks to pass and every
conversation resolved, whoever merges. A merge still takes a prompt, as the set allows no
`gh pr merge`.

### 66.2 The denies of a push stay

A push to `main` would skip the pull request, its checks and its merge commit. The set keeps its
seven rules for the usual forms of one. A rule matches the command as written, so the ruleset stays
the guard of the rest.

Not taken: keeping the deny, which would leave each pull request of no stack for the maintainer to
merge.

## Consequences

- The settings that `--permissions cld` sets up deny no merge. cld removes nothing, so a deny that
  an earlier cld wrote stays until the user removes it
  ([upgrading](../../guide/install-and-upgrade.md)).
- This repository's `.claude/settings.json` follows, as 28.6 keeps it byte for byte what cld
  writes, and `/flow:land` merges here.
- The `--permissions` help and the [guide](../../guide/project-settings.md) drop the merge from
  what `cld` denies. The tests take the deny from this repository's settings, as before.
