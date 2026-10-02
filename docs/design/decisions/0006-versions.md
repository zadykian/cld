# 6. Versions

Status: Accepted (#21, #74). Amended by [13](0013-a-server-per-session.md),
[42](0042-remote-control-is-claudes.md), [43](0043-inside-your-own-tmux.md),
[48](0048-restore-after-reboot.md), [50](0050-one-command-join.md) and
[51](0051-moving-between-sessions.md).

## Context

- `tmux -V` reports the client, while a server runs the tmux that started it
  ([tmux findings](../findings/tmux-sessions.md)).
- Every option and command cld runs is in tmux 3.2 or 3.3, but for `display -C` (3.6), which cld
  leaves out below it. Extended keys' mode 2 came in tmux 3.5, with a fix for a crash on a dead
  pane. tmux 3.5 mangles some Shift keys, and runs `#()` jobs with the user's shell, which breaks
  fish. tmux 3.7 is the first to answer DECRQM 2026, which claude needs for synchronized output
  ([tmux terminal findings](../findings/tmux-terminal.md)).
- tmux starts claude with `execvp`, which searches relative `PATH` entries too, and runs a file
  the system will not execute with `/bin/sh` ([tmux findings](../findings/tmux-sessions.md)).
- The [claude findings](../findings/claude.md) record the release each flag and setting cld
  passes came in.

## Decision

### tmux 3.5a or newer

- cld runs on tmux 3.5a or newer, with no upper bound. It checks `tmux -V` at startup, and
  refuses an older tmux with status 1. Completion and the commands that run no tmux skip the
  check.
- A bug-fix letter counts as a third number, so 3.5a is newer than 3.5. A version cld cannot read
  passes.
- The tests run on the newest release, 3.7c, and on 3.5a, the oldest cld runs on. The newest is
  bumped by hand, as JediTerm is ([decision 7](0007-jediterm-pin.md)); the oldest moves with the
  floor.
- The tests build tmux from source: no Debian or Ubuntu release ships 3.7, and Debian testing's
  package would change with the base image. The job names carry no version, so a bump leaves the
  ruleset's required checks alone. The macOS job's Homebrew tmux running ahead of the pin is the
  signal to bump.
- cld goes by tmux's version in two places alone. From 3.6 it passes `display -C`, on
  [43](0043-inside-your-own-tmux.md)'s line and [51](0051-moving-between-sessions.md)'s messages,
  and below 3.6 it leaves 43's line out. From 3.7 it counts Shift+Enter as passed on under
  `extended-keys on`. The tests' baseline passes `paste-buffer -S` only to 3.7 and newer.

### claude 2.1.232 or newer

- cld starts Claude Code 2.1.232 or newer, with no upper bound. It checks claude only where it
  starts one: in a `join` that creates or brings back a session
  ([decision 50.3](0050-one-command-join.md)), and in `restore`
  ([decision 48.8](0048-restore-after-reboot.md)).
- The check comes after the tools and tmux's, so the cheap checks fail first.
- `claude --version` runs the claude that tmux then starts, as tmux starts it: the one in the
  absolute `PATH` entries ([decision 11.5](0011-go-and-cobra.md)), by its path, in the session's
  directory. tmux gets that path.
- cld compares the `X.Y.Z` as numbers, and lets output without a version pass. A `--version` that
  fails is refused with its output. A claude that cannot run at all ends cld with 127 or 126
  ([decision 11.9](0011-go-and-cobra.md)).
- cld reads the output until claude exits, and one second more at most, so that a wrapper's
  process left in the background cannot hold cld up.
- The check runs a script without `#!` with `/bin/sh`, as `execvp` does, but never a binary for
  another machine.

## Consequences

- Why 3.5a (#74): the floor of #21, 3.7, refused the tmux of every supported Debian, Ubuntu and
  RHEL release, and nothing cld runs needs it. Releases 3.5a and 3.6a pass the whole suite,
  while 3.4 crashes and 3.5 has the key and shell bugs.
- Rejected at #21: a floor of 3.5, which would have left 3.5 and 3.6 untested. Keeping 3.3 with
  fewer versions in the tests would leave a branch no CI runs.
- Ubuntu 24.04 and RHEL 9 and 10 ship an older tmux. Their users need Homebrew, a source build,
  or cld 0.3.0, which runs on tmux 3.3 and newer. The README recommends 3.7 all the same.
- After a tmux upgrade, the sessions started before it run on the older tmux until they end. A
  server per session ([decision 13](0013-a-server-per-session.md)) keeps new sessions off those
  servers. Reading each server's `#{version}` was the alternative.
- A check made elsewhere could pass another claude than the one tmux starts: one found through
  `.` in `PATH`, or a version manager's shim that pins claude per directory.
- Why 2.1.232: the tests never run the real claude, so the floor is the first release that takes
  what cld passes and does what it relies on. Resuming's documented behaviour reached 2.1.232
  ([decision 16.7](0016-resume.md), #26).
- The floor of #21, 2.1.222, kept a project's `false` for Remote Control in force, a reason gone
  with [decision 42](0042-remote-control-is-claudes.md).
- Rejected floors for claude: 2.1.133, which needed the Remote Control promise qualified, and
  2.1.281, the version probed. That one refused the stable channel's 2.1.274. Also rejected: a guide
  listing which behaviours need a newer claude.
- `/exit`'s status and the key mode were checked on 2.1.281 alone.
- The floor rises when cld passes or relies on something newer, once the stable channel has it,
  about a week behind. An upper bound would break cld every few days.
