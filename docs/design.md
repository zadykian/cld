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
| real `claude` under tmux, first 12 s | enables `?2004` bracketed paste, `?2031` colour-scheme reports, `?1004` focus, `?1049` alt screen, `?1000/1002/1003/1006` SGR all-motion mouse; queries XTVERSION (`CSI > 0 q`), kitty keyboard (`CSI ? u`), DA1, DECRQM `?2026`; resets modifyOtherKeys (`CSI > 4 m`); sets the title `✳ <name>`. The pane stayed in key mode `VT10x`: no extended keys were requested in that window |

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
| C3 | Shift+Enter reaches claude distinct from Enter; other Ctrl keys pass through; `C-q d` detaches; `C-q C-q` sends `C-q` | probe input log |
| C4 | mouse wheel and focus in/out reach claude | probe input log |
| C5 | OSC 52 / OSC 9 wrapped in tmux passthrough reach the outer terminal | terminal |
| C6 | claude never sees `TERMINAL_EMULATOR`, including in a session created from another terminal on a server started from the JetBrains terminal | probe env dump |
| C7 | after detach the terminal is clean: no mouse reporting, no alt screen | terminal |

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

## Open decisions

1. Naming contract: reject or sanitise names with whitespace, `.` or `:`.
2. Running `cld` inside another tmux: refuse, nest or switch.
3. Minimum tmux version (3.3 is the floor implied by `allow-passthrough`; `extended-keys` needs 3.2).
4. JediTerm version: match the IDE's bundled one or track the latest.
5. iTerm2 keyboard tests if hosted runners cannot post key events.
