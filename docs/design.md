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
| `tmux kill-session -t cld-rev` beside `cld-review` (tmux 3.3a, 3.7c) | kills `cld-review`: a session target is matched exactly, then as a prefix, then as a pattern; `=cld-rev` matches exactly and finds nothing |
| `claude --worktree NAME` (2.1.281) on the branch `feature`, one commit ahead of `origin/master`, the remote's default | branches `worktree-NAME` from `origin/master`; with `--settings '{"worktree":{"baseRef":"head"}}'` from `feature`'s commit, also when the project's `.claude/settings.local.json` sets `baseRef` to `"fresh"` |
| `cld new -n wt -w` with the real `claude` 2.1.281, in a repository without a remote | claude makes `.claude/worktrees/wt` on the branch `worktree-wt` from `HEAD` and moves there; `pane_current_path` follows it, so `cld list` shows the worktree. After `cld kill` the worktree stays, with its uncommitted files and claude's lock, and `cld new -n wt -w` reopens it |
| the same in a directory whose workspace trust was never accepted | claude prints `Error creating worktree: Workspace trust not yet accepted. Run claude once in this directory and accept the trust dialog, then retry with --worktree.` and exits 1; the pane, and the message with it, closed at once until `remain-on-exit failed` (tmux 3.5 or newer); since then it stays on screen with the hint below it |
| how the real `claude` 2.1.281 exits | status 0 for `/exit`, `Ctrl+C` twice and `Ctrl+D` twice; so `remain-on-exit failed` keeps only the sessions of a claude that failed |
| `remain-on-exit-format` (tmux 3.3a, 3.7c) | a non-empty format scrolls the dead pane up a line to write itself at the bottom, so a short error on the top line goes out of sight; `#{session_name}` is empty in it, `#{window_name}` is not. An empty format writes nothing and scrolls nothing. `#{pane_dead_signal}` is a number on Linux and a name (`term`) where the C library has `sys_signame`, as on macOS (tmux 3.7c) |
| `display-message` from the `pane-died` hook (tmux 3.5a, 3.7c) | with no terminal on the dead pane's window it goes to another session's terminal, the one used last; with no terminal attached at all tmux keeps it, as it keeps errors in its configuration, and shows it as `(null):0: claude exited with ...` in view-mode over the next session a terminal attaches to - any session, a new one too - whose claude then gets no keys until `q`. Under `if -F '#{window_active_clients}'` it reaches only a terminal on that window. After `attach-session` in the same command list, `display-message` reaches the attaching terminal, on its message line |
| a dead pane that had focus reporting (`?1004h`) on, client attached | tmux 3.3a crashes on `kill-session` and on detach; 3.4 on detach and on a focus change of the terminal - both with every session on the server. 3.5a and 3.7c survive keys, wheel, clicks, paste, focus changes, resize, detach, reattach, the terminal closing and `kill-session`; `kill-pane` is safe in all four |
| `cld list` where `LC_ALL`, `LC_CTYPE` and `LANG` do not name UTF-8 - unset or `C`, as over ssh, in containers and cron (tmux 3.3a, 3.4, 3.5a, 3.7c) | tmux writes a command's output to such a client with `_` for each character it cannot print: the tabs, so a session showed as `demo_detached_/tmp` under NAME with STATE and DIRECTORY empty, and a directory's non-ASCII letters (`/tmp/café` became `/tmp/caf_`). `tmux -u` marks the client UTF-8, and the output arrives as it is. The other output cld reads - `cld` or `other` from its session lookup, `list-panes`' `0` or `1` - is printable ASCII and passes unchanged |
| `cld` inside another tmux (`$TMUX` set) | nesting works: the private socket is a different server, and tmux refuses a client with `$TMUX` set only when its tty has the name of one of the server's own panes - but see the next row |
| a dead pane's pty (tmux 3.3a to 3.7c) | tmux closes it but keeps its name (`#{pane_tty}`), and the system hands the name to the next pty opened. A client with `$TMUX` set on that pty - a pane of another tmux - is refused with `sessions should be nested with care, unset $TMUX to force`: tmux compares the client's tty with every pane's, dead or alive. An empty `$TMUX` skips the check; set, even empty, it still makes the client take the terminal for UTF-8 whatever the locale says |
| a bare `tmux new-session -d -s cld-x` run inside claude's pane (tmux 3.3a to 3.7c) | the pane's `TMUX` names cld's socket, so `cld-x` lands on cld's server, as with `tmux -L cld` by hand; until cld marked its sessions, `list`, `join`, `kill` and `new` took it for one of theirs |
| `new-session ... \; set -F -t =NAME: @cld '#{session_id}' \; set -w -t =NAME: remain-on-exit failed ...` (tmux 3.3a to 3.7c) | what follows `new-session` takes effect before tmux sees the new pane's program exit, however soon: the mark is there as the session is, and the window's `remain-on-exit` and `pane-died` hook keep and report a pane whose program exits at once. When `new-session` fails (`duplicate session`) tmux skips the rest, so the other session stays unmarked. `set -t =NAME`, like any command that takes a pane, finds nothing: `=NAME:` names the session |
| `#{@cld}` in a format (tmux 3.3a, 3.7c) | tmux looks a user option up in the server's options, then the pane's, the window's and the global window options, and only then the session's and the global session options: a `@cld 1` set with `-s`, `-g` or `-w` counted for sessions that had none, and a window's `@cld 0` hid a session that had one. Compared with the session's id, a flag set anywhere makes no session cld's; one on the server or a window still hides one |
| how `claude` 2.1.282 resolves `remoteControlAtStartup` (read from its bundle, not run: a live check would connect the session to claude.ai) | the first of the policy settings, the `--settings` (flag) settings and the user settings that has it wins, over the old global-config key; a `false` in the project's `.claude/settings.json` or `settings.local.json` beats all of them, and a `true` there is ignored with a warning. `/config`'s "Enable Remote Control for all sessions" writes the user setting, so `--settings` overrides it either way |
| the environment the bash script handed tmux with `exec env -u TERMINAL_EMULATOR tmux ...` and `exec tmux ...` (bash 5.3.9 and 3.2.57, recorded by the fake tmux, and by `printenv` in its place under `set -euo pipefail`), and claude's in the pane of a server that `cld new` started (tmux 3.7c) | bash exported `PWD` set to the working directory, whatever `PWD` it got; `SHLVL=0` when it got none, and a `SHLVL` it got unchanged; and no `_`, not even one it got: once the script has run a command, bash no longer exports it. It dropped an exported `PS1` and `PS2`; `OLDPWD`, which an interactive bash exports after a `cd` - 3.2.57 always, 5.3.9 when it names no directory; and `RANDOM`, `PPID`, `COMP_WORDBREAKS`, `HISTCMD` and `BASH_VERSINFO`, with 5.3.9 also `SRANDOM`, `BASHPID` and `BASH_ARGV0`, and 3.2.57 `LINENO`. Its own variables that came in exported left with its values: `IFS` (space, tab, newline), `OPTIND=1`, `OPTERR=1`, `BASH`, `BASH_VERSION` and `SHELLOPTS`, with the script's `errexit`, `nounset` and `pipefail` added - a bash that reads it turns them on - and with 5.3.9 also `BASHOPTS`, `LINENO`, `PS4`, `EPOCHSECONDS` and `EPOCHREALTIME`; Debian's 5.2.15 dropped and rewrote the same variables as 5.3.9. Exported functions (`BASH_FUNC_NAME%%`) left in bash's own layout; any other variable passed as it came. The Go cld hands on the environment it got, apart from `TERMINAL_EMULATOR` and `TMUX`. tmux sets a pane's `PWD` from `-c`, so claude sees the same `PWD` either way; the rest comes from the server's environment, that of the cld that started the server: no `SHLVL` where claude saw `SHLVL=0`, that cld's `_` - a shell sets it to the path of the command it runs - where claude saw none, and each of the others as that cld got it |
| the script's name check and `list`'s columns under `en_US.UTF-8`, `C.UTF-8` and `C` (bash 5.3.9, glibc 2.43; bash 3.2 on macOS not checked) | `[[ $name =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]` follows the locale's collation: under `en_US.UTF-8` it matched `é`, `ñ`, `ß`, `Ä`, `ǅ`, `①` and `٣`, so `cld new -n café` made `cld-café`, which `tmux -L cld kill-session -t =cld-café` ends (tmux 3.7c); under `C.UTF-8` and `C` it matched ASCII only. `${#name}` counts characters under a UTF-8 locale and bytes under `C`; `printf '%-*s'` pads by bytes under all three, so `é` took three columns of a four-column NAME |
| where bash itself stepped in for the script (bash 5.3.9 on Ubuntu 26.04 unless noted; tmux 3.7c) | a write to stdout that failed (`/dev/full`, or a descriptor open for reading) ended the script under `set -e` with status 1 and bash's message (`printf: write error: No space left on device`, `cat: -: ...` for the usage), and `new` and `join` did not get as far as their `exec` of tmux; `printf` and `cat` failed the same way under 5.2.15 and 3.2.57. A write to a pipe whose reader had gone ended it by SIGPIPE (the usage with status 141, `cat`'s, under `set -e`), and when it was started with SIGPIPE ignored - from a script under `trap '' PIPE`, say - with status 1 and `printf: write error: Broken pipe` (`cat: -: Broken pipe` for the usage). A tmux that could not run at all ended it with 127 when there was no such file, and 126 for a file the system refuses (`Exec format error`); for a `#!` naming a missing interpreter, 127 with Debian's 5.2.15 and 5.2.37 and Ubuntu's 5.2.21, 126 with 5.3.9 and 3.2.57 as released. A text file without `#!` bash ran as a script. A tmux on the `PATH` without the execute permission, with no executable one on it, bash found all the same - its search takes the first file of that name where none is executable, for `command -v` too - and ran, ending with `Permission denied` and 126; a `claude` like that let `new` go on and hand it to tmux, and a `git` like that made `new -w` say the directory was in no git repository. A tmux that stopped being runnable once it had answered `tmux -V` ended the script from a session lookup (`list-sessions`) with its `die 1`, status 1 and bash's message, and from `kill-session` or the `exec` with 127 or 126. With `PATH` unset bash searched a default path built into it, which differs by build: `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` (Ubuntu's 5.2.21 and 5.3.9), `/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin:.` (Debian's 5.2.15 and 5.2.37), `/usr/gnu/bin:/usr/local/bin:/bin:/usr/bin:.` (3.2.57 as released); macOS's `/bin/bash` was not checked. Started in a directory since removed, bash warned `shell-init: error retrieving current directory: ...` and kept the `PWD` it got: `new` passed that path to tmux with `-c`, and tmux started claude in the home directory (with Debian's 5.2.37); `new -w` said the directory was in no git repository |
| the help cobra 1.10.2 generates with its default templates (pflag 1.0.9), from a test program with cld's commands | commands are listed sorted by name unless `cobra.EnableCommandSorting` is false; then in the order they were added, but for the help command - cobra's or one set with `SetHelpCommand` - which `Execute` moves after all the others as it runs (`InitDefaultHelpCmd`). A flag's value shows as its type (`--name string`) unless its usage names it in backquotes. A command with flags gets ` [flags]` at the end of its usage line, after any argument in its `Use`, unless `Use` has `[flags]` already or `DisableFlagsInUseLine` is set. Nothing is wrapped; a newline in a flag's usage goes on under the usage's column. The help function writes to stdout and drops a write that fails: `help`, and `new --help`, with stdout on `/dev/full` or open for reading only print nothing, on stderr either, and exit 0. `help nope` prints ``Unknown help topic [`nope`]`` and the root's usage on stderr and exits 0; `help new join` shows `new`'s help |
| `TMUX_TMPDIR` under a deep directory | `error connecting to ... (File name too long)`: the socket path hits the ~108-byte `sun_path` limit, so test sandboxes need short socket directories |
| real `claude` under tmux, first 12 s | enables `?2004` bracketed paste, `?2031` colour-scheme reports, `?1004` focus, `?1049` alt screen, `?1000/1002/1003/1006` SGR all-motion mouse; queries XTVERSION (`CSI > 0 q`), kitty keyboard (`CSI ? u`), DA1, DECRQM `?2026`; resets modifyOtherKeys (`CSI > 4 m`); sets the title `✳ <name>`. The pane stayed in key mode `VT10x`: no extended keys were requested in that window - because the probing shell carried `TERMINAL_EMULATOR` (see What the tests found) |

## Distribution

- `cld` changes nothing in the calling shell (no `cd`, no `export`), so it does not need to be a
  shell function. It became an executable: testable in isolation, versioned, installable. Up to
  0.3.0 that was a bash script, `bin/cld`, ending in `exec tmux ...`; since then it is a Go
  program (see 11), built per platform, whose `new` and `join` replace themselves with tmux the
  same way (`execve`).
- Tagged GitHub releases (`vX.Y.Z`) publish a binary per platform, `cld-OS-ARCH` for Linux and
  macOS on amd64 and arm64, with the version stamped in, plus `cld.sha256`, which lists their
  SHA-256 checksums. The install one-liner picks the binary from `uname` and downloads it into
  `~/.local/bin`; `make install PREFIX=...` builds cld for the host from a clone, with Go. The
  releases of the script published `cld`; that download fails once a Go release is the latest.
- The binaries are built with cgo off, on the Linux runner: the Linux ones are static, the
  darwin ones link only system libraries (`libSystem`, `libresolv`), and Go's linker signs the
  darwin/arm64 one ad hoc, which Apple silicon requires. They are not notarized.
- A Homebrew tap is possible later; a `curl | bash` installer is not needed for one binary, which
  the one-liner picks from `uname`.

## Testing

### Layers

1. **Static**: gofmt and go vet; ShellCheck and shfmt for `tests/jediterm/fetch-deps`, the one
   shell script left.
2. **Behaviour against real tmux**: tmux is local and cheap, so it is not faked. Only `claude` is
   replaced, by a *probe* that behaves like claude towards the terminal (the modes above), logs
   its argv, cwd, environment and raw input bytes, and emits OSC sequences on request.
3. **Terminal contract**: the same checks run against several *outer terminals* through drivers.

Isolation needs no seams in the program: `TMUX_TMPDIR` moves the `-L cld` socket into a sandbox,
`HOME` points at a temporary directory, `TMUX` is unset, and the probe is first on `PATH`.
`cld new` and `cld join` attach, so they need a pty; a terminal driver provides one.

### Terminal contract

| # | Check | Evidence |
|---|---|---|
| C1 | tab title is `✳ cld-NAME` and survives claude's own title changes | terminal |
| C2 | tmux's view of the client: `#{client_termtype}`, `#{client_termfeatures}` (`extkeys`, `focus`, `mouse`, `clipboard`, ...) | tmux |
| C3 | tmux asks the terminal for modified keys and takes the request back on detach; Shift+Enter reaches claude distinct from Enter; the Ctrl keys claude binds (`C-b`, `C-_`) pass through; `C-q d` detaches; `C-q C-q` sends `C-q` | probe input log, terminal output |
| C4 | mouse wheel and focus in/out reach claude; over a main-screen program without mouse reporting the wheel scrolls the pane's history | probe input log, tmux |
| C5 | OSC 52 / OSC 9 wrapped in tmux passthrough, and copies through `tmux load-buffer -w`, reach the outer terminal | terminal |
| C6 | claude never sees `TERMINAL_EMULATOR`, including in a session created from another terminal on a server started from the JetBrains terminal | probe env dump |
| C7 | after detach the terminal is clean: no mouse reporting, no alt screen | terminal |
| C8 | a paste reaches claude bracketed and whole; a prefix key inside it is text, not a binding | probe input log |
| C9 | claude exiting ends its session, and with the last session the server; the terminal is left clean. From tmux 3.5, a claude that fails - exit status other than 0, or a signal - keeps its session, with its message and how to end it on screen | terminal, tmux |

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
2. Inside another tmux: `cld` nests; the private socket already allows it. Inside a live pane of
   its own server - claude's external editor, say - a session attached would show inside itself,
   and `new` and `join` refuse, pointing at `C-q d`; tmux refuses there too, but advises to unset
   `$TMUX`. tmux goes by the tty's name, and a dead pane's name comes back with the next pty
   opened (see Findings), so `cld` looks at the live panes itself and gives its client an empty
   `TMUX`, which tmux's check skips.
3. Commands (0.2.0): `new` creates a session and fails if it exists, `join` attaches to one and
   fails if it does not; the name moves to `-n NAME` (default `main`). A bare `cld` fails, and
   `cld NAME` fails naming `cld new -n NAME` and `cld join -n NAME`. Commands address sessions as
   `=cld-NAME`, since tmux would otherwise take `cld-rev` for `cld-review`. `list` shows the
   directory claude is in now (`pane_current_path`), not the one its session started in, and
   nothing at all when no server runs. `kill` ends a session with `kill-session`: claude gets
   SIGHUP, as when its terminal closes.
4. Worktrees: `new -w` passes `--worktree NAME` to claude instead of running `git worktree add`.
   claude then applies what it applies to every worktree it makes - `.worktreeinclude`,
   `worktree.baseRef`, `WorktreeCreate` hooks - reopens an existing one, and one repository keeps
   one worktree layout (`.claude/worktrees/NAME`, branch `worktree-NAME`). A new worktree
   branches from `HEAD`: `cld` adds `"worktree":{"baseRef":"head"}` to its `--settings` (see 10), which
   outranks the user's and the project's settings, so the worktree carries the work it was started
   from rather than the remote's default branch. `cld` checks for a git work tree first, which
   saves a round trip through claude; workspace trust, which claude also requires, lives in
   claude's own state, so claude reports it (see 5). `kill` leaves
   the worktree: claude offers to remove it only when it exits on its own. The worktree is named
   after the session, also when that is the default `main`.
5. Failures stay on screen, from tmux 3.5: with `remain-on-exit failed`, a claude that exits
   with an error or a signal keeps its pane, so what it printed - a startup error above all, which
   would otherwise vanish with the session - stays readable. The format is empty, so tmux does not
   scroll that out of sight; a `pane-died` hook shows how to end the session on the message line
   instead, naming it through the session's one window, named `NAME`. The hook shows it only to a
   terminal on that window (`if -F '#{window_active_clients}'`), since tmux would otherwise show it
   on another session's terminal or over the next session attached (see Findings); `join` shows it
   on attaching to such a session, with a `display-message` in the same command list as
   `attach-session`. `list` shows such a session as `exited`, where it started, and `new` refuses
   the name, pointing at `kill`, rather than replacing the session unseen. tmux 3.3 and 3.4 crash
   over a dead pane that had focus reporting on (see Findings), so there the option stays off and a
   failed session closes as before. The option and the hook go to claude's window only (see 9).
6. tmux 3.3 is the minimum, checked at startup with a clear message.
7. JediTerm is pinned at 3.76, the latest published, and bumped deliberately.
8. iTerm2: not automated yet (see Status).
9. Only cld's own sessions: the private server keeps cld's options away from other tmux use, but
   not other tmux use away from cld's server - everything claude runs inherits `TMUX` (see
   Findings). `new` marks each session it starts, in the tmux command that creates it: the user
   option `@cld` holds the session's id, so a `@cld` that a format finds elsewhere first makes no
   other session cld's (see Findings). `list` shows marked sessions only, and `new`, `join` and
   `kill` refuse a name an unmarked session holds, saying so and pointing at `tmux -L cld ls`.
   What `cld` sets for a failed claude (see 5) goes to claude's window, not the server, so such a
   session whose program fails closes as tmux would close it, rather than staying where `cld`
   neither lists nor kills it. Starting claude without `TMUX` would keep its tmux off cld's
   server, and its passthrough and `load-buffer` copies with it; a server per session would still
   need the mark for a session made on it by hand, and `list` would have to find the servers. The
   mark guards against mistakes, not intent: whatever reaches the socket can set it.
10. Remote Control: `new` starts claude with `--settings '{"remoteControlAtStartup":true}'`, so a
   session can also be continued from claude.ai or the Claude app, not only from a terminal that
   joins it. Flag settings outrank the user's, so this holds whatever `/config` says; claude still
   keeps Remote Control off under an org policy or a project that sets the key to `false` (see
   Findings), and `cld` leaves those alone. With `-w` the worktree setting goes into the same JSON:
   one `--settings` rather than two, whose merging claude does not document.
11. Go and cobra (#20): cld is a Go program built on [cobra](https://github.com/spf13/cobra) and
    pflag, a port of the bash script `bin/cld` as 0.3.0 had it. One language for cld and its
    tests, no bash 3.2 to write for, cobra's shell completion to build on, and key input with
    timeouts under a second; the cost is a binary per platform, and Go to build from a clone. The
    commands, messages, exit statuses and output stay, and so do the tmux commands and the
    environment tmux gets, but for these differences:
    1. the spellings pflag accepts: `-nNAME`, `-n=NAME`, short options combined (`-wn NAME`,
       `-wh`, `-hx`) and a value for a boolean option (`--worktree=true`, `--help=false`,
       `-h=false`), where a later `--help=false` takes back an earlier `-h`. The script refused
       each as an unexpected argument;
    2. where no test pins the message, an unexpected argument is quoted as pflag reports it, which
       can be less than was typed: `-x` for `-wx`, `--foo` for `--foo=bar`, and `--worktree=VALUE`
       for a `-w=VALUE` that is not a boolean;
    3. an argument starting with `-test.` is skipped: pflag leaves it to `go test`;
    4. an empty argument to `list`, `help` or `version` is refused as unexpected - by `help`,
       since 12, as an unknown command; the script took it for the end of the arguments
       (`cld list '' x` listed);
    5. cld looks for `tmux`, `claude` and `git` in the absolute `PATH` entries only, and runs
       `tmux`, `git` and `tty` from there: one found only through a relative entry (`.`, or an
       empty one) counts as not installed, and one that both have runs from the absolute entry,
       where bash ran the first it found. Go's `exec.LookPath` refuses a match in a relative
       entry but stops at it, so cld skips those entries itself. With `PATH` unset cld finds
       none of them, where bash searched a default path built into it, which differs by build
       (see Findings). Within the absolute entries cld searches as bash did: the first
       executable file of that name or, where none is executable, the first file of that name,
       which then cannot run - such a tmux ends cld with 126 (see 9), such a claude fails in its
       pane, and such a git leaves `new -w` saying the directory is in no git repository, as
       with the script;
    6. tmux gets the environment cld got, but for `TERMINAL_EMULATOR`, which `new` removes, and
       `TMUX`, emptied (see decision 2). bash had also changed it on the way (see Findings): it
       set `PWD` and `SHLVL` and dropped `_`; dropped an exported `PS1` and `PS2`, and `OLDPWD`
       (3.2 always, 5.x when it names no directory); put its own values in its variables that
       came in exported - `IFS`, `OPTIND`, `OPTERR`, `BASH`, `BASH_VERSION`, `SHELLOPTS` with
       the script's `errexit`, `nounset` and `pipefail`, and with 5.x `BASHOPTS`, `LINENO`,
       `PS4`, `EPOCHSECONDS` and `EPOCHREALTIME` - or dropped them (`RANDOM`, `PPID`,
       `COMP_WORDBREAKS` and the like); and rewrote exported functions. cld hands on all of
       these as it got them, so claude sees them as the cld that started the server got them: no
       `SHLVL` where it saw `SHLVL=0`, and that cld's `_` where it saw none;
    7. `list` pads a name by its characters: bash took the column's width in characters under a
       UTF-8 locale and in bytes under another (`C`, say), and `printf` padded by bytes (see
       Findings), so a name with non-ASCII letters could come out short of its column. Such
       names came from 8, or from a session renamed by hand; their columns now line up;
    8. names are ASCII, as decision 1 has them, whatever the locale: bash's `[A-Za-z0-9]` followed
       the locale's collation, so under `en_US.UTF-8` and the like (glibc; see Findings) the
       script also took letters and digits such as `é`, `ß`, `①` and `٣`, and made sessions such
       as `cld-café`. cld lists such a session but refuses its name to `join` and `kill`, and
       gives no legacy hint for it; `tmux -L cld kill-session -t =cld-café` ends it;
    9. where bash itself stepped in (see Findings), cld keeps the exit status but not bash's
       words. A write to stdout that fails - `list`, `help` and `version`, and the title `new`
       and `join` print - ends cld with status 1, without handing over to tmux:
       `cld: write error: REASON`. A write to a pipe whose reader has gone ends cld by SIGPIPE,
       as it ended the script - also when cld was started with SIGPIPE ignored, where the script
       ended with status 1 and bash's write error: Go's runtime handles SIGPIPE itself, and does
       not tell one ignored at startup from the default. A tmux the system cannot run at all
       ends cld with 127 when the system reports no such file, a missing `#!` interpreter
       included, as bash 5.2 had it, and with 126 otherwise: `cld: cannot run PATH: REASON`. A
       tmux that is a text file without `#!`, which bash ran as a script, is one that cannot run
       (126), as is one without the execute permission (see 5). A tmux that stops being runnable
       once it has answered `tmux -V` ends cld from a session lookup with status 1 and that
       message, as the script's lookups ended it with bash's;
    10. `new` in a directory that has been removed refuses, with `cld: the current directory no
        longer exists`, where the script went on and tmux started claude in the home directory
        (see Findings); the other commands no longer print bash's warning there.

    Settled with it: the port landed before the tmux and claude guards (#21) and a server per
    session (#22), so both follow in Go; `help`, `-h` and `--help` print the one usage text there
    was, rather than cobra's help per command (reversed by 12); the spellings in 1 are accepted
    rather than refused before pflag sees them; and releases publish plain binaries and one
    `cld.sha256`, rather than archives, so installing stays one `curl` and a `chmod`.
12. Help from cobra (#28): `help`, `-h` and `--help` print the help cobra generates from each
    command's `Use`, `Short` and `Long` and its options, rather than the one usage text the port
    kept (decision 11). Each feature edited that text by hand, next to options whose descriptions
    nobody saw, and the two could drift; now a command's text lives with the command, and
    `cld new -h` shows `new`'s options alone. Settled with it:
    1. cobra's default help and usage templates, with command sorting off, so the commands keep
       their order: `new`, `join`, `kill`, `list`, `help`, `version`. `Execute` moves the help
       command after the others (see Findings), so cld moves `version` back after it before it
       prints the help. An option's usage names its value in backquotes (`-n, --name NAME`), and
       pflag shows `-n`'s default, `main`. cobra wraps nothing, so the texts break their lines by
       hand, within 80 columns, which the test of the help's text holds them to. A template of
       cld's own, in the usage text's layout, was the other way: closer to what cld printed, but
       one more thing for cld to keep, where cobra's changes with cobra and shows in that test;
    2. `help [COMMAND]` shows the help of one of the commands the root's help lists. `-h` and
       `--help`, given first, are `help` spelled otherwise, so `cld -h new` shows `new`'s help. A
       `COMMAND` that is not one of those, the empty one included, and an argument after it are
       refused with status 2 - `cld: help: unknown command 'nope' (see cld help)`, `cld: help:
       unexpected argument 'join' (see cld help)` - where cobra shows the root's usage for the
       one and passes over the other, exiting 0 (see Findings). cld's `help` is a command of its
       own, set with `SetHelpCommand`, so it lacks the `ValidArgsFunction` with which cobra's
       completes command names after `help`: completion (#25) has to give it one. Keeping `help`
       without an argument was the other way; per-command help would then be only `-h`'s;
    3. error messages keep `(see cld help)`, rather than naming the command's help (`see cld
       help new`): no message changes;
    4. `-h` and `--help` are read as before: before a wrong argument they show the help, now of
       the command they are given to, and after an argument they are no option but one more
       argument, and refused (`cld help new -h`, as `cld new review -h`). So `help`'s usage line
       names its options before `COMMAND`, `cld help [flags] [COMMAND]`, with
       `DisableFlagsInUseLine`, where cobra adds ` [flags]` at the end (see Findings), and the test
       of the help's text refuses an option after an argument in any usage line;
    5. the help is rendered into a buffer that `fail.Print` prints, so that a write that fails
       still ends cld with status 1 (see 11.9), where cobra's own help function drops the error;
    6. the root's help holds what the usage text said besides the commands and options: what a
       session is, the private server, Remote Control, the detach keys and failed sessions.
       `list`'s one line in the root's help is shorter than the usage text's, and its own help
       says the rest. `version`'s help names `-V` and `--version`, since cobra lists commands only.

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
  reattach from another directory moved the session's directory for new windows there. Since 0.2.0
  `join` attaches with `attach-session`, which moves neither claude nor the session.
- `TMUX_VERSION` builds a tmux release from source into the test image, so the newest tmux
  reproduces in Docker, not only on the macOS runner.
- tmux 3.7's `paste-buffer` writes control characters as `^X` unless given `-S`; the baseline
  terminal passes `-S` where `paste-buffer` knows it, since a terminal pastes them as they are.
- `capture-pane -e` emits an SGR change at the next cell that differs, which moves between
  redraws and sizes (a colour reset can land before or after a line break); the reattach test
  compares cells - characters and attributes - rather than the captured sequences.
- The Go port (decision 11) is `cmd/cld`, the command line, and `internal/session`, the tmux
  side, whose package comment is the script's header comment. Errors carry an exit status up to
  `main` (`internal/fail`), the only place that exits; `new` and `join` end in `syscall.Exec` of
  tmux, and a tmux command that fails (`tmux -V`, `kill-session`) ends cld with its status after
  its own message; one that cannot run ends it with 127 or 126, as `syscall.Exec` failing does.
  A session lookup (`list-sessions`) that fails ends cld with status 1 and what tmux said, or
  the same `cannot run` message, as the script's `die 1` did.
  cld's own output goes through `fail.Print`, which turns a failed write into an error, the help
  too once cobra has rendered it into a buffer; cobra's help function returns nothing, so what
  it printed for `-h` and `--help` reaches `main` through a variable. Reading the sessions is one
  function returning rows, which `list` lays out.
- cobra's defaults give way to cld's command line (cobra 1.10.2, pflag 1.0.9):
  - the first argument is checked before cobra sees it: cobra takes an unknown command for an
    argument of the root, skips options before the command (`cld -n x new` would run `new`),
    and answers its hidden `__complete` and `__completeNoDesc`. `CompletionOptions` turns its
    `completion` command off;
  - `SilenceErrors` and `SilenceUsage`: cobra would print `Error: MESSAGE` and the usage;
  - `SetInterspersed(false)` on every command: pflag reads options up to the first argument,
    where it would pass over arguments and read every option first (`cld join a -x` would name
    `-x`). Each command's `Args` refuses that argument, and a `--` (`ArgsLenAtDash`), which
    pflag would drop;
  - a `FlagErrorFunc` turns pflag's typed errors (`NotExistError`, `ValueRequiredError`,
    `InvalidValueError`, `InvalidSyntaxError`) into cld's messages, and shows the help when `-h`
    or `--help` came before the error: cobra looks at `-h` only once every option has parsed;
  - `SetHelpCommand` replaces cobra's `help [command]` with cld's `help [COMMAND]`, whose `Args`
    refuses what cobra's passes over (decision 12), and a help function set on the root, which
    every command inherits, renders cobra's default help into a buffer for `fail.Print`: it is
    cobra's own help function, taken from the root before cld sets its own. Each command has
    `-h` and `--help` of its own (`helpOption`), worded as cobra words its own ("help for new");
    `cobra.EnableCommandSorting` is off, and the help function moves `version` back after
    `help`. `tests/testdata/help` holds the help of the root and of each command, which
    `TestHelpText` compares byte for byte and `-update` rewrites;
  - `Version` stays unset, so there is no `--version` or `-v` flag: `version` is a command, and
    `-V` and `--version` its aliases.
- Go has no `ttyname` on Linux or macOS without cgo: cld runs `tty` with its own stdin, as the
  script's `$(tty)` did, to compare its terminal with the live panes', and drops the newline
  after the name, which uutils' `tty` (0.8.0, Ubuntu 26.04) does not print.
- claude's `--settings` are marshalled from a struct, its fields in the order the script wrote
  them, which gives the same bytes.

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
  them is not taken for a binding;
- tmux passes a pane's own mouse modes on to the terminal when `mouse` is off, and forwards the
  wheel to a program that asked for mouse reporting: the real claude 2.1.281 scrolled its
  fullscreen transcript under `mouse off` too. What `mouse on` adds is the wheel over a program
  that draws in the main screen without the mouse, which scrolls the pane's history (C4);
- since 3.7 the default wheel binding hands the wheel to a program in the alternate screen whether
  or not it asked for the mouse, where earlier versions entered copy mode.

The `VT10x` in Findings came from the probing shell, which carried
`TERMINAL_EMULATOR=JetBrains-JediTerm`: claude's `--debug` log read `extendedKeys=no (env:
terminal=pycharm, no answer)` - the leak `new` guards against by leaving `TERMINAL_EMULATOR` out
of the environment it runs tmux with. From a clean environment the
real claude 2.1.281 put the pane in key mode `Ext 2`, and Shift+Enter inserted a newline (checked
by hand in a nested tmux).

## Status

- Baseline and JediTerm contracts run on tmux 3.3a, 3.4, 3.5a and 3.7c (Linux, Docker; 3.7c built
  from source), and the baseline on Homebrew's tmux on macOS.
- iTerm2 is not automated: every level beyond "launch only" needs permissions on the runner -
  controlling iTerm2 over AppleScript or its Python API (with authentication switched off), and
  posting synthetic key events (Accessibility). That is a decision for the maintainer, not
  something the test setup should grant itself.
