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
source and main's at b3dc3d7. The results of the spike on a Mac, #177, are still to come. Until
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
- AppleScript is a preview in Ghostty. cld carries one set of scripts, written against main before
  1.4.0, and shows Ghostty's text as it comes. A later Ghostty that breaks them gets new scripts,
  chosen by feature.

Rejected:

- Ghostty's App Intents: they list terminals with no window or tab, through shortcuts built by hand.
- System Events: a broad Accessibility grant, with no link to a terminal's id.
- `ghostty +new-tab`, which runs on GTK alone.
- tmux's control mode: Ghostty's macOS app does not use it, and it would need a client per server.

### 59.3 The watcher

`--watch` starts one watcher, unless one runs, and returns at once. The watcher is cld again, as
`restore tabs --watching`, in a session of its own, its stdin and stdout `/dev/null`, its stderr
`tabs/watch.log`. It holds the lock of `tabs/watch` on fd 3 for its life, so a second `--watch`
finds it and exits 0, printing nothing.

- **Its starts.** ssh's `LocalCommand` on the Mac, as the author's playbook `zadykian/ws` writes
  it, or a line in the shell's startup file. A restore (59.6) run in a Ghostty pane starts one too.
- **Its guard.** It runs only where `TERM_PROGRAM` is `ghostty`, which Ghostty sets in its panes;
  elsewhere `--watch` fails with status 1.
- **Its identity.** macOS charges an Apple Event to the responsible process, here Ghostty, which
  is signed and carries the Apple Events entitlement. So cld's signature and `cld update` change no
  grant. Started from another app's terminal, the watcher would be charged to that app. #177's
  check 1 sees whether Ghostty asks once to control itself.
- **Its fallback.** That a watcher in its own session stays charged to Ghostty after its pane
  closes is #177's check 2. If not, a LaunchAgent runs `/usr/bin/osascript`, whose identity is
  Apple's, and hands its snapshots to cld.
- **Its ticks.** Every 5 s it takes a snapshot by one osascript, then reads ps and each pane's
  size. A long-lived JXA loop would spare osascript's start at each tick. It waits for #177's
  check 6: the loop's memory, and how long each read stalls Ghostty.
- **Its end.** Before each snapshot it checks Ghostty's pid, sending no Apple Event. As the pid
  goes, it notes the end of the run in the record and exits: it lives as long as one Ghostty run.
  It never takes an osascript error for a quit.
- **Its failures.** Automation denied, AppleScript off, a Ghostty older than 1.4 or a newer record
  stop it, with one line in its log; the next `--watch` starts another. A transient error is
  logged once, then counted.
- **Its races.** It merges under the record's lock. It skips a tick where another cld holds the
  lock, or wrote the record since the snapshot began. That older view would take the windows a
  restore just made for closed.
- **After `cld update`.** It execs the new cld in its own process, keeping its pid, its session,
  the lock and, as #177's check 16 is to show, the responsible process.
- **A handover.** A `--watch` whose watcher reads a Ghostty that has gone starts a new one, which
  waits up to 30 s for the lock as the old one ends. One that finds the lock held while `watch`
  names a watcher that has ended leaves the start to the `--watch` that holds it.

### 59.4 The record

The record lives in `tabs/` beside cld's record of its sessions
([decision 40](0040-session-record.md)), outside `sessions/`, whose expiry removes files it does
not know. The directory is the user's alone. `tabs.json` is written by rename, only where its bytes
change, so a tick that sees nothing new writes nothing. `lock` guards each read and write. A
restore waits for it up to 10 s, and the watcher not at all at a tick, but up to 10 s to note the
end of its run. `watch`'s time is the last tick, `restore` keeps two restores apart, and
`watch.log` is emptied past 1 MB. A record that does not parse is moved aside, with a warning. One
of a newer format is refused, so the newer cld still finds it.

- **Identity.** A pane is its terminal's id, which Ghostty keeps for the surface's life, through
  its restoration and an undo. Window and tab ids are addresses, so the record never stores them.
- **Runs.** A run is Ghostty's pid and its start in ps. The pid comes from `NSRunningApplication`,
  as every `ghostty +ACTION` runs the app's binary too. A new run is a quit of the one before.
- **Panes.** Each holds what it shows: `cld` with its host and session, `ssh` with its host,
  `local`, `other` or `dead`, and since when the binding holds. A field a tick cannot read keeps
  its value, so a pane read badly once is not lost at once.
- **Windows.** Front to back, each tab with its split tree. A terminal that only the app-wide list
  holds is never pending. The quick terminal never enters the record. A window gone from the window
  list but not from the app-wide one, minimized perhaps, stays as last seen, less what closed.
- **Pending.** A window, tab or split whose terminals left every list goes pending. It becomes a
  loss once its panes' logins, Ghostty's children, leave ps: Ghostty keeps a closed surface alive
  until `undo-timeout` ends, whatever its value. Back first, the close was undone: nothing is lost.
- **Losses.** A tab or split lost alone keeps an anchor: a pane still open beside it. At a quit,
  the windows that did not come back are one loss, empty when Ghostty brought every one back. A
  window pending at the quit joins that loss, while a tab or split stays a loss of its own. The
  quit's time is the run's end as the watcher noted it, else its last tick in that run, else now.
- **Bounds.** 32 losses, of this run and the one before, none older than 30 days, as an ended
  session's entry expires after 30 days on the host (40.6).
- **`last`.** A quit gives each `cld` pane, and each `ssh` pane with a host, `last`: the binding
  it cut, for a restore to join again. A restore's typed line sets it too. A tick clears it where
  the pane shows `cld` on any host, another program, or ssh to that host. ssh clears it at once
  until a restore types the line, then after two minutes, as a refused join leaves the host's
  shell. The second run change clears it too.

### 59.5 What a pane shows

The leader of a pane's foreground group, from ps by `pid` and `tty`, tells what the pane shows.

- ssh as the leader, or as the child of `ghostty +ssh` or ws's `ws-join`, makes the pane `ssh`; an
  ssh deeper in the group, a `git push`'s, does not count. The host is ssh's first word that is no
  option, past those that take a value, or the word after `--`, as `cld join --host` gives it. A
  host `--host` would refuse is no host ([62.2](0062-commands-on-another-host.md)), never sanitised.
- An ssh pane is `cld` where its title is cld's: `<marker> cld-NAME`, then `[w]` in a worktree,
  any single marker word, and a NAME that `session.ValidName` takes. The busy markers read as `✳`,
  so the turning writes nothing.
- A shell at its prompt is `local`, any other program `other`, and a pane with no `pid` or `tty`
  `dead`. A pane whose leader ps lacks keeps what it showed, and so does one with cld in front.

The title is the one channel from the host, as Ghostty drops a remote OSC 7. It counts only while
ssh is in front, as a restored pane's title is stale. So cld's title is a contract with another
machine's cld: the marker word may change, but `cld-NAME` after it and `[w]` may not. This amends
decisions [25](0025-title-follows-status.md) and [26](0026-title-marks-worktree.md).

### 59.6 What a restore reopens

A pane Ghostty brought back, at its prompt with a `last`, is joined again in place. A window that a
quit or a close lost is rebuilt: by default those of the last 15 minutes, and the last quit's. Older
ones wait for `--older`, a burst at a time, or `--all`. A tab or split closed alone, usually on
purpose, comes back only with `--closed`. A second restore reopens nothing new.

The dedupe counts: for each session, the panes joined to it by hand since the loss are subtracted
from those to rebuild. A pane it drops opens as a shell in its directory, keeping the layout.
Ghostty is opened where it does not run, so that the windows it restores count first.

### 59.7 The typed line

A pane gets what the user would type: `ssh -t -o ConnectTimeout=10 HOST`, with a login shell that
runs `cld join --existing` for the session, then a shell (58). A pane that showed plain ssh gets
`ssh HOST`.

- **`initial input`, never a `command`.** A `command` runs without the user's rc files, and makes
  a window Ghostty never restores. Typed into the user's shell, the line gets its `PATH`, its agent
  and Ghostty's ssh wrapper.
- **In place**, cld types only into a shell at its prompt, as the snapshot saw it, and checks the
  pane's group again just before. It empties the line first, then types, then presses Enter.
- **Never `--detach-others`**: it would detach the session on another device
  ([decision 23](0023-joining-beside-other-terminals.md)).
- **Terminfo.** For a host that Ghostty's terminfo cache lacks, the line sets
  `TERM=xterm-256color`: Ghostty's wrapper would append its install script to the line's command.
- **Pace.** Lines and new ssh panes go out spaced, so that sshd's `MaxStartups` and a fail2ban
  see no burst; #177's check 15 tunes it.

### 59.8 Split shapes

AppleScript gives a tab's leaves in order, but no split's direction or ratio. A tree cld built is
`made`, and kept while its leaves stay. Otherwise cld infers the tree from the leaves' order and
their panes' sizes in pixels (`inferred`), or makes the leaves a row (`flat`). A rebuilt tab gets
its tree, then `equalize_splits`. Rejected for now: ratios through `resize_split`, which wait for
the sizes of #177's check 10.

### 59.9 cld's own pane

cld finds the pane it runs in by its input's device number, matched to each pane's `tty`, rather
than by running `tty`. A restore never types into it, and its own merge counts it a shell at its
prompt. Where the pane has a `last`, cld execs the same ssh there itself, as `join` execs tmux.

### 59.10 Not taken

Rejected:

- `cld setup tabs`, writing the startup line: ssh's config and the guide's line cover the start.
- A LaunchAgent running cld: its Apple Events would be charged to cld, with a prompt of its own,
  and an update may lose the grant.
- `cld list --json` before a restore: the typed line refuses at join time, with no gap (58.7).
- A record of terminals on the host, set by each join and move: an ssh per lookup, and a contract of
  four parts where the title is one.
- `cld join --on HOST` on the Mac, which changes the habit, and `cld remote` for the command's name.
- Joining Ghostty's restored panes unasked: the user asks with `cld restore tabs`.
- Rebuilt windows' frames, through System Events: a broad grant, and Ghostty keeps the frames of
  the windows it restores itself. Its own window frames are none of the dictionary's.
- Ghostty 1.3.1, whose terminals have no `pid` or `tty`.

Deferred: a connection that drops in a pane that stays open. A detach then an exit within a tick
looks the same, so telling them apart needs `cut: drop` and a typed line's status. ws's `ws-join`
covers drops meanwhile (`zadykian/ws` #56). So within a run no pane gets a `last`.

### 59.11 Help and completion

`restore tabs` has its own help and completes its options, never `--watching`. `--watch` and the
hidden `--watching` stay in every later release with these meanings, as the keys' options do
([decision 51.2](0051-moving-between-sessions.md)): ssh's config and the watcher's exec name them.
Until 59.6 lands, `restore tabs` without `--watch` or `--dry-run` fails with status 2, and its
help names the record alone. `--dry-run` shows Ghostty, the watcher, the panes and losses the record
holds, and notes from `ghostty +show-config`.

### 59.12 Tests

No runner automates Ghostty, as none automates iTerm2 ([decision 8](0008-iterm2.md)). The tests
run cld against a model Mac instead ([testing](../testing/harness.md#the-macs-fakes)): fakes for
osascript, ps, ssh, open and ghostty, ptys for the panes' sizes, and real tmux behind the fake ssh.
A test holds every term the scripts use to Ghostty's dictionary. On the macos job, osacompile checks
their syntax, osascript runs them against a mock Ghostty in JavaScript, and cld reads the real ps.

Ghostty's real answers come from #177's spike alone, and its results go into the findings. Until
then, each behaviour that only a Mac shows is marked UNVERIFIED, with its check, in the one file
that rests on it. A check that fails changes that file, the part of this decision it backs, and
the guide where users see it.

## Consequences

- Each pane comes back to its session with one command, on any Mac that runs the watcher. The
  guide says how to start it, and which of Ghostty's settings it needs.
- A move then a close within one tick records the session before the move. A title fixed in
  Ghostty's config, or set by hand, hides a pane's session.
- Rebuilt windows lose their frame, fullscreen state, tab titles and colours.
- The title and `--existing` are a contract across releases of cld on two machines.
- This release keeps the record and shows it; joining and reopening (59.6 to 59.9) follow in
  #176's next changes.
