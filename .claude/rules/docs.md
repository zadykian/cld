---
paths:
  - "**/*.md"
---

# Docs

What each document holds. How to write them is in [writing.md](writing.md).

- `README.md` is the overview, of 150 lines or fewer: what cld does, installing it, the commands,
  and a line per feature that links its page of the guide.
- [docs/guide.md](../../docs/guide.md) indexes the user guide, a page per topic in `docs/guide/`.
  The guide holds what a user may need beyond the README and `cld help`: caveats, what is not
  checked yet, troubleshooting and upgrading.
- `docs/guide.md` keeps its `Upgrading` heading: released copies of `install.sh` link to it.
- [background-sessions.md](../../docs/guide/background-sessions.md) compares cld with Claude Code's
  background sessions, dated by the claude release checked; the README keeps a paragraph of it.
- Neither the README nor the guide explains how cld works inside: that is for the design docs and
  the package comments.
- [docs/design.md](../../docs/design.md) indexes the design docs in `docs/design/`:
  - `overview.md`: the packages, cld's record, a session's life and the distribution.
  - `testing.md`: the test layers, the terminal contract, the drivers, CI, what the tests found,
    and what is not checked yet.
  - `findings/`: probed behaviour of tmux, claude, the terminals and the environment, with the
    versions checked. A file that would pass 300 lines continues in `NAME-2.md`.
  - `decisions/NNNN-slug.md`: a record per decision, which the `adr` skill adds or amends.
- A change that rests on observed tmux or claude behaviour records the probe and the versions in
  the findings file of its subject, linked from the decision it backs.
- A behaviour change goes together across the code and its comments, the help (`cmd/cld` and
  `tests/testdata/help`), the README, the guide, the decision records and the tests.
- A code comment names a decision as "decision N" or by its record's path, and a finding by its
  file. Markdown links the record or the findings file.
- Links to files and headings must resolve: `make lychee` checks them offline.
