---
name: adr
description: >-
  Add a numbered decision record to cld's design docs, or amend or supersede one. Use when a change
  rests on a new design choice or changes one a decision records.
argument-hint: "[N to amend] title"
---

# Decision records

A decision records why cld does something, once. Code comments name it as "decision N", and docs
link its record, instead of restating the reasons.

## Where they live

- `docs/design/decisions/NNNN-slug.md`: one decision per file. `NNNN` is its number with leading
  zeros, and `slug` its title in lowercase words joined by hyphens.
- `docs/design/findings/`: probed behaviour, with the versions checked, in the file of its subject:
  `tmux-sessions.md`, `tmux-terminal.md`, `claude.md`, `terminals.md` or `environment.md`. A file
  that would pass 300 lines continues in `NAME-2.md`.
- `docs/design.md`: the index, a line per record with its title and its Status.

## The record

```markdown
# N. Title

Status: Accepted (#ISSUE).

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
   goes into the findings first (see below). Name the decisions it amends or replaces, linked.
3. Decision: what cld does, each part as `### N.M Title`. Name the alternatives not taken, and
   why, in a sentence each.
4. Consequences: what changes for users, and what the code, the tests and the docs must keep.
5. Status: `Accepted (#ISSUE).`, naming the issue it closes.
6. Add its line to the index in `docs/design.md`.
7. Point the code comments that rest on it at "decision N" or "decision N.M", and link the record
   from the docs.

## Amend or supersede

- A later decision that changes part of one: write it as a new record, and add `Amended by [M]`
  to the old one's Status, linked. The old text stays unchanged.
- A later decision that replaces one: the old Status gets `Superseded by [M]`.
- Change the old record's Status in the index too.
- Never renumber or delete a record. Move each "decision N" reference that now means M.

## Findings

A finding is behaviour probed by hand: the command or keys, what happened, and the tmux, claude or
terminal versions checked. Record it before the decision that rests on it, in the findings file of
its subject. Each is a bullet: the probe in bold, the versions in parentheses, the result, and a
link to the decision.

## Check

- Follow [writing.md](../../rules/writing.md): sentences of 30 words or fewer, files of 300 lines
  or fewer.
- Run `make vale FILES='PATHS'`, `make sizecheck FILES='PATHS'` and `make lychee`.
- Commit as `docs(design): ...`, or with the change the decision is for.
