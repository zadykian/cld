---
name: adr
description: >-
  Add a numbered decision record to cld's design docs, or amend or supersede one. Use when a change
  rests on a new design choice or changes one a decision records.
argument-hint: "[N to amend] title"
---

# Decision records

A decision records why cld does something, once; code comments and docs link to it as
"decision N" instead of restating the reasons.

## Where they live

- `docs/design/decisions/NNNN-slug.md`: one decision per file. `NNNN` is its number with leading
  zeros, and `slug` its title in lowercase words joined by hyphens.
- `docs/design/findings/`: probed tmux, claude and terminal behaviour, with the versions checked.

Before #124 lands, the decisions are the numbered items under Decisions in `docs/design.md`, and
the findings are rows of its Findings tables. There a new decision is the next numbered item, its
parts `N.1`, `N.2` and so on. It has no Status line: its first sentence names the decisions it
amends or replaces, as decision 51's does.

## The record

```markdown
# N. Title

Status: Accepted

## Context

The problem, the constraints and the findings behind them.

## Decision

What cld does.

### N.1 Title

One part of it, where the decision has several.

## Consequences

What follows for users, the code, the tests and the docs.
```

## Add a decision

1. Take the next number: one above the highest decision there is. Numbers are never reused.
2. Context: the problem and what limits the answer. Link the findings it rests on; a new probe
   goes into the findings first (see below).
3. Decision: what cld does, each part as `### N.M Title`. Name the alternatives not taken, and
   why, in a sentence each.
4. Consequences: what changes for users, and what the code, the tests and the docs must keep.
5. Status: `Accepted`.
6. Point the code comments and docs that rest on it at "decision N" or "decision N.M".

## Amend or supersede

- A later decision that changes part of one: write it as a new record, and set the old one's
  Status to `Amended by M`. The old text stays unchanged.
- A later decision that replaces one: the old Status becomes `Superseded by M`.
- Never renumber or delete a record. Move each "decision N" reference that now means M.

## Findings

A finding is behaviour probed by hand: the command or keys, what happened, and the tmux, claude or
terminal versions checked. Record it before the decision that rests on it, in the findings file of
its subject.

## Check

- Follow [writing.md](../../rules/writing.md): sentences of 30 words or fewer, files of 300 lines
  or fewer.
- Run `make vale FILES='PATHS'`, `make sizecheck FILES='PATHS'` and `make lychee`.
- Commit as `docs(design): ...`, or with the change the decision is for.
