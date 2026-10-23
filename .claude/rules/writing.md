---
paths:
  - "**/*.go"
  - "**/*.md"
---

# Writing

What cld adds to the `writing` skill, which holds the shared policy for comments and docs. This
file wins where they differ. `make sizecheck`, `make vale` and, in Markdown, `make rumdl` check
the measurable part, each warning counting as an error; review checks the rest.

- Behaviour lives in the guide, and reasons in a decision record ([docs.md](docs.md)).
- Docs do not repeat `cld help`; they say what the help leaves out.
- In Go, a comment block is 4 lines or fewer, and a package comment 15 or fewer, as
  `make sizecheck` checks.
- A Go doc comment starts with the name it documents, as `// Setup sets...` does.
- Vale reads the Go comments too, through the Markdown mirrors that `tools/valemirror` writes.
- golangci-lint's `lll` holds each Go line to 100 columns, a comment's too.
