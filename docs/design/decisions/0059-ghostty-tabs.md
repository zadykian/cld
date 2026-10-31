# 59. Ghostty's tabs on a Mac

Status: Accepted (#176).

## Context

On a Mac, the author opens a Ghostty tab, types `ssh ws`, then `cld join -s …` on the host. A move
changes the session a tab shows, and the Mac never learns of it
([decision 51](0051-moving-between-sessions.md)). A quit, a crash, a reboot or a closed window then
loses which pane showed which session. The sessions run on, detached, and each pane must be typed
again from memory.

Under `window-save-state = always`, Ghostty brings a quit's windows back itself, but as fresh local
shells, and never a window the user closed. Ghostty 1.4 gives each terminal in its AppleScript
dictionary the `pid` of its foreground group and its `tty`. With the Mac's process table, they tell
what runs in each pane, and cld's title names the session.

What this rests on in Ghostty is in the
[terminal findings](../findings/terminals.md#ghosttys-applescript-on-macos), read in v1.3.1's
source and v1.4.0's. The results of the spike on a Mac, #177, are still to come. Until
then, each behaviour that only a Mac shows is marked in the one file that rests on it (59.12).

A restore types `cld join --existing`, which never creates a session
([decision 58](0058-joining-only-a-session-that-exists.md)). This decision amends decisions
[60](0060-a-command-for-each-restore.md) and [62](0062-commands-on-another-host.md) (59.1). It also
amends decisions [25](0025-title-follows-status.md) and [26](0026-title-marks-worktree.md): their
title becomes a contract with another machine's cld (59.5).

## Decision

On a Mac, `cld restore tabs --watch` keeps a record of Ghostty's windows, tabs and splits, and of
the ssh host and cld session each pane shows. `cld restore tabs` joins those sessions again where
Ghostty brought their panes back, and reopens the windows a quit or a close lost.

### 59.1 A command of `restore`

`tabs` is a command of `restore`, beside `sessions`
([decision 60](0060-a-command-for-each-restore.md)). cld checks the word after `restore` before
cobra, which would run `restore --help=false tabs` as `restore tabs` (60.1). `restore tabs` takes
no `--host`, an unexpected argument there: it drives the Ghostty of the Mac it runs on.

The gate is the tool: without `osascript` on the `PATH`, `restore tabs` fails with status 1 once
its usage is checked. So its tests run on Linux too, against fakes (59.12). Rejected: darwin alone,
by the check that keeps some `setup` commands to Linux, which would leave the tests to the macos
job. `restore tabs` runs neither tmux nor claude, and checks neither.

This amends 60.1 and 60.3, as `restore` takes and completes `tabs`, and 62.1.

### 59.2 Ghostty 1.4, through osascript

cld drives Ghostty through its AppleScript dictionary alone, by JXA scripts that
`osascript -l JavaScript` runs. Each script is embedded whole and takes its data as one JSON
argument, so Go never builds script text. It prints one JSON document: its result, or the step that
failed with JXA's error number and text. Between commands, a script holds only terminal ids, and
finds a window or tab again by a terminal. A window's id can change while it lives, and a tab's
specifier goes through its window's
([findings](../findings/terminals.md#ghosttys-applescript-on-macos)).

- cld needs Ghostty 1.4 for `pid` and `tty`. It tells 1.4 by the `tty` term, not by the version:
  main has the term, and a build of it reports `1.3.2-dev+0000000`
  ([findings](../findings/terminals.md#ghostty)). With an older Ghostty, cld fails, naming its
  version.
- A snapshot finds Ghostty's process through `NSRunningApplication`, which sends no Apple Event,
  and stops where none runs: an Apple Event would launch Ghostty.
- Automation denied and AppleScript off both fail with -1743, so cld tells them apart by the step.
  Denied, a getter fails. Off, getters answer empty, and `new surface configuration`, which opens
  nothing, fails with Ghostty's text. Each gets its advice.
- AppleScript is a preview in Ghostty. cld carries one set of scripts, written against 1.4.0, and
  shows Ghostty's text as it comes. A later Ghostty that breaks them gets new scripts, chosen by
  feature.

Rejected:

- Ghostty's App Intents: they list terminals with no window or tab, through shortcuts built by hand.
- System Events: a broad Accessibility grant, with no link to a terminal's id.
- `ghostty +new-tab`, which runs on GTK alone.
- tmux's control mode: Ghostty's macOS app does not use it, and it would need a client per server.

