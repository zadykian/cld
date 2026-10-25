---
paths:
  - "**/*.go"
  - "**/*.md"
---

# Writing

The [`writing` skill](https://github.com/zadykian/skills/blob/main/skills/writing/SKILL.md) holds
the policy for comments and docs. `make sizecheck`, `make vale` and, in Markdown, `make rumdl`
check the measurable part, each warning counting as an error; review checks the rest.

- Vale reads the Go comments too, through the Markdown mirrors that `tools/valemirror` writes.
