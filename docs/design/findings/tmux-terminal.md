# tmux findings: the terminal's side

What tmux does between a terminal and a pane, as probed for cld. The conventions are those of
[tmux-sessions.md](tmux-sessions.md): Linux, and tmux 3.6 where an entry names no release. The
images are those `tests/Dockerfile` builds, and the snap is Ubuntu's tmux 3.7c snap. Terminals
themselves are in [terminals.md](terminals.md), and claude's side in [claude.md](claude.md).

## A client starting

- **A client starting on a terminal** (`tty_start_tty` read in 3.3a, 3.4 and 3.7c). tmux sets the
  terminal's mode, then `tcflush(TCOFLUSH)` drops output the terminal has not read. What the list
  wrote just before becoming `tmux attach-session` was lost now and then under load (3.3a and 3.4 in
  Docker). A terminal answers DA1 (`CSI c`) once it has read what came before, which tells that the
  output landed. See [decision 14](../decisions/0014-the-session-list.md).
- **A DA1 answer later than the list's wait** (3.7c; the terminal frozen 1.5 s against a one-second
  wait). tmux takes the first answer for its own query, and its own answer then reached claude's
  pane as keys. Other releases were not checked.
- **Focus reports asked for at each answer** (`tty_send_requests`, `tty_update_features` and the
  answers' handlers read in 3.5a and 3.7c). tmux asks for DA1, DA2 and XTVERSION, in that order.
  Each answer it takes writes `CSI ?1004h` again under `focus-events on`, and the XTVERSION answer
  sets `#{client_termtype}`. Ghostty answers all three, and reports focus at each request. See
  [decision 54.5](../decisions/0054-ghostty.md).
- **Passthrough while a redraw is due** (`tty_client_ready` read in 3.5a and 3.7c). Under
  `allow-passthrough on`, tmux skips a client with a window redraw due, and the sequence never
  reaches it; `all` writes it anyway. A redraw follows each answer above. See
  [decision 54.5](../decisions/0054-ghostty.md).
- **`RGB` from COLORTERM** (`tty-term.c` read in 3.5a and 3.7c; the Ghostty driver on both). 3.7c
  lists `RGB` for a client whose COLORTERM is `truecolor`. 3.5a goes by terminfo alone, where
  `xterm-ghostty`'s `setrgbf` and `setrgbb` give 24-bit colour without the feature's name.
- **Mouse modes as a client attaches** (3.7c; the baseline terminal and JediTerm 3.76). tmux turns
  every mouse mode off and on again several times, the last after the pane's text. A terminal that
  takes that output in pieces, as the JediTerm driver's does, can show the text with mouse reporting
  off for a moment. See [testing](../testing.md).

## Locale

- **Output to a client without a UTF-8 locale** (3.3a, 3.4, 3.5a, 3.7c; `LC_ALL`, `LC_CTYPE` and
  `LANG` unset or `C`). tmux writes `_` for each character it cannot print, tabs included:
  `cld list` showed `demo_detached_/tmp`, and `café` as `caf_`. `tmux -u` marks the client UTF-8,
  and the output arrives intact. See
  [decision 37](../decisions/0037-sessions-of-another-repository.md) and the overview's
  [Joining](../overview.md#joining).
- **An attached client without a UTF-8 locale** (3.7c; `TMUX` unset). The client shows `_` for each
  character beyond ASCII, and `│` as `x` in the line-drawing set, but the title arrives intact. With
  `tmux -u` everything arrives intact; claude 2.1.284's bundle picks no glyphs by the locale.
- **Control characters in `#{pane_current_path}`** (3.3a, 3.4, 3.5a, 3.7c). 3.3a and 3.7c write 0x01
  and ESC as they are to a UTF-8 client, and as `_` under `C` without `-u`. 3.4 and 3.5a write octal
  escapes, `\001` and `\033`, either way.

## Inside another tmux

- **Reading the outer tmux's state from its pane** (3.7c; `cmd-find.c` and `format.c` read).
  `display -p` through the socket `TMUX` names takes the pane whose tty is the client's, else
  `TMUX_PANE`'s, and an option falls back to the global ones. `client_*` reads the client of that
  session most recently active, else any session's, and is empty with none attached. uutils'
  `tty` 0.8.0 prints no newline. See [decision 43.1](../decisions/0043-inside-your-own-tmux.md).
- **cld's server in a pane of a default tmux** (#68; 3.7c for both). Under the outer tmux's
  `extended-keys off`, Shift+Enter reached the program as Enter, and the outer tmux asked its
  terminal for neither modified keys nor focus reports. OSC 52 copies never reached the terminal,
  nor did OSC 9 and 777 notifications, which cld's server writes unwrapped. With `extended-keys on`,
  `extkeys`, `set-clipboard on` and `focus-events on` there, Shift+Enter and copies came through,
  and tmux asked its terminal for modified keys and focus reports. Set while cld's client was
  attached, they took effect once it attached anew. See
  [decision 43](../decisions/0043-inside-your-own-tmux.md).
- **When tmux asks its terminal for modified keys** (3.7c; `tty.c` and `tty-features.c` read;
  `script`'s pty, util-linux 2.41.3, which answers nothing). tmux sends `CSI > 4;2 m` where
  `extended-keys` is `on` or `always` and the terminal has the feature `extkeys`. By default only
  terminals tmux recognises by their answers have it, such as iTerm2 and WezTerm; others need
  `terminal-features`, whose default gives `xterm*` none. VTE, Konsole, Alacritty, kitty, Ghostty
  and Windows Terminal are not recognised.
- **Keys through the user's tmux, by release** (#68; 3.5a, 3.6a and 3.7c in the images). Before 3.7,
  tmux's entry for a tmux terminal lacks `extkeys`, so under the user's `extended-keys on`
  Shift+Enter arrived as Enter; under `always`, and on 3.7c under either, as `CSI 27;2;13~`. 3.5a
  gives `extkeys` to iTerm2, mintty and XTerm, 3.6a to foot too, and 3.7c to WezTerm and tmux too.
  `hyperlinks` goes to iTerm2 and tmux, and from 3.7 to foot. See
  [decision 43.2](../decisions/0043-inside-your-own-tmux.md).
- **A bell, an OSC 9 and an OSC 8 link through the user's tmux** (#68; 3.7c, the user's tmux at its
  defaults; `script`, util-linux 2.41.5). The bell passed on under the default `bell-action any`,
  and the OSC 9 in passthrough rang nothing. The link arrived as its text alone, until
  `terminal-features` gave that tmux's client `hyperlinks`.

## Keys and focus

- **A focus report after the prefix** (`server_client_key_callback` read in 3.5a and 3.7c; 3.7c in
  the image with the Ghostty driver, 3 runs in 30 under load). A focus-in in the prefix table finds
  no binding, so tmux returns to the root table, which has none, and drops it. The key after it
  then went to claude: `C-q d` typed `d`. See [decision 54.5](../decisions/0054-ghostty.md).

- **A dead pane that had focus reporting on** (3.3a, 3.4, 3.5a, 3.7c). 3.3a crashes on
  `kill-session` and detach, and 3.4 on detach and a focus change, taking every session on the
  server. 3.5a and 3.7c survive it all, and `kill-pane` is safe on all four. See
  [decision 6](../decisions/0006-versions.md).
- **Shift keys through extended keys** (3.5 and 3.5a in the images; modifyOtherKeys mode 2). 3.5
  hands Shift+A as `CSI 27;2;65~`, and Shift+Backspace as a Unicode character; 3.5a hands `A` and
  `CSI 27;2;127~`. Shift+Enter arrives as `CSI 27;2;13~` from both. See
  [decision 6](../decisions/0006-versions.md).
- **Two Ctrl+X typed into a raw-mode pane** (3.7c, natively and in the image). From two clients
  50 ms apart they came in two reads; from one command list or paste, in one. tmux writes out what a
  command types in one write; two clients make two writes. See
  [decision 15](../decisions/0015-killing-from-the-list.md).
- **`paste-buffer` of control bytes** (3.5a, 3.7c in the images). 3.7c writes 0x01 and ESC as `^A`
  and `^[` without `-S`, and as they are with it. 3.5a writes them as they are, and refuses `-S`.
- **Synchronized output** (3.7c natively, 3.5a and 3.7c in the images; claude 2.1.284's bundle
  read). Asked DECRQM 2026, 3.7c answers that it supports it, and 3.5a answers nothing. Synchronized
  output came in 3.7. claude asks once the terminal answers XTVERSION, so it draws synchronized
  on 3.7 and newer only.

## Mouse

- **The wheel's default binding** (Debian's 3.5a and 3.6b, 3.7c built from source; `CHANGES`
  read). 3.5a hands the wheel to a pane in a mode or one that asked for the mouse, and otherwise
  enters copy mode. 3.6b and 3.7c hand it to a pane in the alternate screen too. On 3.7c,
  `list-keys` of a single key prints nothing without a client. See
  [decision 36](../decisions/0036-scrollback.md).
- **Clicks with a modifier** (the snap's 3.7c; a pane asking for SGR all-motion reporting). Under
  the default bindings a Ctrl+click reached the pane as its release alone, and an Alt+right-click
  not at all. `C-MouseDown1Pane` swaps panes and `M-MouseDown3Pane` opens a menu, even where the
  pane takes the mouse. Once both are unbound, every click comes whole, and `unbind -n` of an
  unbound key succeeds. See [decision 35](../decisions/0035-modifier-clicks.md).

## Notifications and links

- **Notifications from a pane** (3.7c; OSC 9, 99 and 777 in passthrough, then a BEL). Under
  `allow-passthrough on`, each attached terminal got the three unwrapped, and the BEL under the
  default `bell-action`; with it off, the BEL alone. Sent with no terminal attached, neither reached
  the next one. The pane had tmux's `TERM_PROGRAM`, while `LC_TERMINAL` came through. See
  [decision 29](../decisions/0029-notifications.md).
- **OSC 8 links** (3.7c, the image; `tty-features.c` and `hyperlinks.c` read; ncurses 6.6; claude
  2.1.283 and 2.1.284's bundles read). claude marks links where `TERM_PROGRAM` is `tmux` 3.4 or
  newer. tmux writes a link only to a terminal with the `hyperlinks` feature, which no ncurses 6.6
  entry has and, by XTVERSION, only iTerm2, foot and tmux get. A `TERM` that `xterm*` does not
  match, such as `wezterm`, needs an entry of its own. Features in an entry are separated by `:`:
  `xterm*:extkeys,hyperlinks` added neither, as tmux stops at a feature it does not know. An entry
  set twice at one index stays one. See [decision 30](../decisions/0030-links.md).

## Titles

- **`set-titles` under `status off`** (3.7c; `server-client.c` and `status.c` read). tmux expands
  `set-titles-string` at each redraw, writes the title where it changed, and restores none on
  detach. Setting any option, a user option too, redraws every client, but with `status off` no
  timer does. A `#()` job ending redraws nothing, so a job running `refresh-client -S` a second
  later kept the marker turning. tmux runs a title's job at most once a second per client. With
  `set-titles` on, tmux also sends the active pane's directory (OSC 7) to terminals it credits with
  `osc7`, iTerm2 and foot among them: an empty one for claude, which sets none. See
  [decision 25](../decisions/0025-title-follows-status.md).
- **What a title hook and the busy marker's job cost** (the snap's 3.7c, and 3.7c built from source;
  8 CPUs, load 3 to 9). With the snap a hook took 130 to 230 ms, and the job 14% of a core per
  terminal while busy. Built from source, a hook took 5.5 to 6 ms, and the job 0.5% of a core.
  claude waits for its hooks, so with the snap a turn of 40 tools waits 5 to 9 s more. See
  [decision 39](../decisions/0039-hook-cost.md).

## Messages

- **`display-message` after `new-session` or `attach-session` in one command** (3.7c, in a pane of
  another tmux; `status.c` and `screen-redraw.c` read). It shows on the terminal attached, `-d 0`
  keeping it until a key, a focus report included. Without `-C` (3.6 and newer) the terminal is
  frozen meanwhile. Under `status off` a program entering the alternate screen draws over it, and
  only a full `refresh-client` draws it again. See
  [decision 43.4](../decisions/0043-inside-your-own-tmux.md).
- **That message on 3.5a** (#68; 3.5a, 3.6a and 3.7c in the images). 3.5a has no `-C`, and refuses
  the whole command: no client attaches, and with `new-session` no server starts. Without `-C` the
  terminal stays frozen until a key, and only a full redraw gets through. See
  [decision 43.3](../decisions/0043-inside-your-own-tmux.md).
- **`display-message` with a `%`** (#112; 3.5a, 3.7c in the images). Without `-l` tmux hands the
  text to `strftime` before it reads `#`: `50%done` showed as `5001one` on the 1st. With `-l` (3.4
  and newer) the text shows as given. See
  [decision 51](../decisions/0051-moving-between-sessions.md).

## Popups, jobs and detach-client

- **A popup over a client** (#112; 3.5a, 3.7c in the images). It runs on a pty of its own, with
  `TMUX` and no `TMUX_PANE`, and nothing answers DA1 there. Neither its command nor `-e` expands a
  format, and several words run as they are, without a shell. `-B -w 100% -h 100%` fills the client.
  See [decision 51.1](../decisions/0051-moving-between-sessions.md).
- **`run-shell` from the command line and from a key** (#112; 3.5a, 3.7c). `run-shell` expands its
  command as a format: from a key, `#{client_name}` names the client that pressed it, and
  `#{q:client_name}` quotes it for `sh`. The job has `TMUX` and no `TMUX_PANE`. Through
  `run-shell -b`, a key can show a popup over that client.
- **The shell of `run-shell` and `#()` jobs** (3.5 and 3.5a in the images, `default-shell`
  fish 4.0.2). 3.5 ran them with fish, 3.5a again with `/bin/sh`. fish refuses the title's job,
  `(sleep 1; ...) &`, with status 127. See [decision 6](../decisions/0006-versions.md).
- **The environment of a `run-shell` job and of a popup** (3.5a, 3.7c in the images). Both get the
  server's global environment with the session's over it, not the attached client's. See
  [decision 51](../decisions/0051-moving-between-sessions.md).
- **`#{default-shell}` by the socket** (#112; 3.5a, 3.7c). With a pane's `TMUX` and `TMUX_PANE`, it
  reads the option of that pane's session: the `SHELL` the server started with, unless set.
  `detach-client -E`, below, runs its command with that shell.
- **`detach-client -E`** (#112; 3.5a, 3.7c). The client leaves its session, which runs on, and runs
  `SHELL -c COMMAND` with the session's `default-shell`, in its own directory and environment. `-E`
  expands no format. Bare under `if -F '#{session_attached}'`, it moves the client of the pane's
  session, and does nothing with none attached. Run from a popup over that client, it moves the
  client and closes the popup, its program gone within the second. See
  [decision 51.3](../decisions/0051-moving-between-sessions.md) and, for fish and `sh`'s quoting,
  [environment.md](environment.md).
- **`detach-client` as claude runs a shell command** (the snap's 3.7c; 3.5a and 3.7c in the images;
  `cmd-find.c` and `server-client.c` read in both). `-s =cld-x` detaches every client of the
  session. A bare `detach-client` detaches the client with the latest key, mouse report or focus
  event on the pane's session, found by the tty or `TMUX_PANE`. With no client there it takes
  another session's, and with none on the server it fails, unless run under
  `if -F '#{session_attached}'`. Under cld's mark in that `if`, a server without the mark detached
  nothing. See [decision 44](../decisions/0044-detach-command.md).

## Scrollback

- **What the terminal's scrollback gets** (the snap's 3.7c). Nothing: the outer pane is in the
  alternate screen with no history. OSC 133 marks never reach the terminal; tmux keeps them for copy
  mode's `next-prompt`, which no default key binds.
