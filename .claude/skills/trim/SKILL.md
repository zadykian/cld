---
name: trim
description: >-
  Trim cld's Go comments or Markdown docs to the writing policy and the Vale rules, keeping every
  fact. Use on files that fail Vale or the comment caps, or that restate the code or the help.
argument-hint: FILE...
---

# Trim comments and docs

Trim `$ARGUMENTS` to [writing.md](../../rules/writing.md). Shorter, never poorer: each fact stays
in the one place it belongs.

## Go comments

- Keep a block's purpose in one or two sentences, or a reason the code cannot show. Delete
  sentences that narrate what the code does.
- A reason that a decision records becomes a link: "see decision N".
- Blocks of 4 lines or fewer; a package comment of 15 lines or fewer. Move the details of a long
  package comment to the declarations they explain.
- Names in a doc comment stay first: `// Setup sets...`.

## Docs

- Delete what restates `cld help` or the code; link to the help or the decision instead.
- A fact found in two places stays in the one it belongs to, and the other links there.
- Split long sentences at their semicolons, dashes and colons. Lists beat long sentences.
- Keep headings that something links to: `make lychee` checks anchors.

## Vale

Each rule is an error. Rewrite the sentence; never add an exception or lower a level.

- `Microsoft.SentenceLength`: 30 words or fewer, outside code spans.
- `write-good.TooWordy`: phrases such as `it is`, `however`, `multiple`, `in order to`,
  `additional`, `minimum` or `type of`.
- `proselint.Needless`: needless variants, such as `administrate` for `administer`.

## Before you delete

For each sentence you remove, find where its fact lives: the code, a decision, the guide or the
help. Where it lives nowhere else, keep it, or move it to where it belongs.

## Check

- `make vale FILES='PATHS'`, `make sizecheck FILES='PATHS'`, `make lychee`.
- For Go files, also `gofmt -l .` and `make golangci-lint`: a changed comment line must fit in
  100 columns.
- Commit as `docs(scope): ...` for docs, or `refactor(scope): ...` for comments.
