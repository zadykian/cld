---
paths:
  - "**/*.md"
---

# Docs

What each document holds. How to write them is in the `writing` skill and [writing.md](writing.md).

- `README.md` is the overview: what cld does, installing it, the commands, and a line per feature
  that links its page of the guide.
- [docs/guide.md](../../docs/guide.md) indexes the user guide, a page per topic in `docs/guide/`:
  caveats, what is not checked yet, troubleshooting and upgrading.
- `docs/guide.md` keeps its `Upgrading` heading: released copies of `install.sh` link to it.
- [background-sessions.md](../../docs/guide/background-sessions.md) compares cld with Claude Code's
  background sessions, dated by the claude release checked; the README keeps a paragraph of it.
- [docs/design.md](../../docs/design.md) indexes the design docs in `docs/design/`:
  - `overview.md`: the packages, cld's record, a session's life and the distribution.
  - `testing.md`: the test layers and CI. `testing/` holds a page per subject: `contract.md`, the
    terminal contract with its drivers and what the tests found; `harness.md`; and `status.md`,
    what is not checked yet.
  - `findings/`: probed behaviour of tmux, claude, the terminals and the environment, with the
    versions checked, in a file per subject that stands on its own, never a continuation such as
    `NAME-2.md`.
  - `decisions/NNNN-slug.md`: a record per decision, which the `adr` skill adds or amends.
- A change that rests on observed tmux or claude behaviour records the probe and the versions in
  the findings file of its subject, linked from the decision it backs.
- A behaviour change goes together across the code and its comments, the help (`cmd/cld` and
  `tests/testdata/help`), the README, the guide, the decision records and the tests.
- Links to files and headings must resolve: `make lychee` checks them offline.
