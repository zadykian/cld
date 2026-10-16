# Ghostty's tabs on a Mac

On a Mac, cld keeps a record of Ghostty's windows, tabs and splits, and of the ssh host and cld
session each pane shows ([decision 59](../design/decisions/0059-ghostty-tabs.md)). The habit stays
the same: open a tab, type `ssh ws`, then `cld join -s S` on the host. The Mac needs cld, but
neither tmux nor claude. This release keeps the record and shows it with `--dry-run`; reopening
from it comes later.

## Ghostty's settings

- **Ghostty 1.4 or newer.** 1.3.1's AppleScript gives no terminal's process or tty, and cld refuses
  it.
- **`macos-applescript`**, on by default. Off, cld says so.
- **`window-save-state = always`**, so that Ghostty brings its windows back after Cmd+Q. Under
  `default`, the default, that rests on macOS's own setting for closing windows as an app quits.
- **No `title`** in Ghostty's config, and no title set with Change Terminal Title…. Either hides
  cld's title, and with it the pane's session. A renamed tab is fine.
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

- Where one runs, `--watch` prints nothing and starts none. Outside a Ghostty pane it fails: macOS
  would charge the watcher's Apple Events to the app it runs in.
- Its log is `tabs/watch.log` in `$XDG_STATE_HOME/cld`, by default `~/.local/state/cld`, emptied
  past 1 MB. After `cld update`, the watcher runs the new cld at its next read.

## The Automation prompt

The watcher asks Ghostty through AppleScript, and runs under Ghostty. So macOS may ask, once,
whether Ghostty may control Ghostty: allow it. System Settings › Privacy & Security › Automation
lists the grant under Ghostty. `cld restore tabs --dry-run`, run from Terminal.app or another
app's terminal, asks for that app's grant instead, and `--watch` there starts no watcher.

## What the record misses

- The watcher reads Ghostty every 5 s. A move between sessions, such as `C-q s`, followed by a close
  within that time records the session from before the move.
- cld reads ssh's words from the Mac's `ps`, which joins them with spaces. So an option whose value
  holds a space, such as `-o "ProxyCommand nc %h %p"`, can make cld take the wrong word for the
  host. Keep such options in `~/.ssh/config`.
- A host with characters other than letters, digits and `._@-` is recorded as ssh to no host.
- The quick terminal is not recorded.

## Reading the record

`cld restore tabs --dry-run` reads Ghostty once and records what it shows, as the watcher does, then
prints the record:

- Ghostty's version and pid, and the watcher's pid, its last read and its log's last line;
- the panes a restore would join again, each with the session and what cut it: Ghostty's quit, or a
  line typed that left the pane at its prompt;
- the losses: windows closed or lost as Ghostty quit, and tabs and splits closed one at a time;
- those that Ghostty's undo can still bring back, until `undo-timeout`;
- notes on Ghostty's settings, which `ghostty +show-config` gives.

Without a watcher, it still records what it sees, and a quit since the last read.

## Not checked yet

No test runs Ghostty. What cld relies on was read in Ghostty's source, 1.3.1 and main before 1.4.0
([findings](../design/findings/terminals.md#ghosttys-applescript-on-macos)), and the tests run cld
against a model of it. A run on a real Mac is still to come (#177): the prompt, the watcher's grant
after its pane closes, and Ghostty's answers.
