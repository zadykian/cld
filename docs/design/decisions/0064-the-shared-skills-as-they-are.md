# 64. The shared skills as they are

Status: Accepted.

## Context

[Decision 63](0063-this-repositorys-plugins.md) has this repository take its skills from the
plugins of [zadykian/skills](https://github.com/zadykian/skills). What it adds to a shared skill
goes in `.claude/skill-notes/`, which the skill reads first. Those notes held the issue title's
form, the checks `/flow:land` waits for, who tags a release, and the Go rules for `/docs:trim` and
`/lint:split-file`.

Notes split a skill's steps between the marketplace and each repository, and drift as the skill
changes. The author's repositories now run the shared skills as they are. This amends decision 63.

## Decision

This repository keeps no notes for a shared skill, and `.claude/skill-notes/` goes. Each skill runs
as the marketplace ships it. A rule that holds for all work stays in CLAUDE.md or `.claude/rules/`,
where a skill finds it too.

Not taken: keeping the Go rules as notes for `/lint:split-file`. They hold for any edit of Go, and
`.claude/rules/writing.md` and `.golangci.yml` carry them already.

## Consequences

- `/flow:land` waits for every check, reruns no `macos` flake, and leaves a release's tag to the
  maintainer.
- `/flow:issue` titles an issue as a Conventional Commits header, the skill's own form, not as
  ``type: `cld` - description``.
- `/docs:trim` and `/lint:split-file` take the caps and Go's comment rules from
  `.claude/rules/writing.md`, which loads with each Go file, and the linters' limits from
  `.golangci.yml`.
