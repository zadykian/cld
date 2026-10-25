# 65. The shared rules as they ship

Status: Accepted.

## Context

[Decision 64](0064-the-shared-skills-as-they-are.md) runs the skills of
[zadykian/skills](https://github.com/zadykian/skills) as they ship, but keeps the rules of
`CLAUDE.md` and `.claude/rules/` beside them, where a skill finds them too. Some of those rules
changed a skill's own:

- the commit message's types without `revert`, a fixed list of scopes, and a body of bullets;
- caps of 4 lines a Go comment block, 15 a package comment and 150 lines the README;
- the baselines `tools/sizecheck/baseline.txt` and `tools/valecheck.txt`, which let a listed file
  fail a gate, where the `writing` skill keeps none.

`tools/commitcheck` checked the messages in `make lint`, and so blocked a pull request. The
author's repositories now take the skills' rules as they ship, and
[zadykian/actions](https://github.com/zadykian/actions) checks each pull request against them.
This amends decision 64.

## Decision

### 65.1 cld names its facts

Each shared skill's rules hold as the skill ships them. `CLAUDE.md` and `.claude/rules/` name what
cld alone knows: its layout, its commands and gates, the checks the ruleset on `main` requires,
and the conventions of its code that no skill covers. Where they would restate a skill, they link
it.

### 65.2 The rule checks warn

`.github/workflows/rules.yml` runs the `atomic-commits`, `pr-description` and `banned-terms`
actions of zadykian/actions on each pull request, with their default inputs. They warn and never
block, so no ruleset requires them. `tools/commitcheck` and `make commits` go.

### 65.3 The skill's caps alone

The caps are the `writing` skill's. `tools/sizecheck` keeps its 300 lines a `.go` or `.md` file,
and drops the Go comment caps. Neither it nor `tools/valecheck` keeps a baseline: every file
passes each gate outright.

Not taken: keeping `tools/commitcheck` beside the actions, which would block on what they only
warn of.

## Consequences

- A commit message takes any type the `atomic-commits` skill lists, `revert` too, and any scope.
- `/docs:trim` and `/lint:split-file` take the caps from the `writing` skill, and the linters'
  limits from `.golangci.yml`.
- The `doc-editor` agent goes, as `/docs:trim` does its work.
