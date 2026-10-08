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
