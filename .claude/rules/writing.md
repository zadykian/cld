---
paths:
  - "**/*.go"
  - "**/*.md"
---

# Writing

How comments and docs are written. `make sizecheck` and `make vale` check the measurable part,
each warning counting as an error; review checks the rest.

## What to write

- A comment gives a block's purpose in one or two sentences, or a non-obvious reason with the
  finding behind it. It never narrates the code.
- Each fact lives in one place: code details in code, reasons in a decision record, behaviour in
  the guide. Other places link to it.
- Docs do not repeat `cld help`; they say what the help leaves out.

## The caps

- Sentences of 30 words or fewer; Vale counts the words outside code spans.
- Files of 300 lines or fewer. In Go, a comment block is 4 lines or fewer, and a package comment
  15 or fewer.
- Prose is wrapped at 100 columns, Markdown and Go comments alike.
- Vale refuses wordy phrases (`write-good.TooWordy`), such as `however`, `minimum`, `multiple`,
  `in order to` or `it is`, and needless variants (`proselint.Needless`).
