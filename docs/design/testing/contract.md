# Testing: the terminal contract

The same checks run against each outer terminal, through a driver per terminal
([testing](../testing.md#layers)). A result that rightly differs per terminal is an expectation of
that terminal, never a skip, so a terminal gaining or losing support flips a test. Tests cite the
checks by number.

## The checks

- **C1, title.** The tab's title is `✳ cld-NAME`, with `◐` and `◑` in turn while claude is busy,
  and `✳ cld-NAME [w]` while claude works in a linked git worktree. It survives claude's own title
  changes. Evidence: terminal, probe.
- **C2, the client.** tmux's view of the client: `#{client_termtype}` and
  `#{client_termfeatures}` (`extkeys`, `focus`, `mouse`, `clipboard`, `hyperlinks`). Evidence:
  tmux.
- **C3, keys.** tmux asks the terminal for modified keys, and takes the request back on detach.
  Shift+Enter reaches claude apart from Enter, and the Ctrl keys claude binds (`C-b`, `C-_`) pass
  through. `C-q d` detaches, and `C-q C-q` sends `C-q`. Evidence: probe input log, terminal
  output.
- **C4, mouse and focus.** The wheel and focus changes reach claude, and so do a Ctrl+click and an
  Alt+right-click whole, press and release. Over a main-screen program without mouse reporting,
  the wheel scrolls the pane's history. Evidence: probe input log, tmux.
- **C5, passthrough.** OSC 52, claude's notifications (OSC 9, 99 and 777) in tmux passthrough, the
  bell, copies through `tmux load-buffer -w` and claude's OSC 8 links reach the outer terminal. A
  notification does even while a redraw is due, or from a window the terminal does not show.
  Evidence: terminal, terminal output.
- **C6, environment.** Neither claude nor what it starts through tmux sees a variable that names
  the terminal the session was made in ([decision 33](../decisions/0033-terminal-variables.md)).
  Evidence: probe environment dump, tmux.
- **C7, detach.** After detach the terminal is clean: no mouse reporting, no alternate screen.
  Evidence: terminal.
- **C8, paste.** A paste reaches claude bracketed and whole, and a prefix key inside it stays text.
  Evidence: probe input log.
- **C9, exit.** claude exiting ends its session, and its server unless tmux sessions claude made
  keep it. The terminal is left clean. A claude that fails, by a status other than 0 or a signal,
  keeps its session, with its message and how to end it. Evidence: terminal, tmux.
- **C10, the session list.** `cld list` on a terminal reads the terminal's own keys. Down and Enter
  join the second session, titled `✳ cld-NAME`. Ctrl+X twice kills the selected session, whose row
  stays, selected, as `ended`. Ctrl+R, Ctrl+U, letters and Enter rename an `ended` session
  ([decision 57](../decisions/0057-renaming-from-the-list.md)). Esc puts the terminal back: the main
  screen, no mouse reporting, the cursor visible and the same `stty -g`. In `C-q s`'s popup, Esc
  closes the list, and Down and Enter move the terminal to the second session
  ([decision 51](../decisions/0051-moving-between-sessions.md)).
  Evidence: terminal, probe, tmux.

## Drivers

### tmux, the baseline

An outer tmux server on its own socket provides the pty, on every push. Keys go by name through
`send-keys`, and Shift+Enter, focus and the mouse as raw xterm bytes through `send-keys -H`. A
paste goes through the outer paste buffer, and formats give the title and the modes. Freezing
stops the outer server with SIGSTOP, so it neither reads the pane nor answers it. A key held down
is one command list that the outer server times. The driver's comments give the workarounds for
tmux 3.3a to 3.7 and uutils.

### JediTerm

`jediterm-core`, pinned at 3.76 ([decision 7](../decisions/0007-jediterm-pin.md)), runs headless on
every push. JetBrains publishes it to its Maven repository; it needs only slf4j and annotations.
The driver is Java, as JediTerm is a JVM library, and the Go side talks to it a line per command.
It spawns cld through pty4j with `TERMINAL_EMULATOR=JetBrains-JediTerm`, feeds JediTerm's emulator
with a recording display, and types keys as key events through JediTerm's own encoder. It covers
the emulator, not the IDE around it: what the IDE's keymap takes stays a manual check.

The emulator reads the pty on a thread of its own, even after pty4j reports the process gone.
Once `Running` is false the screen and modes are final, which C7 and C9 read. Modes that change
while cld runs are waited for. So is mouse reporting before a wheel or a click, as tmux turns the
mouse modes off and on again after it draws. Both races showed only under load.

### Ghostty

libghostty-vt, Ghostty's terminal core at the commit `tests/ghostty/deps.txt` pins, runs headless in
the test process ([decision 54](../decisions/0054-ghostty.md)). The Go bindings link it through cgo,
so only `-tags ghostty` builds the driver, and the Docker image builds the library. The driver feeds
the core from a pty of its own, and types through Ghostty's key, mouse, focus and paste encoders. It
copies what the app adds: its answers, environment, default modes and focus reports
([terminals findings](../findings/terminals.md)). The app's keybindings and link clicks stay a
manual check. It reads the pty on one goroutine, which answers each request for focus reports before
it reads on. Once `Running` is false, every byte the program wrote has gone through the core. Its
own tests cover a request split between two reads, a freeze, and what the contract leaves out: a
resize, a key held down and the attributes.

### iTerm2, planned

Not built ([decision 8](../decisions/0008-iterm2.md)). It would run the real app on a hosted macOS
runner, nightly and on tags, in levels:

- Level 0 launches cld from a Dynamic Profile, and tmux and the probe check C2 and C6.
- Level 1 reads the title, the screen and the detach state through the Python API (C1, C5, C7).
  An outside client needs an AppleScript cookie, unless "Allow all apps to connect" is on; its
  `defaults` key is undocumented.
- Level 2 posts real key and mouse events (C3, C4) as CGEvents, since `send_text` cannot express
  modifiers. That needs Accessibility permission on the runner. The fallback is a run on a Mac
  before releases.

A spike comes first. Does the pinned iTerm2 start on a hosted runner without a dialog in the way?
Does the Python API connect with "allow all apps" set through `defaults`? Can one Shift+Enter
event be posted?

## What the tests found

JediTerm 3.76, read from its source and confirmed by the contract
([terminals findings](../findings/terminals.md)):

- It answers no XTVERSION, so tmux falls back to its `xterm*` defaults. The emulator ignores focus
  reporting, OSC 52, and the modifyOtherKeys request that cld's `extkeys` has tmux send.
- It takes OSC 8 links, which tmux writes only with the `hyperlinks` feature
  ([decision 30](../decisions/0030-links.md)).
- Shift+Enter is CR, or ESC CR with its `shiftEnterSendsEscCR` setting, which tmux passes on as
  Meta+Enter.
- Its wheel constants are named the wrong way round, but its UI maps them right.
- tmux sends claude a focus-in when a client attaches, whatever the terminal supports.

Ghostty at 34f39002, through libghostty-vt and the app's source
([terminals findings](../findings/terminals.md)):

- tmux does not know its XTVERSION answer: `extkeys` and `hyperlinks` come from cld's entry for
  `xterm*`, which matches `xterm-ghostty`.
- Shift+Enter arrives as `CSI 27;2;13~`. A paste keeps its line feeds, and Ctrl+Q in it becomes a
  space.
- Its focus report at each of tmux's requests once dropped the prefix of `C-q d` under load, 3 runs
  in 30. A test that types a prefix just after attaching waits for them
  ([decision 54.5](../decisions/0054-ghostty.md)).

tmux 3.3a to 3.7c ([tmux and the terminal](../findings/tmux-terminal.md)):

- A control character in a bracketed paste reaches claude unchanged, and the prefix there binds
  nothing.
- Under `mouse off` tmux passes a pane's mouse modes on, and claude 2.1.281 scrolled its
  transcript. `mouse on` adds the wheel scrolling the history over a main-screen program (C4).
- From 3.6 the wheel goes to a program in the alternate screen, asked for or not; earlier releases
  entered copy mode.
- Under `allow-passthrough on`, a notification sent while a redraw was due never reached the tmux
  or Ghostty terminal, 3 runs in 3 on 3.5a and 3.7c. Nor did one sent from a window the terminal
  did not show, in any terminal ([decision 55](../decisions/0055-passthrough-from-claudes-pane.md)).

A `VT10x` once seen came from a probing shell that carried `TERMINAL_EMULATOR`: the leak that
[decision 33](../decisions/0033-terminal-variables.md) closes. From a clean environment claude
2.1.281 put the pane in key mode `Ext 2`, and Shift+Enter inserted a newline, checked by hand.
