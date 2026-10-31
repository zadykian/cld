# Findings: terminals

What the terminals cld runs in do, apart from tmux and claude. How claude tells them apart is in
[claude.md](claude.md), and how tmux does in [tmux-terminal.md](tmux-terminal.md). iTerm2 has no
probe of its own yet ([decision 8](../decisions/0008-iterm2.md)).

- **A program exiting on a pty4j pty** (pty4j 0.13.13, its source read). `isAlive()` turns false as
  the reaper wakes the reader, while the pty may still hold the program's last output. So the
  JediTerm driver's emulator can lag behind, as check C9 found ([testing.md](../testing.md)).
- **What VS Code's git extension gives its terminal** (microsoft/vscode at d8dfa8f, read
  2026-09-29). `GIT_ASKPASS` and `VSCODE_GIT_ASKPASS_*` by default, and `GIT_EDITOR` and
  `VSCODE_GIT_EDITOR_*` where `git.terminalGitEditor` is on, all naming scripts beside `MAIN`. Each
  script asks the window through `VSCODE_GIT_IPC_HANDLE`'s socket, and exits 1 without it
  ([decision 33](../decisions/0033-terminal-variables.md)).
- **Which terminals take OSC 8 links**. Read on 2026-09-29: the spec in [egmontkob's
  gist](https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda),
  [OSC8-Adoption](https://github.com/Alhadis/OSC8-Adoption), WezTerm's and Alacritty's docs,
  jediterm-core 3.76 and xterm 411's source. kitty, Ghostty, WezTerm, Alacritty from 0.11, VTE's
  terminals, Konsole (off by default), Windows Terminal, VS Code, mintty, foot, iTerm2 and JediTerm
  take them. xterm 411 shows the text alone. WezTerm's `TERM` is `xterm-256color` unless set to
  `wezterm`, and Alacritty's is `alacritty` only where that terminfo entry is installed
  ([decision 30](../decisions/0030-links.md)).

## Ghostty

Ghostty's source at 34f39002, the commit `tests/ghostty/deps.txt` pins, read on 2026-10-08. Its
`build.zig.zon` says `1.3.2-dev`; the app built there reports `1.3.2-dev+0000000` from an archive,
`1.3.2-BRANCH+HASH` from a checkout (`src/build/Config.zig`), and libghostty-vt `0.1.0-dev`. Paths
are in that source. The ghostty driver copies the app ([decision 54](../decisions/0054-ghostty.md)).

- **What the app answers** (`src/termio/stream_handler.zig`). XTVERSION `ghostty VERSION`, DA1
  `CSI ? 62;22;52 c` and DA2 `CSI > 1;10;0 c`. libghostty-vt alone answers XTVERSION `libghostty`,
  DA1 `?62;22` and DA2 `>1;0;0`. The app answers each `CSI ?1004h` with its focus state, which the
  library leaves to its user.
- **What the app gives a program** (`src/termio/Exec.zig`, `src/Surface.zig`). `TERM` is
  `xterm-ghostty`, with `TERMINFO` naming the app's own entry, and `COLORTERM` `truecolor`. It sets
  `TERM_PROGRAM`, `TERM_PROGRAM_VERSION`, `GHOSTTY_RESOURCES_DIR`, `GHOSTTY_BIN_DIR`, also on the
  `PATH`, `GHOSTTY_SHELL_FEATURES` and `GHOSTTY_SURFACE_ID`. It drops `VTE_VERSION` and
  `GHOSTTY_LOG`.
- **The app's defaults** (`src/config/Config.zig`, `src/termio/Termio.zig`). Modes 2027, grapheme
  clustering, and 12, a blinking cursor, start on. A notch of the wheel scrolls three rows, reported
  as three presses.
- **Keys and pastes** (`src/input/key_encode.zig`, `src/input/paste.zig`; the app and libghostty-vt
  share them). With the kitty keyboard protocol off, Shift+Enter is `CSI 27;2;13~`, modifyOtherKeys
  asked for or not. A bracketed paste keeps its line feeds. Sixteen control characters, Ctrl+Q
  among them, become spaces in any paste.
- **What the app keeps for itself** (`src/config/Config.zig`, `src/Surface.zig`). A Ctrl+click over
  a link opens it, and the program gets the press alone. On Linux Ctrl+- shrinks the font, so
  claude's undo, `C-_`, takes Ctrl+Shift+- or Ctrl+/ there, each sending `0x1f`.
- **Its terminfo entry** (`src/terminfo/ghostty.zig`, which needs only Zig's standard library).
  `tests/ghostty/build-lib` prints it with `zig run`, and `tic -x` compiles it. Ubuntu's ncurses
  names it `ghostty`, which cld's entry for `xterm*` would miss.
- **Zig's default CPU** (Zig 0.16.0; CI on 2026-10-09; the dev host's EPYC Milan, without
  AVX-512). `zig build` with no `-Dcpu` builds for the host's CPU. CI's cached library, built on a
  runner with AVX-512, died with SIGILL in `ghostty_terminal_new` on runners without it. Built
  here for `x86_64_v4`, it died the same way. With `-Dcpu=baseline`, as Ghostty's `PACKAGING.md`
  tells packagers, it held no `zmm` instruction, against 49, and the contract passed.

## Ghostty's AppleScript on macOS

What `cld restore tabs` (#176) rests on, read in Ghostty's source and not probed: v1.3.1 and
v1.4.0 both. Main at b3dc3d7 was read on 2026-10-16. Then v1.4.0 was checked against it, on
2026-10-30. Paths are in those sources, and Swift files go by name alone. Each name is unique under
`macos/`. The scripting files match in both but for `Ghostty.sdef`, `ScriptTab.swift`,
`ScriptTerminal.swift` and `ScriptKeyEventCommand.swift`. The spike's results on a Mac follow
(#177).

- **The model** (v1.3.1 and v1.4.0; `macos/Ghostty.sdef`, `AppDelegate+AppleScript.swift`,
  `ScriptWindow.swift`, `ScriptTab.swift`). The application holds windows, a window its tabs, and
  a tab its terminals. A window is one AppKit tab group, listed front to back from
  `NSApp.orderedWindows`, and its tabs come in tab-bar order. A tab's terminals are its split
  tree's leaves, left or top first (`SplitTree.swift`). No class has a split's direction or ratio,
  and a window has no frame.
- **A terminal's id** (v1.3.1 and v1.4.0; `ScriptTerminal.swift`, `SurfaceView_AppKit.swift`,
  `TerminalController.swift`). The id is the surface's UUID, which the code calls stable "for the
  life of a surface". Ghostty's restoration decodes the saved UUID into the new surface, and an
  undo puts the same surfaces back under a new controller. `name` is the surface's title, and
  `working directory` its pwd. On v1.4.0, `name override` reads a title set by hand, `""` where
  there is none, and `""` clears it. A tab's `name override` titles its window alone, leaving its
  terminals' `name`.
- **Window and tab ids** (v1.3.1 and v1.4.0; `ScriptWindow.swift`, `ScriptTab.swift`). They are hex
  addresses: `window-ADDR` while AppKit reports no tab group, `tab-group-ADDR` once it reports one,
  `controller-ADDR` for a controller with no window, and `tab-ADDR` for a tab's controller. So a
  window's id can change while it lives. An undone close makes a new controller, whose id usually
  differs; an address can be reused, so a stale id may name another tab or window. A tab's
  specifier goes through its window's id.
- **Windows against the app-wide terminals** (v1.3.1 and v1.4.0; `AppDelegate+AppleScript.swift`,
  `QuickTerminalController.swift`, `QuickTerminalWindow.swift`). `windows` come from
  `NSApp.orderedWindows`, and the application's `terminals` from `NSApp.windows`, in what the code
  calls undefined order. Neither list's filter drops the quick terminal's controller, and the
  code's comment counts it among the app-wide terminals. Its window is an `NSPanel`, which AppKit
  documents `orderedWindows` as leaving out; the spike checks whether `windows` shows it (check 7).
- **`pid` and `tty`** (v1.4.0, from PR #11922; not in v1.3.1;
  `ScriptTerminal.swift`, `Ghostty.Surface.swift`, `src/pty.zig`). `pid` is `tcgetpgrp` on the
  pty's master: the foreground process group's id. `tty` is the device path `TIOCPTYGNAME` gives,
  cached at its first read. `pid` reads 0 and `tty` `""` while the surface has no pty, or when the
  call fails. By default a child's exit closes its pane, which then leaves `terminals`
  (`src/Surface.zig`). A pane stays open under `wait-after-command`, or after an exit within
  `abnormal-command-exit-runtime`, which macOS counts as abnormal whatever its code. There the pty
  stays open until the surface is freed (`src/termio/Exec.zig`), so `tty` keeps its name, and what
  `pid` reads then is the spike's (check 9).
- **No events** (v1.3.1 and v1.4.0; `macos/Ghostty.sdef`). The dictionary declares no event, and
  Ghostty's only distributed notification tells its own Dock tile plugin of an icon change. So a
  program outside learns of a change only by asking again. The scripting classes are
  `@MainActor`, so each getter and command runs on Ghostty's main thread.
- **`macos-applescript = false`** (v1.3.1 and v1.4.0; `src/config/Config.zig`,
  `AppDelegate+AppleScript.swift`). The default is true, read from the live config at each call.
  When false, the element and property getters return an empty list, `""`, 0, false or nil, with
  no error. The application's own `name`, `version` and `frontmost` still answer. A lookup by id
  returns nil, which the code says makes the specifier fail: an error, whose number is the spike's
  (check 4). Commands fail with `errAEEventNotPermitted`, -1743, and the text "AppleScript is
  disabled by the macos-applescript configuration." `new surface configuration` is such a command,
  though it opens no terminal.
- **A surface configuration** (v1.3.1 and v1.4.0; `ScriptSurfaceConfiguration.swift`,
  `src/apprt/embedded.zig`, `src/termio/Exec.zig`, `TerminalController.swift`). `initial input`
  goes to the pty as written, before any other input, so the pane's own shell reads it as typed.
  Ghostty's App Intent passes its command that way, "to setup things like PATH"
  (`NewTerminalIntent.swift`). A `command` runs as
  `/usr/bin/login [-q] -flp USER /bin/bash --noprofile --norc -c 'exec -l CMD'`, with `-q` where
  `~/.hushlogin` exists. It forces
  `wait-after-command` on, and a window or tab made with one is never restorable. An
  `initial working directory` that is no directory is ignored, and so is a cwd the pane cannot
  access.
- **New windows and tabs** (v1.4.0; `AppDelegate+AppleScript.swift`, `TerminalController.swift`).
  `new window` returns the window, or wraps its controller while AppKit's window order lags.
  `new tab in W` returns the tab. Under `window-new-tab-position = current`, the default, the tab
  goes after W's selected tab, and under `end` after its last. v1.4.0 then selects it at once, where
  v1.3.1 leaves that to a later turn of the run loop. A new window opens in native fullscreen when
  its parent, the main window by preference, is in native fullscreen. With W in non-native
  fullscreen, `new tab` shows an alert sheet on W and fails with "Failed to create tab." With
  `macos-titlebar-style = hidden`, `new tab` makes a window.
- **Splits** (v1.3.1 and v1.4.0; `ScriptTerminal.swift`, `BaseTerminalController.swift`,
  `SplitTree.swift`). `split T direction right`, `left`, `down` or `up` returns the new terminal,
  addressed app-wide by its UUID. It replaces T's leaf with a split at ratio 0.5, and focus moves
  to the new terminal. For `right` and `down`, T keeps the left or top half. v1.3.1 finds T's
  controller by T's window, and v1.4.0 by the tree that holds T (PR #13640).
- **Typing** (v1.3.1 and v1.4.0, but where marked; `ScriptInputTextCommand.swift`,
  `Ghostty.Surface.swift`, `src/Surface.zig`, `ScriptKeyEventCommand.swift`). `input text` takes
  the paste path with paste protection skipped. So the text is encoded as the pastes in
  [Ghostty](#ghostty) are, and bracketed when the program asked for bracketed pastes.
  `send key NAME modifiers MODS` presses a key named as in `Ghostty.Input.Key`, such as `u` or
  `enter`, with shift, control, option or command. v1.3.1 sends the key with no text, and v1.4.0
  derives its text through the keyboard layout (PR #13205).
- **`perform action`** (v1.3.1 and v1.4.0; `src/input/Binding.zig`, `TerminalController.swift`,
  `Ghostty.App.swift`, `SplitTree.swift`). `move_tab:N` moves by N places. On macOS it moves the
  window's selected tab, clamped at either end, where `Binding.zig` says it wraps. With titlebar
  tabs on macOS 26, v1.4.0 moves the tabs passed instead, to the same order. It acts only when the
  terminal is its tab's focused one, yet returns true for any tab with siblings. `equalize_splits`
  gives each split its first child's share of the leaves along its direction, a crossing subtree
  counting as one.
- **`focus` and `select tab`** (v1.3.1 and v1.4.0; `ScriptTerminal.swift`,
  `BaseTerminalController.swift`, `ScriptTab.swift`). `focus` moves focus on a later turn of the
  main run loop, then orders the window front and activates Ghostty. `select tab` orders the tab's
  own window front and makes it key.
- **`window-save-state`** (v1.3.1 and v1.4.0; `src/config/Config.zig`, `AppDelegate.swift`,
  `TerminalController.swift`, `SurfaceView_AppKit.swift`). Ghostty maps it onto AppKit's
  `NSQuitAlwaysKeepsWindows`. Its docs say `default`, the default, saves state only after a forced
  quit or where macOS's own setting asks. So under `default`, what a normal quit saves rests on
  that setting, whose default the spike finds (check 8); `always` saves at every exit. A pane
  comes back with its pwd, UUID and title in a fresh shell, since no command is saved. v1.4.0 also
  marks no window restorable under `never`.
- **`undo-timeout`** (v1.3.1 and v1.4.0; `src/config/Config.zig`, `TerminalController.swift`,
  `BaseTerminalController.swift`, `ExpiringUndoManager.swift`, `src/termio/Exec.zig`). The default
  is 5 s, and 0 turns undo off. Closing a window, tab or split registers an undo that holds its
  live surfaces until the timeout ends. Until then their ptys and processes run on; the code calls
  these "undo/redo operations that contain running ptys". A freed surface tries a SIGHUP to its
  child's process group through `killpg`, and on Darwin ignores an EPERM, which the code calls
  expected. On macOS that child is `/usr/bin/login [-q] -flp USER`.
- **The ssh wrapper** (v1.3.1's `src/shell-integration/zsh/ghostty-integration`; v1.4.0's
  `src/cli/ssh.zig`). `shell-integration-features` holds `ssh-env` and `ssh-terminfo`, both off by
  default, and either makes the shell integration define an `ssh` function. On v1.3.1 that zsh
  function runs `command ssh "$@"` itself. On v1.4.0 it runs `ghostty +ssh FLAGS -- "$@"`, which
  spawns ssh as its child and waits. Both install the terminfo through an ssh of its own, before
  the session's, given the user's words and then the install script. So a remote command among
  those words gets the script appended.
- **`ghostty +ssh-cache`** (v1.3.1's `src/cli/ssh_cache.zig` and
  `src/shell-integration/zsh/ghostty-integration`; v1.4.0's `src/cli/ssh_cache.zig` and
  `src/cli/ssh.zig`). The wrapper keys its cache by `ssh -G`'s `user@hostname`, not by the alias
  typed, and v1.4.0 adds `:port` for a port other than 22. `+ssh-cache --host=KEY` exits 1 for a key
  the cache lacks. v1.4.0 also takes `+ssh-cache KEY`, and still takes `--host=KEY` with a
  deprecation warning on stderr, so that spelling works on both. On v1.4.0 a bare hostname matches
  every user and port, and `+ssh` also wants the entry's terminfo version, which the query
  ignores. v1.3.1 adds the entry once the install succeeds, and v1.4.0 only once the session's ssh
  exits 0.
- **Reading the settings** (v1.3.1 and v1.4.0; `src/cli/args.zig`, `src/cli/show_config.zig`,
  `src/config/formatter_file.zig`). Ghostty parses each value of a packed struct, such as
  `shell-integration-features`, from the struct's defaults. So a later line, in the same file or a
  later one, replaces the whole list. `+show-config` prints, by default, only the options whose
  value differs from the default. So an option set to its default prints no line. On v1.4.0 the
  output goes to a pager only when stdout is a tty.
- **What a pane's programs send** (v1.3.1 and v1.4.0; `src/termio/Exec.zig`,
  `src/termio/stream_handler.zig`, `src/os/hostname.zig`, `src/Surface.zig`,
  `SurfaceView_AppKit.swift`). Every pane's environment has `TERM_PROGRAM=ghostty`. OSC 7 counts
  only when its host is `localhost` or the Mac's own `gethostname`; Ghostty logs and drops one from
  any other host. OSC 0 and 2 set the title, unless a `title` in the config fixes it, and one of 256
  bytes or more is ignored. A title set by hand, in Ghostty's Change Terminal Title dialog or
  through v1.4.0's `name override`, holds later ones back until either clears it. `name` then reads
  it, and a restored window brings it back still holding. Changes settle after 75 ms.
- **Pane sizes** (v1.3.1 and v1.4.0; `src/pty.zig`, `src/termio/Exec.zig`, `src/termio/Termio.zig`,
  `src/renderer/size.zig`, `src/Surface.zig`). `Pty.open` sets the winsize, rows, columns and
  pixels, and each resize sets it again with `TIOCSWINSZ`. The size starts as the grid's: columns
  and rows times the cell's size in pixels. A resize gives the surface's pixels less its padding:
  `window-padding-x` and `-y`, 2 points a side by default, times the display's scale. With
  `window-padding-balance` on, the padding also takes the space the grid leaves.
- **One binary** (v1.3.1 and v1.4.0; `macos/Ghostty.xcodeproj/project.pbxproj`, `main.swift`,
  `src/termio/Exec.zig`). The app's executable is `ghostty`, and `main.swift` runs a `+ACTION`
  argument as a CLI action, exiting before AppKit starts. `GHOSTTY_BIN_DIR`, and the `PATH` entry
  Ghostty appends, name the running binary's directory, where the shell integration finds
  `ghostty`. So the app and `ghostty +ssh` both run `Contents/MacOS/ghostty`.
