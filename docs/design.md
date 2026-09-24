# cld: design and research

This document records the research behind turning `cld` - a tmux + Claude Code launcher that
started life as a function in `~/.bashrc` - into a tested, distributable tool.

## Starting point

```bash
# A private server (-L cld, no ~/.tmux.conf) keeps these options away from any other tmux use:
# - extended-keys on: tmux answers no kitty keyboard query, so claude falls back to
#   modifyOtherKeys, which tmux forwards only when this is on (Shift+Enter and friends)
# - mouse on, focus-events on: claude probes both and hints when they are off; with the
#   mouse off the wheel cannot scroll claude's fullscreen transcript
# - allow-passthrough on: claude wraps its notifications and OSC 52 copies in tmux passthrough
# - status off: claude keeps the whole tab
# - prefix C-q: claude binds C-b (background a task) and nearly every other Ctrl key, but not
#   C-q; detach is C-q d, and C-q C-q sends a C-q through
# -AD attaches detaching other clients. tmux keeps claude's title changes to the pane
# (set-titles is off), so the tab keeps the session name. The server keeps the environment
# of the client that started it, and claude trusts TERMINAL_EMULATOR over TERM_PROGRAM=tmux:
# a server started from the JetBrains terminal would make every claude on it act as if in
# JediTerm (extended keys off, so Shift+Enter submits) even when attached from iTerm2 -
# hence the env -u.
cld() {
    local name="cld-${1:-main}"
    printf '\033]0;✳ %s\007' "$name"
    env -u TERMINAL_EMULATOR tmux -L cld -f /dev/null \
        set -s extended-keys on \; set -s focus-events on \; \
        set -g mouse on \; set -g allow-passthrough on \; set -g status off \; \
        set -g prefix C-q \; bind C-q send-prefix \; \
        new-session -AD -s "$name" -c "$PWD" "claude --name $name"
}
```

## Findings (probed against tmux 3.6 on Linux)

| Probe | Result |
|---|---|
| `cld "a b"` | the command string goes through `sh -c`, so claude receives `--name cld-a b`: `b` becomes an initial prompt |
| `cld foo.bar` | tmux renames the session to `cld-foo_bar` (`.` and `:` are target separators), while claude's `--name` and the tab title keep `cld-foo.bar` |
| argv form (`new-session ... claude --name "$name"`) | documented in tmux(1): a command given as several arguments is executed directly, without `sh -c`; `cld-a b` arrives as one argument |
| `cld` inside another tmux (`$TMUX` set) | nesting works: the private socket is a different server, so tmux does not refuse |
| `TMUX_TMPDIR` under a deep directory | `error connecting to ... (File name too long)`: the socket path hits the ~108-byte `sun_path` limit, so test sandboxes need short socket directories |
| real `claude` under tmux, first 12 s | enables `?2004` bracketed paste, `?2031` colour-scheme reports, `?1004` focus, `?1049` alt screen, `?1000/1002/1003/1006` SGR all-motion mouse; queries XTVERSION (`CSI > 0 q`), kitty keyboard (`CSI ? u`), DA1, DECRQM `?2026`; resets modifyOtherKeys (`CSI > 4 m`); sets the title `✳ <name>`. The pane stayed in key mode `VT10x`: no extended keys were requested in that window - because the probing shell carried `TERMINAL_EMULATOR` (see What the tests found) |

## Distribution

- `cld` changes nothing in the calling shell (no `cd`, no `export`), so it does not need to be a
  shell function. It becomes an executable `bin/cld` that ends in `exec tmux ...`: testable in
  isolation, versioned, installable.
- Tagged GitHub releases (`vX.Y.Z`) publish the script, with its version stamped in, plus a
  SHA-256 checksum. The install one-liner downloads `releases/latest/download/cld` into
  `~/.local/bin`; `make install PREFIX=...` does the same from a clone.
- A Homebrew tap is possible later; a `curl | bash` installer is not needed for a single file.

## Testing

### Layers

1. **Static**: ShellCheck and shfmt.
2. **Behaviour against real tmux**: tmux is local and cheap, so it is not faked. Only `claude` is
   replaced, by a *probe* that behaves like claude towards the terminal (the modes above), logs
   its argv, cwd, environment and raw input bytes, and emits OSC sequences on request.
3. **Terminal contract**: the same checks run against several *outer terminals* through drivers.

Isolation needs no seams in the script: `TMUX_TMPDIR` moves the `-L cld` socket into a sandbox,
`HOME` points at a temporary directory, `TMUX` is unset, and the probe is first on `PATH`.
`new-session -AD` attaches, so it needs a pty; a terminal driver provides one.

### Terminal contract

| # | Check | Evidence |
|---|---|---|
| C1 | tab title is `✳ cld-NAME` and survives claude's own title changes | terminal |
| C2 | tmux's view of the client: `#{client_termtype}`, `#{client_termfeatures}` (`extkeys`, `focus`, `mouse`, `clipboard`, ...) | tmux |
| C3 | tmux asks the terminal for modified keys and takes the request back on detach; Shift+Enter reaches claude distinct from Enter; the Ctrl keys claude binds (`C-b`, `C-_`) pass through; `C-q d` detaches; `C-q C-q` sends `C-q` | probe input log, terminal output |
| C4 | mouse wheel and focus in/out reach claude | probe input log |
| C5 | OSC 52 / OSC 9 wrapped in tmux passthrough, and copies through `tmux load-buffer -w`, reach the outer terminal | terminal |
| C6 | claude never sees `TERMINAL_EMULATOR`, including in a session created from another terminal on a server started from the JetBrains terminal | probe env dump |
| C7 | after detach the terminal is clean: no mouse reporting, no alt screen | terminal |
| C8 | a paste reaches claude bracketed and whole; a prefix key inside it is text, not a binding | probe input log |

Results that legitimately differ per terminal are recorded as per-terminal expectations rather
than skipped, so a terminal gaining or losing support flips a test.

### Drivers

- **tmux (baseline, every push)**: an outer tmux server on its own socket provides the pty.
  Keys through `send-keys`, focus through switching panes, title and modes through format
  variables, OSC 52 through the outer paste buffer. Mouse injection is not possible.
- **JediTerm (headless, every push)**: `jediterm-core` (3.76 at the time of writing) is published
  as `org.jetbrains.jediterm` to `https://packages.jetbrains.team/maven/p/ij/intellij-dependencies`;
  it depends only on slf4j and annotations (no Swing, no pty4j - pty4j lives in the app module).
  `TerminalKeyEncoder` is in core, and `core/tests` already drives the emulator headlessly
  (`BackBufferDisplay`, `TestSession`). The driver spawns `cld` through pty4j with
  `TERMINAL_EMULATOR=JetBrains-JediTerm`, feeds `JediEmulator` + `JediTerminal` with a recording
  display, and encodes keys with JediTerm's own encoder. It covers JediTerm's emulator, not the IDE
  around it (keymap interception stays a manual check).
- **iTerm2 (real app, macOS runner, nightly and tags)**:
  - level 0 - launch only: a Dynamic Profile whose command runs `cld`; assertions come from tmux and
    the probe (C2, C6);
  - level 1 - Python API: title, screen, detach state (C1, C5, C7). External clients need an
    AppleScript cookie unless *Prefs > General > Magic > Allow all apps to connect* is on (its
    `defaults` key is undocumented);
  - level 2 - real key and mouse events (C3, C4): `send_text` cannot express modifiers, so this
    needs posted CGEvents and therefore Accessibility permission on the runner; unknown until
    tried. Fallback: a local run on a Mac before releases. Self-hosted runners are not an option for
    a public repository.

### CI

- every push and pull request: lint; contract x tmux on several tmux versions (Linux containers
  and macOS); contract x JediTerm;
- nightly, on tags and on demand: contract x iTerm2 on macOS, uploading screenshots and logs on
  failure; non-blocking until it proves stable;
- tags: release.

Every Linux job runs the same Docker image a developer runs locally.

### Spike before building the iTerm2 driver

1. Does the pinned iTerm2 start on a hosted macOS runner without a dialog blocking it?
2. Does the Python API connect with "allow all apps" set through `defaults`?
3. Can a single Shift+Enter key event be posted?

## Decisions

1. Naming: names are validated (`[A-Za-z0-9][A-Za-z0-9_-]*`), not sanitised - a silent rename
   would make `cld foo.bar` and the session it attaches to disagree.
2. Inside another tmux: `cld` nests; the private socket already allows it.
3. tmux 3.3 is the minimum, checked at startup with a clear message.
4. JediTerm is pinned at 3.76, the latest published, and bumped deliberately.
5. iTerm2: not automated yet (see Status).

## Implementation notes

Where the implementation departs from the plan above:

- The test harness is Go instead of bats: `go test` with a sandbox package, a terminal package
  (one driver per terminal) and a probe binary that stands in for claude - and, invoked as
  `tmux`, fakes `tmux -V` for the version checks. The JediTerm driver stays Java, because JediTerm
  is a JVM library; the Go side talks to it one line per command.
- The baseline terminal types raw xterm input (`CSI 13;2u`, `CSI I`/`CSI O`, SGR wheel) through
  `send-keys -H`: tmux 3.3a does not know the key name `S-Enter` and types it literally, and an
  outer tmux reports focus changes only to panes of an attached client.
- tmux 3.3a expands `display -p -t =SESSION` to nothing when no client is attached; the tests read
  formats through `list-panes`.
- tmux 3.7 prints nothing for `list-keys -T prefix KEY`, and its `new-session -A` honours `-c`: a
  reattach from another directory moves the session's directory for new windows there. claude keeps
  running where it started, which is what the tests pin.
- `TMUX_VERSION` builds a tmux release from source into the test image, so the newest tmux
  reproduces in Docker, not only on the macOS runner.
- tmux 3.7's `paste-buffer` writes control characters as `^X` unless given `-S`; the baseline
  terminal passes `-S` where `paste-buffer` knows it, since a terminal pastes them as they are.

## What the tests found

JediTerm 3.76 (read from its source, confirmed by the contract):

- it answers no XTVERSION, so tmux records no terminal type and falls back to its defaults for
  `xterm*`: `bpaste`, `clipboard`, `focus`, `title` - but the emulator ignores focus reporting
  (DECSET 1004 is a stub) and does not handle OSC 52. Since cld adds the `extkeys` feature for
  `xterm*` (as Claude Code's tmux docs recommend), tmux also asks it for modifyOtherKeys, which it
  ignores;
- it ignores modifyOtherKeys (`CSI > 4 ; n m`). Shift+Enter becomes ESC CR only with its
  `shiftEnterSendsEscCR` setting, which tmux passes on as Meta+Enter; without it Shift+Enter is CR;
- its wheel constants are named the other way round (`SCROLLDOWN` is xterm's button 64, wheel up),
  but its UI maps an upward turn to it, so the wheel works;
- tmux sends claude a focus-in (`CSI I`) when a client attaches, whatever the terminal supports.

tmux 3.3a to 3.7c:

- a control character inside a bracketed paste reaches claude as it is, and the prefix among
  them is not taken for a binding.

The `VT10x` in Findings came from the probing shell, which carried
`TERMINAL_EMULATOR=JetBrains-JediTerm`: claude's `--debug` log read `extendedKeys=no (env:
terminal=pycharm, no answer)` - the leak `env -u` guards against. From a clean environment the
real claude 2.1.281 put the pane in key mode `Ext 2`, and Shift+Enter inserted a newline (checked
by hand in a nested tmux).

## Status

- Baseline and JediTerm contracts run on tmux 3.3a, 3.4, 3.5a and 3.7c (Linux, Docker; 3.7c built
  from source), and the baseline on Homebrew's tmux under macOS's bash 3.2.
- iTerm2 is not automated: every level beyond "launch only" needs permissions on the runner -
  controlling iTerm2 over AppleScript or its Python API (with authentication switched off), and
  posting synthetic key events (Accessibility). That is a decision for the maintainer, not
  something the test setup should grant itself.
