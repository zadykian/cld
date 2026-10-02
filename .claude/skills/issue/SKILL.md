---
name: issue
description: >-
  Plan, then file a GitHub issue for cld in the compact house style once the maintainer approves the
  plan. Use when asked to file, open or write up an issue, or to plan a change.
argument-hint: "[topic]"
---

# File an issue

Plan, approval, then the issue. Never branch or write code unless the maintainer asks for it.

## 1. Plan

Reply with a plan and ask for approval:

- the goal, in a sentence or two;
- the changes, with `file:line` references;
- what to check, and the tests;
- open decisions, each with a recommendation;
- the landing order, where several issues depend on each other.

"Approved, create the issue" means `gh issue create`, not code.

## 2. Read the latest main

```sh
git fetch origin
git rev-parse --short origin/main      # the SHA the issue cites
git show origin/main:PATH              # read files at that SHA
```

The local checkout may lag main by several commits: cite only lines read at `origin/main`.

## 3. Write it

- **Title:** ``type: `cld` - description``. Name the commands concerned in place of `cld` where
  there are some: ``fix: `cld join`, `cld kill` - refuse a session of another repository``. The
  type is a Conventional Commits type.
- **Summary:** two to four sentences, ending "Line references are to main SHA."
- **`## Today`:** bullets with `file:line` references, the crucial facts only.
- **`## Proposal`:** bullets saying what changes.
- **`## Open decisions`:** only when something is undecided, each with options and a
  recommendation.
- 150 to 350 words in all. Not the long design-document style of #26 and #41.

```markdown
SUMMARY. Line references are to main SHA.

## Today

- `path/file.go:12-30` does X.

## Proposal

- Do Y.
```

## 4. File it

Write the body to a file in the scratchpad, never in the repository, then:

```sh
gh issue create --title 'TITLE' --body-file BODY [--label LABEL]
```

Labels: `feat` gets `enhancement`, `fix` gets `bug`, `docs` gets `documentation`, and the other
types none. Reply with the issue's URL.
