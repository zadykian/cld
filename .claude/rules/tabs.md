---
paths:
  - "internal/tabs/**"
  - "internal/ghostty/**"
  - "cmd/cld/restore_tabs.go"
---

# restore tabs

The invariants a change to `cld restore tabs`, its record of Ghostty's tabs or its scripts keeps,
each linking the part of [decision 59](../../docs/design/decisions/0059-ghostty-tabs.md) that gives
its reasons. Ghostty's behaviour is in the
[terminal findings](../../docs/design/findings/terminals.md#ghosttys-applescript-on-macos).

## Ghostty

- Never store a window's or a tab's id: they are addresses, which change while a window lives and
  return after it. A pane is its terminal's id
  ([59.4](../../docs/design/decisions/0059-ghostty-tabs.md)).
- Between AppleScript commands, a script holds only terminal ids, and finds a window or tab again
  by a terminal it holds ([59.2](../../docs/design/decisions/0059-ghostty-tabs.md)).
- Go never builds script text: a script is embedded whole, and its data goes in argv as one JSON
  argument ([59.2](../../docs/design/decisions/0059-ghostty-tabs.md)).
- Tell Ghostty's errors apart by the step and JXA's number, never by English, but for
  `macos-applescript`'s text ([59.2](../../docs/design/decisions/0059-ghostty-tabs.md)).
- A script sends no Apple Event where Ghostty does not run, nor once the Ghostty an apply began
  with has gone, as one would launch it ([59.2](../../docs/design/decisions/0059-ghostty-tabs.md)).
- Until #177's spike, each behaviour that only a Mac shows is marked UNVERIFIED with its check. It
  lives in the one file that rests on it, whose comment names decision 59
  ([59.12](../../docs/design/decisions/0059-ghostty-tabs.md)).

## The record

- Write `tabs.json` by rename, only where its bytes change, under the record's lock; a time in it
  is in UTC, to the second ([59.4](../../docs/design/decisions/0059-ghostty-tabs.md)).
- The watcher never waits for the record's lock at a tick, only to note the end of Ghostty's run.
  It skips a tick whose snapshot began before another cld wrote the record
  ([59.3](../../docs/design/decisions/0059-ghostty-tabs.md)).
- The record's panes are those of its windows alone, and a field a tick cannot read keeps its
  value ([59.4](../../docs/design/decisions/0059-ghostty-tabs.md)).
- A restore takes its own lock without waiting, and holds it and the record's from its merge until
  the record notes what the apply made ([59.4](../../docs/design/decisions/0059-ghostty-tabs.md)).
- apply.js reports each terminal a build makes as soon as it exists, so that one a later step fails
  still enters the record. A loss is restored only once a build made a pane of it, or nothing of it
  remains to build ([59.6](../../docs/design/decisions/0059-ghostty-tabs.md)).

## The panes

- Type only into a shell at its prompt, as the snapshot saw it. The script reads the pane's group
  again just before, and never types into cld's own pane
  ([59.7](../../docs/design/decisions/0059-ghostty-tabs.md) and
  [59.9](../../docs/design/decisions/0059-ghostty-tabs.md)).
- Never pass `--detach-others`: a restore would detach the session on another device
  ([59.7](../../docs/design/decisions/0059-ghostty-tabs.md)).
- A line goes into a pane as typed input, or as a new terminal's `initial input`, never as a
  Ghostty `command`. Lines that start an ssh go out at the plan's pace
  ([59.7](../../docs/design/decisions/0059-ghostty-tabs.md)).
- A session's name and a host are validated, never sanitised: a NAME `session.ValidName` refuses is
  no session, and a host outside letters, digits and `._@-` no host
  ([59.5](../../docs/design/decisions/0059-ghostty-tabs.md)).

## The command

- No tmux or claude runs or is checked on `restore tabs`'s path: no `session.Check`, `Find`,
  `CheckClaude` or `Lock` ([59.1](../../docs/design/decisions/0059-ghostty-tabs.md)).
- The gate is `osascript` on the `PATH`, after the usage checks, never the system
  ([59.1](../../docs/design/decisions/0059-ghostty-tabs.md)).
- `--watch` starts the watcher only in a Ghostty pane, so that macOS charges its Apple Events to
  Ghostty ([59.3](../../docs/design/decisions/0059-ghostty-tabs.md)).
- The packages print nothing: `cmd/cld/restore_tabs.go` prints the report they return, and the
  watcher's lines go to its log.
