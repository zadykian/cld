---
paths:
  - "**/*.md"
---

# Docs

What each document holds. How to write them is in [writing.md](writing.md).

- `README.md` is the overview: what cld does, installing it, the commands, and a paragraph per
  feature.
- `docs/guide.md` holds what a user may need beyond the README and `cld help`: caveats, what is
  not checked yet, troubleshooting and upgrading.
- The guide compares cld with Claude Code's background sessions, dated by the claude release
  checked; the README keeps a paragraph of it.
- Neither the README nor the guide explains how cld works inside: that is for
  [docs/design.md](../../docs/design.md) and the package comments.
- `docs/design.md` holds the Findings, probed behaviour with the versions checked, the numbered
  Decisions, and the Implementation notes.
- A change that rests on observed tmux or claude behaviour records the probe and the versions in
  Findings.
- A behaviour change goes together across the code and its comments, the help (`cmd/cld` and
  `tests/testdata/help`), the README, the guide, `docs/design.md` and the tests.
- Links to files and headings must resolve: `make lychee` checks them offline.
