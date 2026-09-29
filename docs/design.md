# cld: design and research

This document records the research behind turning `cld` - a tmux + Claude Code launcher that
started life as a function in `~/.bashrc` - into a tested, distributable tool.
What cld offers its users is described in the [README](../README.md) and the
[user guide](guide.md).

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

## Findings

Probed on Linux unless a row says otherwise. A row names the versions it was probed on; the tmux
rows that name none were probed against tmux 3.6.

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
| several terminals on one session: `attach-session` without `-d` from two panes of an outer tmux, 100x30 and 80x20, on a server started with `-f /dev/null` and cld's `pane-died` hook (tmux 3.7c) | both stay attached, and `#{session_attached}` is 2. `window-size` is `latest`: the window takes the size of the terminal a key came from last, and a larger terminal shows it inside a border, the rest of its screen filled with `·`. The hook's `display-message`, under `if -F '#{window_active_clients}'`, reaches one terminal, the one used last, not the one attached last; after `attach-session` in the same command list, it reaches the attaching one. `C-q d` detaches only the terminal it is typed in; `kill-session` then `kill-server` ends every one with `[exited]`, its client exiting with status 0 |
| a dead pane that had focus reporting (`?1004h`) on, client attached | tmux 3.3a crashes on `kill-session` and on detach; 3.4 on detach and on a focus change of the terminal - both with every session on the server. 3.5a and 3.7c survive keys, wheel, clicks, paste, focus changes, resize, detach, reattach, the terminal closing and `kill-session`; `kill-pane` is safe in all four |
| `cld list` where `LC_ALL`, `LC_CTYPE` and `LANG` do not name UTF-8 - unset or `C`, as over ssh, in containers and cron (tmux 3.3a, 3.4, 3.5a, 3.7c) | tmux writes a command's output to such a client with `_` for each character it cannot print: the tabs, so a session showed as `demo_detached_/tmp` under NAME with STATE and DIRECTORY empty, and a directory's non-ASCII letters (`/tmp/café` became `/tmp/caf_`). `tmux -u` marks the client UTF-8, and the output arrives as it is. The other output cld reads - the session's name, `cld-NAME`, and the pids of its panes from its session lookup, `list-panes`' `0` or `1` - is printable ASCII and passes unchanged |
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
| `install.sh` against the release v0.4.0 on GitHub (curl 8.18.0; dash 0.5.12, bash 5.3.9 and busybox's sh on Ubuntu 26.04) | `releases/latest/download/cld.sha256` redirected (302) to `releases/download/v0.4.0/cld.sha256`, and that to `release-assets.githubusercontent.com`, over HTTPS both; a file the release lacks answered 404. Under each shell the script installed `cld 0.4.0`, whose checksum matched the line `sha256sum` wrote in `cld.sha256` on the release runner, `HASH  cld-OS-ARCH`; `CLD_VERSION=9.9.9` ended at curl's 404 for `cld.sha256` |
| `https://github.com/zadykian/cld/releases/latest`, v0.5.0 the latest release (curl 8.18.0, GET and HEAD) | 302 to `https://github.com/zadykian/cld/releases/tag/v0.5.0`, alike for both; `releases/download/v9.9.9/cld.sha256`, of no release, and the `releases/latest` of a repository that does not exist answered 404. cld built with `-X main.version=0.4.0`, run through a symbolic link, updated itself to 0.5.0 from there, replacing the file the link led to; that 0.5.0 knows no `update` |
| `claude --version` (2.1.282, native installer, Linux) | prints `2.1.282 (Claude Code)` and exits 0, in about 20 ms. It leaves nothing running that holds its output: piped to `cat`, it returns as soon. In a directory that has since been removed it prints `error: The current working directory was deleted, so that command didn't work. Please cd into a different directory and try again.` on stderr and exits 1 |
| `claude --help` of 2.1.282 on resuming | `-r, --resume [value]`: "Resume a conversation by session ID, or open interactive picker with optional search term"; `-n, --name <name>`: "Set a display name for this session (shown in the prompt box, /resume picker, and terminal title)"; `--fork-session`: "When resuming, create a new session ID instead of reusing the original". The help says nothing of resuming by name, which Claude Code's docs describe, and names no restriction on giving `--name` with `--resume`. `--resume`'s value is optional (`[value]`): by the rule of commander, whose `.option()` calls the bundle holds, a word starting with `-` after it is read as the next option, not as its value (not run) |
| how `claude` 2.1.282 resumes (read from its bundle, not run) | `--resume ID` with no conversation for the ID prints `No conversation found with session ID: ID` and exits 1. A conversation that runs as a background session (`claude --bg`) is refused, naming `claude attach` and `claude stop`, unless `--fork-session` is given; one open in an interactive claude is not. When Remote Control starts and another process on the machine holds the conversation's Remote Control session, claude leaves Remote Control off with a notice that starts `Remote Control not started here · another Claude Code on this machine ... already has Remote Control for this conversation` and ends `run /remote-control to move it to this terminal`. Not found in the bundle: whether a session that connected at startup, as cld's do, records its Remote Control session in the conversation, and how `remoteControlAtStartup` on the command line combines with a recorded one |
| an argv word that ends in `;` (tmux's `cmd_parse_from_arguments`, read in the 3.3a and 3.7c sources; run on 3.7c, and on 3.3a, 3.4, 3.5a and 3.7c by `TestResume` and `TestDirectoryTmuxWouldChange`) | ends the tmux command, the text before the `;` staying an argument: `a;` reaches the program as `a`, and the next word starts a new tmux command. A word ending in `\;` becomes the text with `;` - `a\;` arrives as `a;`, `a\\;` as `a\;` - and a `;` elsewhere in a word is left alone. The words of a command given to `new-session` are not format-expanded: `#{session_name}` arrives as it is |
| a `#` in `new-session`'s `-c` (tmux 3.3a, 3.4, 3.5a and 3.7c: plain tmux, and `cld new` and `cld resume` before and after the fix, by hand in Docker; `TestDirectoryTmuxWouldChange`) | tmux expands `-c` as a format, after splitting its command at `;`, and `#{session_path}` keeps the result: `/tmp/w/C#S` became `/tmp/w/C` (`#S` is empty then: the session does not exist yet), and `/tmp/w/x#(touch ran)` became `/tmp/w/x` while tmux ran `touch ran` through the shell in the client's directory (with `new-session -d`, 3.3a to 3.5a; with an attached client, as cld's, all four). A `-c` that names no directory starts the program in the home directory, and with 3.3a where the server started. So `cld new`, and `resume`, in such a directory started claude elsewhere, and in one named `x#(command)` ran command. `/tmp/w/C##S` gives `/tmp/w/C#S`: `##` is a `#` |
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
| what tmux writes to the terminal as a client attaches to a session whose program asks for all-motion mouse reporting (tmux 3.7c, the probe as claude; the output of the baseline terminal and of JediTerm 3.76) | tmux turns every mouse mode off (`CSI ? 1006 l`, `? 1000 l`, `? 1002 l`, `? 1003 l`) and then on again as it wants them, after it has drawn: several times as the client attaches, and the last time after the pane's text, as the program's request comes in. A terminal that takes in the output a piece at a time while it is asked about it, as the JediTerm driver's emulator does on a thread of its own, shows the pane's text with mouse reporting off for a moment: C7 read the modes there once, under load (`{AltScreen:true Mouse:false}`), and the driver had no mouse reporting to send the wheel through |
| a program exiting on a pty4j pty (pty4j 0.13.13, read from its source; the JediTerm driver) | pty4j's reaper thread waits for the process and then wakes the reader (`breakRead`): `isAlive()` is false from then on, while the pty may still hold what the program wrote last. Reads return that, and then the end of the stream. The driver's emulator thread takes it all in, but can be behind: C9 read the modes once `isAlive()` was false, before the emulator had taken in the last of what tmux wrote, which turns them off |
| two Ctrl+X (0x18) typed into a pane whose program reads in raw mode, `dd bs=64 count=1` in a loop, a line of hex a read (tmux 3.7c, natively and in the image `tests/Dockerfile` builds): by two tmux clients 50 ms apart, as the baseline terminal's `Keys` types them; by one command list, `send-keys C-x \; send-keys C-x`; and pasted with `paste-buffer -p -S` | from two clients, two reads of a byte each, in 20 rounds of 20 natively and 50 of 50 in the image; from the command list and from the paste, one read of both bytes, every round. tmux adds what a command types to the pane's buffer (`bufferevent_write` in `input-keys.c` and `cmd-paste-buffer.c`, read in the 3.7c source) and writes it out once its event loop comes round, in one write. What two clients type goes in two writes, which a program that has not read the first by the second reads at once all the same, as `cld list` stopped (SIGSTOP) until both had come did (see Implementation notes) |
| SIGTSTP in a Go program that has had it through `os/signal` (Go 1.27.1, Linux 7.0) | after `signal.Stop` or `signal.Reset`, `kill -TSTP` of the process did nothing: `sigdisable` leaves Go's handler in place for any signal that `sigInstallGoHandler` accepts, and the handler drops a `_SigNotify` signal that no channel wants. Never notified, SIGTSTP keeps its default action, since `initsig` skips `_SigDefault` signals. `kill(getpid(), SIGSTOP)` returned before the process stopped, under dash with `set -m`, and it stopped soon after; the SIGCONT of `fg` then reached `os/signal` |
| a job of a `sh -c` script under `set -m` that stops, and a background one that ends: macOS's `sh`, bash 3.2.57 as Apple builds it (read in its source, tag `bash-144`; seen on the CI's macOS 26 arm64 runner; run on Linux as GNU bash 3.2.57 with Apple's change to `jobs.c`), GNU bash 3.2.57 and 5.3.9, dash 0.5.12 | Apple's bash asks `waitpid` to report a stopped child (`WUNTRACED`) only when the shell is interactive, where GNU's asks whenever job control is on: in a script, `set -m` puts the job in a process group of its own, in the foreground, but once the job stops the shell goes on waiting for it to end, and never runs the rest of its script. With `-i` it goes on, `$?` 128 and the signal's number, as the others do in a script. bash 3.2.57, Apple's and GNU's, also reports a background job's end on stderr in a script while job control is on (`[1]+  Done ...`), which 5.3.9 and dash do not; with job control off again (`set +m`) once the job has started, it does not |
| a `list-sessions` client that connects as the server exits with its last session: `new-session -d`, `kill-session`, then `list-sessions -f`, 400 times (tmux 3.3a, 3.4, 3.7c) | the client printed `no server running on ...`, but for `server exited unexpectedly`, failing, in one round on 3.4 and one on 3.7c, and nothing in one on 3.3a and one on 3.7c. cld took the first for no session and reported the second as an error, `cld join` as the list's footer; since 13 it takes both for no server (see the row on a server that exits as it is asked). `TestListJoin`'s last-row case waits for the server to have exited before Enter |
| `kill-session`, then `list-sessions` at once, 400 rounds (tmux 3.3a, 3.4, 3.5a, 3.7c; Docker, 3.7c also natively) | with another session left, `list-sessions` never showed the killed one. With the last one killed, it reported `no server running on ...` every time; under load, eight such loops at once (2400 rounds in Docker), it failed with `server exited unexpectedly` in 40 on 3.3a, 19 on 3.4, none on 3.5a and 10 on 3.7c, and printed nothing, succeeding, in 53, 17, 0 and 1: tmux had not finished exiting. A `list-sessions` right after each failure reported no server. Natively on 3.7c (Linux 7.0, eight loops of 400) all 3200 reported no server |
| `kill-server`, then `list-sessions` at once, 400 rounds (tmux 3.3a, 3.4, 3.5a, 3.7c; Docker) | `no server running on ...` every time on 3.5a and 3.7c, but for `server exited unexpectedly` in 31 rounds on 3.3a and 44 on 3.4. Under load, eight such loops at once (2400 rounds), 3.5a failed so in 5 and 3.7c in 30; a `list-sessions` right after each failure reported no server. Since 13 cld takes both for no server, so the session list's read after a kill passes over the killed session's server as it exits (15.5) |
| `#{session_created}`, `#{session_id}` and `#{pane_pid}` in `list-sessions -F` (tmux 3.3a, 3.4, 3.5a, 3.7c) | `session_created` counts whole seconds: a session killed and made again within the second has the same value. `session_id` starts again at `$0` on a new server - after the last session was killed, say - so a session made again under the name can have the killed one's id; with another session left it gets the next id. `pane_pid` is the pid of the program tmux started in the session's pane, which a dead pane (`remain-on-exit`) keeps; `kill-session` ends such a session, with status 0 (no terminal attached). For a session, `pane_pid` is the active pane's in its current window: after `split-window -d` and `select-pane` onto the new pane it is the new pane's program's. `#{W:#{P:#{pane_pid} }}` gives every pane's, in every window, the active one or not, a dead one's too |
| Ctrl+X in Claude Code's agent view (`claude agents`): [its docs](https://code.claude.com/docs/en/agent-view), read 2026-09-25, and the hints of 2.1.282, read from its bundle, not run | the docs: `Ctrl+X` "Stop the session; press again within two seconds to delete it", and "Press `Esc` to dismiss the confirmation without deleting"; the second press deletes even when the stop failed. A deleted session leaves the list, its transcript stays for `claude --resume`, and agent view removes a worktree Claude created for it, uncommitted changes included - but keeps the worktree and the session when another session uses or has locked it, or it has commits Claude Code cannot confirm are saved elsewhere. The hints: `ctrl+x to stop` or `ctrl+x to delete` among a selected row's hints; `stopped · ctrl+x again to delete · esc to keep` and `ctrl+x again to delete · esc to keep` dim, as other hints; `stopped · ctrl+x again to delete` and `ctrl+x again to delete` in the error colour. Which shows when was not observed |
| a directory whose name holds control characters (0x01, ESC), in `#{pane_current_path}` of `list-sessions -F` and `list-panes -F` (tmux 3.3a, 3.4, 3.5a, 3.7c), for a client under `LANG=C.UTF-8` and `LANG=C` | 3.3a and 3.7c write the characters as they are to a UTF-8 client - under `C.UTF-8`, or with `-u` - and each as `_` under `C` without `-u`; 3.4 and 3.5a write them as octal escapes, `\001` and `\033`, under either, with `-u` or not |
| hint strings in Claude Code 2.1.282 (read from its bundle, not run) | hints are lower case, but for Enter and Esc in some, joined by ` · ` and drawn dim: `↑/↓ to navigate · enter to resume as a background session`, `↑/↓ to navigate · Esc to cancel`, and a list of hints beside `ctrl+x to ...` and `to go back` that ends in `esc to quit` or `esc to close · esc again quits`. Where each shows was not observed |
| cobra 1.10.2's `__complete`, which its completion scripts run on every TAB (read from the source; run with cld, and with a test program for the root hook) | cobra hands a flag's completion function the text after `-n `, `--name ` and `--name=`, and after pflag's `-n=`; a word such as `-nre`, which starts with `-` and has no `=`, it takes for an option name being typed, and offers the options that start with it: none. It prints what the function returns as it is - unfiltered and in the function's order, a tab before each description - one per line, then `:N`, the directive; `__completeNoDesc` and `CLD_COMPLETION_DESCRIPTIONS=0` (`PROGRAM_COMPLETION_DESCRIPTIONS`, else `COBRA_COMPLETION_DESCRIPTIONS`) drop the descriptions. An argument with no completion function gets `CompletionOptions`' default directive, `ShellCompDirectiveDefault` unless the program sets one: `:0`, on which the shell offers file names. A command line cobra cannot read before the word - an unknown command, an option the command does not have - gets `:0` whatever the default, with the error on stderr. A root `PersistentPreRunE` runs before `__complete` and gets the command that runs: `__complete`, also when called as `__completeNoDesc` (only `CalledAs` tells them apart), and `bash` for `completion bash`; one that fails leaves stdout empty and exits 1, which the bash script takes for `:0`. A help function set on the root is inherited by `completion` and its subcommands. `completion`, which cobra cannot run, shows its help and exits 0 for an argument that names no shell; `completion bash x` fails with status 1 and `unknown command "x" for "cld completion bash"`. Command names come in the order of the command's `Commands()`, each described by its `Short`, and cobra's own `help` command completes the same names; an option is described by its usage as written, cut at the first newline, backquotes included. `__complete` drops the error of a write that fails, where the commands that print the scripts return it from `Execute`, as Go words it (`write /dev/stdout: bad file descriptor` for a stdout open for reading only). `__complete` with no word after it fails with `requires at least 1 arg(s), only received 0` and status 1 |
| `cld join -n <TAB>` through cobra 1.10.2's bash script, typed into an interactive bash in a tmux pane (tmux 3.5a): bash 3.2.57 (the `bash:3.2` image) with bash-completion 1.3 built as Homebrew's formula builds it, and bash 5.2.37 with bash-completion 2.16 (Debian trixie) | with bash-completion, both list the names with their states on the second TAB - `bad (exited)  rev (attached)  review (detached)` - complete a prefix to the one name that starts with it, or to what all that do share (`--name=re` too), and list the commands and options with their descriptions. bash 5 adds a space after a completed name, and offers nothing where cld offers nothing (`-n x`, `new -n`). bash 3.2 has no `compopt`, so the script registers `complete -o default -o nospace` and cannot take `default` back: no space follows a completed name, and where cld offers nothing bash offers file names. `source <(cld completion bash)` loads nothing in bash 3.2. Without bash-completion, each TAB prints `_get_comp_words_by_ref: command not found` and bash offers file names. bash-completion 2.16 also loads the script from `~/.local/share/bash-completion/completions/cld` |
| the same through cobra 1.10.2's zsh and fish scripts (zsh 5.9, `compinit` on, the script as `_cld` in a directory on `$fpath` (see the next row); fish 4.0.2, the script in `~/.config/fish/completions`; Debian trixie, tmux 3.5a) | both list the names with their states on the first TAB - zsh as `bad -- exited` and `review  rev -- detached`, one line per state; fish as `bad (exited)  rev (detached)  review (detached)` - and insert the first on the second. Both add a space after a completed name, offer nothing where cld offers nothing, and list the commands and options with their descriptions. fish still shows its autosuggestion for a word that starts like a file name in the directory - the rest of the name, in grey - which TAB does not insert |
| where zsh 5.9 looks for `_cld` (Debian trixie, as an ordinary user without `sudo`; `cld join -n <TAB>` typed into an interactive zsh in a tmux pane, tmux 3.7c) | the user's `${fpath[1]}` is `/usr/local/share/zsh/site-functions`, owned by root with mode 755, so `cld completion zsh > "${fpath[1]}/_cld"`, the Linux line of cobra's help, fails with `permission denied`. With the script in `~/.zfunc/_cld` and `fpath=(~/.zfunc $fpath)` before `autoload -U compinit; compinit` in `~/.zshrc`, `$_comps[cld]` is `_cld` and the first TAB lists `bad -- exited`, `rev -- attached` and `review -- detached`; with the script written into that `${fpath[1]}` as root, the user's `compinit` loads it too |
| where bash-completion 2.16, zsh 5.9 and fish 4.0.2 read a user's completion scripts, and what `source <(cld completion zsh)` in `.zshrc` needs (Debian trixie, each as a new ordinary user, cobra 1.10.2; `cld completion <TAB>` typed into an interactive shell in a tmux pane, tmux 3.7c; Ubuntu 24.04's `/etc/zsh/zshrc` read too) | bash-completion loads `completions/cld` from each directory of `$BASH_COMPLETION_USER_DIR`, split on `:`, or else from `${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion`, before the system's directories, and Debian's `/etc/skel/.bashrc` loads bash-completion. fish's `$fish_complete_path` starts with `~/.config/fish/completions` (`$XDG_CONFIG_HOME/fish/completions`), then `/etc/fish/completions` and `~/.local/share/fish/vendor_completions.d`: a script in the first hides one in the others. fish makes `~/.config/fish` at its first interactive start, not for `fish -c`: writing the script into `completions` before then fails with `warning: Path '/home/u/.config/fish/completions' does not exist`, but fish reads the directory once made for it. `/etc/zsh/zshrc` runs `compinit` for every user on Ubuntu, unless `skip_global_compinit` is set, and not on Debian. With `compdef _gnu_generic foo` between two `compinit`s, `$_comps[foo]` is empty after the second. `source <(cld completion zsh)` in `.zshrc` needs `compinit` before it - without, zsh prints `command not found: compdef` as it starts and `$_comps[cld]` is empty - and cld on the `PATH` by then: without, zsh prints `command not found: cld` |
| the lines `cld setup completion zsh` adds to `.zshrc` (the same systems; `$_comps[cld]`, and `$functions_source[_cld]` after `autoload +X _cld`, printed by `zsh -ic`, and `cld setup completion <TAB>` typed into each of the three shells) | alone, they run `compinit` and register `_cld`, loaded from cld's file; after `compinit` and `compdef _gnu_generic foo`, both stay registered; before them, the later `compinit` registers `_cld` all the same, from `$fpath`, by the script's `#compdef cld`. `$XDG_DATA_HOME` and `$ZDOTDIR` move them as zsh reads them. With the script missing they do nothing, `compinit` included, and cld need not be on the `PATH` as zsh starts. The first TAB lists `bash`, `zsh` and `fish` with their descriptions in zsh, and so do bash - through Debian's own `~/.bashrc` - and fish, from the scripts `setup completion` wrote |
| cld's completion in bash with ble.sh, which edits the command line in readline's place and runs the completion functions itself: cobra 1.10.2's bash script, bash-completion 2.16, ble.sh 0.4.0~git20250806.8060b7a (Ubuntu 26.04's package, bash 5.3.9) and 0.4.0-nightly+d81fd54 (2026-09-08); lines typed into an interactive bash in a tmux pane, tmux 3.7c, from the JetBrains terminal's rcfile, a login shell and bash with ble.sh alone | cld's answers complete as in bash - commands, options, `join -n` and `-s`, `--mcp` after a comma - and ble.sh's menu shows the descriptions, which its adapter for cobra's script reads; choosing one inserts the word alone. Where cld offers nothing (`:4`: `new`, `kill -s`, `--collector-config`, an unknown command), ble.sh offers the directory's file names, on TAB and in grey as a line is typed. The script turns off `-o default` with `compopt` only where `type -t compopt` is `builtin`, and ble.sh makes `compopt` a function of its own while it runs a completion function, so `-o default` stays on; and wherever a function offers nothing, ble.sh offers completions of its own - options it reads from the help, file names - unless the function turns off `ble/default` (`compopt +o ble/default`; `-o ble/no-default` before). From the end of `__start_cld`, `compopt +o ble/default` alone leaves the file names of `-o default`; `compopt +o default +o ble/default` offers nothing, on TAB and in grey, and changes nothing else; bash's own `compopt` fails there, `not currently executing completion function`. ble.sh 0.4.0-devel3 (the latest release, 2023) takes no keys under bash 5.3; under bash 5.2.15 (Debian bookworm) it offers nothing there with cobra's script alone, and says `(no items)`. ble.sh 0.3.4 offers file names wherever a function offers nothing, and has no option against it (read from the source) |
| `--mcp=go` and `-n=c` in bash with ble.sh (the same versions) | complete file names - `--mcp=go.`, the part `go.mod` and `go.sum` share, and `-n=cmd/` - where bash completes `--mcp=goland` and `-n=cld`: cld answers `goland` (cobra's script, as in bash, drops `--mcp=` from the word), but ble.sh's adapter for cobra's script keeps the answers that start with the whole word, `--mcp=go`, when they have descriptions, and then offers its own. With the lines of 27, `--mcp=go`, `-n=c` and `--name=c` complete nothing. With `CLD_COMPLETION_DESCRIPTIONS=0` they complete; `-n c` and `--mcp go` complete either way. The adapter's code is the same in the nightly |
| where ble.sh 0.4.0~git20250806.8060b7a keeps its cache (Ubuntu 26.04, bash 5.3.9, in the test image as root and as a user with no passwd entry, as CI runs it) | in `${XDG_CACHE_HOME:-~/.cache}/blesh` only where that directory exists; else in `cache.d/UID` beside `ble.sh`, `/usr/share/blesh/cache.d` for the package, which only root may write to. As another user, it prints `mkdir: Permission denied` and `ble.sh: failed to initialize $_ble_base_cache`, and bash goes on without ble.sh, completing through readline. With `$USER` empty it also warns `insane environment` and goes on |
| the PATH of a pane that `new-session -d -e PATH=/b COMMAND`, run with `PATH=/c`, makes on a server started with `PATH=/a` (tmux 3.7c, Ubuntu 26.04) | `/c`, the client's: neither the server's nor `-e`'s. Without `-e`, and for `new-window`, `/c` too |
| how long `cld __complete join -n ''` takes, beside `cld list | cat` (tmux 3.7c, in the image `tests/Dockerfile` builds on `debian:trixie`, 8 CPUs, load about 2; natively too, with Ubuntu's tmux 3.7c snap) | in the image about 3 ms with no socket, 22 ms with four sessions and 147 ms over 36 sockets, 16 of them stale, where `cld list | cat` took 9, 27 and 152 ms: one `list-sessions` a socket, as `list` (13.1), and no `tmux -V`. Before a server per session (13) it was one `list-sessions` in all: about 5 ms with no server and 10 ms with four sessions. Natively, where each tmux client of the snap took about 150 ms to start, 6 ms with no socket and 625 ms with four sessions |
| when `claude` 2.1.282 reads its telemetry settings (Linux; this row and the next two with GoLand 2026.2.3 and the JetBrains OpenTelemetry plugin 2.1.5) | at startup only (the docs: environment variables). A running session keeps the endpoint it started with |
| the plugin's receiver | a separate process, `java -jar .../open-telemetry-plugin/satellite/satellite.jar`, listening on all interfaces on a random port (37223 in the probe). Settings › OpenTelemetry › Common has "Use fixed OTLP server port" and "OTLP server port". Not probed: what a GoLand terminal gets from the plugin's terminal customizer (`SatelliteTerminalCustomizer`; its core module names `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_SERVICE_NAME` and `OTEL_METRIC_EXPORT_INTERVAL`), and whether the `env` of `settings.json` wins over it for a claude started there |
| the plugin's protocols | OTLP/gRPC `Export` for traces, metrics and logs: `grpc-status: 0`. OTLP/HTTP `POST /v1/traces` over HTTP/1.1: no HTTP response. gRPC only |
| the collector config from an environment variable (Docker 29.6.0 and `otel/opentelemetry-collector` 0.161.0 on Linux, as in the rows below) | `docker run -e CFG="..." IMAGE --config=env:CFG` works, and so does `validate --config=env:CFG`; so does `-e CFG` with `CFG` in the environment of `docker` itself |
| two `--config` sources | merged in order: maps merge, lists are replaced. A second source that added `headers` and `compression: gzip` to `otlp/remote`, plus `processors: [batch]` to the metrics pipeline, kept the first source's `endpoint`, `tls`, receivers and exporters. `print-config` shows the merged result, with header values `[REDACTED]` |
| `validate` on a bad second source | `'otlpexporter.Config' has invalid keys: bogus_key`, exit 1; on a source that is no YAML, `retrieved value (type=string) cannot be used as a Conf ...`, exit 1. It prints nothing and exits 0 for a valid config, and for an empty second source |
| the exporter type `otlp` | a deprecated alias in 0.161.0: each exporter of that type logs `"otlp" alias is deprecated; use "otlp_grpc" instead` as the collector starts (`validate` says nothing). `otlp_grpc` takes the same settings, and a second source's `headers` and `compression` merge into `otlp_grpc/remote` as they did into `otlp/remote`; the receiver `otlp` is not deprecated. A second source written for `otlp/remote`, as #30's example was, adds an exporter of that name with no endpoint, which `validate` refuses: `exporters::otlp/remote: requires a non-empty "endpoint"`, exit 1 |
| the collector's own metrics, with `--network host` | its Prometheus server takes `localhost:8888` on the host. A second collector there exits 1: `failed to create meter provider: binding address localhost:8888 for Prometheus exporter: listen tcp 127.0.0.1:8888: bind: address already in use`. With `service.telemetry.metrics.level: none` no server starts (`Internal metrics telemetry disabled`), and both run |
| collector startup | logs `Everything is ready. Begin running and processing data.` within 3 s |
| collector startup with `service.telemetry.logs.level: warn` from a second source | `docker logs` shows nothing, the ready line included, and the collector runs: its receiver took connections on its port 0.6 s after `docker run`. The collector starts its extensions, then its pipelines, receivers last, and only then logs the ready line, at info level (`service.go` and `internal/graph/graph.go` of `go.opentelemetry.io/collector/service` v0.161.0) |
| a second source that moves the receiver (`receivers.otlp.protocols.grpc.endpoint: 127.0.0.1:M`) | the collector logs `Everything is ready` and listens on `M` alone: nothing takes connections on the port of the first source. With `service.telemetry.logs.level: warn` as well, its log stays empty |
| a collector whose receiver's port is taken, under `--restart unless-stopped` | logs `Error: cannot start pipelines: failed to start "otlp" receiver: listen tcp 127.0.0.1:4319: bind: address already in use` and exits 1; Docker restarts it at once, so `docker container inspect` can find it `running` again, with a `RestartCount` of 1 |
| `docker update --restart no` on that collector | prints its name, exit 0, whether it was `restarting` or `running` again with a `RestartCount` of 1: it ends `exited` and stays so, its log kept. A restart Docker had already scheduled may still come, once |
| the collector's log while the `--local` endpoint is down | about 30 lines in 45 s after one span: gRPC reconnect warnings for each exporter's channel, backing off, and `Exporting failed. Will retry the request after interval.` |
| a collector whose `--local` endpoint is its own receiver (`cld setup telemetry --local http://127.0.0.1:P --port P` before cld refused it), with the debug exporter added to its metrics pipeline, after `telemetrygen` sent it one metric | the debug exporter counted 1 batch after 2 s and 107 after 5 s, still growing, and `docker stats` showed the collector at 146% CPU: it sends what it receives back to itself, without end |
| where a Go program (Go 1.27.1, Ubuntu 26.04 with systemd-resolved) connects, dialling port P with a listener on `127.0.0.1:P` alone | `127.0.0.1`, `0.0.0.0`, `::`, `::ffff:127.0.0.1`, `localhost` and `foo.localhost` reach the listener; `::1` and `127.0.0.2` are refused |
| the ports the kernel picks, and what keeps a listener off a port (Linux 7.0.0-31-generic, Go 1.27.1; natively and in the image `tests/Dockerfile` builds, `ip_local_port_range` 32768 60999 in both): 5000 listeners on `127.0.0.1:0` one after another, 2000 connections to a listener, a port let go and 200000 listeners on port 0 after it, and sockets whose own range (`IP_LOCAL_PORT_RANGE`) is one port | the kernel picks from `ip_local_port_range` alone: a listener on port 0 got an odd port every time, a connection an even local port. A port let go came back to a listener on port 0 28 times in 200000 natively and 24 in the container, and a listener or a connection whose range was that port alone got it at once. `net.Listen` on `127.0.0.1` and a port - Go sets `SO_REUSEADDR` on a listener - fails with `EADDRINUSE` where a connection has that local port, open or in its 60 s of `TIME_WAIT` after it closed first, and where a socket is bound to it without listening and without `SO_REUSEADDR`, on `0.0.0.0` too. Such a socket refuses connections, and a listener or a connection limited to its port fails, with `EADDRINUSE` and `EADDRNOTAVAIL`; closed, it leaves the port free at once |
| the telemetry tests with ports that a listener on `127.0.0.1:0` had and let go, given to cld as `--port` (while checking #44: four Docker containers and a native run at once, 8 CPUs; again, four containers of eight full suites each, tmux and JediTerm; and in a container whose range `docker run --sysctl` narrowed to 40000 40199) | cld found the port in use: `TestSetupTelemetrySettings/cannot_write` failed so in 1 of 10 suites, on 45187, and in 1 of 32, on 45895 - both odd, as a listener's. In the narrowed range 4 tests failed so in 3 runs of the telemetry tests, two of them given the same port, 40033; in 20 runs 115 tests failed, 85 of them so. With the ports from above the range, handed out in turn, and the fake collector holding the port of one that takes no connections, 2 failed in 20 runs there, both where cld chose the port and the fake collector found it taken as it started; handed out in turn from inside the range, 5 failed in 20 runs, one of them finding its port in use and one a collector ready that takes no connections; without the hold, the tests of the wait and the port, 10 runs at 120 ports, found such a collector ready once, and none with it |
| `docker rm -f` on a missing container | prints `Error response from daemon: No such container: NAME`, exit 0; `docker container inspect` prints the same, exit 1 |
| `encoding/json` on a settings file that is not valid JSON (Go 1.26.0 and 1.27.1) | `json.Decoder`, which cld reads the file with, gives a bare `EOF` for an object cut short (`{`) in 1.26, `unexpected end of JSON input` in 1.27; and the offset of a syntax error before the white space ahead of it in 1.26 - `line 3` for a `}` at the start of line 4 - and after the character in 1.27. `json.Unmarshal` gives the same `*json.SyntaxError`, offset included, in both. `go.mod` asks for Go 1.26, which the macOS job of CI builds with; the Linux image has 1.27 |
| `docker pause` on the collector | `docker container inspect` says `paused`, and the receiver keeps its port: listening on `127.0.0.1:PORT` fails with `EADDRINUSE`, and the kernel accepts a connection there, with nothing to answer it. `docker rm -f` removes the paused container, and frees the port |
| a variable in `docker`'s environment passed on with `docker run -e NAME` (Linux 7.0, 4 KiB pages) | `NAME=VALUE` can be 131071 bytes long - Linux's `MAX_ARG_STRLEN`, 32 pages, less the NUL that ends it - and reaches the collector: with `CLD_TELEMETRY_EXTRA=` and 131051 bytes, `docker run -e CLD_TELEMETRY_EXTRA IMAGE --version` ran, and `cld setup telemetry` with a `--collector-config` of that length started the collector; with one byte more, `docker` could not start: `Argument list too long` (`E2BIG`). A NUL byte cannot be in a variable at all: Go's `exec` refuses such an environment |
| DNS in a container on the default bridge | a name the host resolves through its own DNS (systemd-resolved) resolves the same in the container |
| `cld setup telemetry --local http://127.0.0.1:4319 --remote http://127.0.0.1:4320`, with a collector with the debug exporter on each of those ports standing in for the plugin and a team's collector, and `telemetrygen` sending a span, a metric and a log to the port cld chose | the local one got all three, the remote one the metric only. A rerun kept the port; a second source with a key the exporter does not know was refused by `validate`, and the collector kept running with the settings unchanged; one that moved the receiver onto a taken port made cld report the collector stopped, with its log, within 2.4 s, and leave it `exited` (after one more restart Docker had scheduled) where it had restarted again and again, and a rerun without it recovered on the same port; one that set the collector's log level to `warn` left its log empty, and cld found it ready by its port within 1.5 s, the span reaching the local one. Checked again once cld waited for the port alone, with collectors of the debug exporter on two other ports: the same three signals and the metric, cld done within 1.2 s, `validate` included; with the collector paused, a rerun kept its port, and so did `--port` with that port, where cld had given the new collector another port; a second source that moved the receiver to another port made cld say after 10 s that the collector took no connections on its port, naming it, below the end of the log, with the ready line, and leave the settings - or say that its log was empty, with the level at `warn` too - and a rerun without it recovered on the same port |
| `/.claude/*`, then `!/.claude/settings.json`, in `.gitignore` (git 2.53.0 on Ubuntu 26.04; the tests' cases also on git 2.47.3, in the image `tests/Dockerfile` builds) | git ignores `.claude/settings.local.json` and would add `.claude/settings.json`; `.claude/*` and `!.claude/settings.json`, without the leading slash, do the same. A `.claude/` before them in `.gitignore`, or a `.claude` in `.git/info/exclude`, keeps git out of the directory, and the settings stay ignored whatever follows; `*.json` after them ignores the settings again. git reads a line without the carriage return of a CRLF and without trailing spaces, but a trailing tab stays in the pattern: `!/.claude/settings.json` and a tab excepts nothing. `git check-ignore -v PATH` names the last pattern that matches, an exception too, with exit status 0 either way, as `SOURCE:LINE:PATTERN`, a tab, `PATH`; `-q` without `-v` exits 1 for a path an exception keeps. `-z` needs `--stdin` (`fatal: -z only makes sense with --stdin`), and then ends `SOURCE`, `LINE`, `PATTERN` and `PATH` each with a NUL. Outside a work tree it exits 128: `fatal: not a git repository` |
| `git init -q DIR` as a command of `git rebase --exec` (git 2.53.0 on Ubuntu 26.04) | in the main work tree git runs the command with no `GIT_DIR` (`GIT_EXEC_PATH`, `GIT_PREFIX` and others); in a linked worktree with `GIT_DIR` naming its git directory, `.git/worktrees/NAME`, and `git init DIR` initialises that one again, leaving `DIR` an empty directory. git takes a git directory named other than `.git` for a bare repository: it wrote `core.bare = true` into the repository's `.git/config`, and the main work tree stopped working (`fatal: this operation must be run in a work tree`) |
| `git rev-parse --is-inside-work-tree --git-common-dir`, as `cld new` runs it for the repository's name (24.3), read through the names cld gives (git 2.47.3, in the image `tests/Dockerfile` builds) | in the main work tree, a subdirectory of it and a linked worktree under `.claude/worktrees`, the common git directory is the main work tree's `.git`; in a worktree of a bare repository it is the bare repository, `bare.git`; in a submodule the superproject's `.git/modules/NAME`, whose name is the submodule's path, `module.x`. In `.git` itself `--is-inside-work-tree` is `false`, and outside a repository `git rev-parse` fails (`fatal: not a git repository`) |
| `claude mcp list` (2.1.283, Linux, a scratch `CLAUDE_CONFIG_DIR`) in a project that `cld setup project --mcp goland,jbcontext,rider` wrote, GoLand and Rider 2026.2 running with their MCP servers on 64422 and 64482 | claude lists the three from `.mcp.json` - `claude mcp get jbcontext` names its scope `Project config (shared via .mcp.json)` - as `Pending approval (run claude to approve)` while the folder's workspace trust is not accepted; once it is, all three are `Connected`. Trusted, but without `enabledMcpjsonServers` in `.claude/settings.json`, they stay `Pending approval` |
| the port of a JetBrains IDE's MCP server (GoLand 2026.2.3's `mcpserver` plugin, `McpServerSettings` and `McpServerService` read with `javap`; GoLand 2026.2.3 and Rider 2026.2.1 remote-development backends listening) | the default is 64342 plus an offset per product, chosen by `PlatformUtils.getPlatformPrefix()`: IntelliJ IDEA 0, CLion 20, DataGrip 60, GoLand 80, PhpStorm 100, PyCharm 120, Rider 140, RubyMine 160, RustRover 180, WebStorm 200, any other 0 - so GoLand listens on 64422 and Rider on 64482, as they do here, the two running at once; an authorized endpoint takes the port 100 above (64522, 64582). The MCP Server settings keep a port of their own (`mcpServerPort`), and the system property `idea.mcp.server.force.port` overrides both; where the port was never changed, the options file (`mcpServer.xml`) holds `enableMcpServer` alone. "Copy HTTP Stream Config" in those settings gives `http://127.0.0.1:PORT/stream` |
| `${VAR:-DEFAULT}` in the URL of an `.mcp.json` server (claude 2.1.283, `claude mcp list` and `claude mcp get`, a scratch `CLAUDE_CONFIG_DIR`, the folder trusted, GoLand and Rider as above) | claude expands it as it connects: to DEFAULT where VAR is unset, and to VAR from its environment, or from the `env` of `$CLAUDE_CONFIG_DIR/settings.json`; VAR in the `env` of the project's `.claude/settings.local.json` was not used. It shows the URL with `${VAR}`, the default left out. `${VAR}` without a default, VAR unset: `[Warning] [goland] mcpServers.goland: Missing environment variables: VAR`, and the server fails with `'url' is not a valid URL` |
| real `claude` under tmux, first 12 s | enables `?2004` bracketed paste, `?2031` colour-scheme reports, `?1004` focus, `?1049` alt screen, `?1000/1002/1003/1006` SGR all-motion mouse; queries XTVERSION (`CSI > 0 q`), kitty keyboard (`CSI ? u`), DA1, DECRQM `?2026`; resets modifyOtherKeys (`CSI > 4 m`); sets the title `✳ <name>`. The pane stayed in key mode `VT10x`: no extended keys were requested in that window - because the probing shell carried `TERMINAL_EMULATOR` (see What the tests found) |
| `claude` 2.1.283's own title (read from its bundle; and the `#{pane_title}` of a claude working in a session of cld's, read every 50 ms for 45 s) | `MARKER NAME`, the marker from claude's status: `◐` and `◑` in turn, every 960 ms, while it is `busy`; `✳` while it is `idle` or `waiting` - a permission dialog, an MCP server's question. Where `TMUX`, `STY` or `ZELLIJ` is set and the feature flag `tengu_static_title_under_mux` (on by default) holds, the marker stays `✳`: the pane's title read `✳ cld-NAME` throughout. claude also writes its status to `~/.claude/sessions/PID.json` (`status`, `statusUpdatedAt`), which no documentation names, and sends no OSC 9;4 that tmux records: `#{pane_pb_state}` stayed `hidden` |
| claude's hook events (the linux-x64 bundles of 2.1.232 and 2.1.283, read, not run) | both have `UserPromptSubmit`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest` ("When a permission dialog is displayed"), `Elicitation`, `ElicitationResult`, `Notification` - of the types `permission_prompt`, `idle_prompt`, `elicitation_dialog` and others - `Stop` and `StopFailure` ("Fires instead of Stop when an API error ... ended the turn"). `PostToolUseFailure`'s input has `is_interrupt`. No event comes when the user interrupts claude as it writes |
| hooks given with `--settings` (the real `claude` 2.1.283, alone on a scratch tmux server, in a directory whose trust was accepted, Remote Control off; a second `UserPromptSubmit` hook exited 2, which blocks the prompt, so nothing reached the API) | claude ran them - `SessionStart` as it started, `UserPromptSubmit` on Enter - in its own environment, with `TMUX` naming the server and `TMUX_PANE` its pane: `tmux if -F -t "$TMUX_PANE" '#{!=:#{@cld-status},busy}' 'set @cld-status busy'` set the option on claude's session. No `Stop` followed the blocked prompt: the option stayed `busy` |
| hooks of a conversation claude runs in the background (the real `claude` 2.1.284 on Linux, in a session cld 0.8.1 made on tmux 3.7c; read with `ps`, `/proc/PID/environ`, the daemon's log and the transcript - not made to happen by hand) | two seconds after cld started claude, `claude daemon run` - started the day before, from a claude in the default tmux server - logged `bg spawned ID (slash)` and ran the conversation in a worker of its own: `claude bg-pty-host`, running claude `--session-id ID` with the `--settings` of the claude in the pane word for word. The claude in the pane showed the conversation, whose transcript entries say `"sessionKind":"bg"`. The worker's environment had no `TMUX` and no `TMUX_PANE` - the daemon's had those of its tmux - and its directory was the session's. Every hook failed there: `tmux if -F -t "$TMUX_PANE" ...` went to the default server, which ran with no session; `if -t ''` found no target, which `if` allows, and its `set`, without one either, said `no current session`, exit 1. claude showed that after each tool (`PostToolUse:Bash hook error`), 141 times in two hours, and `@cld-status` stayed unset. Without a target tmux takes the pane in the client's `TMUX_PANE`, and without that the session with the latest activity (`cmd-find.c`, tmux 3.7c): with a session on the default server, the option would have gone there. `tmux -S SOCKET if -F -t =cld-NAME: '#{!=:#{@X},x}' 'set -t =cld-NAME: @X x'`, run from `/tmp` without either variable, set the option on the session, and nothing on the default server |
| Claude Code's background sessions beside cld's (the [agent view](https://code.claude.com/docs/en/agent-view) and [fullscreen](https://code.claude.com/docs/en/fullscreen) docs, read 2026-09-29; the linux-x64 bundles of 2.1.283 and 2.1.284, read; `claude agents --json` of 2.1.284, run beside two sessions of cld's - nothing started, attached or stopped) | the docs: `claude --bg`, `/bg` and `←` hand a conversation to a supervisor process that runs it without a terminal, and `/fork` a copy of it; an attached one renders fullscreen whatever the `tui` setting says, screen reader mode included, and tmux's copy mode sees only the screen; the supervisor stops a session's process once it is done, or waiting for the next message, and has been unattached for about an hour, unless it is pinned, and resumes the conversation on attach; it starts a process that exits unexpectedly again; after a shutdown a session shows failed - stopped past 48 hours - and attaching resumes it; agent view is a research preview, and shows an interactive session only once it has gone to the background. Both bundles: `claude attach`, `logs` and `stop` take a prefix of a short ID, 8 hex digits (`/^[a-f0-9]{8}$/`), and for anything else, a name included, print `No job matching 'X'. Run 'claude agents' to list running sessions.` and exit 1; the setting `disableAgentView`, "Equivalent to CLAUDE_CODE_DISABLE_AGENT_VIEW=1", disables "agent view (`claude agents`, `--bg`, /background, the on-demand daemon)". `claude agents --json` listed each session of cld's as `"kind": "interactive"` and `"name": "cld-NAME-SUFFIX"`, with its `pid`, `sessionId` and `status`, and no `id`, which only a background session has. The idle stop, the restart, the renderer and a shutdown were not seen |
| where claude runs a hook, and what `SessionStart` and `CwdChanged` see (the real `claude` 2.1.283 started in a linked worktree whose trust was accepted, alone on a scratch tmux server, Remote Control off; bash mode's `!cd /tmp`, `!cd` to the main worktree and `!cd` back - which claude answered with the model all the same, three short replies) | `SessionStart` ran in claude's directory, with it as its input's `cwd`. `!cd /tmp` fired `CwdChanged` with `new_cwd` `/tmp`, and `!cd` to the main worktree one with that; each time claude then took its shell back to the worktree the session works in ("Shell cwd was reset"), with no event, and the hooks' own directory, their `cwd` and `#{pane_current_path}` stayed the worktree throughout. `!cd` back to it fired nothing, the shell being there already |
| where claude sets its directory (claude 2.1.283's bundle, read, not run) | a hook runs in the host's project root where a launch sets one, and otherwise in claude's current directory. claude sets that - `process.chdir` and the session's `setCwd`, whose change fires `CwdChanged` - for `--worktree` as it starts; for `EnterWorktree` and `ExitWorktree`, the latter back to the directory it came from; and for a resumed conversation that recorded a worktree. A `WorktreeCreate` hook replaces claude's own making of a worktree (its stdout names the directory), so it is no event to listen to |
| `set-titles` under `status off` (tmux 3.7c: `server-client.c`, `format.c`, `options.c`, `status.c` and `cmd-refresh-client.c` read, and probed with a client in a pane of another server, whose `#{pane_title}` is the title that client sets) | tmux expands `set-titles-string`, a session option, with strftime whenever it redraws a client, and writes the title only where it changed; it restores no title on detach. Setting any option, a user option too, redraws every client on the server: `set @cld-status busy` turned `✳ NAME` into `◐ NAME` at once. With `status off` no timer expands the title again, and a `#()` job in it redraws nothing when it ends - only the status line's jobs do - but a job that runs `refresh-client -S`, which redraws the status alone, that is the title, a second later in the background kept it turning: `◐` and `◑` swapped every 1.0 to 1.3 s until the option changed, with the job naming a tmux whose path has a space, `#`, `%` and parentheses through `#{q:@OPTION}`. tmux runs a title's job at most once a second for each client. With `set-titles` on tmux also hands the active pane's directory (OSC 7) to the terminals it credits with `osc7`, iTerm2 and foot among them: an empty one for claude, which sets none |

## Distribution

- `cld` changes nothing in the calling shell (no `cd`, no `export`), so it does not need to be a
  shell function. It became an executable: testable in isolation, versioned, installable. Up to
  0.3.0 that was a bash script, `bin/cld`, ending in `exec tmux ...`; since then it is a Go
  program (see 11), built per platform, whose `new`, `resume` and `join` replace themselves with
  tmux the same way (`execve`).
- Tagged GitHub releases (`vX.Y.Z`) publish a binary per platform, `cld-OS-ARCH` for Linux and
  macOS on amd64 and arm64, with the version stamped in, plus `cld.sha256`, which lists their
  SHA-256 checksums, and `install.sh`, which picks the binary from `uname`, checks it and installs
  it into `~/.local/bin` (see 20); `cld update` replaces cld with a newer release's binary (see
  21); `make install PREFIX=...` builds cld for the host from a clone, with Go. The releases of
  the script published `cld`; that download fails once a Go release is the latest.
- The binaries are built with cgo off, on the Linux runner: the Linux ones are static, the
  darwin ones link only system libraries (`libSystem`, `libresolv`), and Go's linker signs the
  darwin/arm64 one ad hoc, which Apple silicon requires. They are not notarized.
- A Homebrew tap is possible later. A `curl | sh` installer was first thought not needed for one
  binary, which a one-liner picked from `uname`; one replaced it all the same (see 20).
- Shell completion comes from the binary (see 17): `cld completion SHELL` prints the script, which
  always matches the binary it came from. Releases publish no completion files, and
  `make install` installs none; cobra's scripts ask `cld __complete` for everything at TAB time
  and change only with cobra's templates. `cld setup completion SHELL` writes the script where the
  shell reads it, and `cld update` writes it anew from the release it installs (see 22). A Homebrew formula would generate them at install time
  with `generate_completions_from_executable(bin/"cld", shell_parameter_format: :cobra)`.

## Testing

### Layers

1. **Static**: gofmt and go vet; ShellCheck and shfmt for `tests/jediterm/fetch-deps`, the one
   shell script left.
2. **Behaviour against real tmux**: tmux is local and cheap, so it is not faked. Only `claude` is
   replaced, by a *probe* that behaves like claude towards the terminal (the modes above), logs
   its argv, cwd, environment and raw input bytes, and emits OSC sequences on request; and
   `docker`, for `setup telemetry`, by the same probe, which records the calls and fakes their
   results.
3. **Terminal contract**: the same checks run against several *outer terminals* through drivers.

Isolation needs no seams in the program: `TMUX_TMPDIR` moves cld's sockets (`-L cld-NAME`) into
a sandbox, `HOME` points at a temporary directory, `TMUX` is unset, and the probe is first on
`PATH`. `cld new` and `cld join` attach, so they need a pty; a terminal driver provides one.

### Terminal contract

| # | Check | Evidence |
|---|---|---|
| C1 | tab title is `✳ cld-NAME`, with `◐` and `◑` in turn in place of `✳` while claude is busy and ` [w]` after it while claude is in a linked git worktree, and survives claude's own title changes | terminal, probe |
| C2 | tmux's view of the client: `#{client_termtype}`, `#{client_termfeatures}` (`extkeys`, `focus`, `mouse`, `clipboard`, ...) | tmux |
| C3 | tmux asks the terminal for modified keys and takes the request back on detach; Shift+Enter reaches claude distinct from Enter; the Ctrl keys claude binds (`C-b`, `C-_`) pass through; `C-q d` detaches; `C-q C-q` sends `C-q` | probe input log, terminal output |
| C4 | mouse wheel and focus in/out reach claude; over a main-screen program without mouse reporting the wheel scrolls the pane's history | probe input log, tmux |
| C5 | OSC 52 / OSC 9 wrapped in tmux passthrough, and copies through `tmux load-buffer -w`, reach the outer terminal | terminal |
| C6 | claude never sees `TERMINAL_EMULATOR`, including in a session created in the JetBrains terminal, nor does what claude starts through tmux on its server | probe env dump, tmux |
| C7 | after detach the terminal is clean: no mouse reporting, no alt screen | terminal |
| C8 | a paste reaches claude bracketed and whole; a prefix key inside it is text, not a binding | probe input log |
| C9 | claude exiting ends its session, and its server unless tmux sessions claude made keep it running; the terminal is left clean. A claude that fails - exit status other than 0, or a signal - keeps its session, with its message and how to end it on screen | terminal, tmux |
| C10 | the session list (`cld list` on a terminal) reads the terminal's own keys: Down and Enter join the second session, which shows, with the title `✳ cld-NAME`; Ctrl+X pressed twice kills the selected session - its row goes, and its claude exits; Esc leaves the terminal as it was: the main screen, no mouse reporting, the cursor visible and the same `stty -g` | terminal, probe |

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
  source, in a Linux container) and on Homebrew's current tmux on macOS; contract x JediTerm; the
  completion tests in bash with ble.sh, in the same image built on Ubuntu (27.5);
- nightly, on tags and on demand: contract x iTerm2 on macOS, uploading screenshots and logs on
  failure; non-blocking until it proves stable;
- tags: release.

The Linux job runs the same Docker image a developer runs locally.

Pull requests land on `main` by fast-forward, so `main` holds the very commits CI checked. GitHub
has no such merge method - a merge commit, a squash and a rebase all write commits of their own,
a rebase setting each committer anew - so none is left: the repository allows merge commits alone
and the ruleset on `main` rebase alone, besides a linear history. A push of a pull request's head
to `main` passes the ruleset once its required checks pass and its conversations are resolved, and
GitHub marks the pull request merged. `.github/workflows/fast-forward.yml` makes that push for a
comment `/fast-forward` from someone who can push; a pull request that changes
`.github/workflows` is pushed by hand, since `GITHUB_TOKEN` may not push such a change.

### Spike before building the iTerm2 driver

1. Does the pinned iTerm2 start on a hosted macOS runner without a dialog blocking it?
2. Does the Python API connect with "allow all apps" set through `defaults`?
3. Can a single Shift+Enter key event be posted?

## Decisions

1. Naming: names are validated (`[A-Za-z0-9][A-Za-z0-9_-]*`), not sanitised - a silent rename
   would make `cld foo.bar` and the session it attaches to disagree. Since 13 a name has at most
   64 characters, so that its server's socket path fits (see 13.2). Since 24 a session's name is
   `NAME-SUFFIX`, and the repository's or directory's name that `NAME` defaults to is made one: it
   is no name typed (24.4).
2. Inside another tmux: `cld` nests; the private socket already allows it. Inside a live pane of
   one of its own servers - claude's external editor, say - a session attached would show inside
   a session of cld's, itself or another, both taking `C-q`, and `new`, `resume` and `join`
   refuse, pointing at `C-q d`; tmux refuses there too, but advises to unset `$TMUX`. tmux goes by
   the tty's name, and a dead pane's name comes back with the next pty opened (see Findings), so
   `cld` looks at the live panes itself and gives its client an empty `TMUX`, which tmux's check
   skips. Since 13 it looks only when the socket `TMUX` names is one of cld's, `cld-NAME`, and
   asks that server. `list` prints its table there rather than the interactive list, whose Enter
   would be refused (see 14).
3. Commands (0.2.0): `new` creates a session and fails if it exists, `join` attaches to one and
   fails if it does not; the name moves to `-n NAME` (default `main`, until 24). A bare `cld`
   fails, and `cld NAME` fails naming `cld new -n NAME` and `cld join -n NAME` (but for `cld completion`, a
   command since 17, and `cld setup`, since 18). Commands address sessions as `=cld-NAME`, since tmux would otherwise take
   `cld-rev` for `cld-review`. `list` shows the
   directory claude is in now (`pane_current_path`), not the one its session started in, and
   nothing at all when no server runs; on a terminal it lets you pick a session and join it (see
   14) or kill it (see 15). `kill` ends a session with `kill-session`: claude gets SIGHUP, as when
   its terminal closes (since 13, `kill-session` and then `kill-server` in one tmux command, with the
   same SIGHUP).
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
   after the session, also when that is the default `main`; since 24 by its whole name,
   `--worktree cld-NAME` (24.7).
5. Failures stay on screen: with `remain-on-exit failed`, a claude that exits with an error or a
   signal keeps its pane, so what it printed - a startup error above all, which would otherwise
   vanish with the session - stays readable. The format is empty, so tmux does not scroll that out
   of sight; a `pane-died` hook shows how to end the session on the message line instead, naming it
   through the session's one window, named `NAME` (since 24, as `kill` takes it, `-n` and `-s`,
   written into the hook as the session is made). The hook shows it only to a terminal on that
   window (`if -F '#{window_active_clients}'`), since tmux would otherwise show it on another
   session's terminal or over the next session attached (see Findings); `join` shows it on
   attaching to such a session, with a `display-message` in the same command list as
   `attach-session`. `list` shows such a session as `exited`, where it started, and `new` and
   `resume` refuse the name, pointing at `kill`, rather than replacing the session unseen. The
   option and the hook go to claude's window only (see 9 and 13).
6. Versions (#21): cld runs on tmux 3.7 or newer, the release its tests run on, and starts
   Claude Code 2.1.232 or newer, the first release that does what cld passes and relies on. Both
   are checked at startup and raised by hand, and neither has an upper bound. The tmux check runs
   for every command but `help`, `version`, completion (17.4), `setup telemetry` and `setup
   project`, which run no tmux (18, 19), and refuses an older tmux with `cld: tmux 3.7 or newer
   is required, found 'tmux 3.6b'` and status 1.
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
     Since each session has a server of its own (13), `new` and `resume` start a fresh server with
     the tmux they checked, so after an upgrade only the sessions started before it stay on the
     older tmux, each until it ends; on one shared server, the sessions started while one of those
     ran stayed there too. The gap is accepted, as it was with the 3.3 check, and the user guide
     says to end the sessions started before upgrading tmux; reading the server's `#{version}` as
     well was the alternative.
   - claude is checked where cld starts it: `new` and `resume` (16) run `claude --version` once they
     have found their tools and checked tmux's version, before any other tmux command. The issue
     placed it right after the lookup of `claude`; it comes after the checks every command makes
     instead - the tools, then `tmux -V` - so that cld runs claude only once those cheap checks
     pass, and a missing tool or a tmux too old is reported before a claude too old. `join`, `kill`,
     `list`, completion (17.4), `setup telemetry` (18) and `setup project` (19) never start claude
     and do not check it.
     cld compares the `X.Y.Z` the output starts with as numbers (2.1.30 is older than 2.1.232) and
     refuses an older claude with `cld: claude 2.1.232 or newer is required, found
     '2.1.231 (Claude Code)'` and status 1. It does not say how to update, which depends on how
     claude was installed; the user guide does.
   - `claude --version` runs the claude that tmux then starts, as tmux starts it: the `claude` that
     cld finds in the absolute `PATH` entries (11.5), by its path, with no input, in the current
     directory. `new` and `resume` hand tmux that path rather than the word `claude`, so claude sees
     its path as its `argv[0]`: tmux looks the word up with `execvp`, relative entries included (see
     Findings), and with `.` or an empty entry ahead of the absolute one it started a `claude` in
     the directory claude starts in, which cld had not checked. The directory counts too: a version
     manager's shim - mise's, say - runs the claude that the directory pins, so a check made
     anywhere else could pass a claude other than the one `new` or `resume` starts. A directory that
     has been removed is refused first, with `cld: the current directory no longer exists`, as `new`
     and `resume` refuse it anyway (11.10), because `claude --version` fails there (see Findings);
     they say so before they look the session up. So is one that cannot be entered - its search
     permission, or that of a directory above it, taken away since the shell entered it - with
     `cld: cannot enter the current directory: REASON` (11.10): `claude --version` cannot start
     there, which cld would otherwise report as a claude that cannot run.
   - Output that does not start with a version passes, as a tmux development build does, so that a
     new format locks no one out. A `claude --version` that fails is refused with status 1, what it
     printed (stdout, then stderr) and its exit status or signal, since a claude that cannot report
     its version is unlikely to start. cld reads what claude printed until it exits, and for a
     second more at most: a process it leaves in the background with its output open - a wrapper's
     update check, say - does not hold `new` or `resume` up for as long as it runs. A script without
     `#!`, which the system will not execute, runs with `/bin/sh`, as tmux's `execvp` runs it
     (glibc, see Findings; macOS's libc by its source, not run), and is checked as any other claude:
     Go's `os/exec` does not fall back so. A binary the system will not execute - one for another
     machine, or cut short - is no script, though `execvp` hands it to `/bin/sh` all the same (see
     Findings): cld tells the two apart as bash does (`check_binary_file`: ELF's magic number, or a
     NUL in the first line) and does not run a binary with `/bin/sh`. A claude that cannot run at
     all - no execute permission, a missing `#!` interpreter, such a binary - ends cld as such a
     tmux does (11.9), with `cld: cannot run PATH: REASON` and 127 or 126, since its version is not
     what is wrong. Where the script would have handed such a claude to tmux, which failed in its
     pane (see 11.5), `new` now stops in the terminal.
   - Why 2.1.232: the tests never run the real claude, so there is no tested version to require;
     the minimum is the first release that takes what cld passes and does what it relies on. What
     it passes came earlier - `--worktree` in 2.1.49, `--name` in 2.1.76, `remoteControlAtStartup`
     in the settings in 2.1.119, `worktree.baseRef` in 2.1.133 - but only from 2.1.222 does a
     project's `false` keep Remote Control off despite cld's `--settings`, as 10 and the user guide
     promise (see Findings), and #21 set the minimum there. `resume` (16) relies on behaviour
     documented up to 2.1.232 - the search for a session ID across projects in 2.1.223, and
     variants for live names and Remote Control staying with the claude that has it in 2.1.232
     (16.7) - so the minimum rose to 2.1.232 with it (#26), rather than the user guide saying which
     of `resume`'s behaviours need a newer claude than cld accepts. 2.1.133 would have needed the
     Remote Control promise qualified; 2.1.281, the version probed, would refuse the stable
     channel (2.1.274), which runs about a week behind and passes 2.1.232. The exit status of
     `/exit` and the key mode, recorded for 2.1.281, were not checked on older releases.
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
10. Remote Control: `new` and `resume` start claude with
   `--settings '{"remoteControlAtStartup":true}'`, so a session can also be continued from
   claude.ai or the Claude app, not only from a terminal that joins it; a resumed conversation
   does not keep the settings it was started with, and Claude Code's docs say to pass them again.
   Flag settings outrank the user's, so this holds whatever `/config` says; claude still keeps
   Remote Control off under an org policy or a project that sets the key to `false` (see
   Findings; from 2.1.222, older than the minimum in 6), and `cld` leaves those alone. With `-w`
   the worktree setting goes into the same JSON: one `--settings` rather than two, whose merging
   claude does not document.
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
       their order: `new`, `resume`, `join`, `kill`, `list`, `help`, `version`, with `completion`
       before `help` since 17, and `setup` after `list` since 18. `Execute` moves the help command
       after the others (see Findings), so cld moves `version` back after it before it prints the
       help. An option's usage names its value in backquotes (`-n, --name NAME`), and pflag
       showed `-n`'s default, `main`, until 24. cobra wraps nothing, so the texts break their
       lines by hand, within 80 columns, which the test of the help's text holds them to - all
       but cobra's own last line, `Use "cld COMMAND [command] --help" ...`, which names the
       command, 81 columns for
       `setup completion` (22.6). A template of cld's own, in the usage text's layout, was the
       other way: closer to what cld printed, but one more thing for
       cld to keep, where cobra's changes with cobra and shows in that test;
    2. `help [COMMAND]` shows the help of one of the commands the root's help lists. `-h` and
       `--help`, given first, are `help` spelled otherwise, so `cld -h new` shows `new`'s help. A
       `COMMAND` that is not one of those, the empty one included, and an argument after it - but
       one of that command's own, as in `help setup telemetry` (18.8) - are refused with status
       2 - `cld: help: unknown command 'nope' (see cld help)`, `cld: help: unexpected argument
       'join' (see cld help)` - where cobra shows the root's usage for the one and passes over
       the other, exiting 0 (see Findings). cld's `help` is a command of its own, set with
       `SetHelpCommand`, so it lacks the `ValidArgsFunction` with which cobra's completes command
       names after `help`: completion gives it one (17.3). Keeping `help` without an argument was
       the other way; per-command help would then be only `-h`'s;
    3. error messages keep `(see cld help)`, rather than naming the command's help (`see cld
       help new`): no message changes;
    4. `-h` and `--help` are read as before: before a wrong argument they show the help, now of
       the command they are given to, and after an argument they are no option but one more
       argument, and refused (`cld help new -h`, as `cld new review -h`). So `help`'s usage line
       names its options before `COMMAND`, `cld help [flags] [COMMAND]`, with
       `DisableFlagsInUseLine`, where cobra adds ` [flags]` at the end (see Findings), and the test
       of the help's text refuses an option after an argument in any usage line;
    5. the help is rendered into a buffer that `output.Print` prints, so that a write that fails
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
    environment of the shell that ran `cld new`, or `cld resume` (16). And a crash, or a stray
    `tmux kill-server` or `set -g` from anything claude runs, reaches one session. What it costs:
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
       a release: the user guide says to end them before upgrading, or afterwards with
       `tmux -L cld kill-session -t =cld-NAME`. `list` no longer shows the sessions with
       non-ASCII names that 0.3.0 made (11.7 and 11.8), nor a session renamed by hand, which is
       not the one its server is named after (11.7);
    5. one more tmux process per session, 4 to 5 MB, next to about 400 MB for claude;
    6. where tmux's socket directory ignores case - macOS's default file system, APFS, does, and
       `/private/tmp` is on it (not checked on macOS) - names that differ only in case share one
       socket, where the shared server kept `a` and `A` apart: `tmux -L cld-A` reaches the server
       of session `a`, which has no session `cld-A` (see Findings). `new`, `join` and `kill` - and
       `resume` (16) - would take it for a server that outlived session `A` and point at
       `tmux -L cld-A kill-server`, which ends `a`. So where the server they reach runs without its session, they ask it for
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
    keep the server running - is not reused: `new` refuses the name, and so does `resume` (16),
    pointing at `tmux -L cld-NAME ls` and `tmux -L cld-NAME kill-server`, so that every claude gets
    the environment of the shell that ran `cld new` or `cld resume`; `list` shows nothing for such a
    server, and `join` and `kill` refuse the name the same way, rather than send the user to a
    `cld new` that refuses it (but see cost 6 for a server that is another session's). `new`,
    `resume` and `join` refuse a terminal that is a live pane of any of cld's servers (see 2),
    found through the socket `TMUX` names, and say whose session's server it is; the terminal of any other tmux nests without a check. The
    options stay as they were, the fixed `terminal-features[100]` index too: `new` sets them on a
    fresh server, but two `cld new -n NAME` at once can both set them on one.
14. The session list (#23): on a terminal, `cld list` shows the sessions to pick one and join it, as
    Claude Code's agent view (`claude agents`, a research preview whose keys may change) lists its
    background sessions: `↑`/`↓` move between rows, Enter attaches, Esc leaves. Its footer follows
    Claude Code's hints (see Findings): `↑/↓ to navigate · enter to join · esc to quit` - with
    `ctrl+x to kill` before `esc to quit` since the kill (see 15) - dim, under the rows after a
    blank line; until 23, on a row with a terminal attached - an exited one too - `enter to join`
    read `enter to join and detach its terminal`. The first row is selected, marked `>` and
    in inverse video; `↑`/`↓` stop at the first and the last row, Enter joins, and Esc and Ctrl+C
    leave with status 0 - one Ctrl+C, since the list has no input to clear; once Ctrl+X has armed a
    kill, Esc only disarms it (see 15). Other keys, letters included, do nothing: agent view binds
    none, and its `→` pairs with a `←` to come back, which a cld session does not offer. Keys with
    Alt do nothing either: terminals send them as Esc and the key, so Esc followed within the wait
    for a lone Esc by another key is that key with Alt - but for a second Esc, which stands alone
    unless a sequence follows it (Alt+Up as ESC ESC [ A, as rxvt sends it). A message - why Enter
    could not join, or why a kill ended nothing (see 15) - takes the hints' place until the next
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
    5. rows behave as with `cld join -n NAME`: Enter on a row with a terminal attached joins beside
       that terminal - an exited row's too, whose STATE reads `exited` whether a terminal is
       attached or not; until 23 it detached that terminal, which the footer said first - and on an
       exited row joins and shows claude's last words with the hint - joining is how claude's
       message is read. Asking again, or refusing an exited row, would protect nothing: joining
       ends nothing;
    6. the list reads the sessions when it opens and after its own actions - a failed Enter here,
       and a kill (see 15) - never on a timer or on a key, so a row does not change under a key;
       each read asks every server in turn, as `list` does (see 13.1). A stale row costs at most a
       message, a kill detaching a terminal the list did not show, or joining a session made again
       under the same name, which `cld join -n NAME` would join too. After a read the selection
       stays on its session or, once that is gone, goes to the next row the list showed that is
       still there - the one that took its place - or else to the one above. Leaving and running
       `cld list` again shows what changed elsewhere;
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
    `attach-session` with the hint for an exited claude. Putting the terminal back waits for it
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
15. Killing from the session list (#24): in the list (see 14), Ctrl+X arms the kill of the selected
    session and a second Ctrl+X within two seconds kills it, as `cld kill -n NAME` does; Esc keeps
    it, and the list stays open. This follows agent view, where Ctrl+X stops a session and a second
    press within two seconds deletes it (see Findings). Where agent view has two steps, cld has one:
    `cld kill`'s `kill-session` and `kill-server` end claude, like agent view's stop, and the row
    leaves the list at once, like its delete; there is no stopped row to come back to, and the
    conversation comes back through `cld resume -n NAME`, in a new session (see 16), or
    `claude --resume`. Settled with it:
    1. confirming: the second press, as agent view asks for one before a row leaves its list. On a
       selected row the footer's hints read `↑/↓ to navigate · enter to join · ctrl+x to kill · esc
       to quit` (62 cells; 86 on an attached row until 23); once armed, `ctrl+x again to kill · esc
       to keep`, or `ctrl+x again to kill and detach its terminal · esc to keep` on a row with a
       terminal attached, an exited one too - dim, as agent view draws its own. Where the hints do
       not all fit, `↑/↓ to navigate`, which the arrows need least, goes first: in 61 columns the
       rest then take 44 cells, and `esc to quit` shows whole rather than cut to `esc to qui`. A
       terminal narrower still cuts the rest at its edge, as every line. Esc and the two seconds
       running out disarm it and do nothing more; any other key disarms it and then does what it
       does: an arrow moves the selection, Enter joins, Ctrl+C leaves. A key that has begun holds
       the two seconds back, so that an Esc typed within them keeps the session although the wait
       for a lone Esc ends after them. A key that has come by their end, but that cld, held up
       meanwhile - stopped, or busy - has not read yet, counts as typed within them too - the footer
       still asked for the second Ctrl+X: Esc keeps the session, Ctrl+X kills it, and another key
       disarms the kill. Without that, whichever cld took first, the end or the key, decided, and an
       Esc read late now kept the session and now closed the list. After a kill, whatever came of
       it, Ctrl+X does nothing until none has come for a second, or another key has come. A terminal
       types a key held down again after a delay, half a second or so by default, and then many
       times a second: without the wait, the second Ctrl+X held a little too long armed the kill of
       the row that took the killed one's place, which the selection had moved to, then killed it,
       and so on down the list - sessions the user never selected, which the second press is there
       to keep a stray key from ending. A second covers the usual delays; a Ctrl+X that comes
       meanwhile, as each repeat does, starts it again, and another key ends it at once, since only
       the key pressed last repeats. A Ctrl+X pressed on purpose right after a kill does nothing,
       and one after the wait arms the kill as before. One press, as agent view's stop, would be
       quicker, but a stray key would end a session and detach whoever is on it - through Remote
       Control too, which cld cannot see (`attached` counts terminals only). A `y/n` question is not
       how agent view asks. Ctrl+X is the byte 0x18 in raw mode;
    2. the session the list runs in: moot, since the list is not interactive in a live pane of one
       of cld's servers (14.3), where a kill would take the list, and the claude it came from, down
       with it;
    3. the same session: the list kills the session on the row only while its claude is the one
       the list read. The list reads the pids of the session's panes (`#{pane_pid}`) from each
       server, and the lookup asks for them with the session's name: a session that has none of
       the pids the list read counts as gone, `no session 'b'`. A session made again has a new
       claude, and a dead pane keeps its pid, so an exited row is killed as any other (see
       Findings). Every pane counts, `#{W:#{P:#{pane_pid} }}`: `#{pane_pid}` alone is the active
       pane's, and claude's window split by hand, the other pane selected since the list read it,
       made the session look gone. `#{session_created}`, in whole seconds, and `#{session_id}`,
       which starts again at `$0` on a new server - as a session made again gets one since 13 - do
       not tell a session made again from the one killed. The check narrows the window to the one
       `cld kill` has between its lookup and the kill. `cld kill -n NAME` goes on killing by name;
    4. one kill step: the list and `cld kill` run the same code, `End` in `internal/session` -
       after the name's check, the lookup and the refusal of a server that runs without its
       session (13), then `kill-session -t =cld-NAME` and `kill-server` in one tmux command. `End`
       returns its errors, and `cld kill` prints them and exits as before: a lookup that fails
       with its message and status 1, a kill that fails with tmux's own message and status. The
       list shows the lookup's errors in the footer, without the advice meant for the command
       line, as Enter does (`no session 'b'`, `session 'b' has ended, but its tmux server still
       runs`), and what tmux says when the kill fails - `tmux kill-session: exit status N` when it
       says nothing - rather than letting it write over the list;
    5. after the kill, whether it ended the session or not, the list reads the sessions again, and
       the selection moves as after any read (14.6): to the row that took the killed row's place,
       or the one above it for the last row; with none left, `no sessions`. The killed session's
       server exits after it, and a read that meets it exiting passes over it, as `list` does since
       13: tmux says that no server is running there or, now and then, that the server exited
       unexpectedly (see Findings). Before 13 the read met the shared server exiting with its last
       session and failed, and was made once more. When the sessions cannot be read, the list says
       why and keeps its rows, but for the one it killed. The kill runs beside the list, as Enter's
       lookup does: Esc, Ctrl+C and the signals leave, killing its tmux, and other keys do nothing
       until it ends. What the kill did by then counts for the table Esc and Ctrl+C print: once
       the kill's tmux command has returned, the table leaves the session out, or shows the
       sessions read again. An Esc typed right after the second Ctrl+X, meant as the armed
       footer's `esc to keep`, leaves once the wait for a lone Esc is over, when the kill has
       usually ended. A kill cut short may or may not have ended the session, whose row then stays;
    6. like `cld kill`, the kill leaves a `cld new -w` worktree where it is, where agent view's
       delete removes the worktree Claude created. Killing several sessions at once, a stopped
       state and removing worktrees are not part of it.
16. Resume (#26): `resume [-n NAME] [SESSION]` (since 24 `resume [-n NAME] [-s SUFFIX] [SESSION]`,
    `-s` or SESSION needed, 24.5) joins the commands of decision 3. It brings back a
    conversation whose session is gone - ended by `kill` or the list's Ctrl+X (15), a reboot or a
    crashed server - in a new session `cld-NAME`, made as `new` makes it: the same checks and
    refusals (the name, claude on the `PATH` and its version (6), a live pane of one of cld's
    servers (2), a session of that name, and a server that runs without it (13)), the same title,
    tmux command on a server of its own (13), options and hook. Only claude's arguments differ:
    `new`'s, without `-w`'s `--worktree` and worktree base, then `--resume cld-NAME`, or
    `--resume SESSION`.
    1. SESSION is whatever `claude --resume` takes - an ID, a name, a search term for claude's
       picker - for a conversation cld did not start, or one of several that share a name. It is
       one argument, handed to claude as it is, as one word. An empty one is refused, and one
       starting with `-`, which claude would read as an option (see Findings); so is a `--`,
       after which pflag would hand claude what follows (`resume -n x -- -p`). Options come
       before SESSION, the order `SetInterspersed(false)` gives every command, and whatever
       follows it is refused, an option included: `cld resume x -n y` names `-n`. So `resume`'s
       usage line names its options first, `cld resume [-n NAME] [flags] [SESSION]`, with
       `DisableFlagsInUseLine`, as `help`'s does (see 12.4); its help describes SESSION.
    2. `--name cld-NAME` goes with `--resume` always, SESSION or not: the session runs
       `claude --name cld-NAME`, as cld's help says, and a conversation resumed through SESSION
       is meant to take the session's name, so that the next `resume -n NAME` finds it - at the
       cost of the name it had. How claude combines the two is not probed (see Status).
    3. No `-w`: claude takes a worktree conversation back to its worktree itself, and the docs do
       not say how `--worktree` combines with `--resume`; so `resume` needs no git either. The
       worktree base (decision 4) is left out: it also governs the worktrees claude makes during a
       session, and cld cannot tell a worktree conversation from another; passing it on every
       resume would change conversations started without `-w`.
    4. No lookup of its own: cld does not read claude's transcripts (`~/.claude/projects`, or
       `$CLAUDE_CONFIG_DIR`), whose format Claude Code's docs call internal. claude resolves the
       conversation and reports what it cannot find, as it reports workspace trust for `-w`
       (decision 4); a claude that fails keeps its session as a failed `new` does (decision 5).
       By name, claude looks in the current repository and its worktrees, so `resume` runs where
       the conversation belongs; a session ID it finds from any directory (from 2.1.223).
    5. A session whose claude exited is refused, pointing at `kill`, as `new` refuses it
       (decision 5), rather than respawned with `--resume`: a claude that failed at startup has
       no conversation to resume, and `kill` then `resume` covers the rest. A split pane in a
       live session would break the one pane that `join`'s hint, the dead-pane check and `list`
       rely on.
    6. Not guarded: a conversation open elsewhere. cld sees tmux sessions, not conversations; two
       claudes on one conversation interleave their messages in one transcript, as Claude Code's
       docs say, and the README says so. claude gets the environment of the shell that ran
       `resume`, its server being its own (13), `CLAUDE_CONFIG_DIR` included, which decides where
       claude looks for conversations; on the one server all sessions shared before 13, it got
       that of the cld that had started the server.
    7. The behaviour `resume` relies on is documented up to claude 2.1.232: resuming by name
       (2.0.64), `--name` (2.1.76), the transcript following claude into a worktree (2.1.198),
       the search for a session ID across projects (2.1.223), and variants for live names and
       Remote Control staying with the claude that has it (both 2.1.232). `resume` checks
       claude's version as `new` does, and the minimum (6) rose to 2.1.232 with `resume`, so
       every claude that passes the check has all of it: the user guide need not say which of it
       needs a newer claude.
    8. tmux ends a command at an argv word ending in `;` (see Findings), and SESSION can end in
       one, as can the directory `new` and `resume` give tmux with `-c`: each of cld's words that
       does goes to tmux with a `\` before the `;`, which tmux drops. tmux then expands `-c` as a
       format (see Findings), in which a `#` starts a variable or a shell command, `#(...)`: the
       directory goes with every `#` doubled, since `##` is a `#`. So tmux gets the directory,
       and claude every word, as cld has them. Before, `new` failed in a directory ending in `;`,
       with status 1 and no session, and in one with a `#` it started claude elsewhere, and ran
       the command in `#(...)`. The test harness's outer tmux does the same for the command it
       starts and its directory.
    9. A resumed session is a session like any other: `list` shows it, the interactive list joins
       it with Enter (14) and kills it with Ctrl+X (15), and `kill` ends it with its server (13).
       `resume` is the way back from a kill by mistake, `cld kill`'s or the list's: neither leaves
       a stopped session to come back to (15), but the conversation stays in claude's history, and
       `resume -n NAME`, run where the session ran (16.4), brings it back in a new session.
17. Shell completion (#25): `cld completion SHELL` prints a completion script for bash, zsh or
    fish, with which `cld join -n <TAB>` offers the names `cld list` shows (since 24 `-n` their
    NAME and `-s` their SUFFIX, 24.8).
    1. It is cobra's: `completion SHELL` prints cobra's script (PowerShell's too, undocumented),
       which asks `cld __complete`, or `__completeNoDesc`, what to offer on every TAB. The
       scripts know nothing of cld's commands, change only with cobra's templates, and need no
       copy per shell to keep in step; `__complete` is tested from Go without a shell. The
       first-argument check lets `completion`, `__complete` and `__completeNoDesc` through, so
       `cld completion` no longer gets the legacy hint of 3.
    2. `join -n` offers every session `list` shows - `attached`, `detached` and `exited`, since
       `join` takes each - read as `list` reads them (`Tmux.Sessions`: the server of each socket
       `cld-NAME`, stale sockets included, 13.1), in its order, that of the names, and described
       by its state: what `join` will do, attach beside another terminal - take the session from
       it, until 23 - or show why claude exited. Every name `list` shows is one `join` takes: it
       reads no socket whose NAME `join` would refuse, and shows neither a session renamed by hand
       nor the sessions of 0.3.0's shared server, `café` among them (13.4, 11.8), so completion
       checks no name itself. cld keeps the names that start with what was typed, as cobra does
       for commands and options, so that every shell offers the same names whatever its own
       matching (zsh's `matcher-list`, fish's fuzzy matching). `-n NAME`, `--name NAME`,
       `--name=NAME` and `-n=NAME` complete; `-nNAME` does not (see Findings).
    3. Only `join -n` offers session names, and `help` the commands it takes (see 12.2), with
       their `Short`s, as cobra's help command does; since 19, `setup project --mcp` offers the MCP
       servers it takes (19.8). `new -n` offers none: it refuses a name a
       session holds, and cld keeps no record of the sessions that are gone. Nor does
       `resume -n`, for the same reason, or `resume`'s SESSION (16): offering the conversations
       claude keeps would mean reading its transcripts, which cld does not (16.4). `kill -n`
       offers none either, as the request named `join` only; it could take the same function
       later. No argument offers file names, since none is a file (since 18, but for
       `setup telemetry --collector-config`'s `FILE`, which offers none either, 18.9): the root's
       default directive is `ShellCompDirectiveNoFileComp`, and where cobra answers
       `ShellCompDirectiveDefault` all the same - a command line it cannot read before the word,
       such as `cld joni -n <TAB>` - cld turns its `:0` into `:4`. bash 3.2, without `compopt`,
       offers file names wherever cld offers nothing (see Findings); the user guide says so.
    4. Completion makes none of the startup checks (6): `new`, `resume`, `join`, `kill` and
       `list` call them, and there is no root `PersistentPreRunE`. `cld completion SHELL` works
       where neither tmux nor claude is installed, and `__complete` does not check tmux's
       version, which would cost a `tmux -V` on every TAB: with a tmux the check refuses it
       offers what that tmux lists, and `join` then refuses the tmux. So a TAB costs what
       `cld list` costs but for that `tmux -V`: a read of the socket directory and one
       `list-sessions` a socket (13.1; see Findings). It never runs claude, `claude --version`
       included, which `new` and `resume` alone run as they start claude: completing their
       arguments checks no claude either. It starts no server - `list-sessions` does not - and
       never opens the session list (14): it reads the sessions itself rather than run `list`,
       and a completion script runs it with stdout not a terminal anyway. With no server, no
       tmux, a tmux that fails or cannot run, or a socket directory it cannot read (13.1), it
       offers nothing and exits 0, and says what went wrong on stderr (`cobra.CompErrorln`),
       which the scripts discard.
    5. The help of `completion`'s commands is cobra's `Long`, which says where each script goes
       and what it needs; `completion` alone shows its own help, as with cobra. The `Short`s, and
       `completion`'s `Long`, are cld's, in the words of the other commands' help (12). They read
       their arguments as cld's other commands do, with cld's `-h` and `--help`, cld's messages
       and exit status 2: an unknown SHELL, an argument after it, an option after an argument and
       one they do not have are refused, where cobra would show the help and exit 0, fail with
       status 1, or read the option first. A typo such as `cld completion tcsh > FILE` would
       otherwise fill FILE with the help and succeed.
    6. What cobra prints - the help (12.5), the scripts, the answers to `__complete` - goes out
       through `output.Print`, so that a write that fails ends cld with status 1 and cld's message,
       as with cld's own output: cobra's help function and `__complete` drop the error and would
       exit 0, and the commands that print the scripts return it, in Go's words
       (`write /dev/stdout: ...`). When cobra has printed nothing - for `kill`, say - nothing is
       written, since even an empty write to a stdout that cannot take one fails. `__complete`
       without the word to complete, which the scripts always pass, is a mistake on the command
       line: status 2 and cld's message, where cobra fails with its own and status 1.
    7. The scripts ship only through `cld completion SHELL`, not as release assets or files that
       `make install` installs (see Distribution). Since 22, `cld setup completion SHELL` writes
       one where the shell reads it.
    8. `cld -<TAB>` offers `-h` and `--help`, the root's options, but not `-V` and `--version`,
       which only the first-argument check knows: making them the root's options would list them
       in the root's help beside `version`, which names them. An option is described by the first
       line of its usage, as cobra takes it, backquotes included (see Findings).

    Out of scope: a positional `cld join NAME`, completing claude's conversation names (16.4), and
    hints printed under the prompt (cobra's ActiveHelp).
18. Telemetry (#30): `cld setup telemetry [--local URL] [--remote URL] [--port PORT]
    [--collector-config FILE]` runs a local OpenTelemetry Collector in Docker, the container
    `cld-telemetry`, and points Claude Code's user settings at it. claude exports each signal to
    one endpoint, and can send the same signal to one place only; the collector sends traces,
    metrics and logs to `--local` - the JetBrains OpenTelemetry plugin in the IDE, say, whose spans
    for model requests, tool calls, MCP calls and hooks, per agent, show where a long multi-agent
    run spends its time - and metrics only to `--remote`, a team's collector, which keeps getting
    them while the IDE is closed: each exporter queues on its own, and the local one drops data
    after 30 s rather than retrying for 5 min. At least one of the two is needed; a URL is
    `http://HOST:PORT` (plaintext gRPC) or `https://HOST:PORT` (TLS), as
    `OTEL_EXPORTER_OTLP_ENDPOINT` takes it - `HOST` an IP address or a name whose labels start
    and end with a letter or digit, the last not all digits (`127.1` would be looked up as a
    name), `PORT` without a leading zero - and mistakes are usage errors (status 2). Settled
    with it:
    1. Linux only: the container runs with `--network host`, so that `127.0.0.1` in a URL is the
       host itself and reaches a receiver that listens on loopback only. On any other system cld
       fails with status 1 (`cld: setup telemetry works on Linux only`) before it looks at
       anything but `-h` and `--help`, which show the help. Not taken: on macOS, publishing the
       port and rewriting loopback URLs to `host.docker.internal`, which waits for a probe on
       Docker Desktop, where host networking is an opt-in;
    2. the port, on `127.0.0.1`: `--port` - refused when something else listens there, the
       running collector not counting, nor a paused one, which keeps its port (see Findings);
       else the running or paused collector's, from its label `cld.port`, so that claude sessions
       running keep sending to it; else, the collector not running, that port if nothing else
       holds it - after a reboot another program may, the port being in the ephemeral range; else
       one the kernel picks (listening on `127.0.0.1:0`). cld holds a port it checks with a
       listener, closed just before the collector starts. Not 4317, which another collector or
       Jaeger is likely to have. Never the port of a `--local` or `--remote` URL whose host leads
       to the collector's receiver on `127.0.0.1` - that address, `0.0.0.0` or `::`, which are
       dialled as the host itself, `localhost` or a name under it - since the collector would
       send what it receives to itself without end (see Findings): as `--port` it is a usage
       error, as the running collector's port a runtime one (status 1), which changes nothing,
       and a stopped collector's port, or one the kernel picks, is passed over. `::1` and
       `127.0.0.2` do not reach the receiver. Not taken: the first free port from 14317 upward;
    3. extra configuration is `--collector-config FILE` only: YAML the collector merges over
       cld's config (see Findings), whose parts have fixed names to refer to - the receiver
       `otlp`, the exporters `otlp_grpc/local` and `otlp_grpc/remote`, the pipelines - for auth
       headers, TLS, compression, timeouts, processors or more exporters. Not taken:
       `--remote-header KEY=VALUE`;
    4. the config reaches the container as copies in environment variables, read with
       `--config=env:CLD_TELEMETRY_CONFIG` and, for the file, `--config=env:CLD_TELEMETRY_EXTRA`.
       The container needs no file of the host, `validate` checks exactly what will run, and
       edits to the file apply when setup runs again; `docker inspect` shows them, secrets in the
       file included. cld hands them to `docker` in its environment (`-e NAME`), not on its
       command line, which every user of the host can list. The file must fit in a variable: cld
       refuses one with a NUL byte, and one longer than Linux takes a variable,
       `MAX_ARG_STRLEN` - 32 pages, the name and the NUL that ends it included: 131051 bytes of
       file with 4 KiB pages (see Findings) - with status 1, where `docker` could not run. Not
       taken: a bind-mounted file;
    5. the order catches a mistake before anything that runs is replaced: `docker` looked for (as
       `tmux` is), the settings read and parsed, the file read; the port; `docker run --rm IMAGE
       validate`, whose refusal changes nothing; `docker rm -f cld-telemetry`, then `docker run
       -d --name cld-telemetry --restart unless-stopped --network host --label cld.port=PORT` -
       `unless-stopped` so that the collector is back after a reboot or a restart of Docker,
       where claude's settings still send to it, and a `docker stop` stays stopped; up to 10 s
       for the collector to be ready: for its receiver to take connections on `127.0.0.1:PORT`,
       where claude will send. The collector starts its receivers last, as it logs `Everything is
       ready`, so the port tells as soon, and it tells what the log cannot: a
       `--collector-config` may keep that line out of the log, raising its level or sending it
       elsewhere, or move the receiver to another port, where the collector is ready but out of
       claude's reach (see Findings). Not taken: the line, with the port for a quiet log, which
       let the moved receiver pass; forcing the level to info with `--set`, which would have
       overridden a `debug` too. A container that stops or restarts first (see Findings), or
       takes no connections by then, leaves the settings as they were, and cld shows the last 20
       lines of its log, or says that it is empty, names the port of one that takes no
       connections, and says that the collector that ran before is gone; one that stopped or
       restarted is left for `docker logs`, but `docker update --restart no` stops Docker
       restarting it again and again, and at every boot, which cld says. Then the settings, read
       again - what claude or anything else wrote meanwhile stays - and a report of the port,
       where each signal goes and the keys changed. Running it again gives the same result;
    6. the settings: cld sets these keys in `env`, `CLAUDE_CODE_ENABLE_TELEMETRY=1`,
       `OTEL_METRICS_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` and, for the collector,
       `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:PORT`; with `--local` it also sets
       `OTEL_TRACES_EXPORTER` and `OTEL_LOGS_EXPORTER` to `otlp`, and to `1` both
       `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA`, which turns claude's traces on, and
       `OTEL_LOG_TOOL_DETAILS`, and it removes these four without `--local` - tool details put
       Bash commands and MCP server and tool names on events and spans, which only the local
       endpoint gets; and it always removes the six keys
       `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_{ENDPOINT,PROTOCOL}`, which would bypass the
       collector. Every other key stays where and as it was, and so does everything outside
       `env`; the file is replaced atomically, keeping its mode and indentation, and a symbolic
       link to it stays one. A missing file is created, and one that has the settings already is
       not written again;
    7. the collector config departs from the one #30 proposed where the pinned image required
       it (see Findings): the exporters are `otlp_grpc/local` and `otlp_grpc/remote`, `otlp`
       being a deprecated alias in 0.161.0 that every start warns about and a later image may
       drop; and the collector's own metrics are off, since with host networking their server
       takes `localhost:8888` and the collector exits when another collector has it;
    8. `setup` is cld's first command with commands of its own, as `completion` (17) is cobra's.
       `run` checks its first argument before cobra, as it checks cld's: `telemetry` - since 19,
       `project` too, and since 22 `completion`, followed by a shell - or `-h` or `--help`, setup's
       help; cobra would run `telemetry` for `cld setup
       --local URL telemetry`, taking `--local` for an option of setup's. `help setup telemetry`
       shows telemetry's help: `help`'s `COMMAND` may be followed by one of that command's own (see
       12.2), as with cobra's help command - so `help completion bash` now shows what `completion
       bash --help` shows (17.5), where `help` refused it. Error messages keep `(see cld help)`
       (12.3). The usage line, `cld setup telemetry [--local URL] [--remote URL] [flags]`, folds
       `--port` and `--collector-config` into `[flags]`: with them it would be 90 columns, past the
       80 of 12.1;
    9. completion (17) takes `setup` as it takes cld's other commands: `cld <TAB>` offers it with
       its `Short`, after `list` and before `completion`, which cobra adds after the commands cld
       adds; `cld setup <TAB>` offers `telemetry` (and since 19 `project` before it), and `cld setup
       telemetry --<TAB>` its options; `help <TAB>` offers `setup`, and `help setup <TAB>`
       `telemetry`, as `help` takes them (17.3). `run`'s check of the argument after `setup` applies
       where `setup` runs, not to `__complete setup ...`, and setup's checks are `setup telemetry`'s
       own - Linux in its `Args`, `docker` in its `RunE` - as the check of claude is `new`'s and
       `resume`'s (17.4), not a root hook's, which completion would run: completion runs no
       `docker`. Nothing completes the URLs, the port or `--collector-config`'s `FILE`, the one
       argument of cld's that is a file: no argument offers file names (17.3), a gap left for later;
    10. the image, `otel/opentelemetry-collector:0.161.0`, is pinned and bumped deliberately, as
        JediTerm is (7). Out of scope: other systems, a header option, turning it off (removing
        the container and cld's keys), overriding the image, and the plugin's port, which is set
        in the IDE.

19. Project settings: `cld setup project [--mcp SERVER]` sets claude up in the project in the
    current directory, as cld's own repository has it: `.claude/settings.json`, the settings the
    project shares through git - `$schema`, cld's `permissions.allow`, and `autoUpdatesChannel`,
    `plansDirectory`, `autoMemoryEnabled`, `theme` and `autoCompactEnabled` - an empty
    `.claude/settings.local.json`, holding its `$schema` alone, and `/.claude/*` and
    `!/.claude/settings.json` in `.gitignore`, so that git ignores what claude keeps in `.claude`
    but for the shared settings. `--mcp` adds MCP servers: `goland`, `jbcontext` and `rider`.
    Settled with it:
    1. the settings are those of this repository's `.claude/settings.json`: `--mcp goland` writes
       it byte for byte, and its `.mcp.json`, which the tests check, so a change to either goes
       with one to `internal/project`. They are Go data there - the allow list and the other keys
       - rather than an embedded copy of the file, so that each server's entries go where the
       file has goland's. They include the maintainer's tools (`dotnet`, `go`, `make`,
       `docker build`, `gh pr merge`) and `theme`, which are nobody else's defaults: a project
       edits them afterwards, and cld adds them back only when it runs there again, since it never
       removes anything (see 4);
    2. `--mcp SERVER`, given again or separated by commas (`--mcp goland,jbcontext`), takes
       `goland`, `jbcontext` or `rider`; any other name, the empty one included (`--mcp goland,`),
       is a usage error. Each server is written once, in that order, whatever the order given. A
       server is its entry in `.mcp.json`, its name in `enabledMcpjsonServers`, without which
       claude asks before it starts the server (see Findings), and `mcp__NAME` in
       `permissions.allow`, which Claude Code's docs have match every tool of the server, so that
       claude uses them without asking (not probed); `jbcontext` also
       allows `Bash(jbcontext:*)`, since its hooks and instructions have claude run
       `jbcontext search`. `goland` and `rider` are the servers built into the IDEs, over
       streamable HTTP at `http://127.0.0.1:${GOLAND_MCP_PORT:-64422}/stream` and
       `http://127.0.0.1:${RIDER_MCP_PORT:-64482}/stream`. `.mcp.json` is shared through git,
       and each developer's IDE has a port of its own: 64342 plus an offset per product by default,
       64422 for GoLand and 64482 for Rider in 2026.2, or another in its MCP Server settings (see
       Findings). So the port is a variable that claude expands as it reads the file, with the
       default after it (see Findings): a developer whose IDE listens elsewhere sets
       `GOLAND_MCP_PORT` or `RIDER_MCP_PORT` once for the machine - in the shell's profile, or in
       the `env` of `~/.claude/settings.json`; the project's `.claude/settings.local.json` does not
       work. An entry with the port written out, as cld wrote it first, differs, and is replaced
       (see 4). `jbcontext` is `jbcontext mcp` over stdio, looked up on the `PATH` by claude, as
       JetBrains Context's `jbcontext setup-agent` configures it for a user. Not taken: the ports
       written out, which only fit the IDEs of whoever ran cld; an option for them, which would
       write one developer's port for all; the IDEs' servers in the local scope, kept per user
       in `~/.claude.json`, which each developer would have to set up; the servers as arguments
       (`cld setup project goland`), which cld's commands take as options elsewhere too;
    3. the project is the current directory, where `cld new` starts claude, which reads its
       `.claude/settings.json` and `.mcp.json` there; it need not be a git repository, and in a
       subdirectory of one `.gitignore` is that directory's, its patterns anchored there;
    4. files that exist are edited in place, through `internal/configfile`, which `setup
       telemetry` uses for its settings (18.6): cld sets its keys - the last of a key given twice,
       which claude reads - where their values differ, however they are written (`1.0` is `1`, an
       object's members in any order); adds the entries `permissions.allow` and
       `enabledMcpjsonServers` lack, after theirs; replaces a server's entry in `.mcp.json` that
       differs, whole, since a merge would keep a stdio server's old `args` beside `mcp`, or a
       `command` beside a `url`; and keeps everything else: other keys, entries and servers,
       `permissions.deny`, their order, indentation and values, byte for byte, and the file's
       mode and a symbolic link to it. It removes nothing: a server given before stays when it is
       not given again. `.claude/settings.local.json`, someone's own, is only made, where nothing
       is - a symbolic link to no file counts as something. What is missing is created, `.claude`
       included, 0644 and 0755 less the umask. Running it again changes nothing;
    5. `.gitignore` gets `/.claude/*` and then `!/.claude/settings.json`, where they are not there
       as git reads them (see Findings): without the leading slash, which a pattern with a slash
       before its end does not need, and with a carriage return or trailing spaces, but not a tab;
       the exception counts only after the last line that ignores `.claude/*`, where it wins. What
       is missing goes at the end, in the file's line endings. `/.claude/*`, not `.claude/`: git
       does not look into a directory it ignores, so no exception would reach the settings;
    6. the order catches a mistake before anything changes: every file is read and edited first -
       one that is not valid JSON, whose `permissions` or `mcpServers` is no object, or whose
       `permissions.allow` or `enabledMcpjsonServers` is no array, stops cld with status 1, as does
       a `.gitignore` it cannot read - then each file that changes is written, in the order
       `.claude/settings.json`, `.claude/settings.local.json`, `.mcp.json`, `.gitignore`; a write
       that fails ends cld naming the files written before it. `.mcp.json` is read only with
       `--mcp`. The report has a line a file: created, updated with what changed - the keys, as
       `permissions.allow` or `mcpServers.rider`, or the lines added - or left as it was;
    7. in a git work tree cld then asks git whether it ignores `.claude/settings.json` all the
       same (`git check-ignore -v -z --stdin`): a `.claude/` or `.claude` elsewhere - before cld's
       lines, in `.git/info/exclude` or git's other excludes - keeps git out of the directory, and
       a `*.json` after them ignores the file again. It says so after the report, naming the
       pattern, its file and its line, and exits with status 1; it does not edit patterns it did
       not write. Without git, outside a work tree or when git fails, it does not ask. git is
       looked for as tmux is (11.5);
    8. it runs neither tmux nor claude, so neither check of 6 applies to it, and it works on macOS
       as on Linux. `run` takes `project` after `setup` as it takes `telemetry` (18.8), `help setup
       project` shows its help, and setup's `Short` names both. Completion offers `project`
       before `telemetry` after `setup`, and after `--mcp` the servers that start with what was
       typed, each described by its URL or command; after a comma, the servers the list does not
       have yet, the list before them.

    Out of scope: removing what cld wrote, other servers, finding the port an IDE listens on, and
    settings per kind of project.
20. An install script: the README installs cld with
    `curl -fsSL https://github.com/zadykian/cld/releases/latest/download/install.sh | sh`, in
    place of the two lines it had, which picked the binary from `uname` and downloaded it with
    curl straight into `~/.local/bin/cld`. They checked no checksum, knew `x86_64` and `aarch64`
    alone, wrote over cld as they downloaded it, and were two lines to copy. Settled with it:
    1. the script is a release asset, `install.sh`, which `make dist` copies beside the binaries,
       outside `cld.sha256`: the latest release's script goes with that release's binaries, and
       the release's checks run its tests before it is published. One read from `main` on
       `raw.githubusercontent.com` would work before the first release that has it, but could
       run ahead of the binaries it downloads. It installs the latest release
       (`releases/latest/download/NAME`, which GitHub redirects to the release's tag; see
       Findings), or the one `CLD_VERSION` names, as `X.Y.Z` or `vX.Y.Z`: 0.4.0 or later, since
       the earlier releases published a script, which the user guide installs by hand. Anything
       else is refused before a download;
    2. it is POSIX sh, so that `| sh` runs it under dash, bash, busybox or macOS's sh, and `| bash`
       as well. It needs `curl`, which fetched it, and `sha256sum` or `shasum`, checked before a
       download. Everything is in functions that the last line calls, so that a download cut
       short defines functions or fails to parse, and runs nothing, which the tests check at the
       end of every line;
    3. the binary is `cld-OS-ARCH` from `uname -s`, `Linux` or `Darwin`, and `uname -m`, `x86_64`
       or `amd64`, `aarch64` or `arm64`, and on macOS `arm64` also where `sysctl.proc_translated`
       is 1, a shell that Rosetta 2 translates, where the native binary runs too (Apple's
       documentation; not probed). Other systems and machines are refused, naming them;
    4. it downloads `cld.sha256` first, then the binary, into a temporary file in the directory it
       installs to, `CLD_INSTALL_DIR`, `~/.local/bin` by default, made where missing. The file is
       checked against its line in `cld.sha256`, made 0755, run with `--version`, and only then
       renamed to `cld`, replacing what is there, a symbolic link too, never what it points to.
       In that directory, rather than `$TMPDIR`, the rename replaces cld at once, a cld that runs
       keeps its old file, and a `/tmp` mounted `noexec` does not stop the run. Anything that fails
       ends it with status 1 and a message, `install.sh: ...`, after curl's or mkdir's own,
       leaving the directory as it was: the temporary file goes on exit, and on HUP, INT and TERM;
    5. it prints `installed cld X.Y.Z as DIR/cld`, DIR as `cd` and `pwd` spell it, absolute and
       without a trailing slash, and warns where DIR is not on the `PATH`, or where another `cld`
       comes first there. It edits no shell profile;
    6. `CLD_RELEASES_URL` stands in for `https://github.com/zadykian/cld/releases`, for the tests:
       they serve releases of scripts that print a version as cld does, so that every platform's
       binary runs, with a fake `uname`, and pipe the installer into `sh` and `bash`.

    Out of scope: wget, checking tmux and claude, which cld checks as it starts (see 6), editing
    shell profiles, and signatures: `cld.sha256` comes from the release the binary does, so it
    catches a download gone wrong, not a release replaced.
21. Self-update: `cld update` replaces cld with the latest release's binary, where that release is
    newer than the one cld runs as. Before it, upgrading meant running the install command again,
    which does not know where cld is unless told (`CLD_INSTALL_DIR`). Settled with it:
    1. the latest release is the tag that `releases/latest` redirects to, read without following
       the redirect (see Findings). GitHub's API, `repos/zadykian/cld/releases/latest`, says the
       same in JSON, but takes 60 calls an hour from an address without a token, which a shared
       address runs out of. Versions compare by their numbers, X.Y.Z, so 0.10.0 comes after
       0.4.0; where cld is that release, or newer, update says so, exits with status 0 and
       downloads nothing. A version that is no X.Y.Z - `dev`, which `go build` and `make install`
       stamp by default - is refused with status 1 before anything is asked: a build from source
       is neither compared with the releases nor replaced by one;
    2. it is Go, not a run of `install.sh`: cld knows the platform it was built for
       (`runtime.GOOS`, `runtime.GOARCH`) and the file it runs from, which the script would have
       to be told, and needs neither curl nor a shell. It downloads the files `install.sh` does,
       `cld.sha256` and `cld-OS-ARCH`, from the release's tag rather than `latest/download`, so
       that a release published meanwhile cannot mix the two; an amd64 cld under Rosetta 2 stays
       amd64;
    3. it replaces the file cld runs from, `os.Executable` with symbolic links resolved - what
       runs, where `install.sh` replaces what is at its own path. The new binary goes into a
       temporary file in that file's directory, is checked against its line in `cld.sha256`,
       takes the old file's permissions, and must print `cld X.Y.Z` for the release when run
       with `--version`; only then is it renamed over the old file. A cld that runs meanwhile, a
       list open on a terminal, say, keeps its old file;
    4. anything that fails ends it with status 1 and a message - the address and what it
       answered, or the connection's error - leaving cld as it was and the temporary file
       removed. SIGINT, SIGTERM and SIGHUP before the rename cancel the download and remove the
       file too, and end cld with 128 plus the signal's number, printing nothing, as a shell
       reports such an end; `fail.Status` carries it to main. After the rename they change
       nothing;
    5. it runs neither tmux nor claude, so it makes none of their checks (see 6), and runs where
       neither is installed. It reaches GitHub through Go's default transport, a proxy the
       environment names included, waiting 30 seconds at most for an answer to begin; redirects
       stay on HTTPS, as `install.sh`'s do. `CLD_RELEASES_URL` stands in for the releases'
       address, as for `install.sh`, for the tests;
    6. the tests build cld as release 0.4.0 once, copy it into the sandbox and run its update,
       with nothing on the `PATH`, against releases an HTTP server of theirs serves, whose
       binaries are scripts that print a version: the one for the host's platform replaces cld;
    7. since 22, once cld is replaced, update has the new cld print anew the completion scripts
       `setup completion` wrote, and writes those that differ; one it cannot write gets a warning,
       not a failure (22.5).

    Out of scope: an option to check without updating, updating to a given release (the install
    command takes `CLD_VERSION`), looking for a newer release as other commands run, and updating
    tmux or claude. cld 0.5.0 and earlier have no `update`: the user guide says to run the install
    command once more.

22. Setting completion up: `cld setup completion SHELL`, for `bash`, `zsh` or `fish`, writes the
    script that `cld completion SHELL` prints where the shell reads it, and `cld update` writes it
    anew. Before it, the README and the user guide gave lines to run by hand for each shell: for
    zsh a directory to make and two lines to add to `.zshrc`, for fish a directory that exists only
    once fish has started interactively (see Findings), and for none a way to keep the script in
    step with cobra's templates. Settled with it:
    1. a command of `setup`'s, with a command of its own for each shell, `setup completion zsh`,
       so that each has its help, and `help` and completion take them as they take setup's
       commands (12.2, 17.3). `run` checks the argument after `setup completion` as it checks the
       one after `setup` (18.8): a shell, or `-h` or `--help`, where cobra would run zsh's for
       `setup completion --help=false zsh`. A missing or unknown shell, `powershell` included, is a
       usage error. The script is cobra's, generated as `completion SHELL` generates it, with
       descriptions, so the file holds byte for byte what that prints;
    2. where each goes: bash's in bash-completion 2's user directory - `completions/cld` in the
       first directory of `$BASH_COMPLETION_USER_DIR`, or else in
       `${XDG_DATA_HOME:-~/.local/share}/bash-completion` - which it reads at the first TAB, before
       the system's directories; fish's in `${XDG_CONFIG_HOME:-~/.config}/fish/completions`, the
       first directory fish reads, where cobra's help and cld's docs had users write the script by
       hand, and where such a script would hide one in `~/.local/share/fish/vendor_completions.d`,
       the directory first thought of. zsh reads no directory of the user's (see Findings), so its
       script goes in `${XDG_DATA_HOME:-~/.local/share}/cld/zsh/_cld`, which lines at the end of
       `${ZDOTDIR:-~}/.zshrc` load. cld reads `ZDOTDIR` from its environment, not from `.zshenv`;
    3. the lines put that directory first on `$fpath` and register `_cld` for `cld` with
       `compdef`. They run `compinit`, which defines `compdef`, only where nothing before them has,
       since a second one drops the completions set up after the first, and with `-i`, which leaves
       out the directories `compaudit` finds insecure instead of asking as zsh starts, as it would
       where Homebrew's are group-writable. A `compinit` after them finds `_cld` on `$fpath` by its
       `#compdef` line. They do nothing where the script is missing, so that a `.zshrc` shared
       between machines stays quiet where cld set nothing up, and run no cld as zsh starts, unlike
       `source <(cld completion zsh)`, which needs cld on the `PATH` by then. They name the
       directory by the variables zsh reads, not by where cld wrote it, so that they hold on every
       machine; cld adds them once, and not again while `.zshrc` has their first line, wherever it
       is and whatever follows it;
    4. the files are written as `setup project` writes its own (19.4), with `internal/configfile`:
       whole, through a symbolic link, directories made, and only where they change. Both are read
       before either is written, so that a `.zshrc` cld cannot edit - a directory, a link that
       leads nowhere - leaves the script unwritten too. cld reports each file, created, updated or
       left as it was, and, where it wrote one, that a new shell takes it. It removes nothing, and
       edits no `~/.bashrc`: loading bash-completion there is Debian's and Ubuntu's default, and
       on macOS the user's to add, as Homebrew's caveats say;
    5. `cld update`, once it has replaced cld (21.3), runs the new cld's `completion SHELL` for
       each shell whose script is where `setup completion` writes it and starts as cobra's does,
       and writes what it prints where it differs, naming each script it wrote. The new release
       prints it because the script changes only with cobra's templates, which that release may
       have bumped. `completion SHELL` is a command every release has, so no release after this
       one has to keep another for update's sake; running its `setup completion` instead would also
       put back lines a user took out of `.zshrc`. A script without descriptions (one that runs
       `__completeNoDesc`) is printed again with `--no-descriptions`; a file that does not start
       as cobra's does is someone's own, and left. cld is updated by then, so a script it cannot
       write is no failure of the update's: cld warns on stderr - `cld: warning: cannot update the
       completion script for SHELL, FILE: WHY. Run cld setup completion SHELL manually` - goes on
       with the next shell's, and exits with status 0, as the maintainer asked. `output.Warn`
       writes the warning, cld's first. The refresh runs after update's handling of signals
       (21.4), so a signal then ends cld as it ends other commands, and can leave
       `internal/configfile`'s temporary file beside the script. `install.sh` writes no script;
    6. `cld setup completion`'s help ends with cobra's own line,
       `Use "cld setup completion [command] --help" for more information about a command.`, 81
       columns: the test of the help's width passes over that line of cobra's template, which
       names the command and holds no text of cld's to break (12.1);
    7. the tests write the scripts into the sandbox's home directory with nothing on the `PATH`,
       where the variables say, then start the shells the test image installs - bash with
       bash-completion 2, zsh and fish - to load them: bash-completion's own loader, zsh's
       `$_comps` and `$functions_source`, fish's `complete -C`. update's tests serve releases whose
       binaries print a script for `completion SHELL`.

    Out of scope: PowerShell, whose script `cld completion powershell` still prints;
    bash-completion 1's directory, which is in Homebrew's prefix, shared by every user; checking
    that `~/.bashrc` loads bash-completion; removing what cld wrote; and `install.sh` writing the
    scripts anew.

23. Joining beside other terminals: `cld join` attaches to the session beside any terminal attached
    to it already, and `cld join --detach-others` detaches those, as every `join` did before
    (`attach-session -d` since 0.2.0, the script's `new-session -AD` before it). Moving from one
    terminal to another works either way, but only a join that leaves the others attached lets two
    terminals show one claude - a laptop's and a desktop's, say - and a `cld join` typed in the
    wrong tab, or run from a script, no longer takes a session from whoever is on it. Settled with
    it:
    1. the option is long only, `--detach-others`, as the maintainer named it; `-d`, tmux's own
       letter, stays free. Its column in `join`'s help moves the usages to the 25th column, where
       the first line of `-n`'s, which `new`, `resume`, `join` and `kill` share, took 81 columns:
       it lost its "the" (12.1);
    2. tmux sizes claude's window to the terminal used last (`window-size latest`, tmux's default,
       which the private server keeps): claude redraws as the terminals take turns, and a larger
       terminal shows the rest of its screen dotted (see Findings). cld sets no `window-size`;
    3. the list's Enter joins as `cld join` does, beside the other terminals (14.5), and its footer
       no longer warns on an attached row but for the armed kill, which still ends that terminal
       (15.1). The list has no key for `--detach-others`: `C-q d` in the other terminal, or
       `cld join --detach-others` from the shell, takes a session over;
    4. `C-q d` detaches only the terminal it is typed in, and `cld kill` ends every terminal on the
       session, each with `[exited]` and status 0 (see Findings). The `pane-died` hint reaches the
       terminal used last, not every one on the window (5); a terminal that joins later gets it
       from `join`, as before. `list` shows such a session `attached`: STATE says whether a
       terminal is, not how many;
    5. the tests: a join beside a terminal, both attached, keys from the first reaching claude,
       `C-q d` in one leaving the others attached, and `--detach-others` detaching them all;
       join's tmux command with the option and without; the list joining an attached row and an
       exited one beside their terminals, with the same footer as on a detached row.

    Out of scope: a key in the list that joins as `--detach-others` does, a short option, and
    showing in `list` how many terminals are attached.
24. Names from the repository: a session is named `NAME-SUFFIX`, from `-n NAME` and `-s SUFFIX`,
    which `new`, `resume`, `join` and `kill` take together: `NAME` is by default the name of the
    git repository the current directory is in, or outside one of the directory, and `SUFFIX`, for
    `new`, the next index within `NAME`; `-w`'s worktree takes the session's whole name. Before,
    `-n` named the session whole and defaulted to `main` (3), so every `cld new` past the first -
    in another repository, often - needed a name typed, and a name said nothing of where its
    session belonged; a worktree was named `NAME`, and nothing in `.claude/worktrees` or the
    branches told cld's from claude's own. In the decisions before this one, and in
    `internal/session`, `NAME` is a session's whole name, all that tmux sees; below, that is `S`.
    Settled with it:
    1. without `-s`, `new` names the session `NAME-INDEX`: `INDEX` is 0 or, where sessions
       `NAME-INDEX` run, one above the highest `INDEX` among them. The maintainer asked for the
       highest plus one rather than the lowest free index: a gap stays, so the names keep the order
       they were given in. `new` reads the socket directory as `list` does (13.1) and asks only the
       servers of sockets `cld-NAME-DIGITS`, one after another: a server that has outlived its
       session counts, since `new` would refuse its name (13), and a stale socket does not. `NAME`
       is compared ignoring case, since a socket directory that ignores case reaches one server for
       both spellings (13.6). Only running sessions count: a name comes back once its session has
       ended, and its conversation then shares the name with the next (16), and the next `new -w`
       of that name reopens its worktree (24.7). Counting the conversations would mean reading
       claude's transcripts (16.4), and counting worktrees would tie the names to directories cld
       does not manage. Two `new` at once can take one name, and the second ends with tmux's
       `duplicate session: cld-S` (see 13's closing paragraph), as two `new -n NAME` did before;
    2. `-n` and `-s` go together, in either order: one way to name a session, whose two parts
       default where they can. The first version of this decision had them exclude each other,
       `-n` naming the session whole and `-s` after the repository's name; the maintainer asked for
       them together, for simplicity. Each is checked as a whole name was (1, 13.2), `-n` first,
       its length, whatever its characters, then its characters, with messages of its own for `-s`
       (`invalid suffix ' '`) - the empty one and one of spaces are invalid - since `SUFFIX` is the
       whole name where `NAME` leaves nothing (24.4); then the two together, which are refused
       where they make a name longer than 64 characters (`session name '...' is longer than 64
       characters; give a shorter -n NAME or -s SUFFIX`). All of these are mistakes on the command
       line, status 2, and come before any tool is looked for. A name made longer by the
       repository's or directory's name, or by the index, is refused the same way with status 1,
       as the command line alone does not decide it;
    3. `NAME`'s default is the name of the directory that holds the repository's common git
       directory, when that is `.git`, so that the subdirectories and linked worktrees of a
       repository - `.claude/worktrees` among them - share it; otherwise, as for a worktree of a
       bare repository or for a submodule, whose git directory is under the superproject's
       `.git/modules`, it is the git directory's own name, without `.git`. One `git rev-parse
       --is-inside-work-tree --git-common-dir` gives both, the directory as a path from the current
       one or a whole path (see Findings). The remote's name was the other way, but a repository
       without a remote would have none, and two clones of one remote would share their sessions'
       names. Outside a work tree - in `.git` itself too - where git is not on the `PATH`, and
       where it fails, the default is the name of the current directory as `os.Getwd` gives it:
       the shell's `PWD` where that is the current directory, so the name `pwd` shows, a symbolic
       link's rather than that of the directory it leads to. In `/root`, `cld new` makes
       `cld-root-0`; only `-w` needs git. The index alone, as this decision first had it outside a
       repository, was the other way: the maintainer asked for the directory's name, so that those
       names say where their sessions started too;
    4. the repository's or directory's name is made a NAME rather than refused, as decision 1
       refuses a name that is typed: each run of the characters a NAME cannot have becomes `-`, and
       `-` and `_` go from either end - `my.site` is `my-site`, `.dotfiles` `dotfiles`. Where
       nothing is left - the root directory, or a name in another script - `S` is `SUFFIX` alone.
       Refusing it would have failed every `cld new` in a repository such as `user.github.io`,
       which no option but `-n` could have helped. The name made is what `list`, the title and
       claude's `--name` show;
    5. `join` and `kill` need `-s` - `NAME` defaults as for `new`, but the next index names no
       session - and are refused without it (`join: missing -s SUFFIX (see cld list)`, status 2);
       `resume` needs `-s` or SESSION, with which its session gets the index `new` would give, as
       no conversation has the name of an index not given yet. The maintainer chose this over
       keeping `main`, which `new` no longer makes, over the only session of `NAME`, and over its
       highest index: `cld list` picks a session without its name (14). The usage lines name
       `-s SUFFIX` unbracketed where it is needed, `join [-n NAME] -s SUFFIX [--detach-others]`,
       and the test of the help's text takes a plain word starting with `-` for an option, and the
       word after it for its value (12.4). `-s`'s column moves the usages of `new`, `resume` and
       `kill` to the 25th, where `join`'s were (23.1). From a directory other than its own, a
       session takes `-n` as well; one whose name has no `-`, made where `NAME` leaves nothing,
       takes `-s` alone only in such a directory, `/` say, or `cld list`, whose Enter and Ctrl+X
       take every session - those of 0.7.1 and earlier, `main` among them, too;
    6. cld's messages name a session as `join` and `kill` take it (`session.Options`): its name
       split at its last `-`, `-n` what comes before and `-s` what follows, where both are NAMEs,
       and else `-s` alone - `session 'api-fix' exists; attach to it with cld join -n api -s fix`.
       So does the `pane-died` hint (5), which named the session through its window,
       `cld kill -n #{window_name}`: cld writes the options into the hook as it makes the session,
       and `join` into its own `display-message`, rather than have tmux take the window's name
       apart;
    7. `new -w` gives claude `--worktree cld-S`: the worktree `.claude/worktrees/cld-S` on the
       branch `worktree-cld-S` (4), named as its session and apart from the worktrees claude names
       itself - those of `claude --worktree` without a name, or of its subagents - as the maintainer
       asked. The worktree of an earlier `new -n NAME -w`, `NAME`, stays; `resume` takes its
       conversation back there, as claude does whatever the worktree's name (16.3);
    8. completion: `join -n` offers, for the names `list` shows, what comes before their last `-`,
       where that and what follows are NAMEs, once each, described by the number of its sessions;
       `join -s` offers the SUFFIX of the sessions whose names start with `NAME-` - `-n`'s, or the
       repository's or directory's - where `join` takes it, described by their states; where
       `NAME` leaves nothing, every name. A TAB there runs `git rev-parse` besides `list`'s reads
       (17.4). `new`, `resume` and `kill` offer none, as their `-n` did not (17.3);
    9. the tests: the default names in a repository, outside one and in the root directory, with
       running sessions, a gap, a server without its session, a stale socket and the names of
       another repository or directory, of none and in other letters, and with `-n`; `resume
       SESSION` named alike; the repository's name from its root, a subdirectory, a linked
       worktree, a bare repository's worktree and a submodule, the directory's name outside one and
       in `.git`, a symbolic link's with `PWD` and its target's without, and names made a NAME; `-n`
       and `-s` alone, together and in either order; the option errors, the missing `-s` and names
       made too long; the messages and the hint; completion; `-w`'s worktree name. The sandbox's
       work directory is named `_`, which leaves nothing, so that the tests that are not about
       names name their sessions exactly with `-s`, as they did with `-n`.

    Out of scope: filling gaps, counting what outlives a session - its conversation, its worktree
    - and completing `kill`'s options.
25. The tab's title follows claude's status: `✳ cld-NAME`, and `◐` and `◑` in turn in place of
    `✳` while claude is busy - the markers claude's own title has outside tmux; under tmux it keeps
    them at `✳` (see Findings). claude tells tmux through hooks that `new` and `resume` give it with
    `--settings`, and tmux sets the title of every terminal on the session from that. Settled with
    it:
    1. the status comes from claude's hooks, which claude documents and 2.1.232 already has, so
       the minimum of 6 stands. Rejected: reading claude's status from
       `~/.claude/sessions/PID.json`, which would catch interrupts too but which claude documents
       nowhere, and which would take a process per session to watch it; telling it from the
       pane's output, which typing and redraws make too; passing claude's title on, which never
       turns under tmux; and taking `TMUX` away from claude, which needs it for its passthrough;
    2. the events: `UserPromptSubmit`, `PostToolUse` and `ElicitationResult` make claude busy -
       `PostToolUse` also after a permission answered - `PermissionRequest` and `Elicitation` make
       it wait, and `Stop`, `StopFailure`, `Notification` of the type `idle_prompt` and
       `PostToolUseFailure` with `is_interrupt` make it idle; a failure that is no interrupt
       leaves it busy. An interrupt as claude writes has no event: the title stays busy until the
       next prompt, or until claude, idle for a minute (its default), notifies `idle_prompt`.
       Waiting shows `✳`, as claude's title does;
    3. a hook runs tmux, by the path cld checked, quoted for sh, on claude's server by its socket
       and for claude's session by name, both written in as the session is made - `tmux -S
       SOCKET if -F -t =cld-NAME: ... "set -t =cld-NAME: ..."` - and sets `@cld-status` on that
       session only where it changes: setting any option redraws every terminal on the server,
       and `PostToolUse` comes with every tool. claude does not always run a hook in its pane: a
       conversation it runs in the background runs them without `TMUX` and `TMUX_PANE` (see
       Findings), where `-t "$TMUX_PANE"` on the server `TMUX` names, as cld 0.8.0 had it,
       reached the default server - and failed there, or would have set the option on a session
       of it. The `set` names the session too: without `-t` tmux takes the session used last,
       which may be one claude made. The socket goes as an absolute path, since the hook runs in
       claude's directory. It prints nothing, since what a `UserPromptSubmit` hook prints goes to
       the model; a tmux that fails says so, which claude shows the user;
    4. `set-titles` on, `set-titles-string`
       `#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} cld-NAME`, the marker
       `@cld-busy` and the path `@cld-tmux` go on claude's session, not the server, as 5's options
       go on claude's window: a session claude makes keeps tmux's. A claude that exited is not
       busy, though a turn it failed in left the option so. The title leaves claude's own, `#T`,
       out, so that C1 still holds;
    5. the turning: `@cld-busy`, expanded with strftime, is `◐` in even seconds and `◑` in odd
       ones, followed by a `#()` job that in the background, a second later, runs
       `refresh-client -S` for the terminal the title was expanded for, which expands it anew,
       job and all. A second rather than claude's 960 ms: tmux runs the job again only in another
       second, and a refresh within the same one would end the turning. The job names tmux by
       `#{q:@cld-tmux}`, since a path in the format could hold a `#` or `%`, which formats and
       strftime take up, or a `)`, which ends the job. No job runs for an idle claude or a session
       no terminal is on, and the last one ends within a second of the turn;
    6. cld still prints `✳ cld-NAME` before tmux starts, and the list before it hands the terminal
       over (14), which tmux replaces on attach with the session's title as it stands. A terminal
       that detaches keeps the title tmux set last - a busy marker, if claude was busy - and a
       session an older cld started keeps `✳ cld-NAME`, as it has neither the hooks nor the title;
    7. the tests: the probe runs the hooks of its `--settings` as claude would (`hook EVENT
       JSON`), and fails a test on one that fails or prints anything; the events one by one, and
       the status each leaves; C1 with `✳`, then `◐` and `◑` in turn and `✳` again, and `✳` for a
       claude that fails in a turn, on each terminal; the title's options on claude's session and
       not the server's; the settings and the tmux command word for word, with the tmux cld found;
       the hooks run without `TMUX` and `TMUX_PANE` (the probe's `unsetenv`), as a conversation in
       the background runs them, beside a session claude made and one on the default server, which
       get neither option; the socket's absolute path under a relative `TMUX_TMPDIR`. The probe
       draws its settings as `{...}`, which with the hooks no longer fit a line.

    Out of scope: a marker for `waiting` of its own (claude's title has none), a title that shows
    more than the marker, and the title a terminal keeps after it detaches.
26. The tab marks a worktree: while claude works in a linked git worktree - one `new -w` has
    claude make, one it enters with `EnterWorktree`, one `cld new` runs in - the title ends in
    ` [w]`, as in `✳ cld-NAME [w]`, and loses it as claude leaves. It follows claude as 25's
    status does. Settled with it:
    1. the hooks of 25 keep `@cld-worktree` on claude's session, `1` or `0`, and the title adds
       `#{?@cld-worktree, [w],}` after the name. The hooks are `SessionStart` and `CwdChanged`:
       claude runs a hook in its directory, and sets that directory - firing `CwdChanged` - as
       `--worktree` starts it, as it enters or leaves a worktree, and as a resumed conversation
       takes it back to one (see Findings). Whether `--worktree` moves claude before
       `SessionStart` or after it, one of the two runs in the worktree;
    2. a hook asks git in its own directory: a linked worktree's `--git-dir` is not its
       `--git-common-dir` (with `--path-format=absolute`, git 2.31 and newer); the main worktree's
       is, and outside a repository both are empty. `CwdChanged`'s `new_cwd` is not taken: it
       names where claude's shell went, which claude takes back without an event when it leaves
       the worktree a session works in;
    3. git goes by the path cld finds in the PATH's absolute entries, as tmux does: the hook runs
       in claude's directory, where a relative entry would find a git of the project's own. Where
       cld finds no git, the two hooks are left out, and the title never says `[w]`. What the hook
       prints goes to /dev/null: `SessionStart`'s output goes to the model. The settings are
       encoded without HTML's escapes, so that `2>/dev/null` reads so in claude's arguments;
    4. rather than follow claude, `[w]` could have stood for the `-w` a session started with -
       fixed, and with no hooks - but it would miss `EnterWorktree`, `cld new` in a worktree and
       a resumed worktree conversation (the maintainer chose to follow claude);
    5. the tests: the hooks as claude starts in the main worktree, a linked one and its
       subdirectory, and as it moves among them, a directory outside the repository and `.git`;
       no hooks for the worktree where cld finds no git; C1 with ` [w]` idle, busy and for a
       claude that failed, on each terminal.

    Out of scope: `[w]` for a shell `cd` into a worktree that the session's directory does not
    follow, and saying which worktree.

27. Completion in bash with ble.sh: where cld offers no file names, bash with ble.sh offers none
    either. ble.sh, a line editor that runs in bash in readline's place, completes through the
    script `setup completion bash` writes, but offered the directory's file names wherever cld
    offers nothing, against 17.3 (see Findings); the maintainer uses it. Settled with it:
    1. `completion bash` prints cobra's script with five lines of cld's at the end of
       `__start_cld`, the function that completes `cld`, and `setup completion bash` writes the
       same (22.1): where the directive has `ShellCompDirectiveNoFileComp` and `compopt` is a
       function in a shell ble.sh runs in (`$BLE_VERSION` set), `compopt +o default +o
       ble/default` - what cobra's lines do in bash, which they leave undone under ble.sh, and
       ble.sh's own completions off. In bash, and in bash with ble.sh loaded but not attached, the
       lines do nothing. The zsh and fish scripts, and `completion powershell`, are cobra's as
       they were;
    2. cld inserts them before the `}` that ends `__start_cld`, the one place where
       `__cld_process_completion_results` is followed by it, and panics where cobra's template
       no longer has that: a bump of cobra that changes it fails every test that prints the
       script. The script still starts as cobra's does, so `update` writes it anew where the
       new release prints another (22.5), with the lines; one without descriptions gets them too;
    3. other ways: the fix in ble.sh's adapter for cobra's script, which would help every cobra
       program but leave cld's users waiting on a ble.sh release, and does not keep `-o default`
       from offering file names; a bash script of cld's own, one more copy to keep in step with
       cobra's (17.1); lines in `~/.bashrc`, which cld edits none of (22.4);
    4. not done: `--mcp=SERVER`, `-n=NAME` and `--name=NAME`, which ble.sh completed with file
       names while descriptions are on, and with the lines completes with nothing (see
       Findings) - its adapter's to fix; the user guide says to write `-n NAME`. ble.sh 0.3
       offers file names wherever cld offers nothing, and cannot be told not to;
    5. the tests: `TestSetupCompletionBashBleSh` types lines into bash in a tmux pane, with
       bash-completion and ble.sh loaded by `~/.bashrc` as Ubuntu's `~/.bashrc` and ble.sh's
       instructions load them, and one TAB after each: a command, an option, `join -n`'s NAME and
       `-s`'s SUFFIX, and nothing for four arguments where the file in the directory would
       complete; with several, the commands' descriptions on screen. TAB runs a widget that runs
       ble.sh's `complete`, then writes its status and the command line to a file, which the test
       waits for: ble.sh cancels a completion when a key comes, so no key can follow TAB before it
       is done, and a completion it cancelled (status 148), which would leave the line as it was,
       fails the test. Keys typed before ble.sh draws its prompt are lost, so the test waits for
       it, and it runs `new-session` itself, with the cld under test first on the PATH, which
       tmux gives the pane from the client (see Findings). The sandbox's home has a `~/.cache`,
       as a user's does, where ble.sh keeps its cache: without one it writes beside itself, which
       only root may, and as CI's user it did not load (see Findings). It runs where ble.sh is
       installed, and skips elsewhere - the Debian image and macOS among them.
       `make docker-blesh-check` builds the test image on Ubuntu 26.04 with its package `ble.sh`
       and runs the completion tests there, with `CLD_BLESH` naming ble.sh, so that the test
       fails where it would skip; CI's `blesh` job runs them the same way.

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
- A stale socket that the real tmux reads is a socket nothing listens on (`staleSocket` in the
  tests), not a plain file (`socket`, which only the fake tmux reads). tmux says no server is
  running for either on Linux, whose `connect` refuses a connection to a file that is no socket
  as it refuses one to a socket nothing listens on; macOS's reports the file as no socket
  (`ENOTSOCK`), and tmux fails with that, so `cld new` without `-s` ended there on the macOS
  runner, before its session (#51). A file of that name that is no socket is nothing tmux or cld
  makes, and cld reports tmux's error for it, as `list` does (13.1).
- The Go port (decision 11) is `cmd/cld`, the command line, and `internal/session`, the tmux
  side, whose package comment is the script's header comment. Errors carry an exit status up to
  `main` (`internal/fail`), the only place that exits; `new`, `resume` and `join` end in
  `syscall.Exec` of tmux, and a tmux command that fails (`tmux -V`, the kill) ends cld with its
  status after its own message; one that cannot run ends it with 127 or 126, as `syscall.Exec`
  failing does.
  A session lookup (`list-sessions`) that fails ends cld with status 1 and what tmux said, or
  the same `cannot run` message, as the script's `die 1` did. How cld finds a program on the
  `PATH` (11.5) and ends when it cannot run one (11.9) is `internal/tool`, which `setup
  telemetry` shares for `docker` (decision 18).
  cld's own output goes through `output.Print` (`internal/output`), which turns a failed write
  into an error, so that every value `internal/fail` makes is one of cld's ends, never its
  output. What cobra prints - the help, the completion scripts and the answers to `__complete`
  (decision 17) - goes to a buffer, the root's output, which `run` then prints with
  `output.Print`: cobra's help function and `__complete` drop the error of a write that fails.
  Reading the sessions is one function returning rows, which `list` lays out and completion
  filters.
- cobra's defaults give way to cld's command line (cobra 1.10.2, pflag 1.0.9):
  - the first argument is checked before cobra sees it: cobra takes an unknown command for an
    argument of the root, and skips options before the command (`cld -n x new` would run
    `new`). `completion`, `__complete` and `__completeNoDesc` pass (decision 17), and a bare
    `__complete` is refused there, where cobra's `Args` would refuse it with its own message. The
    argument after `setup` is checked the same way (decision 18.8) where `setup` runs, and the one
    after `setup completion` (decision 22.1), and not after `__complete`, which completes them;
  - `SilenceErrors` and `SilenceUsage`: cobra would print `Error: MESSAGE` and the usage;
  - `SetInterspersed(false)` on every command: pflag reads options up to the first argument,
    where it would pass over arguments and read every option first (`cld join a -x` would name
    `-x`). Each command's `Args` refuses that argument - `resume`'s takes it as SESSION and
    refuses the next (see 16) - and a `--` (`ArgsLenAtDash`), which pflag would drop;
  - a `FlagErrorFunc` turns pflag's typed errors (`NotExistError`, `ValueRequiredError`,
    `InvalidValueError`, `InvalidSyntaxError`) into cld's messages, and shows the help when `-h`
    or `--help` came before the error: cobra looks at `-h` only once every option has parsed;
  - `SetHelpCommand` replaces cobra's `help [command]` with cld's `help [COMMAND]`, whose `Args`
    refuses what cobra's passes over (decision 12) and takes, as cobra's does, a command's own
    command after it (`help setup telemetry`, 18.8), and whose `ValidArgsFunction` completes
    both as cobra's does (17.3); `helpTopic` reads them for the two. A help function set on the
    root, which every command inherits, `completion`'s included, renders cobra's default help
    into the root's output: it is cobra's own help function, taken from the root before cld sets
    its own. Each command has `-h` and `--help` of its own (`helpOption`), worded as cobra words
    its own ("help for new"); `cobra.EnableCommandSorting` is off, and the help function moves
    `version` back after `help`. `tests/testdata/help` holds the help of the root and of each
    command, which `TestHelpText` compares byte for byte and `-update` rewrites;
  - `Version` stays unset, so there is no `--version` or `-v` flag: `version` is a command, and
    `-V` and `--version` its aliases;
  - cld makes cobra's `completion` command itself (`InitDefaultCompletionCmd`, which `Execute`
    then leaves alone) to give it and its commands `-h` and `--help` (`helpOption`), `Args` and
    a `FlagErrorFunc` of cld's, `SetInterspersed(false)`, and `Short`s in cld's words.
    `completion` gets a `RunE` that returns `pflag.ErrHelp`: cobra shows the help of a command
    it cannot run before it looks at the arguments. The root's output is set first, since each
    shell's command writes its script to the output the root had when the command was made;
  - `CompletionOptions.SetDefaultShellCompDirective(ShellCompDirectiveNoFileComp)`, which cobra
    reports on stderr (`Completion ended with directive: ...`), and the last line of
    `__complete`'s answer, the directive, is `:4` where cobra wrote `:0` all the same
    (decision 17.3);
  - `join`'s `name` option has a completion function (`RegisterFlagCompletionFunc`), which
    reads the sessions as `list` does, with `Tmux.Sessions`, on a `Tmux` from `session.Find`:
    tmux found on the `PATH`, and no other check. Walking the sockets needs only tmux's path.
- Go has no `ttyname` on Linux or macOS without cgo: cld runs `tty` with its own stdin, as the
  script's `$(tty)` did, to compare its terminal with the live panes', and drops the newline
  after the name, which uutils' `tty` (0.8.0, Ubuntu 26.04) does not print.
- claude's `--settings` are marshalled from a struct, its fields in the order the script wrote
  them, which gives the same bytes.
- A server per session (decision 13): `lookup` reports whether a session's server runs and whether
  the session is on it, and `new`, `resume`, `join` and `kill` need both, to refuse a server that
  outlives its session. `noServer` takes three of tmux's messages for no server: `no server running on` (a stale
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
  it join's checks and kill's steps (`listSource`). A `fail.Error` keeps the advice for the
  command line (`Advice`, such as ` (see cld help)`) apart from its `Message`: `main` prints both,
  the list's footer the message. The list's lookups and its kill run tmux with the terminal, in
  raw mode, as their stdin; `list-sessions` and the kill's `kill-session` and `kill-server` leave
  its mode alone (`TestListJoin`'s gone case and `TestListKill`'s kill case read it with
  `stty -a`).
- The baseline terminal types the list's keys by name with `send-keys`, which sends, on tmux
  3.7c: `Up` ESC [ A, `Down` ESC [ B, `Escape` 0x1b, `Enter` 0x0d, `C-c` 0x03 and `C-x` 0x18, and
  after the program in the pane has sent CSI ?1h (application cursor keys) ESC O A and ESC O B for
  `Up` and `Down`. The JediTerm driver types `C-x` as any Ctrl+letter: a key-pressed event of `X`
  with Ctrl, the control character as its key char.
- No test holds the two seconds back with a key that has begun (15.1): that matters only to an Esc
  typed in their last 100 ms, the wait for a lone Esc, which a test cannot time from outside cld
  without flaking under load. `TestListKill`'s timeout case pins the two seconds between two and
  four. A key read late (15.1) is a check where the two seconds' timer fires: with a byte waiting to
  be read, the kill stays armed, with no timer, for the key the byte begins. `TestListKill`'s read
  late case stops cld (SIGSTOP) within the two seconds, types the key, and has cld go on (SIGCONT)
  once they are over and the key has reached the terminal - the test sees it readable, as cld does -
  so that the key and the timer are there to take together: an Esc three times, then a Ctrl+X, which
  kills. Without the check, cld took the timer first about half the time, and the case failed 4 runs
  out of 4. Where cld stopped two seconds after the first Ctrl+X or later, the timer may have fired
  first: the round is tried again, and the third time the case is skipped.
- The wait after a kill (15.1) goes by a timer, which each Ctrl+X starts again. When it fires with a
  byte waiting to be read - cld held up while the key went on repeating - it starts again, and the
  byte, read next, ends it or starts it again: a late read cannot end the wait early and let the
  repeats queued behind it arm and kill. No test holds cld up so. `TestListKill`'s held down case
  types the second Ctrl+X held down (`Hold`, timed by the terminal): again half a second later, then
  20 a second for a second, past the second after the kill; it checks that one session was killed,
  and that another key ends the wait at once, and a second and a half without Ctrl+X too. Two Ctrl+X
  that reach cld a second apart are two presses: where typing them took 400 ms or more beyond their
  waits, time that may have come between two of them, the case cannot tell and is skipped. Typed
  with `Keys`, a tmux client started for each, the repeats came some 5 a second natively, and under
  heavy load two of them came a second apart now and then, and killed the next session too.
- The tests that check something between the two presses of Ctrl+X do so in `armThen`: when a
  second has gone by since the first press, it disarms the kill with a letter, if it is still
  armed, and presses Ctrl+X and then the second key with nothing between them. Under heavy load
  the checks took over two seconds, and the second press only armed the kill again.
- `Modes` tells whether the cursor is visible: in the baseline terminal the outer pane's
  `#{cursor_flag}`, 1, and 0 after CSI ?25l (tmux 3.7c); in JediTerm what its display's
  `setCursorVisible` last received - the emulator's `CursorVisible` mode (DECTCEM) calls it -
  starting as visible.
- The JediTerm driver types `Up` and `Down` as key-pressed events of `VK_UP` and `VK_DOWN`, which
  JediTerm's encoder turns into ANSI or application cursor sequences as the program asked, and
  `Escape` as a key-pressed `VK_ESCAPE` with the key char ESC: the encoder has no code for Escape
  (jediterm-core 3.76, read with `javap`), and JediTerm's key processing passes a pressed key
  without one on when its key char is a control character, as it does for a Ctrl+letter.
- The JediTerm driver's emulator takes in the pty's output on a thread of its own, a piece at a
  time, while the driver answers the tests from what it has taken in so far. `running` stays true
  until that thread has read the pty to the end, which it goes on doing after pty4j has reported
  the process gone (see Findings): once `Running` is false, the screen and the modes are final,
  and C7 and C9 read them once. Modes that change while the program runs are waited for
  (`waitModes`), and so is mouse reporting before the wheel (`WheelUp`: the driver's `wheel-up`
  sends nothing while it is off), as tmux turns every mouse mode off and on again after it draws
  (see Findings). Both races showed only under load; a sleep in the emulator thread whenever mouse
  reporting goes off, and before each read once the process has exited, made them fail every time.
- `TestMain` unsets every `GIT_*` variable before it builds or runs anything, and `gitInit` runs
  git in the sandbox's environment, as cld runs: the tests run from `git rebase --exec` in a
  linked worktree set `core.bare = true` in the repository's config (see Findings).
  `TestGitVariables` sets `GIT_DIR` itself, to a git directory named other than `.git`, and checks
  that `gitInit` leaves it alone.
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
  program, the session leader.
- Tests that need to know the list has taken a key that changes nothing on the screen count its
  frames in the output log: each starts with `CSI 1;1H`. The list draws once it has taken the keys
  that have come, so that two keys `Keys` types 50 ms apart, a tmux client each, draw one frame
  when cld, held up, has not taken the first by the time the second comes: tmux writes them to the
  pane in two writes, but cld then reads both before it draws (see Findings). In suites run five
  at once on 8 CPUs, four in Docker and one natively, `TestListKill`'s keys during the kill case
  got two frames for the list and its two Ctrl+X, and waited in vain for a third, once in ten; in
  thirty more, `TestListJoin`'s frame-a-key case got two for the list and its two arrows, once. So
  a test that counts frames types a key once the frame before it is in the log, and pastes keys
  meant to come together (`Paste`), which tmux writes in one write: the keys during the kill case
  pastes the two Ctrl+X that start the kill, which draw one frame whatever the load, and the
  frame-a-key case types its second arrow once the frame for the first is in the log; thirty
  suites run as above passed. Typed with `Keys`, the second Ctrl+X also came over two seconds after
  the first under heavier load natively, where Ubuntu's tmux snap is slow to start a client, and
  only armed the kill again: typing the two took 3 to 5 s, and the old steps failed 7 runs in 96,
  sixteen test processes at once beside 24 busy loops, where the pasted case passed all 96. The
  old case failed 8 runs in 9 with cld stopped (SIGSTOP) until both Ctrl+X had reached the
  terminal - in the ninth, the frame cld draws as it goes on (SIGCONT) made up the count - and 7
  in 7 with the two pasted, as two arrows pasted failed the old frame-a-key case 7 in 7.
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
- `resume` (decision 16) is a cobra command whose `Tmux.Resume` shares `Tmux.New`'s code: both
  call one function, `create`, that makes the session on its own server, and differ in claude's
  arguments only, so what `new` passes later reaches `resume` too, apart from the worktree. The
  contract tests (C1-C10) run through `new`, and cover `resume` with it.
- `setup telemetry` (decision 18) is `internal/telemetry`. The settings edit needs no new
  dependency: `json.Decoder`'s tokens read the top-level object and `env` into lists of keys and
  raw values in the file's order, and the file is written back from them, the values byte for
  byte, one member a line in the indentation of its first member - so a file Claude Code wrote
  comes back as it was but for cld's keys. The collector config is YAML written by hand, its
  strings quoted as JSON strings are, which YAML reads the same (`[::1]:4317` would otherwise be
  a list). docker's failures are told apart by what it prints and its status: `No such
  container` from `inspect` means none, and `docker run` exits with 125 when docker fails and
  with the collector's status otherwise. The file's side of the edit - reading and writing it,
  its mode and a symbolic link kept, and a JSON file's members, with an object or array that
  changes written one member or element a line in the file's indentation - is
  `internal/configfile`, and only the env keys are `internal/telemetry`'s, so that other commands
  can edit claude's files the same way: `setup project` (19) does, adding its values indented
  with `json.Indent`.
- `setup project` (decision 19) is `internal/project`. Its tests run the real git, which checks
  what the `.gitignore` cld writes does (`git status --ignored`), and compare what `--mcp goland`
  writes with the repository's own `.claude/settings.json` and `.mcp.json`, which they read from
  the directory above `tests`. The write that fails, of `.gitignore` after the settings, comes
  from a symbolic link to a file whose path is 4095 bytes long, as for `setup telemetry`.
- The tests fake docker with the probe, which TestMain links as `docker` next to `claude`, so
  every sandbox finds it before the real one. It records each call, with its environment, in
  `docker.jsonl`, keeps the one container's state in a file (`inspect`, `rm -f` and `run -d` read
  and change it), and records whether the port of the label was free as `run -d` came, which pins
  that cld lets go of it in time. A container that `run -d` starts running takes connections on
  that port, as the collector's receiver: the probe starts itself as one, in a session of its own
  and with none of the pipes cld reads docker's output from, and it listens until `rm -f` removes
  the file it keeps - `rm -f` returns once the port is free, as Docker's does - or the test
  removes the sandbox. Environment variables give the container's state before, the state a
  container starts in and its restart count, whether it takes connections, the log, a call that
  fails, and a file that `run -d` writes, as claude might while cld waits. `update` only answers.
  The wait of 10 s for a collector that never takes connections is waited out in full by two
  tests, one of them with the ready line in its log. Where the test holds the port as that of the
  collector that ran before, which cld does not check, the new collector's receiver cannot have
  it, and the test's listener takes the connection. A write of the settings that fails once the
  collector runs comes from a `CLAUDE_CONFIG_DIR` whose `settings.json` is 4095 bytes long, the
  most Linux takes: the temporary file beside it has a longer name. A rename that fails after the
  temporary file is written has no such cause, so the removal of that file is not tested. The
  limit on a `--collector-config` is checked against the kernel: the longest file passes to
  docker, and a program run with a variable one byte longer fails with `E2BIG`.
- The ports the telemetry tests give cld with `--port`, or hold as another program's or a running
  collector's, come from outside the kernel's ephemeral range (`testPort`): above it, else below
  it from 1024. A port that a listener on port 0 had and let go, as the tests took them before, is
  the kernel's pick again for any listener on port 0 or connection in the network namespace (see
  Findings): another test's, or the cld of one where cld chooses the port, took it now and then
  before the test's own cld listened there, which then found it in use. Outside the range only a
  program that asks for the port by number takes it; the tests of a process get the ports in
  turn, no two the same one, from a random start, passing over one that something listens on -
  given the same port, two tests of a process had failed too. For the same reason the fake
  docker's receiver holds the port of a container that takes no connections, bound without
  listening: cld waits 10 s on that port, which it chose, and another test's cld could choose it
  too, and take the connections. What is left is cld's own: in the few milliseconds between cld
  letting go of a port it chose, or `rm -f` freeing the running collector's, and `run -d`, another
  test's cld choosing its port can get that one, as any program can outside the tests. In the
  range of 200 ports in Findings that failed 2 tests in 20 runs; the default range has 141 times
  as many.

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
terminal=pycharm, no answer)` - the leak `new` and `resume` guard against by leaving
`TERMINAL_EMULATOR` out of the environment they run tmux with. From a clean environment the
real claude 2.1.281 put the pane in key mode `Ext 2`, and Shift+Enter inserted a newline (checked
by hand in a nested tmux).

## Status

- Baseline and JediTerm contracts run on tmux 3.7c (Linux, Docker, built from source), and the
  baseline on Homebrew's tmux on macOS. Until the minimum rose to 3.7 (see 6), they also ran on
  3.3a, 3.4 and 3.5a.
- `setup telemetry` is tested against a fake docker on Linux, and on macOS only for its refusal;
  it was checked by hand against the real collector image (see Findings), with collectors of the
  debug exporter in the plugin's place: the plugin itself, and claude sending through the
  collector, are still to check.
- `setup project` was checked by hand with git 2.53.0 and `claude mcp list` 2.1.283 (see
  Findings), the ports from their variables included; a claude session calling the servers' tools,
  and whether a session, rather than `claude mcp list`, takes the variables from the project's
  `.claude/settings.local.json`, were not.
- `install.sh` is tested against releases the tests serve, under the `sh` and `bash` of the Linux
  image and of the macOS runner, and was run by hand against the release v0.4.0 on Linux (see
  Findings). Rosetta 2's `sysctl.proc_translated` was not probed on a Mac.
- `cld update` is tested against releases the tests serve, on Linux and macOS, and was run by
  hand against GitHub, from 0.4.0 to 0.5.0 on Linux (see Findings). An update on macOS, and one
  from the first release that has `update`, are still to check.
- `setup completion` is tested with bash 5.2.37 and bash-completion 2.16, zsh 5.9 and fish 4.0.2
  in the Linux image, and with the macOS runner's own zsh; the runner has neither fish nor
  bash-completion, so those tests skip there. Homebrew's `bash-completion@2`, and `compinit -i`
  where Homebrew's directories are group-writable, were not checked on a Mac, nor an update from
  a release that has `setup completion` to one that prints other scripts.
- `new -w`'s worktree `cld-NAME-SUFFIX` (24.7) was not run with the real `claude`: its name has the
  characters of the `wt` probed with 2.1.281 (see Findings). The repository's name is tested with
  git 2.47.3 in the Linux image, and with the git of the macOS runner.
- The title's hooks were run by the real `claude` 2.1.283 only as far as a prompt that a hook
  blocked (see Findings), which costs no call to the API. A real turn - `Stop` at its end,
  `PermissionRequest` and `PostToolUse` around a permission, `PostToolUseFailure` with
  `is_interrupt` for `Esc` in a tool, `idle_prompt` a minute after an interrupt as claude writes -
  was not run: each takes a prompt to the API, so the maintainer runs or allows it. The title in
  iTerm2, and the empty OSC 7 tmux sends it, were not seen. Of the worktree's hooks (26),
  `SessionStart` in a linked worktree and `CwdChanged` for bash mode's `cd` were seen with the
  real claude; `--worktree`, `EnterWorktree`, `ExitWorktree` and a resumed worktree conversation
  were read in its bundle, not run. The hooks of a conversation claude 2.1.284 ran in the
  background were seen failing as cld 0.8.1 wrote them (see Findings); the hooks as they are now
  were run by hand without `TMUX` and `TMUX_PANE`, not by a real background worker. What makes
  claude run a conversation in the background, and whether its worker outlives `cld kill` - its
  hooks would then say that no server runs - were not checked.
- iTerm2 is not automated: every level beyond "launch only" needs permissions on the runner -
  controlling iTerm2 over AppleScript or its Python API (with authentication switched off), and
  posting synthetic key events (Accessibility). That is a decision for the maintainer, not
  something the test setup should grant itself.
- `resume`'s probes with the real `claude` (#26: claude 2.1.282 on tmux 3.7c) are not run: each
  starts or resumes conversations, and the Remote Control ones connect to claude.ai, so the
  maintainer runs or allows them. They are: `claude --name X --resume X` and `--resume ID`,
  whether claude accepts them and which name the conversation keeps; `--resume cld-NAME` with one,
  several (after `/clear`, and after repeated `cld new -n NAME`) and no conversations of that
  name, in a git repository and outside one, with what claude shows and its exit status;
  `--resume cld-rev` with only `cld-review` stored and with both; leaving the launch picker with
  Esc, and its exit status; another project's conversation picked after `Ctrl+A`; a
  `cld new -n wt -w` conversation resumed from the main checkout and from a subdirectory, and
  what `cld list` shows; the directory another project's conversation resumes in by ID;
  `/resume cld-NAME` inside `cld new -n NAME`; and whether a session that turned Remote Control on
  at startup, as cld's do, records its Remote Control session in the conversation. Until then the
  user guide and decision 16 go by Claude Code's docs, and by claude's `--help` and bundle (see
  Findings), and the user guide says which of it is not checked.
