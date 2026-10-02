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
