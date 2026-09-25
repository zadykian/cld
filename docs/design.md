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
| `cld list` where `LC_ALL`, `LC_CTYPE` and `LANG` do not name UTF-8 - unset or `C`, as over ssh, in containers and cron (tmux 3.3a, 3.4, 3.5a, 3.7c) | tmux writes a command's output to such a client with `_` for each character it cannot print: the tabs, so a session showed as `demo_detached_/tmp` under NAME with STATE and DIRECTORY empty, and a directory's non-ASCII letters (`/tmp/café` became `/tmp/caf_`). `tmux -u` marks the client UTF-8, and the output arrives as it is. The other output cld reads - the session's name, `cld-NAME`, from its session lookup, `list-panes`' `0` or `1` - is printable ASCII and passes unchanged |
| `cld` inside another tmux (`$TMUX` set) | nesting works: the private socket is a different server, and tmux refuses a client with `$TMUX` set only when its tty has the name of one of the server's own panes - but see the next row |
| a dead pane's pty (tmux 3.3a to 3.7c) | tmux closes it but keeps its name (`#{pane_tty}`), and the system hands the name to the next pty opened. A client with `$TMUX` set on that pty - a pane of another tmux - is refused with `sessions should be nested with care, unset $TMUX to force`: tmux compares the client's tty with every pane's, dead or alive. An empty `$TMUX` skips the check; set, even empty, it still makes the client take the terminal for UTF-8 whatever the locale says |
| a bare `tmux new-session -d -s cld-x` run inside claude's pane (tmux 3.3a to 3.7c) | the pane's `TMUX` names cld's socket, so `cld-x` lands on cld's server, as with `tmux -L cld` by hand; until cld marked its sessions, `list`, `join`, `kill` and `new` took it for one of theirs |
| `new-session ... \; set -F -t =NAME: @cld '#{session_id}' \; set -w -t =NAME: remain-on-exit failed ...` (tmux 3.3a to 3.7c) | what follows `new-session` takes effect before tmux sees the new pane's program exit, however soon: the mark is there as the session is, and the window's `remain-on-exit` and `pane-died` hook keep and report a pane whose program exits at once. When `new-session` fails (`duplicate session`) tmux skips the rest, so the other session stays unmarked. `set -t =NAME`, like any command that takes a pane, finds nothing: `=NAME:` names the session |
| `#{@cld}` in a format (tmux 3.3a, 3.7c) | tmux looks a user option up in the server's options, then the pane's, the window's and the global window options, and only then the session's and the global session options: a `@cld 1` set with `-s`, `-g` or `-w` counted for sessions that had none, and a window's `@cld 0` hid a session that had one. Compared with the session's id, a flag set anywhere makes no session cld's; one on the server or a window still hides one |
| `tmux -V` beside a server started by an older tmux (a 3.5a server from Debian's package with a 3.7c client built from source, in the image that `tests/Dockerfile` built with `BASE=debian:trixie TMUX_VERSION=3.7c` before #21, which installed Debian's tmux 3.5a beside the source build; for #21 also a 3.6 server on the default socket with a 3.7c client) | `tmux -V` reports the client: `tmux 3.7c`. The server keeps running the tmux that started it - `#{version}` read `3.5a`, and `3.6` - and answers the newer client: the 3.7c client made a session on the 3.5a server with `new-session`, and set `remain-on-exit failed` on its window. So a check of `tmux -V` passes after an upgrade while the sessions on the old server - on one shared server, as before decision 13, the new ones too - run on it until it exits |
| how `claude` 2.1.282 resolves `remoteControlAtStartup` (read from its bundle, not run: a live check would connect the session to claude.ai) | the first of the policy settings, the `--settings` (flag) settings and the user settings that has it wins, over the old global-config key; a `false` in the project's `.claude/settings.json` or `settings.local.json` beats all of them, and a `true` there is ignored with a warning. `/config`'s "Enable Remote Control for all sessions" writes the user setting, so `--settings` overrides it either way |
| what the claude minimum rests on (#21): Claude Code's changelog, and the linux-x64 npm bundles of 2.1.118, 2.1.119, 2.1.133, 2.1.221 and 2.1.222, read for the issue, not run | `--worktree` came in 2.1.49, `-n`/`--name` in 2.1.76 and the `worktree.baseRef` setting in 2.1.133 (changelog). `remoteControlAtStartup` moved into the settings in 2.1.119, with `/config`'s other settings ("now persist to `~/.claude/settings.json`", changelog): 2.1.118 reads it from the global config (`~/.claude.json`) only, which `--settings` does not reach. 2.1.133 and 2.1.221 decide from the merged settings, then the global config, and in the merge flag settings outrank the project's and the local ones, so cld's `true` beats a project's `false`. 2.1.222 returns `false` first when the project or local settings have it, then takes the first of the policy, flag and user settings, as the row above records for 2.1.282; its changelog agrees: repo-local settings "can no longer turn it on (they can still turn it off)". On 25 September 2026 npm's `stable` tag was at 2.1.274 and `latest` at 2.1.282; for the issue, Homebrew's default `claude-code` cask and the apt, dnf and apk `stable` repositories served 2.1.274 too |
| how tmux starts a command given as several words, such as `new-session -c DIR claude ...` (tmux 3.7c, glibc 2.43, Ubuntu 26.04) | with `execvp`, in `DIR`, and with the `PATH` of the client that ran `new-session`, also for a second session on a server that a client with another `PATH` started. So a bare `claude` is looked up in every entry, relative ones included, from `DIR`: with `PATH=.:/abs`, or `:/abs`, and a `claude` in both, tmux started the one in `DIR`, where cld had checked `/abs/claude`. A script without `#!`, which the system will not execute (`ENOEXEC`), `execvp` ran with `/bin/sh`, by name and by path |
| `new-session -c DIR` with a `DIR` its user cannot enter, and a binary for another machine as the command (tmux 3.7c, glibc 2.41, in the image `tests/Dockerfile` builds on `debian:trixie`; the first as `nobody`) | tmux started the command in the home directory, printing nothing, and `new-session` exited 0: for a `DIR` of mode `000`, and for one inside a directory of mode `000`. An arm64 ELF binary on x86_64, which the system will not execute (`ENOEXEC`), `execvp` ran with `/bin/sh` all the same, as a script: a `/bin/sh` that logged its arguments recorded `sh PATH --version`, and the pane died with dash's status 2 |
| `claude --version` (2.1.282, native installer, Linux) | prints `2.1.282 (Claude Code)` and exits 0, in about 20 ms. It leaves nothing running that holds its output: piped to `cat`, it returns as soon. In a directory that has since been removed it prints `error: The current working directory was deleted, so that command didn't work. Please cd into a different directory and try again.` on stderr and exits 1 |
| the environment the bash script handed tmux with `exec env -u TERMINAL_EMULATOR tmux ...` and `exec tmux ...` (bash 5.3.9 and 3.2.57, recorded by the fake tmux, and by `printenv` in its place under `set -euo pipefail`), and claude's in the pane of a server that `cld new` started (tmux 3.7c) | bash exported `PWD` set to the working directory, whatever `PWD` it got; `SHLVL=0` when it got none, and a `SHLVL` it got unchanged; and no `_`, not even one it got: once the script has run a command, bash no longer exports it. It dropped an exported `PS1` and `PS2`; `OLDPWD`, which an interactive bash exports after a `cd` - 3.2.57 always, 5.3.9 when it names no directory; and `RANDOM`, `PPID`, `COMP_WORDBREAKS`, `HISTCMD` and `BASH_VERSINFO`, with 5.3.9 also `SRANDOM`, `BASHPID` and `BASH_ARGV0`, and 3.2.57 `LINENO`. Its own variables that came in exported left with its values: `IFS` (space, tab, newline), `OPTIND=1`, `OPTERR=1`, `BASH`, `BASH_VERSION` and `SHELLOPTS`, with the script's `errexit`, `nounset` and `pipefail` added - a bash that reads it turns them on - and with 5.3.9 also `BASHOPTS`, `LINENO`, `PS4`, `EPOCHSECONDS` and `EPOCHREALTIME`; Debian's 5.2.15 dropped and rewrote the same variables as 5.3.9. Exported functions (`BASH_FUNC_NAME%%`) left in bash's own layout; any other variable passed as it came. The Go cld hands on the environment it got, apart from `TERMINAL_EMULATOR` and `TMUX`. tmux sets a pane's `PWD` from `-c`, so claude sees the same `PWD` either way; the rest comes from the server's environment, that of the cld that started the server: no `SHLVL` where claude saw `SHLVL=0`, that cld's `_` - a shell sets it to the path of the command it runs - where claude saw none, and each of the others as that cld got it |
| the script's name check and `list`'s columns under `en_US.UTF-8`, `C.UTF-8` and `C` (bash 5.3.9, glibc 2.43; bash 3.2 on macOS not checked) | `[[ $name =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]` follows the locale's collation: under `en_US.UTF-8` it matched `é`, `ñ`, `ß`, `Ä`, `ǅ`, `①` and `٣`, so `cld new -n café` made `cld-café`, which `tmux -L cld kill-session -t =cld-café` ends (tmux 3.7c); under `C.UTF-8` and `C` it matched ASCII only. `${#name}` counts characters under a UTF-8 locale and bytes under `C`; `printf '%-*s'` pads by bytes under all three, so `é` took three columns of a four-column NAME |
| where bash itself stepped in for the script (bash 5.3.9 on Ubuntu 26.04 unless noted; tmux 3.7c) | a write to stdout that failed (`/dev/full`, or a descriptor open for reading) ended the script under `set -e` with status 1 and bash's message (`printf: write error: No space left on device`, `cat: -: ...` for the usage), and `new` and `join` did not get as far as their `exec` of tmux; `printf` and `cat` failed the same way under 5.2.15 and 3.2.57. A write to a pipe whose reader had gone ended it by SIGPIPE (the usage with status 141, `cat`'s, under `set -e`), and when it was started with SIGPIPE ignored - from a script under `trap '' PIPE`, say - with status 1 and `printf: write error: Broken pipe` (`cat: -: Broken pipe` for the usage). A tmux that could not run at all ended it with 127 when there was no such file, and 126 for a file the system refuses (`Exec format error`); for a `#!` naming a missing interpreter, 127 with Debian's 5.2.15 and 5.2.37 and Ubuntu's 5.2.21, 126 with 5.3.9 and 3.2.57 as released. A text file without `#!` bash ran as a script. A tmux on the `PATH` without the execute permission, with no executable one on it, bash found all the same - its search takes the first file of that name where none is executable, for `command -v` too - and ran, ending with `Permission denied` and 126; a `claude` like that let `new` go on and hand it to tmux, and a `git` like that made `new -w` say the directory was in no git repository. A tmux that stopped being runnable once it had answered `tmux -V` ended the script from a session lookup (`list-sessions`) with its `die 1`, status 1 and bash's message, and from `kill-session` or the `exec` with 127 or 126. With `PATH` unset bash searched a default path built into it, which differs by build: `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` (Ubuntu's 5.2.21 and 5.3.9), `/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin:.` (Debian's 5.2.15 and 5.2.37), `/usr/gnu/bin:/usr/local/bin:/bin:/usr/bin:.` (3.2.57 as released); macOS's `/bin/bash` was not checked. Started in a directory since removed, bash warned `shell-init: error retrieving current directory: ...` and kept the `PWD` it got: `new` passed that path to tmux with `-c`, and tmux started claude in the home directory (with Debian's 5.2.37); `new -w` said the directory was in no git repository |
| the help cobra 1.10.2 generates with its default templates (pflag 1.0.9), from a test program with cld's commands | commands are listed sorted by name unless `cobra.EnableCommandSorting` is false; then in the order they were added, but for the help command - cobra's or one set with `SetHelpCommand` - which `Execute` moves after all the others as it runs (`InitDefaultHelpCmd`). A flag's value shows as its type (`--name string`) unless its usage names it in backquotes. A command with flags gets ` [flags]` at the end of its usage line, after any argument in its `Use`, unless `Use` has `[flags]` already or `DisableFlagsInUseLine` is set. Nothing is wrapped; a newline in a flag's usage goes on under the usage's column. The help function writes to stdout and drops a write that fails: `help`, and `new --help`, with stdout on `/dev/full` or open for reading only print nothing, on stderr either, and exit 0. `help nope` prints ``Unknown help topic [`nope`]`` and the root's usage on stderr and exits 0; `help new join` shows `new`'s help |
| `TMUX_TMPDIR` under a deep directory | `error connecting to ... (File name too long)`: the socket path hits the ~108-byte `sun_path` limit, so test sandboxes need short socket directories. The path `TMUX_TMPDIR/tmux-UID/cld-NAME` has to stay within 107 bytes: with a NAME of 64 characters it overflows from a `TMUX_TMPDIR` of 32 characters as UID 0, and of 29 with a four-digit UID (tmux 3.3a, 3.4, 3.5a, 3.7c, as UID 0 and 1000) |
| a pane's environment on one server shared by two sessions and on a server per session: the server started from a shell with `FOO=first`, the second session created from one with `FOO=second`, `VIRTUAL_ENV`, a longer `PATH` and another `SSH_AUTH_SOCK` (tmux 3.3a, 3.4, 3.5a, 3.7c; `sh -c 'env > FILE; sleep 600'` in claude's place) | on the shared server the second pane got `FOO=first` and no `VIRTUAL_ENV`: only `PATH`, which tmux takes from the client that runs the command, and the `update-environment` variables (`SSH_AUTH_SOCK`, `DISPLAY`, ...) came from the shell that created the session. On servers of their own each pane got exactly the environment of the shell that created it |
| a bare `tmux new-session -d -s cld-x` inside a pane of server `cld-c` (tmux 3.3a, 3.4, 3.5a, 3.7c) | the session lands on `cld-c`, whose socket the pane's `TMUX` names; `tmux -L cld-x` finds no socket (`error connecting to ... (No such file or directory)`). `kill-session -t =cld-c` leaves the server running with `cld-x`; `kill-server` ends both. The pane's program gets SIGHUP from either, as when its terminal closes. Once `kill-server` has returned, the server answered no more in 200 tries (3.3a, 3.7c); once `kill-session` of a server's last session had, the server still answered 1 or 2 times in 200 |
| a terminal attached to session `cld-a` when its server, `cld-a`, is killed, with and without another session on the server (tmux 3.3a, 3.4, 3.5a, 3.7c) | `kill-server` alone ends the terminal's client with `[server exited]` and status 1: tmux tells its clients that the server is shutting down, which the client takes for an error. `kill-session -t =cld-a \; kill-server`, one command, ends it with `[exited]` and status 0, as `kill-session` did on the shared server: the client hears that its session exited before the server shuts down. The pane's program and the other session's get SIGHUP, and once the command has returned the server answered no more in 200 tries (3.3a, 3.7c). But `kill-server` only signals the server (`kill(getpid(), SIGTERM)`), which then closes each new connection at once until its clients have gone and it exits (`server_accept` and `server_loop` in `server.c`, 3.3a, 3.4, 3.7c): a client that connects meanwhile fails with `server exited unexpectedly`. With a terminal attached, `list-sessions` run the moment the command returned failed so in 4 of 86 rounds and `new-session` in 1 (3.7c; none in 99 with 3.3a), and a `new-session` run the moment a `list-sessions` had failed so failed the same way (14 times with 3.7c, once with 3.3a). `cld kill -n r` with a terminal attached, then at once `cld new -n r`, reached a fresh server in 100 of 100 rounds (3.3a, 3.7c) |
| a socket directory that ignores case: a casefold tmpfs (Linux 7.0, `chattr +F` on the directory) as `TMUX_TMPDIR`, with session `cld-a` on server `cld-a` (tmux 3.7c) | `tmux -L cld-A` reaches server `cld-a`, whose socket file is the one `cld-a`; `#{socket_path}` there is the path the server was started on, `.../cld-a`. Until cld read it, `new`, `join` and `kill -n A` took the server for one that outlived session `A` and pointed at `tmux -L cld-A kill-server`, which ended `a`. Over a stale socket `cld-a`, `new-session` on `cld-A` starts a fresh server, and the socket is `cld-A` from then on. macOS, whose default APFS ignores case, was not checked |
| `list-sessions` on a server that exits as it asks - its last session ends, or `kill-server` runs (tmux 3.3a, 3.4, 3.5a, 3.7c; servers started and ended in a loop beside a loop of `cld list`, for 15 s) | the client connects, and the server closes the connection without an answer: tmux fails with `server exited unexpectedly`, status 1 (`CLIENT_EXIT_LOST_SERVER` in tmux's `client.c`). Until `list` passed over it, it failed so in 24 of 173 runs (3.3a), 46 of 294 (3.4), 19 of 310 (3.5a) and 6 of 147 (3.7c); since, in none of 311, 224, 368 and 381. A client that the server tells it is shutting down exits with no output and status 0 instead (read from `client.c`, not seen) |
| the socket of `tmux -L NAME` (tmux 3.3a, 3.4, 3.5a, 3.7c) | tmux never removes it: not when the server exits with its last session, not on `kill-server`, not on SIGKILL. `list-sessions` on such a stale socket fails with `no server running on DIR/NAME`, on a name never used with `error connecting to DIR/NAME (No such file or directory)`, both with status 1; `new-session` on a stale socket starts a fresh server there. The socket is in `tmux-UID` under `TMUX_TMPDIR`, or under `/tmp` where `TMUX_TMPDIR` is unset, empty or names nothing that exists; tmux resolves a symlink in it. A socket path of 107 bytes works on Linux, one of 108 fails with `File name too long`. Where `tmux-UID` is a file, not a directory, every command fails with `DIR/tmux-UID is not a directory`, status 1 |
| what a server per session costs (tmux 3.3a, 3.4, 3.5a, 3.7c, in Docker, on a host busy with other builds) | 3.8 MB (3.3a) to 5.1 MB (3.5a) resident per server, the same with 10 sessions on it, next to about 400 MB for claude; one `list-sessions` took 6-12 ms, and one per socket over 36 sockets, 16 of them stale, 136-282 ms. #22's plan measured 3-6 ms and 115-170 ms on an idle machine (3.3a, 3.7c) |
| a program's rows in the main screen of a 40-column pane that narrows to 20 (tmux 3.3a, 3.7c) | tmux reflows them: three 39-character rows under two short lines became six lines, and the two lines above them and the first half of the first row went into the history; a cursor left at the start of the first row ended at the top left of the screen, on that row's second half. A program that redraws its lines in place, relative to where it left the cursor, then draws over the wrong lines, and can recover only by clearing the screen, and what the shell showed above it with it |
| a tmux client starting on a terminal (`tty_start_tty` in `tty.c`, read in tmux 3.3a, 3.4 and 3.7c) | tmux sets the terminal's mode and then calls `tcflush(TCOFLUSH)`, which throws away output the terminal has not read yet. What the list wrote last before it became `tmux attach-session` - leaving the alternate screen, the cursor shown, the title - was lost now and then under load (tmux 3.3a and 3.4 in Docker, `TestListJoin`'s enter case and C10's join): the tab kept its old title. A stopped (SIGSTOP) outer tmux did not lose it (3.7c, Linux 7.0), so it takes a loaded machine too. A terminal answers primary device attributes (DA1, `CSI c`) once it has read what came before: tmux with `CSI ? 1 ; 2 c` (3.3a; 3.7c built with sixel `CSI ? 1 ; 2 ; 4 c`), JediTerm 3.76 with `CSI ? 6 c` |
| a DA1 answer later than the list's wait for it, the list having become `tmux attach-session` (tmux 3.7c; the terminal frozen for 1.5 s against a one-second wait) | tmux asks for DA1 itself as it starts and takes the first answer for its own; the next, its own, reached claude's pane as keys (`CSI ? 1 ; 2 c` in the probe's input). Answered in time, the probe read no answer. Other versions were not checked |
| SIGTSTP in a Go program that has had it through `os/signal` (Go 1.27.1, Linux 7.0) | after `signal.Stop` or `signal.Reset`, `kill -TSTP` of the process did nothing: `sigdisable` leaves Go's handler in place for any signal that `sigInstallGoHandler` accepts, and the handler drops a `_SigNotify` signal that no channel wants. Never notified, SIGTSTP keeps its default action, since `initsig` skips `_SigDefault` signals. `kill(getpid(), SIGSTOP)` returned before the process stopped, under dash with `set -m`, and it stopped soon after; the SIGCONT of `fg` then reached `os/signal` |
| a job of a `sh -c` script under `set -m` that stops, and a background one that ends: macOS's `sh`, bash 3.2.57 as Apple builds it (read in its source, tag `bash-144`; seen on the CI's macOS 26 arm64 runner; run on Linux as GNU bash 3.2.57 with Apple's change to `jobs.c`), GNU bash 3.2.57 and 5.3.9, dash 0.5.12 | Apple's bash asks `waitpid` to report a stopped child (`WUNTRACED`) only when the shell is interactive, where GNU's asks whenever job control is on: in a script, `set -m` puts the job in a process group of its own, in the foreground, but once the job stops the shell goes on waiting for it to end, and never runs the rest of its script. With `-i` it goes on, `$?` 128 and the signal's number, as the others do in a script. bash 3.2.57, Apple's and GNU's, also reports a background job's end on stderr in a script while job control is on (`[1]+  Done ...`), which 5.3.9 and dash do not; with job control off again (`set +m`) once the job has started, it does not |
| a `list-sessions` client that connects as the server exits with its last session: `new-session -d`, `kill-session`, then `list-sessions -f`, 400 times (tmux 3.3a, 3.4, 3.7c) | the client printed `no server running on ...`, but for `server exited unexpectedly`, failing, in one round on 3.4 and one on 3.7c, and nothing in one on 3.3a and one on 3.7c. cld took the first for no session and reported the second as an error, `cld join` as the list's footer; since 13 it takes both for no server (see the row on a server that exits as it is asked). `TestListJoin`'s last-row case waits for the server to have exited before Enter |
| a directory whose name holds control characters (0x01, ESC), in `#{pane_current_path}` of `list-sessions -F` and `list-panes -F` (tmux 3.3a, 3.4, 3.5a, 3.7c), for a client under `LANG=C.UTF-8` and `LANG=C` | 3.3a and 3.7c write the characters as they are to a UTF-8 client - under `C.UTF-8`, or with `-u` - and each as `_` under `C` without `-u`; 3.4 and 3.5a write them as octal escapes, `\001` and `\033`, under either, with `-u` or not |
| hint strings in Claude Code 2.1.282 (read from its bundle, not run) | hints are lower case, but for Enter and Esc in some, joined by ` · ` and drawn dim: `↑/↓ to navigate · enter to resume as a background session`, `↑/↓ to navigate · Esc to cancel`, and a list of hints beside `ctrl+x to ...` and `to go back` that ends in `esc to quit` or `esc to close · esc again quits`. Where each shows was not observed |
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

Isolation needs no seams in the program: `TMUX_TMPDIR` moves cld's sockets (`-L cld-NAME`) into
a sandbox, `HOME` points at a temporary directory, `TMUX` is unset, and the probe is first on
`PATH`. `cld new` and `cld join` attach, so they need a pty; a terminal driver provides one.

### Terminal contract

| # | Check | Evidence |
|---|---|---|
| C1 | tab title is `✳ cld-NAME` and survives claude's own title changes | terminal |
| C2 | tmux's view of the client: `#{client_termtype}`, `#{client_termfeatures}` (`extkeys`, `focus`, `mouse`, `clipboard`, ...) | tmux |
| C3 | tmux asks the terminal for modified keys and takes the request back on detach; Shift+Enter reaches claude distinct from Enter; the Ctrl keys claude binds (`C-b`, `C-_`) pass through; `C-q d` detaches; `C-q C-q` sends `C-q` | probe input log, terminal output |
| C4 | mouse wheel and focus in/out reach claude; over a main-screen program without mouse reporting the wheel scrolls the pane's history | probe input log, tmux |
| C5 | OSC 52 / OSC 9 wrapped in tmux passthrough, and copies through `tmux load-buffer -w`, reach the outer terminal | terminal |
| C6 | claude never sees `TERMINAL_EMULATOR`, including in a session created in the JetBrains terminal, nor does what claude starts through tmux on its server | probe env dump, tmux |
| C7 | after detach the terminal is clean: no mouse reporting, no alt screen | terminal |
| C8 | a paste reaches claude bracketed and whole; a prefix key inside it is text, not a binding | probe input log |
| C9 | claude exiting ends its session, and its server unless tmux sessions claude made keep it running; the terminal is left clean. A claude that fails - exit status other than 0, or a signal - keeps its session, with its message and how to end it on screen | terminal, tmux |
| C10 | the session list (`cld list` on a terminal) reads the terminal's own keys: Down and Enter join the second session, which shows, with the title `✳ cld-NAME`; Esc leaves the terminal as it was: the main screen, no mouse reporting, the cursor visible and the same `stty -g` | terminal |

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

- every push and pull request: lint; contract x tmux on one pinned tmux release (3.7c, built from
  source, in a Linux container) and on Homebrew's current tmux on macOS; contract x JediTerm;
- nightly, on tags and on demand: contract x iTerm2 on macOS, uploading screenshots and logs on
  failure; non-blocking until it proves stable;
- tags: release.

The Linux job runs the same Docker image a developer runs locally.

### Spike before building the iTerm2 driver

1. Does the pinned iTerm2 start on a hosted macOS runner without a dialog blocking it?
2. Does the Python API connect with "allow all apps" set through `defaults`?
3. Can a single Shift+Enter key event be posted?

## Decisions

1. Naming: names are validated (`[A-Za-z0-9][A-Za-z0-9_-]*`), not sanitised - a silent rename
   would make `cld foo.bar` and the session it attaches to disagree. Since 13 a name has at most
   64 characters, so that its server's socket path fits (see 13.2).
2. Inside another tmux: `cld` nests; the private socket already allows it. Inside a live pane of
   one of its own servers - claude's external editor, say - a session attached would show inside
   a session of cld's, itself or another, both taking `C-q`, and `new` and `join` refuse, pointing
   at `C-q d`; tmux refuses there too, but advises to unset `$TMUX`. tmux goes by the tty's name,
   and a dead pane's name comes back with the next pty opened (see Findings), so `cld` looks at
   the live panes itself and gives its client an empty `TMUX`, which tmux's check skips. Since 13
   it looks only when the socket `TMUX` names is one of cld's, `cld-NAME`, and asks that server.
   `list` prints its table there rather than the interactive list, whose Enter would be refused
   (see 14).
3. Commands (0.2.0): `new` creates a session and fails if it exists, `join` attaches to one and
   fails if it does not; the name moves to `-n NAME` (default `main`). A bare `cld` fails, and
   `cld NAME` fails naming `cld new -n NAME` and `cld join -n NAME`. Commands address sessions as
   `=cld-NAME`, since tmux would otherwise take `cld-rev` for `cld-review`. `list` shows the
   directory claude is in now (`pane_current_path`), not the one its session started in, and
   nothing at all when no server runs; on a terminal it lets you pick a session and join it (see
   14). `kill` ends a session with `kill-session`: claude gets SIGHUP, as when its terminal closes
   (since 13, `kill-session` and then `kill-server` in one tmux command, with the same SIGHUP).
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
5. Failures stay on screen: with `remain-on-exit failed`, a claude that exits with an error or a
   signal keeps its pane, so what it printed - a startup error above all, which would otherwise
   vanish with the session - stays readable. The format is empty, so tmux does not scroll that out
   of sight; a `pane-died` hook shows how to end the session on the message line instead, naming it
   through the session's one window, named `NAME`. The hook shows it only to a terminal on that
   window (`if -F '#{window_active_clients}'`), since tmux would otherwise show it on another
   session's terminal or over the next session attached (see Findings); `join` shows it on
   attaching to such a session, with a `display-message` in the same command list as
   `attach-session`. `list` shows such a session as `exited`, where it started, and `new` refuses
   the name, pointing at `kill`, rather than replacing the session unseen. The option and the hook
   go to claude's window only (see 9 and 13).
6. Versions (#21): cld runs on tmux 3.7 or newer, the release its tests run on, and starts
   Claude Code 2.1.222 or newer, the first release that does what cld passes and relies on. Both
   are checked at startup and raised by hand, and neither has an upper bound. The tmux check runs
   for every command but `help` and `version`, and refuses an older tmux with
   `cld: tmux 3.7 or newer is required, found 'tmux 3.6b'` and status 1.
   - It reads `tmux -V`: the major and minor version, after `next-` for a development build
     (`next-3.9` is 3.9, `3.8-rc2` 3.8); a version without them (`master`) passes. Letters mark
     bug-fix releases and are not compared, so 3.7 to 3.7c all pass, and there is no upper bound.
   - The tests run on one tmux, 3.7c, built from source in the Docker image: no released Debian
     or Ubuntu version ships 3.7, and a package from Debian testing would change whenever the base
     image does. The release is pinned in `tests/Dockerfile` and the `Makefile` and bumped by hand
     together with the minimum and these docs, as JediTerm is (7). CI's Linux job is named `linux`,
     without the version, so a bump leaves the ruleset's required checks alone; the macOS job
     installs Homebrew's current tmux, which runs ahead of the pin when Homebrew moves - the signal
     to bump.
   - The cost falls on distribution packages: Debian 13 ships 3.5a (3.6b in trixie-backports),
     Debian 12 3.3a (3.5a in bookworm-backports), Ubuntu 24.04 3.4 and 26.04 3.6a. Their users
     need Homebrew, Debian testing or unstable, or a source build - or cld 0.3.0, which runs on
     tmux 3.3 and newer. A minimum of 3.5 would have removed the same code - `remain-on-exit`
     stayed off below it (see Findings) - and kept those users, but 3.5 and 3.6 would run
     untested; keeping 3.3 and dropping versions from the tests alone would leave a branch that no
     CI runs.
   - The check reads the client: a server keeps running the tmux that started it (see Findings).
     Since each session has a server of its own (13), `new` starts a fresh server with the tmux it
     checked, so after an upgrade only the sessions started before it stay on the older tmux, each
     until it ends; on one shared server, the sessions started while one of those ran stayed there
     too. The gap is accepted, as it was with the 3.3 check, and the README says to end the
     sessions started before upgrading tmux; reading the server's `#{version}` as well was the
     alternative.
   - claude is checked where cld starts it: `new` runs `claude --version` once it has found its
     tools and checked tmux's version, before any other tmux command. The issue placed it right
     after the lookup of `claude`; it comes after the checks every command makes instead - the
     tools, then `tmux -V` - so that cld runs claude only once those cheap checks pass, and a
     missing tool or a tmux too old is reported before a claude too old. `join`, `kill` and `list`
     never start claude and do not check it. cld compares the `X.Y.Z` the output starts with as
     numbers (2.1.30 is older than 2.1.222) and refuses an older claude with
     `cld: claude 2.1.222 or newer is required, found '2.1.221 (Claude Code)'` and status 1. It
     does not say how to update, which depends on how claude was installed; the README does.
   - `claude --version` runs the claude that tmux then starts, as tmux starts it: the `claude`
     that cld finds in the absolute `PATH` entries (11.5), by its path, with no input, in the
     current directory. `new` hands tmux that path rather than the word `claude`, so claude sees
     its path as its `argv[0]`: tmux looks the word up with `execvp`, relative entries included
     (see Findings), and with `.` or an empty entry ahead of the absolute one it started a
     `claude` in the directory claude starts in, which cld had not checked. The directory counts
     too: a version manager's shim - mise's, say - runs the claude that the directory pins, so a
     check made anywhere else could pass a claude other than the one `new` starts. A directory
     that has been removed is refused first, with `cld: the current directory no longer exists`,
     as `new` refuses it anyway (11.10), because `claude --version` fails there (see Findings);
     `new` now says so before it looks the session up. So is one that cannot be entered - its
     search permission, or that of a directory above it, taken away since the shell entered it -
     with `cld: cannot enter the current directory: REASON` (11.10): `claude --version` cannot
     start there, which cld would otherwise report as a claude that cannot run.
   - Output that does not start with a version passes, as a tmux development build does, so that a
     new format locks no one out. A `claude --version` that fails is refused with status 1, what it
     printed (stdout, then stderr) and its exit status or signal, since a claude that cannot report
     its version is unlikely to start. cld reads what claude printed until it exits, and for a
     second more at most: a process it leaves in the background with its output open - a
     wrapper's update check, say - does not hold `new` up for as long as it runs. A script without
     `#!`, which the system will not execute, runs with `/bin/sh`, as tmux's `execvp` runs it
     (glibc, see Findings; macOS's libc by its source, not run), and is checked as any other
     claude: Go's `os/exec` does not fall back so. A binary the system will not execute - one for
     another machine, or cut short - is no script, though `execvp` hands it to `/bin/sh` all the
     same (see Findings): cld tells the two apart as bash does (`check_binary_file`: ELF's magic
     number, or a NUL in the first line) and does not run a binary with `/bin/sh`. A claude that
     cannot run at all - no execute permission, a missing `#!` interpreter, such a binary - ends cld
     as such a tmux does (11.9), with `cld: cannot run PATH: REASON` and 127 or 126, since its
     version is not what is wrong. Where the script would have handed such a claude to tmux, which
     failed in its pane (see 11.5), `new` now stops in the terminal.
   - Why 2.1.222: the tests never run the real claude, so there is no tested version to require;
     the minimum is the first release that takes what cld passes and does what it relies on. What
     it passes came earlier - `--worktree` in 2.1.49, `--name` in 2.1.76, `remoteControlAtStartup`
     in the settings in 2.1.119, `worktree.baseRef` in 2.1.133 - but only from 2.1.222 does a
     project's `false` keep Remote Control off despite cld's `--settings`, as 10 and the README
     promise (see Findings). 2.1.133 would have needed that promise qualified; 2.1.281, the version
     probed, would refuse the stable channel (2.1.274), which runs about a week behind. The exit
     status of `/exit` and the key mode, recorded for 2.1.281, were not checked on older releases.
     The minimum rises when cld starts to pass a flag, or to rely on behaviour, that needs a newer
     claude, and only once the stable channel has that release. An upper bound, or an exact match,
     would break cld every few days: npm published 2.1.280 to 2.1.282 on 22 to 24 September 2026.
7. JediTerm is pinned at 3.76, the latest published, and bumped deliberately.
8. iTerm2: not automated yet (see Status).
9. Only cld's own sessions (from 0.2.1; replaced by 13): the private server keeps cld's options
   away from other tmux use, but not other tmux use away from cld's server - everything claude runs
   inherits `TMUX` (see Findings). `new` marks each session it starts, in the tmux command that
   creates it: the user option `@cld` holds the session's id, so a `@cld` that a format finds
   elsewhere first makes no other session cld's (see Findings). `list` shows marked sessions only,
   and `new`, `join` and `kill` refuse a name an unmarked session holds, saying so and pointing at
   `tmux -L cld ls`. What `cld` sets for a failed claude (see 5) goes to claude's window, not the
   server, so such a session whose program fails closes as tmux would close it, rather than staying
   where `cld` neither lists nor kills it. Starting claude without `TMUX` would keep its tmux off
   cld's server, and its passthrough and `load-buffer` copies with it; a server per session would
   still need the mark for a session made on it by hand, and `list` would have to find the servers.
   The mark guards against mistakes, not intent: whatever reaches the socket can set it.
10. Remote Control: `new` starts claude with `--settings '{"remoteControlAtStartup":true}'`, so a
   session can also be continued from claude.ai or the Claude app, not only from a terminal that
   joins it. Flag settings outrank the user's, so this holds whatever `/config` says; claude still
   keeps Remote Control off under an org policy or a project that sets the key to `false` (see
   Findings; from 2.1.222, the minimum in 6), and `cld` leaves those alone. With `-w` the worktree
   setting goes into the same JSON: one `--settings` rather than two, whose merging claude does
   not document.
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
       `tmux`, `git` and `tty` from there - and since #21 `claude` as well: its `--version`, and the
       one `new` hands tmux by its path (see decision 6). One found only through a relative entry
       (`.`, or an empty one) counts as not installed, and one that both have runs from the absolute
       entry, where bash ran the first it found. Go's `exec.LookPath` refuses a match in a relative
       entry but stops at it, so cld skips those entries itself. With `PATH` unset cld finds none of
       them, where bash searched a default path built into it, which differs by build (see
       Findings). Within the absolute entries cld searches as bash did: the first executable file of
       that name or, where none is executable, the first file of that name, which then cannot run -
       such a tmux ends cld with 126 (see 9), and such a git leaves `new -w` saying the directory is
       in no git repository, as with the script. Such a claude failed in its pane with the script,
       and at first with the Go cld; since #21 `new` ends with 126 there, as with such a tmux,
       because it cannot run its `claude --version` (see decision 6);
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
       names came from 8, or from a session renamed by hand; their columns lined up until 13,
       since which `list` shows neither (see 13.4);
    8. names are ASCII, as decision 1 has them, whatever the locale: bash's `[A-Za-z0-9]` followed
       the locale's collation, so under `en_US.UTF-8` and the like (glibc; see Findings) the
       script also took letters and digits such as `é`, `ß`, `①` and `٣`, and made sessions such
       as `cld-café`. cld lists such a session (until 13, see 13.4) but refuses its name to
       `join` and `kill`, and gives no legacy hint for it; `tmux -L cld kill-session -t =cld-café`
       ends it;
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
        (see Findings); the other commands no longer print bash's warning there. Since #21 it
        also refuses one it cannot enter - its search permission, or that of a directory above
        it, taken away since - with `cld: cannot enter the current directory: REASON`: given it
        with `-c`, tmux starts claude in the home directory as well (see Findings). The Go cld
        had handed such a directory to tmux when its environment had no `PWD`, and with one
        refused it as `cannot get the current directory: stat .: permission denied`, since Go's
        `Getwd` looks at `.` first to check `PWD`.

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
13. A server per session (#22), in place of the mark (see 9): session `cld-NAME` runs on a tmux
    server of its own, `tmux -L cld-NAME`, with the same options, and cld looks for that one
    session on that one server, by its whole name. Whatever claude runs inherits `TMUX` and
    reaches claude's own server, where a session it makes has another name - `cld-NAME` is taken -
    so no mark is needed, and none is set (see Findings). 9's reasons against this no longer
    hold: a session made by hand on server `cld-NAME` has another name too, unless it spells out
    cld's scheme on purpose (`tmux -L cld-x new -s cld-x`), and the mark did not guard against
    intent either; and `list` finds the servers with one read of a directory and one
    `list-sessions` a socket (see Findings). It also fixes the environment: tmux starts a pane
    with the environment of the client that started the server, but for `PATH` and the
    `update-environment` variables, so on the shared server every claude had the first session's
    `CLAUDE_CONFIG_DIR`, `VIRTUAL_ENV`, `AWS_PROFILE`, `LANG` and the like; now each has the
    environment of the shell that ran `cld new`. And a crash, or a stray `tmux kill-server` or
    `set -g` from anything claude runs, reaches one session. What it costs:
    1. `list` reads tmux's socket directory, `tmux-UID` under `TMUX_TMPDIR` - or under `/tmp` where
       that is unset, empty or names nothing, as tmux falls back (see Findings) - and asks the
       server of each socket `cld-NAME` whose NAME is valid for its session `cld-NAME`, one after
       another, with `-u` as before; it shows them in the order of their names, as tmux listed the
       sessions of one server. A stale socket says no server is running there and is passed over, as
       is a server that exits while `list` asks it - its claude exits, a `cld kill` runs - which
       tmux reports as `server exited unexpectedly` (see Findings); on one shared server, only the
       last session's end ended the server. Sockets pile up, one for each name ever used, until
       `/tmp` is cleaned: tmux removes none, and cld does not either, since tmux replaces a stale
       socket under a lock and an unlocked `rm` could race a `cld new` and remove the socket of the
       server it had just started (read from tmux's client code, not tested). A socket directory
       that `list` cannot read - a file in its place, say - ends it with the reason, and `new`,
       `join` and `kill` with tmux's message (see Findings). No test covers the fallback to
       `/tmp`, which would read the user's own sockets;
    2. NAME has at most 64 characters, checked before its characters, with a message of its own: the
       socket path has to fit in `sun_path`, 107 bytes and a NUL on Linux (see Findings) and 103 on
       macOS (not checked here), which leaves about 88 characters of NAME in Linux's default
       directory, `/tmp/tmux-UID`, and 77 in macOS's, `/private/tmp/tmux-UID`. A name longer than 64
       gets no legacy hint either (see 3). Under a `TMUX_TMPDIR` longer than those directories,
       counted once tmux has resolved its symlinks (see Findings), a shorter NAME can still
       overflow `sun_path` - as `$TMPDIR` on macOS, `/var/folders/...` (`/private/var/folders/...`
       resolved), would beyond about 33 characters (worked out, not checked) - and tmux fails with
       `File name too long` (see Findings): only a socket that is stale or missing counts as no
       server, so `new`, `join` and `kill` end with tmux's message rather than take the name for a
       session that does not exist;
    3. `C-q s`, `C-q (` and `C-q )` no longer switch between cld's sessions, and tmux's paste
       buffers are no longer shared between them; cld documented neither;
    4. `tmux -L cld ls` no longer shows every session: each is `tmux -L cld-NAME ls`. The sessions
       of cld 0.3.0 and earlier stay on the `-L cld` server, where cld does not look, not even for
       a release: the README says to end them before upgrading, or afterwards with
       `tmux -L cld kill-session -t =cld-NAME`. `list` no longer shows the sessions with
       non-ASCII names that 0.3.0 made (11.7 and 11.8), nor a session renamed by hand, which is
       not the one its server is named after (11.7);
    5. one more tmux process per session, 4 to 5 MB, next to about 400 MB for claude;
    6. where tmux's socket directory ignores case - macOS's default file system, APFS, does, and
       `/private/tmp` is on it (not checked on macOS) - names that differ only in case share one
       socket, where the shared server kept `a` and `A` apart: `tmux -L cld-A` reaches the server
       of session `a`, which has no session `cld-A` (see Findings). `new`, `join` and `kill` would
       take it for a server that outlived session `A` and point at `tmux -L cld-A kill-server`,
       which ends `a`. So where the server they reach runs without its session, they ask it for
       the socket it was started on (`#{socket_path}`), and if that names a NAME that differs only
       in case, they refuse the name as clashing with that session, pointing at no `kill-server`.
       Names stay case-sensitive: where the directory keeps case, as on Linux, `a` and `A` are two
       sessions.

    Settled with it: `kill` runs `kill-session` and then `kill-server`, in one tmux command, which
    also ends what claude started through tmux, so nothing lingers and the next `cld new -n NAME`
    starts a fresh server. The session goes first so that a terminal attached to it ends as it did,
    with `[exited]` and status 0: `kill-server` alone tells it the server exited, with status 1.
    claude gets SIGHUP as with `kill-session` alone. Once `cld kill` returns the server answers no
    more, but it has not always gone: `kill-server` only signals it, and until its clients have
    gone it still takes a connection and closes it at once, which tmux reports as
    `server exited unexpectedly`. cld's lookups count that as no server, and a `cld new -n NAME`
    run the moment `cld kill` returned started a fresh server every time it was tried (see
    Findings). A server that outlives its session - claude exited, and the tmux sessions it made
    keep the server running - is not reused: `new` refuses the name, pointing at
    `tmux -L cld-NAME ls` and `tmux -L cld-NAME kill-server`, so that every claude gets the
    environment of the shell that ran `cld new`; `list` shows nothing for such a server, and `join`
    and `kill` refuse the name the same way, rather than send the user to a `cld new` that refuses
    it (but see cost 6 for a server that is another session's). `new` and `join` refuse a terminal
    that is a live pane of any of cld's servers (see 2), found through the socket `TMUX` names, and
    say whose session's server it is; the terminal of any other tmux nests without a check. The
    options stay as they were, the fixed `terminal-features[100]` index too: `new` sets them on a
    fresh server, but two `cld new -n NAME` at once can both set them on one.
14. The session list (#23): on a terminal, `cld list` shows the sessions to pick one and join it, as
    Claude Code's agent view (`claude agents`, a research preview whose keys may change) lists its
    background sessions: `↑`/`↓` move between rows, Enter attaches, Esc leaves. Its footer follows
    Claude Code's hints (see Findings): `↑/↓ to navigate · enter to join · esc to quit`, dim, under
    the rows after a blank line; on a row with a terminal attached - an exited one too - `enter to
    join` reads `enter to join and detach its terminal`. The first row is selected, marked `>` and
    in inverse video; `↑`/`↓` stop at the first and the last row, Enter joins, and Esc and Ctrl+C
    leave with status 0 - one Ctrl+C, since the list has no input to clear. Other keys, letters
    included, do nothing: agent view binds none, and its `→` pairs with a `←` to come back, which a
    cld session does not offer. Keys with Alt do nothing either: terminals send them as Esc and the
    key, so Esc followed within the wait for a lone Esc by another key is that key with Alt - but
    for a second Esc, which stands alone unless a sequence follows it (Alt+Up as ESC ESC [ A, as
    rxvt sends it). A message - why Enter could not join - takes the hints' place until the next
    key. Settled with it:
    1. when: the list is interactive when stdin and stdout are terminals, `TERM` is set and not
       `dumb`, which cannot move the cursor, and cld is in the terminal's foreground (its process
       group is the terminal's, `TIOCGPGRP`); otherwise `cld list` prints the table, byte for byte
       as before - to a pipe or a file (`cld list | cat`, `$(cld list)` in a script), for a
       completion, and in the background (`cld list &`), where setting the terminal up would stop
       the job (SIGTTOU). A look now costs an Esc, and `cld list | cat` still prints and returns.
       A script that leaves `cld list` the terminal, as one run from a shell does unless it
       redirects the output, gets the list too and waits for a key: it pipes the output for the
       table. A flag (`list -i`) would cost the list's main use an option; a bare `cld` opening it
       would reverse decision 3;
    2. where: on the alternate screen, from its top line, redrawn whole after each key and on
       SIGWINCH - once for the bytes of a key, and once for keys that come together, pasted say -
       each line cleared before it is drawn. Every line is cut at the terminal's width, counted in
       cells - two for a wide character, one for an ambiguous one such as `↑`, `·` or `é` - so that
       none wraps; a control character in a name or a directory shows as `?`. The rows scroll to
       keep the selection in view. Leaving brings the shell's screen back; Esc and Ctrl+C then print
       the table from the rows the list last read, so the scrollback holds what `cld list` printed
       before, and Enter prints nothing more. Drawing in place below the prompt would take only the
       lines it needs, but a terminal that reflows its lines as it narrows - tmux does (see
       Findings) - breaks the redraw, and recovering means clearing what the shell showed;
    3. in a live pane of one of cld's servers, where join refuses to attach (see 2), `list` prints
       the table, as before, rather than a list whose Enter would be refused;
    4. with no sessions, or no server, `cld list` prints nothing and exits 0, on a terminal too:
       the list opens only with something to pick. Once its last row has gone, it shows `no
       sessions` under its header, over `esc to quit`, and leaving prints nothing;
    5. rows behave as with `cld join -n NAME`: Enter on a row with a terminal attached detaches that
       terminal, which the footer says first - an exited row's too, whose STATE reads `exited`
       whether a terminal is attached or not - and on an exited row joins and shows claude's last
       words with the hint - joining is how claude's message is read. Asking again, or refusing an
       exited row, would protect nothing: a detach ends nothing;
    6. the list reads the sessions when it opens and after its own actions - a failed Enter here -
       never on a timer or on a key, so a row does not change under a key; each read asks every
       server in turn, as `list` does (see 13.1). A stale row costs at most a message, detaching a
       terminal the list did not show, or joining a session made again under the same name, which
       `cld join -n NAME` would join too. After a read the selection stays on its session or, once
       that is gone, goes to the next row the list showed that is still there - the one that took
       its place - or else to the one above. Leaving and running `cld list` again shows what changed
       elsewhere;
    7. Go (see 11) on `golang.org/x/term`, which the module already required for the probe:
       `MakeRaw`, which clears `ISIG` so that Ctrl+C arrives as the byte 0x03, `GetSize` and
       `Restore`. The list reads a byte at a time, and only once there is one: reads block, so a
       goroutine waits in `select` (`golang.org/x/sys/unix`; macOS's `poll` does not support
       terminals) and the list reads, so that nothing reads the keys after its last one - they stay
       for the shell after Esc and Ctrl+C, and for tmux after Enter (but see below). A timer tells
       a lone Esc (0x1b) from the start of an arrow's `ESC [ A`: 100 ms, to cover a sequence split
       between two reads (over ssh, say; not observed). The list accepts `ESC O A` and `ESC O B`
       too, which the arrows send once a program has turned application cursor keys on and left
       them so; it sets neither mode. Cells are counted with `golang.org/x/text/width`: East Asian
       Wide and Fullwidth take two, combining marks and format characters none, the rest -
       ambiguous ones included - one. `go-runewidth` (0.0.23) takes ambiguous characters for two
       under a CJK locale, unless `RUNEWIDTH_EASTASIAN` says otherwise, and brings `uax29`. Bubble
       Tea (2.0.10) would parse the keys and redraw for cld, but requires 17 modules, 8 of them
       directly.

    Enter runs join's own steps, split in two (`Joinable` and `Attach` in `internal/session`):
    join's checks - the name, then the lookup - while the list still owns the terminal, in raw mode,
    then the terminal put back, then the rest of join - an empty `TMUX`, the title and
    `attach-session -d` with the hint for an exited claude. Putting the terminal back waits for it
    to have read the list's last output: tmux throws away what the terminal has not read yet as its
    client starts (see Findings), which lost the main screen, the cursor and the title under load.
    Still in raw mode, the list leaves the alternate screen, shows the cursor, writes the title and
    asks the terminal for its primary device attributes (DA1, `CSI c`), which a terminal answers
    once it has read what came before; on the answer, or after five seconds without one, it restores
    the terminal's mode and `Attach` goes on, writing the title again. Keys typed with Enter, before
    the answer, are read and dropped; those after it reach claude. So does an answer later than the
    wait: tmux asks the same question as it starts and takes the first answer for its own (see
    Findings). Every terminal the list runs on answers, so the wait is long enough for the round
    trip of a slow link, and only a terminal that does not answer waits it out. `cld join` and
    `cld new` write the title just before tmux starts too, but after no screen of the list's; they
    do not wait. The lookup, and the read of the sessions after one that fails, run beside the list,
    which goes on taking keys and signals: Esc, Ctrl+C, SIGTERM, SIGHUP, SIGINT and SIGQUIT leave at
    once, killing the lookup's tmux and waiting for it, and other keys do nothing until it ends - a
    server that hangs does not hold the list, as it would not hold `cld join`, which Ctrl+C ends
    outside raw mode. A signal that comes before cld becomes tmux ends it with 128 and the signal's
    number, joining nothing. One that comes during the lookup ends it at once, and the tab keeps its
    title - unless `os/signal`, which passes a signal on some time after Go's runtime took it,
    passes it on only once the lookup has ended: then, as for a signal during the wait for the
    terminal's answer, the tab has the session's title, which has to come before the question. The
    list lets go of the signals only once the terminal is back: a signal then either reaches it by
    the time `signal.Stop` returns, where the list looks for one a last time, or ends cld by its
    default action, as it would without the list (`os/signal`, read in Go 1.27.1). The list shows a
    check that fails in its footer without the advice meant for the command line (`no session 'b'`,
    `session 'b' has ended, but its tmux server still runs`), reads the sessions again and stays
    open; `cld join` prints the same errors with it, as before. A session that ends between the
    lookup and the attach fails in tmux, as it does for `cld join`. The cursor is hidden while the
    list is open, and the main screen, the cursor and the terminal's mode come back on every way
    out: Esc, Ctrl+C, Enter before the handover to tmux, an error, and SIGTERM, SIGHUP, SIGINT and
    SIGQUIT, which end cld with 128 and the signal's number - SIGQUIT's own action, a dump of Go's
    goroutines, would leave the terminal as the list had it. Keys typed once the footer shows are
    neither echoed nor held for a line: raw mode comes before the first frame. Ctrl+Z and Ctrl+\ are
    keys in raw mode, which do nothing; SIGTSTP and SIGSTOP come from outside, and the list does as
    less and vim do. SIGTSTP puts the terminal back and stops cld, and SIGCONT, after any stop, has
    the list take the terminal again - raw mode, the alternate screen - and draw it all: SIGSTOP
    leaves the list on the screen, where the shell writes, and bash puts its own mode back as it
    takes the terminal. Go drops SIGTSTP once `os/signal` has had it (see Findings), so cld stops
    itself with SIGSTOP - the shell reports that signal - and waits for the SIGCONT, since the stop
    comes some time after `kill` returns. SIGTSTP stops nothing in an orphaned process group, where
    nothing in the session would have it go on: cld goes on at once where its process group is its
    session leader's - a pane's program, or a job of a shell without job control - which only a
    shell with job control takes a job out of. A SIGTSTP during the handover stops cld once the
    terminal is back, before it becomes tmux. In the background, after `bg`, taking the terminal
    again stops cld (SIGTTOU) until `fg`.

## Implementation notes

Where the implementation departs from the plan above:

- The test harness is Go instead of bats: `go test` with a sandbox package, a terminal package
  (one driver per terminal) and a probe binary that stands in for claude - and, invoked as
  `tmux`, fakes `tmux -V` for the tmux version check. As claude it answers `claude --version`
  for the claude version check (6) with `CLD_FAKE_CLAUDE_VERSION`, or `99.0.0 (Claude Code)` so
  that raising the minimum leaves it alone; it answers before anything else, `CLD_PROBE_FAIL`
  included, and writes no record, so that only the claude in a session counts as one. The
  JediTerm driver stays Java, because JediTerm is a JVM library; the Go side talks to it one line
  per command.
- The baseline terminal types raw xterm input (`CSI 13;2u`, `CSI I`/`CSI O`, SGR wheel) through
  `send-keys -H`: tmux 3.3a does not know the key name `S-Enter` and types it literally, and an
  outer tmux reports focus changes only to panes of an attached client.
- tmux 3.3a expands `display -p -t =SESSION` to nothing when no client is attached; the tests read
  formats through `list-panes`.
- tmux 3.7 prints nothing for `list-keys -T prefix KEY`, and its `new-session -A` honours `-c`: a
  reattach from another directory moved the session's directory for new windows there. Since 0.2.0
  `join` attaches with `attach-session`, which moves neither claude nor the session.
- The test image builds tmux from source, the release `TMUX_VERSION` (see 6), on `debian:trixie`.
- tmux 3.7's `paste-buffer` writes control characters as `^X` unless given `-S`; the baseline
  terminal always passes `-S`, since a terminal pastes them as they are.
- `capture-pane -e` emits an SGR change at the next cell that differs, which moves between
  redraws and sizes (a colour reset can land before or after a line break); the reattach test
  compares cells - characters and attributes - rather than the captured sequences.
- The Go port (decision 11) is `cmd/cld`, the command line, and `internal/session`, the tmux
  side, whose package comment is the script's header comment. Errors carry an exit status up to
  `main` (`internal/fail`), the only place that exits; `new` and `join` end in `syscall.Exec` of
  tmux, and a tmux command that fails (`tmux -V`, the kill) ends cld with its status after
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
- A server per session (decision 13): `lookup` reports whether a session's server runs and whether
  the session is on it, and `new`, `join` and `kill` need both, to refuse a server that outlives its
  session. `noServer` takes three of tmux's messages for no server: `no server running on` (a stale
  socket), `error connecting to` with `No such file or directory` (none), and
  `server exited unexpectedly` (the server exited while tmux asked it); any other error connecting,
  `File name too long` above all, ends cld with tmux's message. `Sessions` reads the socket
  directory with `os.ReadDir`, which sorts by name, and takes from each server's answer only a line
  for the session named like the server. `OwnPane` asks the server `TMUX` names with `-S` and that
  path, whatever `TMUX_TMPDIR` is now. In the tests, `Sandbox.Tmux` takes the server to run against;
  `Sessions` and `Clients` go over every socket `cld-*`, and `Sessions` names a session that is not
  on the server named like it `SERVER/SESSION`, so that one on the wrong server shows. The fake tmux
  answers `list-sessions` with `no server running` unless a test gives it sessions: `new` now tells
  a server without its session from no server. It fails as a server exiting does for the servers
  `CLD_FAKE_TMUX_EXITED` names, and with `CLD_FAKE_TMUX_REAL` runs a real tmux for all but
  `list-sessions`, so that a second `cld new` can reach a running server as the one of two at once
  that loses the race does.
- The session list (decision 14) is `internal/picker`; `cmd/cld` decides when it runs and hands
  it join's checks. A `fail.Error` keeps the advice for the command line (`Advice`, such as
  ` (see cld help)`) apart from its `Message`: `main` prints both, the list's footer the message.
  The list's lookups run tmux with the terminal, in raw mode, as their stdin; `list-sessions`
  leaves its mode alone (`TestListJoin`'s gone case reads it with `stty -a`).
- The baseline terminal types the list's keys by name with `send-keys`, which sends, on tmux
  3.7c: `Up` ESC [ A, `Down` ESC [ B, `Escape` 0x1b, `Enter` 0x0d and `C-c` 0x03, and after the
  program in the pane has sent CSI ?1h (application cursor keys) ESC O A and ESC O B for `Up` and
  `Down`.
- `Modes` tells whether the cursor is visible: in the baseline terminal the outer pane's
  `#{cursor_flag}`, 1, and 0 after CSI ?25l (tmux 3.7c); in JediTerm what its display's
  `setCursorVisible` last received - the emulator's `CursorVisible` mode (DECTCEM) calls it -
  starting as visible.
- The JediTerm driver types `Up` and `Down` as key-pressed events of `VK_UP` and `VK_DOWN`, which
  JediTerm's encoder turns into ANSI or application cursor sequences as the program asked, and
  `Escape` as a key-pressed `VK_ESCAPE` with the key char ESC: the encoder has no code for Escape
  (jediterm-core 3.76, read with `javap`), and JediTerm's key processing passes a pressed key
  without one on when its key char is a control character, as it does for a Ctrl+letter.
- The baseline terminal logs the pane's output with `dd bs=65536` rather than `cat`: uutils'
  `cat` (0.8.0, Ubuntu 26.04) held the last read back until the next one came, so the log missed
  what a program wrote last, where GNU's `cat` (9.1) did not; `dd` writes each read as it comes,
  GNU's and uutils' alike.
- The baseline terminal types `S-Up` (`CSI 1;2A`), `M-j` (ESC j), `M-Escape` (ESC ESC) and `M-Up`
  (ESC ESC [ A) as raw bytes, and freezes (`Freeze`) by stopping the outer tmux server with SIGSTOP:
  it then neither reads the pane nor answers it, until the test thaws it. It types a key held down
  (`Hold`) in one command list, `send-keys` and `run-shell -d` with no command, which only waits,
  in turn: the outer server times the repeats, each within a millisecond of its time on tmux 3.7c,
  where `Keys` starts a tmux client for each key - 150 ms apiece natively, which under heavy load
  now and then left a second between two keys.
- The stop cases run `cld list` as a job of an interactive `sh` (`sh -i`), which has job control,
  goes on with its script when the job stops and puts it back with `fg` once the test says so;
  before `fg` the script puts its own mode back, as bash does when it takes the terminal back and
  dash does not. It is interactive for macOS's `sh`, which under `set -m` in a script never hears
  that the job stopped (see Findings); there, bash has put its own mode back by the time the script
  reads it, so only dash, on Linux, checks the mode cld leaves while stopped. The background case
  turns job control on for `&` and off again (`set +m`), so that bash 3.2 does not report the
  job's end on the screen. Without job control, cld runs in the process group of the pane's
  program, the session leader. Tests that need to know the list has taken a key that changes
  nothing on the screen count its frames in the output log: each starts with `CSI 1;1H`.
- Tests that need cld's exit status or the terminal's mode run `cld list` under `sh`, which writes
  them - `$?`, `stty -g` before and after cld, and `tty` - to files, and then sleeps: once the
  outer pane's program exits, tmux writes `Pane is dead` onto the screen the tests read. The
  terminal's screen and its output log trail cld's exit status - the log more, through a pipe to
  `dd` - so the tests wait for what they expect there rather than read it once.
- `TestListJoin`'s busy-terminal case checks that Enter does not hand the terminal over before the
  terminal has answered, and that the answer does not reach claude: a tmux first on the `PATH` holds
  Enter's lookup while the test freezes the baseline terminal, and once the lookup is let go cld's
  process has not become tmux (`/proc/PID/comm`, which a tmux client may set to `tmux: client`, or
  `ps -o comm=`) a second and a half later, when the test thaws the terminal; claude's probe then
  reads a key typed after the join, and no answer before it. For a terminal that does not answer,
  the test thaws it only once cld has become tmux, five seconds after the lookup at the earliest.
  Frozen after Enter instead, a terminal that had not stopped yet answered in time now and then
  under load. Freezing the terminal does not make it lose the title (see Findings), so the case
  checks the wait and the answer rather than the title; the enter case checks that the main screen,
  the cursor, the title and the question reach the terminal in that order, in one piece, from its
  output log. The same held lookup lets the tests signal cld, or press Esc or Ctrl+C, while the
  lookup runs, and check that its tmux is gone by the time cld has exited.

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

- Baseline and JediTerm contracts run on tmux 3.7c (Linux, Docker, built from source), and the
  baseline on Homebrew's tmux on macOS. Until the minimum rose to 3.7 (see 6), they also ran on
  3.3a, 3.4 and 3.5a.
- iTerm2 is not automated: every level beyond "launch only" needs permissions on the runner -
  controlling iTerm2 over AppleScript or its Python API (with authentication switched off), and
  posting synthetic key events (Accessibility). That is a decision for the maintainer, not
  something the test setup should grant itself.
