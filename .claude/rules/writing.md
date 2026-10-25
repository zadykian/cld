---
paths:
  - "**/*.go"
  - "**/*.md"
---

# Writing

The [`writing` skill](https://github.com/zadykian/skills/blob/main/skills/writing/SKILL.md) holds
the policy for comments and docs. `make sizecheck`, `make vale` and, in Markdown, `make rumdl`
check the measurable part, each warning counting as an error; review checks the rest.

- In Go, a package comment is 15 lines or fewer, as `make sizecheck` checks.
- Vale reads the Go comments too, through the Markdown mirrors that `tools/valemirror` writes.
