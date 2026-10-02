# In the terminal

## The tab's title and claude's status

The title and `STATE` in `cld list` follow hooks that cld gives claude. They miss:

- an interrupt (`Esc`) while claude writes: busy until the next prompt, or claude's idle
  notification a minute later;
- a prompt that a `UserPromptSubmit` hook of yours blocks: busy until the next one;
- your answer to a permission: `waiting` until the tool has run, and after a refusal until the turn
  ends (not checked yet);
- everything, under `disableAllHooks` or a policy that allows only managed hooks.

A terminal that detaches keeps the title it had. A session of a cld before 0.8.0 shows no status.

Each hook starts a tmux client, and a busy title has each terminal start three programs a second.
With Ubuntu's tmux snap, claude waits a tenth of a second and more at each event, and a busy title
takes some 14% of a core per terminal.

The ` [w]` follows claude's working directory, not a shell command's `cd`, and needs git on your
`PATH` as the session starts. A worktree entered and left within one answer can leave it stale
until claude next changes directory.

## The mouse and links

- In claude's fullscreen view the wheel and clicks reach claude. claude opens a link on Ctrl+click
  or Alt+click, but not on a plain click in Ghostty, nor on Cmd+click in Ghostty or Warp on macOS.
- In the main screen, or once claude has failed, tmux takes the mouse. The wheel opens copy mode, a
  drag copies, a middle-click pastes tmux's copy, and a right-click opens tmux's menu.
- For your terminal's own selection, drag with Shift held: Option in iTerm2, Fn in Terminal.app (not
  checked yet).
- tmux passes claude's links on where `TERM` is `xterm*`, `wezterm` or `alacritty`, to iTerm2 and
  tmux, and to foot from tmux 3.7. Other terminals show plain text.
- Another tmux that cld runs in passes links on, from 3.4, only to a terminal it knows takes them.
- A session of an older cld shows links as plain text. It keeps tmux's own Ctrl+click until it
  ends, so there Alt+click opens a link.

## Scrollback

- In copy mode, `PgUp` and the arrows move, `C-r` and `C-s` search, and `q` leaves. Where `EDITOR`
  or `VISUAL` named vi as the session started, `?` and `/` search.
- In claude's fullscreen renderer, `Ctrl+O` then `/` searches the transcript, and `Ctrl+O` then `[`
  writes it out for copy mode. `/tui` names the renderer, and switches it.
- A session of an older cld keeps tmux's 2000 lines.
- Claude Code's screen-reader mode relies on the terminal's scrollback and turn marks (OSC 133),
  which a session leaves empty. Read the history in copy mode, or run `claude` without cld.

## JetBrains IDEs

In a JetBrains IDE's terminal (JediTerm), Shift+Enter needs the IDE's setting that sends ESC CR.
claude's clipboard copies and focus reports do not arrive there
([what the tests found](../design/testing.md#what-the-tests-found)).

## Terminals that keep C-q

`C-q d` detaches only where the terminal passes `Ctrl+Q` on. Two terminals are known to keep it,
and neither fix is checked yet:

- VS Code on macOS and Windows, and its remote windows, keep it for Quick Open View. Give it to the
  terminal in `settings.json`:

  ```json
  "terminal.integrated.commandsToSkipShell": ["-workbench.action.quickOpenView"]
  ```

- JetBrains IDEs with the Visual Studio 2022 keymap, which Rider bundles, keep it for Find Action.
  Keep "Override IDE shortcuts" on in Settings › Tools › Terminal, and remove `Ctrl+Q` from Find
  Action in Settings › Keymap.

Without the key, `! cld detach` in claude detaches the terminal, as does closing the tab. Once
claude has failed, `!` runs nothing: run the `cld detach` its line names from another terminal.

## Notifications

`"auto"`, the default of claude's `preferredNotifChannel`, goes by `TERM_PROGRAM`. tmux sets that
to `tmux`, so `"auto"` sends nothing. Name your terminal's channel in claude's user settings:

| Terminal | `preferredNotifChannel` | claude sends |
|---|---|---|
| iTerm2 | `"iterm2"`, or `"iterm2_with_bell"` to ring the bell too | OSC 9 |
| kitty | `"kitty"` | OSC 99 |
| Ghostty | `"ghostty"` | OSC 777 |
| Terminal.app, any other | `"terminal_bell"` | the bell |

- Every terminal on the session gets them; a session with none attached shows none, then or later.
- The setting serves claude outside cld too, where `"terminal_bell"` rings where `"auto"` sends
  nothing. cld sets no channel, as the next terminal to join may be another
  ([decision 29](../design/decisions/0029-notifications.md)).
- iTerm2 needs "Notification Center Alerts" on, with "Send escape sequence-generated alerts" under
  "Filter Alerts" (Settings › Profiles › Terminal).
- Not checked yet: a real claude notifying through cld, and iTerm2, kitty and Ghostty showing it.
