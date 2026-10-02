# Idle sessions

A claude holds some 0.2 to 0.5 GB of memory while its session runs, used or not: hence the sweep
([decision 46](../design/decisions/0046-idle-sessions.md)).

- A session is idle while no terminal is attached, from the last key typed into one or the last
  attach. claude working alone (`/loop`), Remote Control and a `busy` status do not count.
- `C-q d` is a key; a terminal that closes, or that another detaches, is not.
- To keep a session, join it now and then, or set `CLD_IDLE_DAYS` in your shell's profile: `90`,
  `0.5`, or `0` for none.
- cld never ends the session it runs in. A session driven through Remote Control alone ends at the
  next `cld list` run elsewhere.
- A terminal that attaches, or a key typed, as cld ends the session keeps it.
- `cld join` without `-s` ends idle sessions after taking its index, so it never gives a name it has
  just freed. Where a server answers with an error, it warns and ends none; `cld list` fails.
- Claude Code removes a transcript 30 days after its last write by default (`cleanupPeriodDays`).
  So a session idle for 30 days can lose its conversation soon after: raise `cleanupPeriodDays`,
  or lower `CLD_IDLE_DAYS`.
- `cld restore` goes by the last start or prompt instead (see [after a reboot](after-a-reboot.md)).
