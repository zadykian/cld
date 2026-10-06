---
name: land
description: >-
  Land a stack of approved cld pull requests on main, bottom first, by fast-forward, and tag a
  release if asked.
disable-model-invocation: true
argument-hint: "PR... [vX.Y.Z]"
---

# Land a stack

Land the pull requests `$ARGUMENTS` on main, the bottom of the stack first. main takes
fast-forwards only: GitHub's merge methods are refused, and `.claude/settings.json` denies
`gh pr merge` and pushes to main.

## Each pull request, in order

1. **Base.** `gh pr view N --json baseRefName,headRefOid,state`. Where the base is the branch
   that just landed, retarget the pull request at main:

   ```sh
   gh api -X PATCH repos/zadykian/cld/pulls/N -f base=main
   ```

   `gh pr edit --base` fails on the Projects (classic) deprecation, so use the API.
2. **On main.** `git fetch origin`, then `git merge-base --is-ancestor origin/main HEAD_SHA`.
   Where main is not an ancestor of the head, stop: the stack needs a rebase (below).
3. **Checks.** `gh pr checks N`: every check green, `lint`, `linux`, `linux-oldest`, `blesh` and
   `macos`. A one-off `macos` flake, such as `fork failed: Device not configured`, gets one rerun:
   `gh run rerun RUN --failed`. Stop on any other failure, and on a second flake.
4. **Conversations.** Every review conversation resolved; the ruleset refuses the push otherwise.
5. **Land.**
   - A pull request that changes `.github/workflows`: the workflow's token may not push it. Give
     the maintainer `git push origin SHA:main` to run, with the head's SHA, and wait for them.
   - Any other: `gh pr comment N --body /ff`.
6. **Wait** until `gh pr view N --json state -q .state` says `MERGED`. Where the workflow comments
   that it did not move main, report its reason and stop.

Then go on with the next pull request.

## A rebased stack

Rebase only when the maintainer asks. Then push each branch with its old head as the lease:

```sh
git push origin --force-with-lease=BRANCH:OLD_SHA BRANCH
```

The checks run again; wait for them before landing.

## Release

Only when the maintainer gives a version, once the stack has landed:

1. `git fetch origin`, and check the version: before 1.0, breaking changes (a `!` in a commit
   since the last tag) bump the minor.
2. Tag main's head, and push the tag:

   ```sh
   git tag -a vX.Y.Z -m vX.Y.Z origin/main
   git push origin vX.Y.Z
   ```

3. The release workflow runs the checks and publishes. Report its run, `gh run list --limit 3`,
   and the release once published: `gh release view vX.Y.Z`.
