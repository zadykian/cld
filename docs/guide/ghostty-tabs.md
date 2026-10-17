# Ghostty's tabs on a Mac

On a Mac, cld keeps a record of Ghostty's windows, tabs and splits, and of the ssh host and cld
session each pane shows ([decision 59](../design/decisions/0059-ghostty-tabs.md)). After Ghostty
quits, or a window closes, `cld restore tabs` joins each pane's session again, and reopens what
Ghostty did not bring back. The habit stays the same: open a tab, type `ssh ws`, then
`cld join -s S` on the host. The Mac needs cld, but neither tmux nor claude.

## Ghostty's settings

- **Ghostty 1.4 or newer.** 1.3.1's AppleScript gives no terminal's process or tty, and cld refuses
  it.
- **`macos-applescript`**, on by default. Off, cld says so.
- **`window-save-state = always`**, so that Ghostty brings its windows back after Cmd+Q. Under
  `default`, the default, that rests on macOS's own setting for closing windows as an app quits.
- **No `title`** in Ghostty's config, and no title set by hand, with Change Terminal Title… or, on
  1.4, AppleScript's `name override`. Each hides cld's title, and with it the pane's session. A
  renamed tab is fine.
- **`macos-titlebar-style`** other than `hidden`, under which a new tab opens as a window.
- **`ssh-terminfo` in `shell-integration-features`**, which installs Ghostty's terminfo on each host
  you reach. Give the whole list on one line: a later `shell-integration-features` line replaces it,
  in the same file or one it includes.

`cld restore tabs --dry-run` notes a `window-save-state` other than `always`, a `title`, a hidden
titlebar and a list without `ssh-terminfo`.

## Starting the watcher

The record is kept by a watcher, which `cld restore tabs --watch` starts from a Ghostty pane. It
ends as Ghostty quits, so start it again at each launch: from ssh, or from your shell.

- **From ssh.** The author's playbook, `zadykian/ws`, writes it into `~/.ssh/config` on the Mac, as
  a `LocalCommand` of the hosts it sets up. ssh then starts the watcher as each connection opens.
- **From zsh**, on a Mac without the playbook, in `~/.zshrc`. It checks for macOS too: Ghostty's
  ssh wrapper may send `TERM_PROGRAM` to a host that shares the file.

  ```sh
  [ "$TERM_PROGRAM" = ghostty ] && [ "$(uname)" = Darwin ] && command -v cld >/dev/null &&
    cld restore tabs --watch
  ```

- A restore run in a Ghostty pane starts one too.
- Where one runs, `--watch` prints nothing and starts none. Outside a Ghostty pane it fails: macOS
  would charge the watcher's Apple Events to the app it runs in.
- Its log is `tabs/watch.log` in `$XDG_STATE_HOME/cld`, by default `~/.local/state/cld`, emptied
  past 1 MB. After `cld update`, the watcher runs the new cld at its next read.

## The Automation prompt

The watcher asks Ghostty through AppleScript, and runs under Ghostty. So macOS may ask, once,
whether Ghostty may control Ghostty: allow it. System Settings › Privacy & Security › Automation
lists the grant under Ghostty. `cld restore tabs`, run from Terminal.app or another app's terminal,
asks for that app's grant instead. There a restore starts no watcher, and `--watch` fails.

## What a restore reopens

- **Panes Ghostty brought back.** Each pane that showed a cld session, or ssh to a host, as Ghostty
  quit comes back as a shell. cld types its line into each one that is at its prompt.
- **Windows.** Those lost in the last 15 minutes, and those of Ghostty's last quit, whatever its
  age. `--older` adds one more burst at each run: the newest of those left, and those lost up to
  10 minutes before it. `--all` takes every one the record holds.
- **Tabs and splits closed one at a time**, usually on purpose, only with `--closed`. A tab goes
  back into the window of a pane that was beside it, at its place, and a split beside that pane.
  Where that pane has gone too, either opens as a window.
- Windows open oldest first, so the newest ends in front. Tabs open newest first, as an undo
  would, so that each lands at its place. A second restore reopens nothing new.
- **Not twice.** For each session, and each host of plain ssh, cld counts the panes that joined it
  since the loss. As many of the panes that would join it are left a shell in their directory,
  which keeps the layout.
- **Waiting.** A window closed within Ghostty's `undo-timeout` can still come back by undo, so cld
  leaves it. Run the restore again once that time has passed.
- **Ghostty not running.** cld opens it, and waits up to 10 s for the windows it brings back, so
  that those count first.
- **cld's own pane.** cld never types into the pane it runs in. Where that pane has a session to
  join, cld prints its report, then `Joining session 'S' on HOST here`, and runs its ssh there
  itself.

cld types only into a shell at its prompt. It sends Ctrl+U first, which empties the line in zsh's
default keymap, then the line as a paste, then Enter. Ctrl+U is the key where U sits on a US
layout: under a layout that moves U, Dvorak say, half-typed text stays on the line. Each line that
starts an ssh goes out 0.3 s after the one before, with 2 s more after every 8.

## What rebuilt windows lose

- Their frame, size and place on the screen, their fullscreen state, and their tabs' titles and
  colours. Windows that Ghostty restores itself keep them.
- A tab's splits come back as cld built them, or as their panes' sizes lay them out, and their
  sizes are evened out. A tab keeps the layout cld last saw while a split is zoomed, and the rest
  of it as panes close. Where the sizes fit no layout, as in a resize cld read halfway, it keeps it
  too, or else comes back as a row, left to right.
- A tab first seen with a split zoomed, or in a resize cld read halfway, can take another layout
  until its sizes are whole again. A split dragged elsewhere in a tab cld built stays as built
  where the panes' new sizes fit that layout too.
- **Even sizes** can fit more than one layout, so cld takes the one nearest 1.4 times as wide as
  tall. In a window over 1.4 times as tall as wide, or 2.8 times as wide as tall, half the tabs of
  even sizes come back in another layout. So do a few others. In a window taller than wide, or over
  twice as wide as tall, so can a tab of four panes or more whose sizes Ghostty evened out. Panes of
  one size can come back in another order, such as a grid of four with the top right and bottom left
  ones swapped.
- With `window-padding-balance`, or a `window-padding-x` or `-y` over 4 points, a tab of three
  panes or more may come back as a row, or in another layout.
- **The fullscreen cascade.** A new window opens in native fullscreen where Ghostty's main window
  is in it. So once one is, each rebuilt window opens fullscreen, in a Space of its own. A tab
  rebuilt into a window in non-native fullscreen fails with Ghostty's `Failed to create tab.` Leave
  fullscreen before a restore.

## When a restore fails

- A pane cld could not type into, or a window, tab or split it could not open, is a warning and the
  status 1. So is a tab that lands at another place than its own.
- Where none of a closed window's panes, or of a quit's windows, opened, the next restore tries
  again. Where some opened, it counts as reopened, and the warning counts them.
- **A connection that fails** leaves its pane at its prompt, still bound to its session. The next
  restore types the line again, and `--dry-run` lists the pane as typed and back at its prompt.
- **A join the host refuses**, for a session that ended there on purpose or no longer exists, leaves
  the pane in the host's shell with cld's message. Two minutes on, cld forgets the pane's session.
- **A restore that stops**, as Ghostty quits, macOS withdraws its grant or osascript runs out of
  time, marks nothing reopened. Run `--dry-run` before running it again: a second run may open a
  pane twice.
- **A pane left busy.** In a pane where a program starts after cld reads Ghostty, cld types nothing,
  and says so. The watcher then drops the session of a pane where it sees a program other than the
  shell or ssh, so later restores leave it too.
- One restore runs at a time; a second fails at once.

## Terminfo

With `ssh-terminfo`, Ghostty's ssh wrapper installs its terminfo on a host at your first ssh, and
caches the host once that worked. A restore asks that cache, through `ghostty +ssh-cache`, for each
host. For a host it lacks, the wrapper would add its install script to the line's remote command.
So cld types `TERM=xterm-256color command ssh …` there, past the wrapper, and notes it.

The session then runs as `xterm-256color`. Run `ssh HOST` once from a Ghostty pane, and leave it
with `exit`: later restores keep Ghostty's `TERM`. Where cld finds no `ghostty`, on the `PATH` or
as the running app's, no host counts as cached.

## sshd and fail2ban

The spacing is meant to keep fewer than 10 logins in progress at once, past which sshd's default
`MaxStartups` drops some. Logins that fail all the same count towards a fail2ban's ban. ws's bans
an address for 10 minutes after 5 failed logins. Over ws's tunnel, the Mac's connections all come
from one address, so a ban shuts out `ssh ws`. `ssh ws-ssh` comes from another address meanwhile
([troubleshooting](troubleshooting.md)).

## A name made again

The line joins a session by its name, with `cld join --existing`. A session made again under that
name, by `cld join -s S`, or by a bare `cld join` once the index has expired after 30 days, is
joined all the same. The Mac cannot tell
([decision 58](../design/decisions/0058-joining-only-a-session-that-exists.md)).

## What the record misses

- The watcher reads Ghostty every 5 s. A move between sessions, such as `C-q s`, followed by a close
  within that time records the session from before the move.
- cld reads ssh's words from the Mac's `ps`, which joins them with spaces. So an option whose value
  holds a space, such as `-o "ProxyCommand nc %h %p"`, can make cld take the wrong word for the
  host. Keep such options in `~/.ssh/config`.
- A host with characters other than letters, digits and `._@-` is recorded as ssh to no host.
- The quick terminal is not recorded.
- **A dropped connection.** A connection that drops in a pane that stays open, the Mac asleep say,
  leaves the pane at its prompt. No restore joins it again: cld cannot tell a drop from a detach
  followed by `exit`. ws's `ws-join` covers drops
  ([zadykian/ws#56](https://github.com/zadykian/ws/issues/56)).

## Before a restore

`cld restore tabs --dry-run` reads Ghostty once and records what it shows, as the watcher does, then
prints what a restore would do:

- Ghostty's version and pid, and the watcher's pid, its last read and its log's last line;
- the lines it would type, each with the pane's directory and what cut its session: Ghostty's quit,
  or a line typed that left the pane at its prompt;
- the windows, tabs and splits it would reopen, each pane with its session, its host, or the
  directory of its shell;
- those that Ghostty's undo can still bring back, until `undo-timeout`;
- those it would leave, each with the option that reopens it;
- notes on the plan, and on Ghostty's settings, which `ghostty +show-config` gives.

Without a watcher, it still records what it sees, and a quit since the last read. Where Ghostty
does not run, it plans as if Ghostty brought nothing back, and writes nothing.

## Not checked yet

No test runs Ghostty. What cld relies on was read in Ghostty's source, 1.3.1 and 1.4.0
([findings](../design/findings/terminals.md#ghosttys-applescript-on-macos)), and the tests run cld
against a model of it. A run on a real Mac is still to come (#177): the prompt, the watcher's grant
after its pane closes, and Ghostty's answers. Among them are a restore's typing, its tab moves and
its spacing, and the panes' sizes that give a tab's layout.
