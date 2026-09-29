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
| a tmux client like those of `cld new`, `resume`, `join` and the list's Enter where `LC_ALL`, `LC_CTYPE` and `LANG` do not name UTF-8 and `TMUX` is unset (tmux 3.7c; a stub claude printing `UTF8TEST: ✳ ⏺ café │ end` on a private server, the client in a pane of an outer tmux under `env -u LANG -u LC_ALL -u LC_CTYPE`, and under `LANG=C`) | the client was no UTF-8 one (`#{client_utf8}` 0), and the outer pane showed `UTF8TEST: _ _ caf_ x end`: `_` for each character that is not ASCII, and `│` as `x` between SO and SI, in the terminal's line-drawing set. With `tmux -u` the client was UTF-8 (1) and the line arrived as it is. The title, a `set-titles-string` of `✳ cld-x`, reached the outer pane as it is either way. claude 2.1.284's bundle, searched for `LC_ALL`, `LC_CTYPE` and `LANG`, reads them only to format times and in the environment it gives the commands it runs (read, not run): nothing there picks its glyphs by the locale |
| `cld` inside another tmux (`$TMUX` set) | nesting works: the private socket is a different server, and tmux refuses a client with `$TMUX` set only when its tty has the name of one of the server's own panes - but see the next row |
| a dead pane's pty (tmux 3.3a to 3.7c) | tmux closes it but keeps its name (`#{pane_tty}`), and the system hands the name to the next pty opened. A client with `$TMUX` set on that pty - a pane of another tmux - is refused with `sessions should be nested with care, unset $TMUX to force`: tmux compares the client's tty with every pane's, dead or alive. An empty `$TMUX` skips the check; set, even empty, it still makes the client take the terminal for UTF-8 whatever the locale says |
| a bare `tmux new-session -d -s cld-x` run inside claude's pane (tmux 3.3a to 3.7c) | the pane's `TMUX` names cld's socket, so `cld-x` lands on cld's server, as with `tmux -L cld` by hand; until cld marked its sessions, `list`, `join`, `kill` and `new` took it for one of theirs |
| `new-session ... \; set -F -t =NAME: @cld '#{session_id}' \; set -w -t =NAME: remain-on-exit failed ...` (tmux 3.3a to 3.7c) | what follows `new-session` takes effect before tmux sees the new pane's program exit, however soon: the mark is there as the session is, and the window's `remain-on-exit` and `pane-died` hook keep and report a pane whose program exits at once. When `new-session` fails (`duplicate session`) tmux skips the rest, so the other session stays unmarked. `set -t =NAME`, like any command that takes a pane, finds nothing: `=NAME:` names the session |
| `remain-on-exit failed`, an empty `remain-on-exit-format` and cld's `pane-died` hook in `new-session`'s command list, on claude's window (`set -w`, `set-hook -w`) and on its pane (`-p`), a terminal attached from an outer tmux, then `split-window -d -t =NAME: 'sleep 1; exit 5'` (#78; tmux 3.7c, in the image `tests/Dockerfile` builds) | on the window, the split pane took the window's options: it stayed dead with status 5, and the hook wrote `claude exited with status 5: ...` to the terminal while claude ran. On the pane - `=NAME:`, the session's one pane then - `show -pv` reads `failed` and `show-hooks -p` the hook, the window has none of them (`show -w` prints nothing), and a new pane takes the window's: the split pane closed, with no hint, leaving claude's. claude's pane exiting 3 then stayed dead with the hint on the terminal, and `attach-session` with `if -F '#{pane_dead}'` in the same command list showed it again; a program that exits at once (`false`) stayed dead, as the row above has it on the window, and a session made on that server whose program exits 4 closed |
| `#{@cld}` in a format (tmux 3.3a, 3.7c) | tmux looks a user option up in the server's options, then the pane's, the window's and the global window options, and only then the session's and the global session options: a `@cld 1` set with `-s`, `-g` or `-w` counted for sessions that had none, and a window's `@cld 0` hid a session that had one. Compared with the session's id, a flag set anywhere makes no session cld's; one on the server or a window still hides one |
| a tmux server named `cld-NAME` that cld did not start - `tmux -L cld-x new -s other`, `tmux -L cld-y new -s cld-y`, and a pane of `tmux -L cld-outer new -s outer` running cld (tmux 3.7c; cld 0.8.2, and as #69 left it, in the tests that 34 added) | cld took each for one of its servers. In the pane of `cld-outer`, `new` was refused with `this terminal is a pane of the tmux server of session 'outer'; detach with C-q d first`, where `C-q` is not bound, and `list` printed its table. `new`, `resume`, `join` and `kill -s x` said that session `x` had ended and pointed at `tmux -L cld-x kill-server`, which ends the user's sessions - as #69 left it, `new`, `resume` and `join` pointed at `cld kill -s x`, and `kill -s x` ended the user's server, printing nothing; `list` showed `y`, completion offered it, `new -s y` said it existed, and `kill -s y` ended the user's server |
| `#{@cld}`, `#{prefix}` and the mark of 34 in the formats cld runs - `lookup`'s `list-sessions` filter, `lingering`'s format, `kill`'s `if -F` and `OwnPane`'s `list-panes -a` - on servers of a private socket directory (tmux 3.7c: the snap, and for `display-message` and `if -F` the Docker image of `tests/Dockerfile`) | on a server started with `set -s @cld 1`, `#{@cld}` read `1` for every session, one made later too, and a window's or a session's `@cld 0` hid it from none; with tmux's defaults it read empty. `#{prefix}`, a session option, read `C-q` on a server started with `set -g prefix C-q`, as cld 0.8.2 and earlier start theirs, and `C-b` with tmux's defaults; a session that set its own `C-b` read that. The mark, `@cld` or else the prefix `C-q`, read `1` on both kinds of cld's server - but for that session on the unmarked one - and `0` on the other. There the filter left session `cld-NAME` out, and `list-sessions` printed nothing, with status 0; `list-panes` printed an empty line for each pane. `display-message -p` read the mark on a server without a session too: `1` with `@cld` or the global prefix `C-q`, `0` with neither. Beside the format of a server that has outlived its session (13) it read `1 1` on a marked server that had, where `if -F` ran its command, and `0 0` once `set -su @cld` had taken the mark away, where it ran the other |
| `tmux -V` beside a server started by an older tmux (a 3.5a server from Debian's package with a 3.7c client built from source, in the image that `tests/Dockerfile` built with `BASE=debian:trixie TMUX_VERSION=3.7c` before #21, which installed Debian's tmux 3.5a beside the source build; for #21 also a 3.6 server on the default socket with a 3.7c client) | `tmux -V` reports the client: `tmux 3.7c`. The server keeps running the tmux that started it - `#{version}` read `3.5a`, and `3.6` - and answers the newer client: the 3.7c client made a session on the 3.5a server with `new-session`, and set `remain-on-exit failed` on its window. So a check of `tmux -V` passes after an upgrade while the sessions on the old server - on one shared server, as before decision 13, the new ones too - run on it until it exits |
| what cld needs of tmux (#74): the whole suite, tmux and JediTerm terminals, in the image `tests/Dockerfile` builds with `TMUX_VERSION=3.5a`, `3.6a` and `3.5` - the last with the minimum lowered to 3.5 for the run - and tmux's `CHANGES` up to 3.7c and its commits from 3.5 to 3.5a, read | 3.5a and 3.6a pass every test, once the baseline terminal passes `paste-buffer -S` to 3.7 and newer only: with it, four tests that paste fail on 3.5a (`command paste-buffer: unknown flag -S`). 3.5 passes every test but the version checks. The probe written up in #74 found the same on 3.6b, and on 3.4 the failures of `TestClaudeFailingDetached` and `TestListJoin/exited` besides the crash above, and read the options and commands cld runs as all in 3.2 or 3.3, extended keys' mode 2 as 3.5 |
| keys with Shift through extended keys, typed as CSI u - `CSI 65;2u`, `CSI 127;2u`, `CSI 13;2u` - into a client of a server set up as cld's (`extended-keys on`, `extkeys` for `xterm*`), whose pane asked for modifyOtherKeys mode 2 (tmux 3.5 and 3.5a, in the images `tests/Dockerfile` builds) | 3.5 hands the pane Shift+A as `CSI 27;2;65~` and Shift+Backspace as `CSI 27;2;1106324~`, Backspace taken for a Unicode character; 3.5a hands it `A` and `CSI 27;2;127~` (its commits "Report shifted keys like S-A as A" and "Do not translate BSpace as Unicode"). Shift+Enter arrives as `CSI 27;2;13~` from both, which is why the contract (C3) passes on 3.5 |
| the shell of `run-shell` and `#()` jobs, with `default-shell` fish 4.0.2 (tmux 3.5 and 3.5a, in the same images) | 3.5 ran `run-shell 'echo "[$version]"'` with fish, which printed `[4.0.2]`; 3.5a ran it with `/bin/sh`, which printed `[]` (its `CHANGES`: "Revert to using /bin/sh for #() and run-shell and if-shell"). fish refuses the title's job, `(sleep 1; ...) >/dev/null 2>&1 &`: `command substitutions not allowed in command position`, status 127 |
| DECRQM 2026, synchronized output, typed in a shell pane (tmux 3.7c natively and tmux 3.5a and 3.7c in the images `tests/Dockerfile` builds), and claude 2.1.284's bundle, read, not run | 3.7c answers `CSI ? 2026 ; 2 $ y` (supported, reset), 3.5a nothing; tmux's `CHANGES` has the answer, and synchronized output for applications, in 3.7. Where `TMUX` is set, claude takes synchronized output for unlikely ("tmux answers for itself"); once the terminal answers XTVERSION, as tmux does, it asks DECRQM 2026 and takes it for supported on a status of 1 to 3, and logs `no reply to DECRQM 2026` otherwise. So claude draws with it on tmux 3.7 and newer only |
| `paste-buffer` of `a`, 0x01, `b`, ESC, `c` into a pane that reads raw bytes (tmux 3.5a and 3.7c, in the same images) | 3.7c writes `a^Ab^[c` without `-S` and the bytes as they are with it; 3.5a writes them as they are, and refuses `-S` with `command paste-buffer: unknown flag -S` |
| how `claude` 2.1.282 resolves `remoteControlAtStartup` (read from its bundle, not run: a live check would connect the session to claude.ai) | the first of the policy settings, the `--settings` (flag) settings and the user settings that has it wins, over the old global-config key; a `false` in the project's `.claude/settings.json` or `settings.local.json` beats all of them, and a `true` there is ignored with a warning. `/config`'s "Enable Remote Control for all sessions" writes the user setting, so `--settings` overrides it either way |
| what the claude minimum rests on (#21): Claude Code's changelog, and the linux-x64 npm bundles of 2.1.118, 2.1.119, 2.1.133, 2.1.221 and 2.1.222, read for the issue, not run | `--worktree` came in 2.1.49, `-n`/`--name` in 2.1.76 and the `worktree.baseRef` setting in 2.1.133 (changelog). `remoteControlAtStartup` moved into the settings in 2.1.119, with `/config`'s other settings ("now persist to `~/.claude/settings.json`", changelog): 2.1.118 reads it from the global config (`~/.claude.json`) only, which `--settings` does not reach. 2.1.133 and 2.1.221 decide from the merged settings, then the global config, and in the merge flag settings outrank the project's and the local ones, so cld's `true` beats a project's `false`. 2.1.222 returns `false` first when the project or local settings have it, then takes the first of the policy, flag and user settings, as the row above records for 2.1.282; its changelog agrees: repo-local settings "can no longer turn it on (they can still turn it off)". On 25 September 2026 npm's `stable` tag was at 2.1.274 and `latest` at 2.1.282; for the issue, Homebrew's default `claude-code` cask and the apt, dnf and apk `stable` repositories served 2.1.274 too |
| how tmux starts a command given as several words, such as `new-session -c DIR claude ...` (tmux 3.7c, glibc 2.43, Ubuntu 26.04) | with `execvp`, in `DIR`, and with the `PATH` of the client that ran `new-session`, also for a second session on a server that a client with another `PATH` started. So a bare `claude` is looked up in every entry, relative ones included, from `DIR`: with `PATH=.:/abs`, or `:/abs`, and a `claude` in both, tmux started the one in `DIR`, where cld had checked `/abs/claude`. A script without `#!`, which the system will not execute (`ENOEXEC`), `execvp` ran with `/bin/sh`, by name and by path |
| `new-session -c DIR` with a `DIR` its user cannot enter, and a binary for another machine as the command (tmux 3.7c, glibc 2.41, in the image `tests/Dockerfile` builds on `debian:trixie`; the first as `nobody`) | tmux started the command in the home directory, printing nothing, and `new-session` exited 0: for a `DIR` of mode `000`, and for one inside a directory of mode `000`. An arm64 ELF binary on x86_64, which the system will not execute (`ENOEXEC`), `execvp` ran with `/bin/sh` all the same, as a script: a `/bin/sh` that logged its arguments recorded `sh PATH --version`, and the pane died with dash's status 2 |
| `install.sh` against the release v0.4.0 on GitHub (curl 8.18.0; dash 0.5.12, bash 5.3.9 and busybox's sh on Ubuntu 26.04) | `releases/latest/download/cld.sha256` redirected (302) to `releases/download/v0.4.0/cld.sha256`, and that to `release-assets.githubusercontent.com`, over HTTPS both; a file the release lacks answered 404. Under each shell the script installed `cld 0.4.0`, whose checksum matched the line `sha256sum` wrote in `cld.sha256` on the release runner, `HASH  cld-OS-ARCH`; `CLD_VERSION=9.9.9` ended at curl's 404 for `cld.sha256` |
| `https://github.com/zadykian/cld/releases/latest`, v0.5.0 the latest release (curl 8.18.0, GET and HEAD) | 302 to `https://github.com/zadykian/cld/releases/tag/v0.5.0`, alike for both; `releases/download/v9.9.9/cld.sha256`, of no release, and the `releases/latest` of a repository that does not exist answered 404. cld built with `-X main.version=0.4.0`, run through a symbolic link, updated itself to 0.5.0 from there, replacing the file the link led to; that 0.5.0 knows no `update` |
| `claude --version` (2.1.282, native installer, Linux) | prints `2.1.282 (Claude Code)` and exits 0, in about 20 ms. It leaves nothing running that holds its output: piped to `cat`, it returns as soon. In a directory that has since been removed it prints `error: The current working directory was deleted, so that command didn't work. Please cd into a different directory and try again.` on stderr and exits 1 |
| `claude --help` of 2.1.282 on resuming | `-r, --resume [value]`: "Resume a conversation by session ID, or open interactive picker with optional search term"; `-n, --name <name>`: "Set a display name for this session (shown in the prompt box, /resume picker, and terminal title)"; `--fork-session`: "When resuming, create a new session ID instead of reusing the original". The help says nothing of resuming by name, which Claude Code's docs describe, and names no restriction on giving `--name` with `--resume`. `--resume`'s value is optional (`[value]`): by the rule of commander, whose `.option()` calls the bundle holds, a word starting with `-` after it is read as the next option, not as its value (not run) |
| how `claude` 2.1.282 resumes (read from its bundle, not run) | `--resume ID` with no conversation for the ID prints `No conversation found with session ID: ID` and exits 1. A conversation that runs as a background session (`claude --bg`) is refused, naming `claude attach` and `claude stop`, unless `--fork-session` is given; one open in an interactive claude is not. When Remote Control starts and another process on the machine holds the conversation's Remote Control session, claude leaves Remote Control off with a notice that starts `Remote Control not started here · another Claude Code on this machine ... already has Remote Control for this conversation` and ends `run /remote-control to move it to this terminal`. Not found in the bundle: whether a session that connected at startup, as cld's do, records its Remote Control session in the conversation, and how `remoteControlAtStartup` on the command line combines with a recorded one |
| how `claude` 2.1.283 moves a conversation to its background sessions (read from its bundle, not run, and again from 2.1.284's, which has the same strings; what cld then shows, the refused `cld resume`, and `←` then `cld kill` seen in sessions of cld's with the real claude 2.1.283 on tmux 3.7c, as #70 reports; the transcripts' `continued-in` entries read in claude's history; Claude Code's agent-view docs read; the hooks' tmux command run against a private tmux 3.7c server) | agent view, on unless `disableAgentView` or `CLAUDE_CODE_DISABLE_AGENT_VIEW=1` turns it off ("Disable agent view (`claude agents`, `--bg`, /background, the on-demand daemon)"), moves a conversation out of an interactive claude three ways: `/background` (`/bg`, "Send this session to the background and free the terminal"); "Move to background and exit" in the dialog `/exit` shows while background work runs, beside "Exit and stop tasks" and "Stay"; and `←` on an empty prompt, while `/config`'s `← opens agents` (`leftArrowOpensAgents`, on by default) is on. claude's daemon runs the conversation on as a copy, with a new session ID and the same name - which Claude Code's agent-view docs say it numbers, as `NAME (2)`, where a background session on the list has it already; the old transcript gets a `continued-in` entry with the copy's ID, and `/resume` passes it over (`filtered from /resume: continued in ID`). `/bg` and the dialog exit claude with status 0: under `remain-on-exit failed` the pane, the session and its server went, the terminal showed `[exited]` and `cld list` nothing. After `←` claude stays in the pane, in agent view; `cld kill` ends it, and the copy goes on. The docs say that `Esc` there returns to the conversation, and `Enter` or `→` on a row attaches to it, and that detaching (`←`, `Ctrl+Z`, `/exit`, `Ctrl+C` or `Ctrl+D` twice) never stops a background session (not run: `Esc`, `Enter` and `→`; `←` was seen). `--resume` by the name then finds the copy, which claude refuses while a live background process holds it, unless `--fork-session` is given: ``Session UUID is running as a background session (ID). Run `claude attach ID` to open it, or `claude stop ID` first to resume it here. Add --fork-session to branch off a copy instead.``, without a job ID ``Run `claude agents` to find its id, then ...``, status 1, which leaves a session of cld's `exited`; `/resume` inside claude refuses it without the last sentence. `claude stop ID`: "Stop a background session. Its conversation is kept: `claude attach <id>` opens it again, `claude --resume` works once it is stopped" in the command's description, and "... kept; resume it later with `claude attach <id>`." in its usage; `claude attach ID`: "Open the background session in this terminal". A worker runs with the `--settings` of the claude in the pane (see the hooks of a conversation claude runs in the background, below). A tmux 3.7c server whose last session ended - its program exited with status 0 under `remain-on-exit failed` - leaves its socket, and the hooks' `tmux -S SOCKET if -F -t =cld-NAME: ...` then prints `no server running on SOCKET`, status 1; once a new server on that socket has a session `cld-NAME`, as a later `cld new` of the name starts one, the same command exits 0 and sets the option on that session |
| an argv word that ends in `;` (tmux's `cmd_parse_from_arguments`, read in the 3.3a and 3.7c sources; run on 3.7c, and on 3.3a, 3.4, 3.5a and 3.7c by `TestResume` and `TestDirectoryTmuxWouldChange`) | ends the tmux command, the text before the `;` staying an argument: `a;` reaches the program as `a`, and the next word starts a new tmux command. A word ending in `\;` becomes the text with `;` - `a\;` arrives as `a;`, `a\\;` as `a\;` - and a `;` elsewhere in a word is left alone. The words of a command given to `new-session` are not format-expanded: `#{session_name}` arrives as it is |
| a `#` in `new-session`'s `-c` (tmux 3.3a, 3.4, 3.5a and 3.7c: plain tmux, and `cld new` and `cld resume` before and after the fix, by hand in Docker; `TestDirectoryTmuxWouldChange`) | tmux expands `-c` as a format, after splitting its command at `;`, and `#{session_path}` keeps the result: `/tmp/w/C#S` became `/tmp/w/C` (`#S` is empty then: the session does not exist yet), and `/tmp/w/x#(touch ran)` became `/tmp/w/x` while tmux ran `touch ran` through the shell in the client's directory (with `new-session -d`, 3.3a to 3.5a; with an attached client, as cld's, all four). A `-c` that names no directory starts the program in the home directory, and with 3.3a where the server started. So `cld new`, and `resume`, in such a directory started claude elsewhere, and in one named `x#(command)` ran command. `/tmp/w/C##S` gives `/tmp/w/C#S`: `##` is a `#` |
| the environment the bash script handed tmux with `exec env -u TERMINAL_EMULATOR tmux ...` and `exec tmux ...` (bash 5.3.9 and 3.2.57, recorded by the fake tmux, and by `printenv` in its place under `set -euo pipefail`), and claude's in the pane of a server that `cld new` started (tmux 3.7c) | bash exported `PWD` set to the working directory, whatever `PWD` it got; `SHLVL=0` when it got none, and a `SHLVL` it got unchanged; and no `_`, not even one it got: once the script has run a command, bash no longer exports it. It dropped an exported `PS1` and `PS2`; `OLDPWD`, which an interactive bash exports after a `cd` - 3.2.57 always, 5.3.9 when it names no directory; and `RANDOM`, `PPID`, `COMP_WORDBREAKS`, `HISTCMD` and `BASH_VERSINFO`, with 5.3.9 also `SRANDOM`, `BASHPID` and `BASH_ARGV0`, and 3.2.57 `LINENO`. Its own variables that came in exported left with its values: `IFS` (space, tab, newline), `OPTIND=1`, `OPTERR=1`, `BASH`, `BASH_VERSION` and `SHELLOPTS`, with the script's `errexit`, `nounset` and `pipefail` added - a bash that reads it turns them on - and with 5.3.9 also `BASHOPTS`, `LINENO`, `PS4`, `EPOCHSECONDS` and `EPOCHREALTIME`; Debian's 5.2.15 dropped and rewrote the same variables as 5.3.9. Exported functions (`BASH_FUNC_NAME%%`) left in bash's own layout; any other variable passed as it came. The Go cld hands on the environment it got, apart from `TERMINAL_EMULATOR` (since 33, also the other variables that name the terminal to claude) and `TMUX`. tmux sets a pane's `PWD` from `-c`, so claude sees the same `PWD` either way; the rest comes from the server's environment, that of the cld that started the server: no `SHLVL` where claude saw `SHLVL=0`, that cld's `_` - a shell sets it to the path of the command it runs - where claude saw none, and each of the others as that cld got it |
| the script's name check and `list`'s columns under `en_US.UTF-8`, `C.UTF-8` and `C` (bash 5.3.9, glibc 2.43; bash 3.2 on macOS not checked) | `[[ $name =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]` follows the locale's collation: under `en_US.UTF-8` it matched `é`, `ñ`, `ß`, `Ä`, `ǅ`, `①` and `٣`, so `cld new -n café` made `cld-café`, which `tmux -L cld kill-session -t =cld-café` ends (tmux 3.7c); under `C.UTF-8` and `C` it matched ASCII only. `${#name}` counts characters under a UTF-8 locale and bytes under `C`; `printf '%-*s'` pads by bytes under all three, so `é` took three columns of a four-column NAME |
| where bash itself stepped in for the script (bash 5.3.9 on Ubuntu 26.04 unless noted; tmux 3.7c) | a write to stdout that failed (`/dev/full`, or a descriptor open for reading) ended the script under `set -e` with status 1 and bash's message (`printf: write error: No space left on device`, `cat: -: ...` for the usage), and `new` and `join` did not get as far as their `exec` of tmux; `printf` and `cat` failed the same way under 5.2.15 and 3.2.57. A write to a pipe whose reader had gone ended it by SIGPIPE (the usage with status 141, `cat`'s, under `set -e`), and when it was started with SIGPIPE ignored - from a script under `trap '' PIPE`, say - with status 1 and `printf: write error: Broken pipe` (`cat: -: Broken pipe` for the usage). A tmux that could not run at all ended it with 127 when there was no such file, and 126 for a file the system refuses (`Exec format error`); for a `#!` naming a missing interpreter, 127 with Debian's 5.2.15 and 5.2.37 and Ubuntu's 5.2.21, 126 with 5.3.9 and 3.2.57 as released. A text file without `#!` bash ran as a script. A tmux on the `PATH` without the execute permission, with no executable one on it, bash found all the same - its search takes the first file of that name where none is executable, for `command -v` too - and ran, ending with `Permission denied` and 126; a `claude` like that let `new` go on and hand it to tmux, and a `git` like that made `new -w` say the directory was in no git repository. A tmux that stopped being runnable once it had answered `tmux -V` ended the script from a session lookup (`list-sessions`) with its `die 1`, status 1 and bash's message, and from `kill-session` or the `exec` with 127 or 126. With `PATH` unset bash searched a default path built into it, which differs by build: `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` (Ubuntu's 5.2.21 and 5.3.9), `/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin:.` (Debian's 5.2.15 and 5.2.37), `/usr/gnu/bin:/usr/local/bin:/bin:/usr/bin:.` (3.2.57 as released); macOS's `/bin/bash` was not checked. Started in a directory since removed, bash warned `shell-init: error retrieving current directory: ...` and kept the `PWD` it got: `new` passed that path to tmux with `-c`, and tmux started claude in the home directory (with Debian's 5.2.37); `new -w` said the directory was in no git repository |
| the help cobra 1.10.2 generates with its default templates (pflag 1.0.9), from a test program with cld's commands | commands are listed sorted by name unless `cobra.EnableCommandSorting` is false; then in the order they were added, but for the help command - cobra's or one set with `SetHelpCommand` - which `Execute` moves after all the others as it runs (`InitDefaultHelpCmd`). A flag's value shows as its type (`--name string`) unless its usage names it in backquotes. A command with flags gets ` [flags]` at the end of its usage line, after any argument in its `Use`, unless `Use` has `[flags]` already or `DisableFlagsInUseLine` is set. Nothing is wrapped; a newline in a flag's usage goes on under the usage's column. The help function writes to stdout and drops a write that fails: `help`, and `new --help`, with stdout on `/dev/full` or open for reading only print nothing, on stderr either, and exit 0. `help nope` prints ``Unknown help topic [`nope`]`` and the root's usage on stderr and exits 0; `help new join` shows `new`'s help |
| `TMUX_TMPDIR` under a deep directory | `error connecting to ... (File name too long)`: the socket path hits the ~108-byte `sun_path` limit, so test sandboxes need short socket directories. The path `TMUX_TMPDIR/tmux-UID/cld-NAME` has to stay within 107 bytes: with a NAME of 64 characters it overflows from a `TMUX_TMPDIR` of 32 characters as UID 0, and of 29 with a four-digit UID (tmux 3.3a, 3.4, 3.5a, 3.7c, as UID 0 and 1000) |
| a pane's environment on one server shared by two sessions and on a server per session: the server started from a shell with `FOO=first`, the second session created from one with `FOO=second`, `VIRTUAL_ENV`, a longer `PATH` and another `SSH_AUTH_SOCK` (tmux 3.3a, 3.4, 3.5a, 3.7c; `sh -c 'env > FILE; sleep 600'` in claude's place) | on the shared server the second pane got `FOO=first` and no `VIRTUAL_ENV`: only `PATH`, which tmux takes from the client that runs the command, and the `update-environment` variables (`SSH_AUTH_SOCK`, `DISPLAY`, ...) came from the shell that created the session. On servers of their own each pane got exactly the environment of the shell that created it |
| a pane's environment on a server started from a shell with `TERM=xterm-kitty`, `TERM_PROGRAM=ghostty`, `TERMINAL_EMULATOR`, `__CFBundleIdentifier`, `CURSOR_TRACE_ID`, `VisualStudioVersion`, `VSCODE_GIT_ASKPASS_MAIN`, `SSH_AUTH_SOCK=/first` and `DISPLAY=:1`, and after a client attached from a shell with `SSH_AUTH_SOCK=/second` and `DISPLAY=:2` (tmux 3.7c; `sh -c 'env > FILE; sleep 600'` in claude's place, a window made after the attach, `show-environment` and `/proc/PID/environ`) | tmux set `TERM=tmux-256color`, its `default-terminal`, `TERM_PROGRAM=tmux` and `TERM_PROGRAM_VERSION=3.7c`; every other variable reached the pane as it came. The attach set `SSH_AUTH_SOCK=/second` and `DISPLAY=:2` in the session's environment (`update-environment`), and the window made after it got them; the pane's program kept `/first` and `:1`, and so did the server's global environment |
| a bare `tmux new-session -d -s cld-x` inside a pane of server `cld-c` (tmux 3.3a, 3.4, 3.5a, 3.7c) | the session lands on `cld-c`, whose socket the pane's `TMUX` names; `tmux -L cld-x` finds no socket (`error connecting to ... (No such file or directory)`). `kill-session -t =cld-c` leaves the server running with `cld-x`; `kill-server` ends both. The pane's program gets SIGHUP from either, as when its terminal closes. Once `kill-server` has returned, the server answered no more in 200 tries (3.3a, 3.7c); once `kill-session` of a server's last session had, the server still answered 1 or 2 times in 200 |
| a terminal attached to session `cld-a` when its server, `cld-a`, is killed, with and without another session on the server (tmux 3.3a, 3.4, 3.5a, 3.7c) | `kill-server` alone ends the terminal's client with `[server exited]` and status 1: tmux tells its clients that the server is shutting down, which the client takes for an error. `kill-session -t =cld-a \; kill-server`, one command, ends it with `[exited]` and status 0, as `kill-session` did on the shared server: the client hears that its session exited before the server shuts down. The pane's program and the other session's get SIGHUP, and once the command has returned the server answered no more in 200 tries (3.3a, 3.7c). But `kill-server` only signals the server (`kill(getpid(), SIGTERM)`), which then closes each new connection at once until its clients have gone and it exits (`server_accept` and `server_loop` in `server.c`, 3.3a, 3.4, 3.7c): a client that connects meanwhile fails with `server exited unexpectedly`. With a terminal attached, `list-sessions` run the moment the command returned failed so in 4 of 86 rounds and `new-session` in 1 (3.7c; none in 99 with 3.3a), and a `new-session` run the moment a `list-sessions` had failed so failed the same way (14 times with 3.7c, once with 3.3a). `cld kill -n r` with a terminal attached, then at once `cld new -n r`, reached a fresh server in 100 of 100 rounds (3.3a, 3.7c) |
| a socket directory that ignores case: a casefold tmpfs (Linux 7.0, `chattr +F` on the directory) as `TMUX_TMPDIR`, with session `cld-a` on server `cld-a` (tmux 3.7c) | `tmux -L cld-A` reaches server `cld-a`, whose socket file is the one `cld-a`; `#{socket_path}` there is the path the server was started on, `.../cld-a`. Until cld read it, `new`, `join` and `kill -n A` took the server for one that outlived session `A` and pointed at `tmux -L cld-A kill-server`, which ended `a`. Over a stale socket `cld-a`, `new-session` on `cld-A` starts a fresh server, and the socket is `cld-A` from then on. macOS, whose default APFS ignores case, was not checked |
| a server that outlives its session, and one without a session (tmux 3.7c, the snap, on a private socket; `server.c` of 3.7c read) | session `cld-x`, with `remain-on-exit failed`, ran `sh -c 'tmux new-session -d -s side sleep 6011; sleep 0.5; exit 0'`: its bare `tmux` made `side` on `cld-x`, and once the program exited with status 0 the server ran on with `side` alone. `list-sessions -F '#{socket_path}'` printed `.../cld-x` once for each session, two lines for two, with status 0; through a symlink `cld-X` to the socket `cld-x`, `.../cld-x` too. `kill-server` returned 0 and ended the server and `side`'s `sleep`; `list-sessions` then said `no server running`, and a `new-session` on the socket started a fresh server, with another `#{pid}`. A server started with `start-server \; set -s exit-empty off` answered `list-sessions` with nothing and status 0. So would the server a `cld new` starts, to a client that came before `new-session` had made its session: `server_start` listens on the socket, and releases the lock that clients starting a server take, before the server runs the starting client's command (read, not seen: 40 rounds of such a `new-session` beside a loop of `list-sessions`, whose clients take 150 ms to start with the snap, gave 218 answers naming the session and 22 no server) |
| a format that tells a server that outlived session `cld-x` (tmux 3.5, 3.5a, 3.6a, 3.7c in Docker; 3.7c, the snap, on a private socket) | `display-message -p '#{&&:#{S:1},#{&&:#{==:#{N/s:cld-x},0},#{==:#{b:socket_path},cld-x}}}'` on `-L cld-x`, from a client attached to nothing, printed 1 on a server with `side` alone, one with `cld-xx` alone, and one whose `cld-x` was renamed with `rename-session`; 0 on one with `cld-x` beside `side`, one without a session (`start-server \; set -s exit-empty off`), and server `cld-y`, with `side`, reached through a symlink `cld-x` to its socket (`#{b:socket_path}` is `cld-y`, the path it was started on). `if -F` with it and `kill-server` ended the server where it printed 1, with status 0, and left it running with status 0 where it printed 0: the format is expanded, and the command it picks is put after it in the client's queue, which the server runs on before it takes another client's commands (`cmd_if_shell_exec`, `cmdq_next`, read in 3.7c). `N/s:` compares whole names. Before 3.6 there is no `#{!:}`, which 3.5 and 3.5a expanded to nothing, and `#{&&:}` takes two operands, `a,b,c` as `a` and `b,c`, so that `#{&&:1,1,0}` was 1 (0 on 3.6a and 3.7c). A tab in the format came out as `_` to a client without a UTF-8 locale, as in Docker |
| `list-sessions` on a server that exits as it asks - its last session ends, or `kill-server` runs (tmux 3.3a, 3.4, 3.5a, 3.7c; servers started and ended in a loop beside a loop of `cld list`, for 15 s) | the client connects, and the server closes the connection without an answer: tmux fails with `server exited unexpectedly`, status 1 (`CLIENT_EXIT_LOST_SERVER` in tmux's `client.c`). Until `list` passed over it, it failed so in 24 of 173 runs (3.3a), 46 of 294 (3.4), 19 of 310 (3.5a) and 6 of 147 (3.7c); since, in none of 311, 224, 368 and 381. A client that the server tells it is shutting down exits with no output and status 0 instead (read from `client.c`, not seen) |
| the socket of `tmux -L NAME` (tmux 3.3a, 3.4, 3.5a, 3.7c) | tmux never removes it: not when the server exits with its last session, not on `kill-server`, not on SIGKILL. `list-sessions` on such a stale socket fails with `no server running on DIR/NAME`, on a name never used with `error connecting to DIR/NAME (No such file or directory)`, both with status 1; `new-session` on a stale socket starts a fresh server there. The socket is in `tmux-UID` under `TMUX_TMPDIR`, or under `/tmp` where `TMUX_TMPDIR` is unset, empty or names nothing that exists; tmux resolves a symlink in it. A socket path of 107 bytes works on Linux, one of 108 fails with `File name too long`. Where `tmux-UID` is a file, not a directory, every command fails with `DIR/tmux-UID is not a directory`, status 1 |
| what a server per session costs (tmux 3.3a, 3.4, 3.5a, 3.7c, in Docker, on a host busy with other builds) | 3.8 MB (3.3a) to 5.1 MB (3.5a) resident per server, the same with 10 sessions on it, next to about 400 MB for claude; one `list-sessions` took 6-12 ms, and one per socket over 36 sockets, 16 of them stale, 136-282 ms. #22's plan measured 3-6 ms and 115-170 ms on an idle machine (3.3a, 3.7c) |
| a program's rows in the main screen of a 40-column pane that narrows to 20 (tmux 3.3a, 3.7c) | tmux reflows them: three 39-character rows under two short lines became six lines, and the two lines above them and the first half of the first row went into the history; a cursor left at the start of the first row ended at the top left of the screen, on that row's second half. A program that redraws its lines in place, relative to where it left the cursor, then draws over the wrong lines, and can recover only by clearing the screen, and what the shell showed above it with it |
| a tmux client starting on a terminal (`tty_start_tty` in `tty.c`, read in tmux 3.3a, 3.4 and 3.7c) | tmux sets the terminal's mode and then calls `tcflush(TCOFLUSH)`, which throws away output the terminal has not read yet. What the list wrote last before it became `tmux attach-session` - leaving the alternate screen, the cursor shown, the title - was lost now and then under load (tmux 3.3a and 3.4 in Docker, `TestListJoin`'s enter case and C10's join): the tab kept its old title. A stopped (SIGSTOP) outer tmux did not lose it (3.7c, Linux 7.0), so it takes a loaded machine too. A terminal answers primary device attributes (DA1, `CSI c`) once it has read what came before: tmux with `CSI ? 1 ; 2 c` (3.3a; 3.7c built with sixel `CSI ? 1 ; 2 ; 4 c`), JediTerm 3.76 with `CSI ? 6 c` |
| a DA1 answer later than the list's wait for it, the list having become `tmux attach-session` (tmux 3.7c; the terminal frozen for 1.5 s against a one-second wait) | tmux asks for DA1 itself as it starts and takes the first answer for its own; the next, its own, reached claude's pane as keys (`CSI ? 1 ; 2 c` in the probe's input). Answered in time, the probe read no answer. Other versions were not checked |
| what tmux writes to the terminal as a client attaches to a session whose program asks for all-motion mouse reporting (tmux 3.7c, the probe as claude; the output of the baseline terminal and of JediTerm 3.76) | tmux turns every mouse mode off (`CSI ? 1006 l`, `? 1000 l`, `? 1002 l`, `? 1003 l`) and then on again as it wants them, after it has drawn: several times as the client attaches, and the last time after the pane's text, as the program's request comes in. A terminal that takes in the output a piece at a time while it is asked about it, as the JediTerm driver's emulator does on a thread of its own, shows the pane's text with mouse reporting off for a moment: C7 read the modes there once, under load (`{AltScreen:true Mouse:false}`), and the driver had no mouse reporting to send the wheel through |
| tmux's default wheel binding, the `WheelUpPane` line of `list-keys -T root` on a server started with `-f /dev/null` (Debian's 3.5a and 3.6b, from trixie and trixie-backports, and 3.7c built from source, in Docker; tmux's `CHANGES` and `key-bindings.c` read at 3.5a, 3.6 and 3.7c) | 3.5a hands the wheel to the pane (`send-keys -M`) where it is in a mode (`#{pane_in_mode}`) or its program asked for the mouse (`#{mouse_any_flag}`), and otherwise enters copy mode; 3.6b and 3.7c add `#{alternate_on}`, so a program in the alternate screen gets the wheel whether or not it asked for the mouse. `CHANGES` lists it under 3.5a to 3.6: "Don't enter copy mode on mouse wheel in alternate screen (issue 3705)". `list-keys -T root WheelUpPane` printed the binding on 3.5a and 3.6b, and nothing on 3.7c, with status 0: 3.7c shows a single binding found as a message on the client's status line (`cmd-list-keys.c`), and without a client only in the server's messages (`status.c`) |
| clicks over a pane whose program asks for SGR all-motion mouse reporting, under `mouse on`: a Ctrl+click, an Alt+right-click, an Alt+click, a Ctrl+right-click and a click, each typed as its SGR press and release into the pane of an outer tmux that runs the client, over a stub in the alternate screen that logs its input (tmux 3.7c, the snap's, on private sockets) | with tmux's default bindings the Ctrl+click reached the stub as its release alone, `CSI < 16 ; 20 ; 10 m`, and the Alt+right-click not at all: `C-MouseDown1Pane` runs `swap-pane -s @` and `M-MouseDown3Pane` opens the pane menu, neither asking whether the pane takes the mouse, where `MouseDown1Pane` hands the press on, and the other `Pane` mouse keys of the root table - middle and right click, drag, wheel, double and triple click - do so for a pane that takes the mouse (`#{mouse_any_flag}`). The two are the root table's only `Pane` mouse keys with a modifier. The other three clicks came whole. After `unbind -n C-MouseDown1Pane ; unbind -n M-MouseDown3Pane`, every click came whole: tmux hands a mouse key it has no binding for to the pane. `unbind -n` of a key that is not bound succeeds: the command list, run again on a server that had it, went on to its `new-session` |
| how claude opens a link it is clicked on (claude 2.1.284's bundle, read, not run) | on the release of a click whose press it saw: the press starts a selection at its cell, and a release that selected nothing opens the OSC 8 link claude drew at the cell, 500 ms later unless a second click makes a double click of it - where the click carries Ctrl or Alt (`button & 24` of the SGR report), the terminal is Ghostty by XTVERSION, or on macOS Ghostty or Warp by `TERM_PROGRAM`, whose Cmd+click comes without a bit; never in VS Code's terminal (`TERM_PROGRAM=vscode`, or XTVERSION `xterm.js`), which opens links itself. A release without its press starts nothing. Under tmux, `TERM_PROGRAM` and XTVERSION are tmux's, so only Ctrl or Alt opens a link. On Linux a right-click pastes the clipboard where nothing is selected, and a middle-click the primary selection |
| where claude's agent teams put a teammate that runs in a tmux pane, claude inside tmux (claude 2.1.284's bundle, `TmuxBackend`, read, not run) | in claude's own window: the first teammate splits claude's pane (`split-window -d -t PANE -h -l 70%`, the pane `TMUX_PANE` named at startup), the next ones split the teammates' panes (`split-window -d -t PANE -v` or `-h`), the window then laid out anew with `select-layout main-vertical`. So claude's window, on cld's server, can hold more panes than claude's. `teammateMode` is `in-process` by default: the backend registry picks tmux where it is `tmux`, or `auto` inside tmux |
| a program exiting on a pty4j pty (pty4j 0.13.13, read from its source; the JediTerm driver) | pty4j's reaper thread waits for the process and then wakes the reader (`breakRead`): `isAlive()` is false from then on, while the pty may still hold what the program wrote last. Reads return that, and then the end of the stream. The driver's emulator thread takes it all in, but can be behind: C9 read the modes once `isAlive()` was false, before the emulator had taken in the last of what tmux wrote, which turns them off |
| two Ctrl+X (0x18) typed into a pane whose program reads in raw mode, `dd bs=64 count=1` in a loop, a line of hex a read (tmux 3.7c, natively and in the image `tests/Dockerfile` builds): by two tmux clients 50 ms apart, as the baseline terminal's `Keys` types them; by one command list, `send-keys C-x \; send-keys C-x`; and pasted with `paste-buffer -p -S` | from two clients, two reads of a byte each, in 20 rounds of 20 natively and 50 of 50 in the image; from the command list and from the paste, one read of both bytes, every round. tmux adds what a command types to the pane's buffer (`bufferevent_write` in `input-keys.c` and `cmd-paste-buffer.c`, read in the 3.7c source) and writes it out once its event loop comes round, in one write. What two clients type goes in two writes, which a program that has not read the first by the second reads at once all the same, as `cld list` stopped (SIGSTOP) until both had come did (see Implementation notes) |
| SIGTSTP in a Go program that has had it through `os/signal` (Go 1.27.1, Linux 7.0) | after `signal.Stop` or `signal.Reset`, `kill -TSTP` of the process did nothing: `sigdisable` leaves Go's handler in place for any signal that `sigInstallGoHandler` accepts, and the handler drops a `_SigNotify` signal that no channel wants. Never notified, SIGTSTP keeps its default action, since `initsig` skips `_SigDefault` signals. `kill(getpid(), SIGSTOP)` returned before the process stopped, under dash with `set -m`, and it stopped soon after; the SIGCONT of `fg` then reached `os/signal` |
| a job of a `sh -c` script under `set -m` that stops, and a background one that ends: macOS's `sh`, bash 3.2.57 as Apple builds it (read in its source, tag `bash-144`; seen on the CI's macOS 26 arm64 runner; run on Linux as GNU bash 3.2.57 with Apple's change to `jobs.c`), GNU bash 3.2.57 and 5.3.9, dash 0.5.12 | Apple's bash asks `waitpid` to report a stopped child (`WUNTRACED`) only when the shell is interactive, where GNU's asks whenever job control is on: in a script, `set -m` puts the job in a process group of its own, in the foreground, but once the job stops the shell goes on waiting for it to end, and never runs the rest of its script. With `-i` it goes on, `$?` 128 and the signal's number, as the others do in a script. bash 3.2.57, Apple's and GNU's, also reports a background job's end on stderr in a script while job control is on (`[1]+  Done ...`), which 5.3.9 and dash do not; with job control off again (`set +m`) once the job has started, it does not |
| a `list-sessions` client that connects as the server exits with its last session: `new-session -d`, `kill-session`, then `list-sessions -f`, 400 times (tmux 3.3a, 3.4, 3.7c) | the client printed `no server running on ...`, but for `server exited unexpectedly`, failing, in one round on 3.4 and one on 3.7c, and nothing in one on 3.3a and one on 3.7c. cld took the first for no session and reported the second as an error, `cld join` as the list's footer; since 13 it takes both for no server (see the row on a server that exits as it is asked). `TestListJoin`'s last-row case waits for the server to have exited before Enter |
| `kill-session`, then `list-sessions` at once, 400 rounds (tmux 3.3a, 3.4, 3.5a, 3.7c; Docker, 3.7c also natively) | with another session left, `list-sessions` never showed the killed one. With the last one killed, it reported `no server running on ...` every time; under load, eight such loops at once (2400 rounds in Docker), it failed with `server exited unexpectedly` in 40 on 3.3a, 19 on 3.4, none on 3.5a and 10 on 3.7c, and printed nothing, succeeding, in 53, 17, 0 and 1: tmux had not finished exiting. A `list-sessions` right after each failure reported no server. Natively on 3.7c (Linux 7.0, eight loops of 400) all 3200 reported no server |
| `kill-server`, then `list-sessions` at once, 400 rounds (tmux 3.3a, 3.4, 3.5a, 3.7c; Docker) | `no server running on ...` every time on 3.5a and 3.7c, but for `server exited unexpectedly` in 31 rounds on 3.3a and 44 on 3.4. Under load, eight such loops at once (2400 rounds), 3.5a failed so in 5 and 3.7c in 30; a `list-sessions` right after each failure reported no server. Since 13 cld takes both for no server, so the session list's read after a kill passes over the killed session's server as it exits (15.5) |
| what `claude` 2.1.283 and 2.1.284 do on SIGHUP (read from their bundles, not run) | an interactive claude shuts down as for SIGTERM, but with the status 129 (143 for SIGTERM): it prints its resume hint where its stdout is a terminal, runs its cleanups, waits for its pending writes, kills the shell commands still running, runs its `SessionEnd` hooks, then exits. The hooks get the reason of the shutdown, `other` unless its caller names one - of `clear`, `resume`, `logout`, `prompt_input_exit` (`/exit`) and `other` - and are aborted after `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS` or, unset, after 1.5 s or the longest `timeout` of a `SessionEnd` hook, 60 s at most. A failsafe forces the exit, giving claude's output 0.5 s first, and claude moves it as the shutdown goes on: armed with the signal for that budget plus 5 s, 5 s at least; armed again once the cleanups have ended, or had their 2 s, for the budget plus 5 s, 15 s at least, where writes are still pending; put off 2 s, twice at most, while a refresh of the OAuth token is held; and armed again after the hooks, as claude drains its output, for 2 s - longer for the bytes left to write, at 256 KiB/s, 30 s at most - plus 1.5 s. So until the drain it falls due 6.5 s after the signal with the default budget, 17 s where writes are pending, 21 s with the refresh held too, and 65, 67 and 71 s where a hook's `timeout` is 60 s; the drain sets it anew, 3.5 to 31.5 s past the drain's start (`shutdown`, `armShutdownFailsafe`, `waitForHeldOAuthRefresh` and `armFailsafeAndDrainStdout` in the bundles). A conversation claude runs in the background (`CLAUDE_BG_BACKEND=daemon`) ignores SIGHUP unless it owns its controlling terminal |
| `cld kill` of a session whose program stands in for claude: a bash script that, on SIGHUP, sleeps 1.5 s for a `SessionEnd` hook, writes a line to its terminal and exits 129 (Ubuntu's tmux 3.7c snap, natively; cld of main 18f63ab) | `cld kill` returned after 0.52 s with status 0, and `tmux -L cld-S ls` then said `no server running`. The script, its parent now PID 1, went on: its hook ended 1.56 s after `cld kill` returned, and its line failed with `Input/output error`, the pty's master being closed |
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
| the same through a symbolic link, `link` to the directory that holds the repository `work`, from `link/work` with `PWD` naming it (git 2.53.0 on Ubuntu 26.04, and 2.47.3 in the image `tests/Dockerfile` builds) | the common git directory is `.git` from the main work tree and `../.git` from a subdirectory, which cld joins to the current directory as `PWD` names it, `link/work/.git`; from a linked worktree under `link/work/.claude/worktrees` it is the real path, `real/work/.git`. `--path-format=absolute` gives the real path from all three. So one repository's home (37) has two paths, which `os.SameFile` finds one directory |
| `set -t =cld-a: @cld-home VALUE`, then `#{@cld-home}` and `#{n:@cld-home}` in `list-sessions -F` (tmux 3.7c, in the image `tests/Dockerfile` builds; `format.c` and `options.c` read) | `set` keeps the value as it is: a tab, a space, a `;` inside it and `#{session_name}`, unexpanded; a `;` at its end ends the command, and the value with it, where `\;` keeps it. `#{@cld-home}` gives the value as set, `#{n:@cld-home}` its length in bytes - 8 for `/x/café` - and an unset option expands to nothing, of length `0`. A client whose locale does not name UTF-8 gets `_` for the tab and for the `é`, as `cld list` did (see above), and with `-u` both as they are. A window option `@cld-home` hides the session's, as for `@cld` |
| `claude mcp list` (2.1.283, Linux, a scratch `CLAUDE_CONFIG_DIR`) in a project that `cld setup project --mcp goland,jbcontext,rider` wrote, GoLand and Rider 2026.2 running with their MCP servers on 64422 and 64482 | claude lists the three from `.mcp.json` - `claude mcp get jbcontext` names its scope `Project config (shared via .mcp.json)` - as `Pending approval (run claude to approve)` while the folder's workspace trust is not accepted; once it is, all three are `Connected`. Trusted, but without `enabledMcpjsonServers` in `.claude/settings.json`, they stay `Pending approval` |
| the port of a JetBrains IDE's MCP server (GoLand 2026.2.3's `mcpserver` plugin, `McpServerSettings` and `McpServerService` read with `javap`; GoLand 2026.2.3 and Rider 2026.2.1 remote-development backends listening) | the default is 64342 plus an offset per product, chosen by `PlatformUtils.getPlatformPrefix()`: IntelliJ IDEA 0, CLion 20, DataGrip 60, GoLand 80, PhpStorm 100, PyCharm 120, Rider 140, RubyMine 160, RustRover 180, WebStorm 200, any other 0 - so GoLand listens on 64422 and Rider on 64482, as they do here, the two running at once; an authorized endpoint takes the port 100 above (64522, 64582). The MCP Server settings keep a port of their own (`mcpServerPort`), and the system property `idea.mcp.server.force.port` overrides both; where the port was never changed, the options file (`mcpServer.xml`) holds `enableMcpServer` alone. "Copy HTTP Stream Config" in those settings gives `http://127.0.0.1:PORT/stream` |
| `${VAR:-DEFAULT}` in the URL of an `.mcp.json` server (claude 2.1.283, `claude mcp list` and `claude mcp get`, a scratch `CLAUDE_CONFIG_DIR`, the folder trusted, GoLand and Rider as above) | claude expands it as it connects: to DEFAULT where VAR is unset, and to VAR from its environment, or from the `env` of `$CLAUDE_CONFIG_DIR/settings.json`; VAR in the `env` of the project's `.claude/settings.local.json` was not used. It shows the URL with `${VAR}`, the default left out. `${VAR}` without a default, VAR unset: `[Warning] [goland] mcpServers.goland: Missing environment variables: VAR`, and the server fails with `'url' is not a valid URL` |
| `git check-ignore --verbose -z --stdin` given several paths under `.claude` (git 2.53.0 on Ubuntu 26.04; the tests' cases also on git 2.47.3, in the image `tests/Dockerfile` builds) | it answers each path a pattern matches, an exception too, in the order given, as four NUL-ended fields, and leaves out a path none matches; it exits 1 only where none matches any. A path given with a trailing slash is taken for a directory before it exists: `skills/` matches `.claude/skills/`, not `.claude/skills`, until the directory exists; `.claude/` matches every path under `.claude`, the directory there or not. With `/.claude/*` and then `!/.claude/settings.json` in `.gitignore`, as cld wrote them until 28, git ignores `.claude/commands/`, `agents/`, `skills/`, `rules/`, `hooks/` and `CLAUDE.md`: `git status --untracked-files=all` does not show a new `.claude/skills/s/SKILL.md`, and `git add` refuses it (`The following paths are ignored by one of your .gitignore files`, `hint: Use -f`), exit 1. With `.claude/skills` a symbolic link to a directory, `.claude/skills/` ends it, whatever else it was given, with `fatal: pathspec '.claude/skills/' is beyond a symbolic link`, exit 128; `.claude/skills` it answers, by `/.claude/*`, as `git status --ignored` shows the link. It does not answer for a path git tracks, nor for a directory holding files added with `git add -f`, although git ignores a new one there (`git status --untracked-files=all` does not show `.claude/skills/b/SKILL.md` beside a tracked `.claude/skills/a/SKILL.md`); with `--no-index` it answers for both - for a tracked `.claude/settings.json` too, by `*.json` - and for a submodule, `.claude/skills` added with `git submodule add -f`, checked out or not (a clone that has not run `git submodule update`, an empty directory), which it does not answer for with the index; `git ls-files --stage -z -- .claude/skills` names that one `160000 OBJECT 0`, a tab and `.claude/skills`, from a subdirectory too, as a path from it. A repository in `.claude/commands` that is no submodule it answers for either way |
| what claude keeps out of git itself (claude 2.1.284's bundle, read, not run; and this repository's `.git/info/exclude`) | claude appends to the repository's `.git/info/exclude`, where it lacks the line `# claude-code-runtime`, that line and `**/.claude/` patterns for its runtime files: `scheduled_tasks.lock`, `scheduled_tasks.json`, `routines/.state/`, `worktrees/`, `checkpoints/`, `mailbox/`, `agent-registry.json`, `agent-memory-local`, `first-run`, `assistant-daemon-state.json` - this repository's has them. Where it writes `.claude/settings.local.json` and git does not ignore that, it appends `**/.claude/settings.local.json` to the user's global excludes (`core.excludesfile`, else `$XDG_CONFIG_HOME/git/ignore` or `~/.config/git/ignore`). Neither covers the plans: `plansDirectory` is "relative to project root", and by default `~/.claude/plans/`. The bundle's own table of the settings files marks `.claude/settings.json` "Commit" and `settings.local.json` "Gitignore", and has them load user, project, local, a later one overriding |
| the defaults of the settings cld wrote until 28 (claude 2.1.284's bundle, read, not run) | where no settings file sets them, `autoMemoryEnabled` is `true`, `autoCompactEnabled` `true`, `theme` `"dark"` and `autoUpdatesChannel` `"latest"`: the values cld wrote. All four are read from any settings file, the project's `.claude/settings.json` among them, which overrides `~/.claude/settings.json` |
| read-only commands and permission rules (claude 2.1.284's bundle, read, not run; git 2.53.0, run in a scratch repository) | claude runs bare read-only commands without asking - the bundle's own advice says "many bare read-only commands (`ls`, `cat`, `git status`, ...) are auto-allowed by Claude Code and never prompt" - and checks the options of `git diff`, `git log`, `git show`, `git status` and others against lists of safe ones, which have no `--output`. A rule `Bash(PREFIX:*)` is a prefix rule, as `Bash(PREFIX *)` is: it matches `PREFIX` alone or followed by a space and anything - "prefix STRING matches with NO flag-level analysis", in the bundle's words, which go on: "`git log --output=<file>` and `git diff --output=<file>` write arbitrary files ... so `Bash(git log *)` admits every flag form those validators deliberately reject". With git 2.53.0, `git log -1 --format='[core]%n%x09fsmonitor = "touch PWNED; echo"%n' --output=.git/config` and then `git status` made the file `PWNED`. The target of an output redirection (`>`) claude checks as a file it would create, whatever rule matches the command |
| the tools of the IDEs' MCP servers and of `jbcontext mcp` (`initialize`, then `tools/list`: over streamable HTTP to GoLand 2026.2.3's server on 64422, and over stdio to JetBrains Context 0.9.15's `jbcontext mcp`) | GoLand's server has 44 tools, `execute_terminal_command`, `execute_run_configuration`, `apply_patch`, `execute_sql_query` and `build_project` among them, and marks 16 with `readOnlyHint`: `analyze_calls`, `get_all_open_file_paths`, `get_file_problems`, `get_project_dependencies`, `get_project_modules`, `get_repositories`, `get_run_configurations`, `get_symbol_info`, `git_status`, `lint_files`, `list_directory_tree`, `read_file`, `search_file`, `search_regex`, `search_symbol`, `search_text`; none has `destructiveHint`. `jbcontext mcp` has one tool, `code_search`. `jbcontext`'s own commands include `logout`, `remove-index`, `upgrade`, `setup-agent` and `remove-agent` beside `search`. Rider's server was not probed |
| real `claude` under tmux, first 12 s | enables `?2004` bracketed paste, `?2031` colour-scheme reports, `?1004` focus, `?1049` alt screen, `?1000/1002/1003/1006` SGR all-motion mouse; queries XTVERSION (`CSI > 0 q`), kitty keyboard (`CSI ? u`), DA1, DECRQM `?2026`; resets modifyOtherKeys (`CSI > 4 m`); sets the title `✳ <name>`. The pane stayed in key mode `VT10x`: no extended keys were requested in that window - because the probing shell carried `TERMINAL_EMULATOR` (see What the tests found) |
| which terminal `claude` takes itself to be in, and when it turns extended keys on (the linux-x64 bundles of 2.1.282, 2.1.283 and 2.1.284, read, not run; alike in the three) | the first that holds of: `CURSOR_TRACE_ID` set (Cursor); `VSCODE_GIT_ASKPASS_MAIN` naming `cursor`, Windsurf or Devin, or `antigravity`; `__CFBundleIdentifier` naming `vscodium`, `windsurf` or `devin`, `com.google.android.studio`, or one of 16 JetBrains names (`goland`, `pycharm`, `intellij`, `jetbrains`, ...); `VisualStudioVersion` set; `TERMINAL_EMULATOR=JetBrains-JediTerm` (`pycharm`); `TERM` `xterm-ghostty`, or with `kitty`; `TERM_PROGRAM` - in a pane, tmux sets it to `tmux`, and `TERM` to its own (see the pane's environment above); then `TMUX` and others. Extended keys are on from the start for `iTerm.app`, `kitty`, `WezTerm`, `ghostty`, `tmux`, `windows-terminal` and `WarpTerminal`; for any other terminal claude asks, and with no answer leaves them off (`extendedKeys=no (env: terminal=pycharm, no answer)`, see What the tests found). claude reads `__CFBundleIdentifier` for more than that: `com.googlecode.iterm2`, under tmux too, makes `/terminal-setup` offer iTerm2's clipboard setting, and computer use on macOS takes it for the terminal's app. The variables claude's background sessions drop from their environment include `CURSOR_TRACE_ID`, `__CFBundleIdentifier`, `TERMINAL_EMULATOR`, VS Code's askpass (`GIT_ASKPASS`, `VSCODE_GIT_ASKPASS_MAIN`, `_NODE` and `_EXTRA_ARGS`, `VSCODE_GIT_IPC_HANDLE`), `TERM_PROGRAM` and `TMUX`, but not `VisualStudioVersion`, and more: `LC_TERMINAL`, `ITERM_SESSION_ID`, `KITTY_WINDOW_ID`, `WT_SESSION`, `SSH_CONNECTION` and `SSH_ASKPASS` among them; not `GIT_EDITOR` or any `VSCODE_GIT_EDITOR_*`. claude runs its Bash tool's commands with `GIT_EDITOR=true`, whatever `GIT_EDITOR` it got |
| the helpers VS Code's git extension gives git in its terminal (`extensions/git/src` of microsoft/vscode at d8dfa8f, 2026-09-29: `askpass.ts`, `askpassManager.ts`, `gitEditor.ts`, `ipc/ipcServer.ts`, `ipc/ipcClient.ts`, `askpass-main.ts`, `git-editor-main.ts`, the scripts; read, not run) | with `git.terminalAuthentication` (on by default) the askpass: `GIT_ASKPASS` naming `askpass.sh`, or `askpass-empty.sh` where the extension has no IPC server, and `VSCODE_GIT_ASKPASS_NODE`, `VSCODE_GIT_ASKPASS_EXTRA_ARGS` and `VSCODE_GIT_ASKPASS_MAIN` naming `askpass-main.js` in the same directory, the extension's (on Windows, a copy of it); `SSH_ASKPASS` goes to the extension's own git, not the terminal. With `git.terminalGitEditor` (off by default) the editor: `GIT_EDITOR` naming `git-editor.sh` (or `git-editor-empty.sh`) in the same directory, in double quotes, and `VSCODE_GIT_EDITOR_NODE`, `VSCODE_GIT_EDITOR_EXTRA_ARGS` and `VSCODE_GIT_EDITOR_MAIN` naming `git-editor-main.js`. Whatever the settings, where the extension runs its IPC server, `VSCODE_GIT_IPC_HANDLE`, the window's socket. Each script runs `NODE` on `MAIN`, which asks the window through the socket, and without `VSCODE_GIT_IPC_HANDLE` exits 1 at once (`Missing VSCODE_GIT_IPC_HANDLE`) |
| `claude` 2.1.283's own title (read from its bundle; and the `#{pane_title}` of a claude working in a session of cld's, read every 50 ms for 45 s) | `MARKER NAME`, the marker from claude's status: `◐` and `◑` in turn, every 960 ms, while it is `busy`; `✳` while it is `idle` or `waiting` - a permission dialog, an MCP server's question. Where `TMUX`, `STY` or `ZELLIJ` is set and the feature flag `tengu_static_title_under_mux` (on by default) holds, the marker stays `✳`: the pane's title read `✳ cld-NAME` throughout. claude also writes its status to `~/.claude/sessions/PID.json` (`status`, `statusUpdatedAt`), which no documentation names, and sends no OSC 9;4 that tmux records: `#{pane_pb_state}` stayed `hidden` |
| claude's hook events (the linux-x64 bundles of 2.1.232 and 2.1.283, read, not run) | both have `UserPromptSubmit`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest` ("When a permission dialog is displayed"), `Elicitation`, `ElicitationResult`, `Notification` - of the types `permission_prompt`, `idle_prompt`, `elicitation_dialog` and others - `Stop` and `StopFailure` ("Fires instead of Stop when an API error ... ended the turn"). `PostToolUseFailure`'s input has `is_interrupt`. No event comes when the user interrupts claude as it writes |
| hooks given with `--settings` (the real `claude` 2.1.283, alone on a scratch tmux server, in a directory whose trust was accepted, Remote Control off; a second `UserPromptSubmit` hook exited 2, which blocks the prompt, so nothing reached the API) | claude ran them - `SessionStart` as it started, `UserPromptSubmit` on Enter - in its own environment, with `TMUX` naming the server and `TMUX_PANE` its pane: `tmux if -F -t "$TMUX_PANE" '#{!=:#{@cld-status},busy}' 'set @cld-status busy'` set the option on claude's session. No `Stop` followed the blocked prompt: the option stayed `busy` |
| hooks of a conversation claude runs in the background (the real `claude` 2.1.284 on Linux, in a session cld 0.8.1 made on tmux 3.7c; read with `ps`, `/proc/PID/environ`, the daemon's log and the transcript - not made to happen by hand) | two seconds after cld started claude, `claude daemon run` - started the day before, from a claude in the default tmux server - logged `bg spawned ID (slash)` and ran the conversation in a worker of its own: `claude bg-pty-host`, running claude `--session-id ID` with the `--settings` of the claude in the pane word for word. The claude in the pane showed the conversation, whose transcript entries say `"sessionKind":"bg"`. The worker's environment had no `TMUX` and no `TMUX_PANE` - the daemon's had those of its tmux - and its directory was the session's. Every hook failed there: `tmux if -F -t "$TMUX_PANE" ...` went to the default server, which ran with no session; `if -t ''` found no target, which `if` allows, and its `set`, without one either, said `no current session`, exit 1. claude showed that after each tool (`PostToolUse:Bash hook error`), 141 times in two hours, and `@cld-status` stayed unset. Without a target tmux takes the pane in the client's `TMUX_PANE`, and without that the session with the latest activity (`cmd-find.c`, tmux 3.7c): with a session on the default server, the option would have gone there. `tmux -S SOCKET if -F -t =cld-NAME: '#{!=:#{@X},x}' 'set -t =cld-NAME: @X x'`, run from `/tmp` without either variable, set the option on the session, and nothing on the default server |
| Claude Code's background sessions beside cld's (the [agent view](https://code.claude.com/docs/en/agent-view) and [fullscreen](https://code.claude.com/docs/en/fullscreen) docs, read 2026-09-29; the linux-x64 bundles of 2.1.283 and 2.1.284, read; `claude agents --json` of 2.1.284, run beside two sessions of cld's - nothing started, attached or stopped) | the docs: `claude --bg`, `/bg` and `←` hand a conversation to a supervisor process that runs it without a terminal, and `/fork` a copy of it; an attached one renders fullscreen whatever the `tui` setting says, screen reader mode included, and tmux's copy mode sees only the screen; the supervisor stops a session's process once it is done, or waiting for the next message, and has been unattached for about an hour, unless it is pinned, and resumes the conversation on attach; it starts a process that exits unexpectedly again; after a shutdown a session shows failed - stopped past 48 hours - and attaching resumes it; agent view is a research preview, and shows an interactive session only once it has gone to the background. Both bundles: `claude attach`, `logs` and `stop` take a prefix of a short ID, 8 hex digits (`/^[a-f0-9]{8}$/`), and for anything else, a name included, print `No job matching 'X'. Run 'claude agents' to list running sessions.` and exit 1; the setting `disableAgentView`, "Equivalent to CLAUDE_CODE_DISABLE_AGENT_VIEW=1", disables "agent view (`claude agents`, `--bg`, /background, the on-demand daemon)". `claude agents --json` listed each session of cld's as `"kind": "interactive"` and `"name": "cld-NAME-SUFFIX"`, with its `pid`, `sessionId` and `status`, and no `id`, which only a background session has. The idle stop, the restart, the renderer and a shutdown were not seen |
| where claude runs a hook, and what `SessionStart` and `CwdChanged` see (the real `claude` 2.1.283 started in a linked worktree whose trust was accepted, alone on a scratch tmux server, Remote Control off; bash mode's `!cd /tmp`, `!cd` to the main worktree and `!cd` back - which claude answered with the model all the same, three short replies) | `SessionStart` ran in claude's directory, with it as its input's `cwd`. `!cd /tmp` fired `CwdChanged` with `new_cwd` `/tmp`, and `!cd` to the main worktree one with that; each time claude then took its shell back to the worktree the session works in ("Shell cwd was reset"), with no event, and the hooks' own directory, their `cwd` and `#{pane_current_path}` stayed the worktree throughout. `!cd` back to it fired nothing, the shell being there already |
| where claude sets its directory (claude 2.1.283's bundle, read, not run) | a hook runs in the host's project root where a launch sets one, and otherwise in claude's current directory. claude sets that - `process.chdir` and the session's `setCwd`, whose change fires `CwdChanged` - for `--worktree` as it starts; for `EnterWorktree` and `ExitWorktree`, the latter back to the directory it came from; and for a resumed conversation that recorded a worktree. A `WorktreeCreate` hook replaces claude's own making of a worktree (its stdout names the directory), so it is no event to listen to |
| `set-titles` under `status off` (tmux 3.7c: `server-client.c`, `format.c`, `options.c`, `status.c` and `cmd-refresh-client.c` read, and probed with a client in a pane of another server, whose `#{pane_title}` is the title that client sets) | tmux expands `set-titles-string`, a session option, with strftime whenever it redraws a client, and writes the title only where it changed; it restores no title on detach. Setting any option, a user option too, redraws every client on the server: `set @cld-status busy` turned `✳ NAME` into `◐ NAME` at once. With `status off` no timer expands the title again, and a `#()` job in it redraws nothing when it ends - only the status line's jobs do - but a job that runs `refresh-client -S`, which redraws the status alone, that is the title, a second later in the background kept it turning: `◐` and `◑` swapped every 1.0 to 1.3 s until the option changed, with the job naming a tmux whose path has a space, `#`, `%` and parentheses through `#{q:@OPTION}`. tmux runs a title's job at most once a second for each client. With `set-titles` on tmux also hands the active pane's directory (OSC 7) to the terminals it credits with `osc7`, iTerm2 and foot among them: an empty one for claude, which sets none |
| how `claude` notifies (the linux-x64 bundles of 2.1.283 and 2.1.284, read, not run; the settings reference, read 2026-09-29, agrees: `"auto"` "does nothing elsewhere") | the setting `preferredNotifChannel` - any settings file, `--settings` too, and `/config`'s "Local notifications" - is `auto` by default, or `iterm2`, `iterm2_with_bell`, `kitty`, `ghostty`, `terminal_bell` or `notifications_disabled`. `auto` goes by the terminal claude detects: after the IDEs' markers, `TERMINAL_EMULATOR`, and a `TERM` of `xterm-ghostty` or with `kitty` in it, `TERM_PROGRAM`, whatever it says. `iTerm.app`, `kitty` and `ghostty` get their channel, `Apple_Terminal` the bell where its profile's audible bell is off, and anything else, `tmux` too, nothing (`no_method_available`); for a conversation claude runs in the background, the terminal of the client attached to it comes first. With `TMUX` set, a channel's sequences go in tmux passthrough, `ESC P tmux;` and the sequence with each `ESC` doubled, then `ESC \`: `iterm2` OSC 9 with the message (`TITLE: MESSAGE` where there is a title), `kitty` three OSC 99 (the title, `Claude Code` by default; the body; focus), `ghostty` OSC 777 `notify;TITLE;BODY`, each ended by BEL (by ST where claude detects kitty); `terminal_bell` a BEL, not wrapped, and `iterm2_with_bell` OSC 9 and the BEL. The `Notification` hooks run before claude sends, whatever the channel |
| what reaches the terminal from a pane that notifies as claude does on each channel: OSC 9, 99 and 777 in tmux passthrough, then a BEL (tmux 3.7c; the pane on a server started with `-f /dev/null` and `allow-passthrough` on, and off; two terminals attached to its session - each a client in a pane of another server, which piped the pane's output to a file - then none) | with `allow-passthrough on`, each terminal got the three sequences with the passthrough taken off, and the BEL, under tmux's default `bell-action` and `visual-bell`; with it off, the BEL alone. Sent with no terminal attached, neither reached the terminal that attached next. The pane had `TERM_PROGRAM=tmux`, `TERM_PROGRAM_VERSION=3.7c` and `TERM=tmux-256color`, where the client that started the server had `TERM_PROGRAM=iTerm.app`; its `LC_TERMINAL=iTerm2` came through as it was |
| claude's links under tmux, and what tmux passes on (the bundles of claude 2.1.283 and 2.1.284, read, not run; tmux 3.7c's `tty-features.c`, `tty-term.c`, `tty.c` and `hyperlinks.c` read, and run in the image `tests/Dockerfile` builds, a client attached from a pty of `script`, which answers no query, XTVERSION included, with ncurses 6.6's terminfo) | claude marks file paths and URLs as OSC 8 links, `ESC ] 8 ; ; URI BEL TEXT ESC ] 8 ; ; BEL`, where `TERM_PROGRAM` is `tmux` and `TERM_PROGRAM_VERSION` 3.4 or newer. tmux keeps a link with the pane's cells, and writes it to a terminal only where that has the `hyperlinks` feature - the capability `Hls`, which no terminfo entry of ncurses 6.6 has, nor those WezTerm and Alacritty ship - as `ESC ] 8 ; id=tmuxN ; URI ESC \`, the text and `ESC ] 8 ; ; ESC \`. Its table of terminals known by XTVERSION gives the feature to iTerm2, foot and tmux; WezTerm, XTerm, mintty and rxvt-unicode are in it without. An OSC 8 printed in a pane reached a client with `TERM=xterm-256color`, under cld's `terminal-features` entry `xterm*:extkeys`, as its text alone (features `bpaste,ccolour,clipboard,cstyle,extkeys,focus,title`), and whole under `xterm*:extkeys:hyperlinks`; with `TERM=wezterm` or `TERM=alacritty` only under an entry for that `TERM`, `wezterm:hyperlinks` or `alacritty:hyperlinks`. An entry's features are separated by `:`: `xterm*:extkeys,hyperlinks` added neither, as tmux stops at a feature it does not know. Entries set twice at their indexes, as two `cld new` at once set them, stayed one each |
| which terminals take OSC 8 links, and under which `TERM` (OSC 8's spec, [egmontkob's gist](https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda), and the list [OSC8-Adoption](https://github.com/Alhadis/OSC8-Adoption), read 2026-09-29; WezTerm's docs `term.md` and `hyperlinks.md`; Alacritty's changelog and `alacritty_terminal/src/tty/mod.rs`; the classes of jediterm-core 3.76; xterm 411's `misc.c`) | kitty (`xterm-kitty`), Ghostty (`xterm-ghostty`), WezTerm (`xterm-256color`, or `wezterm` where its `term` says so), Alacritty since 0.11 (`alacritty` where that terminfo entry is installed, otherwise `xterm-256color`), VTE's terminals, Konsole (off by default), Windows Terminal, VS Code, mintty, foot, iTerm2 and JediTerm, whose emulator handles OSC 8 (`setLinkUriStarted`), take links. A terminal that parses OSC as ECMA-48 and takes no links shows the text alone - xterm 411 has no OSC 8, and ignores it as it ignores any code it does not know; the spec names VTE up to 0.48.1, Windows Terminal up to 0.9, Emacs's terminal and screen (for URIs of 700 characters or more) as garbling them |
| `new-session` and `attach-session` without a terminal to attach from (tmux 3.7c, Ubuntu's snap, on a private socket: stdin `/dev/null`, and a pty from `script` with `TERM` `dumb`, empty, unset and `nosuchterm`; `cld new`, `resume` and `join` before 31, in `TestRefusesWithoutATerminal`) | status 1 each time, with tmux's message alone: `open terminal failed: not a terminal` for stdin `/dev/null`, `open terminal failed: terminal does not support clear` for `dumb`, empty and unset, `missing or unsuitable terminal: nosuchterm` for a `TERM` terminfo does not know. `new-session` starts the server before it looks at the terminal, and the socket stays behind, where `tmux -L NAME ls` then finds no server running; `attach-session` leaves the session and its server as they were. cld had printed the title to stdout first, whatever stdout was. With stdin a terminal and stdout a pipe, tmux draws on stdin's terminal and writes only `[exited]` to the pipe as it ends |
| `history-limit` and a pane that prints 5000 lines a second after it starts (tmux 3.5, 3.6a and 3.7c, each on a private server in the image `tests/Dockerfile` builds with it; 3.7c also natively, the snap) | by default `show -gv history-limit` is `2000`, and the pane keeps 1977 lines of history: tmux drops a tenth of the limit when it is reached. With `set -g history-limit 50000` before `new-session` in one command, `#{history_limit}` is `50000` and the pane keeps 4977 lines on all three. With the option after `new-session`, in the same command or a later one, 3.7c gives the pane `50000` too, and it keeps 4977 lines; 3.5 and 3.6a keep `2000` and 1977 lines. tmux(1) of 3.5 and 3.6a says the option "applies only to new windows", that of 3.7c "Set the maximum number of lines held in pane history" |
| a terminal attached to a session (tmux 3.7c, the snap: a client of a private server in a pane of another, which stands for the terminal, `script -f` recording what the client writes) | the outer pane has `alternate_on=1 history_size=0` while the inner one, after `seq 3000`, has 2978 lines of history: the terminal's scrollback gets nothing. `OSC 133 ; A` and `; C`, `; D`, written in the inner pane, reach the terminal not at all; tmux marks the lines, and copy mode's `next-prompt` goes to them, but no key is bound to it or to `previous-prompt` by default |
| what a full history costs (tmux 3.7c, the snap: `history-limit` 50000, a 120x40 pane printing 60000 lines of 100 characters, the server's resident size) | from 3.6 MB to 35 MB for plain ASCII; to 156 MB where every character has an RGB colour, a new one every ten characters; the same coloured lines under the default 2000 took 10 MB |
| claude's renderers, and scrollback ([fullscreen rendering](https://code.claude.com/docs/en/fullscreen) and [screen readers](https://code.claude.com/docs/en/accessibility) in Claude Code's docs, read 2026-09-29; the bundle of 2.1.284, read, not run) | the classic renderer draws in the main screen and "keeps the conversation in your terminal's native scrollback so `Cmd+f` and tmux copy mode work as usual"; the fullscreen renderer draws in the alternate screen and scrolls its own transcript, which `Ctrl+O`, then `/`, searches, and `Ctrl+O`, then `[`, writes into the scrollback. Which one starts depends on the `tui` setting (`/tui default`, `/tui fullscreen`), `CLAUDE_CODE_NO_FLICKER`, `CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN`, feature flags and when claude was first used. Screen-reader mode (`--ax-screen-reader`, `CLAUDE_AX_SCREEN_READER=1`, `axScreenReader`) always runs the classic renderer, and relies on the terminal's scrollback for reading back and on OSC 133 marks for jumping between turns: the bundle writes `A` as a turn starts and `C`, `D` as it ends in that mode, but in WezTerm, with no exception for tmux |

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

1. **Static**: gofmt and go vet; ShellCheck and shfmt for the two shell scripts, `install.sh` and
   `tests/jediterm/fetch-deps`.
2. **Behaviour against real tmux**: tmux is local and cheap, so it is not faked. Only `claude` is
   replaced, by a *probe* that behaves like claude towards the terminal (the modes above), logs
   its argv, cwd, environment and raw input bytes, and emits OSC sequences on request; and
   `docker`, for `setup telemetry`, by the same probe, which records the calls and fakes their
   results.
3. **Terminal contract**: the same checks run against several *outer terminals* through drivers.

Isolation needs no seams in the program: `TMUX_TMPDIR` moves cld's sockets (`-L cld-NAME`) into
a sandbox, `HOME` points at a temporary directory, `TMUX` is unset, and the probe is first on
`PATH`. `cld new` and `cld join` attach, so they need a pty: a terminal driver provides one, and
for the tests that hand over to the fake tmux, the sandbox's own (see 31.6).

### Terminal contract

| # | Check | Evidence |
|---|---|---|
| C1 | tab title is `✳ cld-NAME`, with `◐` and `◑` in turn in place of `✳` while claude is busy and ` [w]` after it while claude is in a linked git worktree, and survives claude's own title changes | terminal, probe |
| C2 | tmux's view of the client: `#{client_termtype}`, `#{client_termfeatures}` (`extkeys`, `focus`, `mouse`, `clipboard`, `hyperlinks`, ...) | tmux |
| C3 | tmux asks the terminal for modified keys and takes the request back on detach; Shift+Enter reaches claude distinct from Enter; the Ctrl keys claude binds (`C-b`, `C-_`) pass through; `C-q d` detaches; `C-q C-q` sends `C-q` | probe input log, terminal output |
| C4 | mouse wheel and focus in/out reach claude, and a Ctrl+click and an Alt+right-click whole, the press and the release; over a main-screen program without mouse reporting the wheel scrolls the pane's history | probe input log, tmux |
| C5 | OSC 52 and notifications - claude's OSC 9, 99 and 777 - wrapped in tmux passthrough, the bell, copies through `tmux load-buffer -w`, and claude's OSC 8 links reach the outer terminal | terminal, terminal output |
| C6 | claude never sees a variable that names the terminal a session was created in - `TERMINAL_EMULATOR` from the JetBrains terminal, `__CFBundleIdentifier`, `CURSOR_TRACE_ID`, `VisualStudioVersion`, VS Code's askpass and git editor (33) - nor does what claude starts through tmux on its server | probe env dump, tmux |
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
- **iTerm2 (planned: real app, macOS runner, nightly and tags; not built, see 8 and Status)**:
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

- every push and pull request: lint; contract x tmux on two pinned tmux releases, the newest
  (3.7c) and the oldest cld runs on (3.5a, advisory), each built from source in a Linux container
  (6), and on Homebrew's current tmux on macOS; contract x JediTerm; the completion tests in bash
  with ble.sh, in the same image built on Ubuntu (27.5);
- planned with the iTerm2 driver, and not in `ci.yml` (see 8 and Status): nightly, on tags and on
  demand, contract x iTerm2 on macOS, uploading screenshots and logs on failure; non-blocking
  until it proves stable;
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
   asks that server; since 34, only where that server has cld's mark. `list` prints its table
   there rather than the interactive list, whose Enter would be refused (see 14).
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
   options and the hook go to claude's pane only (see 9 and 13), since #78: on its window, any
   other pane there that failed - a teammate's that claude splits off (see Findings), one split
   by hand - stayed dead with the hint, `claude exited`, while claude ran on.
6. Versions (#21, #74): cld runs on tmux 3.5a or newer, the oldest release its tests run on, and
   starts Claude Code 2.1.232 or newer, the first release that does what cld passes and relies
   on. Both are checked at startup and raised by hand, and neither has an upper bound. The tmux
   check runs for every command but `help`, `version`, completion (17.4), `setup telemetry`,
   `setup project`, `update` and `setup completion`, which run no tmux (18, 19, 21, 22), and
   refuses an older tmux with `cld: tmux 3.5a or newer is required, found 'tmux 3.4'` and
   status 1.
   - It reads `tmux -V`: the major and minor version, after `next-` for a development build
     (`next-3.9` is 3.9, `3.8-rc2` 3.8), then the letter of a bug-fix release as a third number,
     `a` as 1 and none as 0, so that 3.5 is older than 3.5a; a version without them (`master`)
     passes, and there is no upper bound. Until #74 letters were not compared: the minimum, 3.7,
     needed none.
   - The tests run on two tmux releases, each built from source in the Docker image: the newest,
     3.7c, in CI's job `linux`, and the oldest cld runs on, 3.5a, in `linux-oldest`
     (`make docker-check TMUX_VERSION=3.5a`). No released Debian or Ubuntu version ships 3.7, and
     a package from Debian testing would change whenever the base image does. The newest is
     pinned in `tests/Dockerfile` and the `Makefile` and bumped by hand together with these docs,
     as JediTerm is (7); the oldest is pinned in the workflow and raised with the minimum. Neither
     job's name carries the version, so a bump leaves the ruleset's required checks alone.
     `linux-oldest` is advisory: the ruleset requires `linux` and `macos`, and requiring it too is
     the maintainer's call. The macOS job installs Homebrew's current tmux, which runs ahead of
     the pin when Homebrew moves - the signal to bump. cld does nothing per tmux version; the
     tests' baseline terminal passes `paste-buffer -S`, new in 3.7, only to a tmux that has it.
   - Why 3.5a (#74): from #21 the minimum was 3.7, the one release the tests ran on - a minimum of
     3.5 would have removed the same code (`remain-on-exit` stayed off below it, see Findings) but
     3.5 and 3.6 would have run untested, and keeping 3.3 while dropping versions from the tests
     alone would have left a branch that no CI runs. That refused the tmux of every supported
     Debian, Ubuntu and RHEL release, while nothing cld runs needs 3.7: every option and command
     it uses is in 3.2 or 3.3, and extended keys' mode 2 and the fix for the crash on a dead pane
     with focus reporting came in 3.5 (see Findings). 3.5a and 3.6a pass the whole suite, and
     3.6b did in #74's probe; 3.4 crashes, and failed two tests there. Plain 3.5 passes it too,
     but hands claude Shift+A as `S-A` and mangles Shift+Backspace from a terminal that reports
     them in CSI u, and runs `#()` jobs with the user's shell, where fish refuses the title's job
     (25): 3.5a fixed both, so the floor has its letter (see Findings).
   - The cost falls on distribution packages: Debian 13 ships 3.5a (3.6b in trixie-backports),
     Debian 12 3.3a (3.5a in bookworm-backports), Ubuntu 24.04 3.4 and 26.04 3.6a, RHEL 9 and 10
     3.2a and 3.3a. The users of those below 3.5a need Homebrew, Debian testing or unstable, or a
     source build - or cld 0.3.0, which runs on tmux 3.3 and newer. The README recommends 3.7 all
     the same: claude draws with synchronized output only where tmux answers its DECRQM 2026,
     which 3.7 is the first to do (see Findings).
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
   `tmux -L cld ls`. What `cld` sets for a failed claude (see 5) goes to claude's window (its pane
   since #78), not the server, so such a session whose program fails closes as tmux would close
   it, rather than staying where `cld` neither lists nor kills it. Starting claude without `TMUX`
   would keep its tmux off cld's server, and its passthrough and `load-buffer` copies with it; a
   server per session would still need the mark for a session made on it by hand, and `list`
   would have to find the servers.
   The mark guards against mistakes, not intent: whatever reaches the socket can set it. 34
   marks cld's servers instead.
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
    6. tmux gets the environment cld got, but for `TERMINAL_EMULATOR`, which `new` removes (since
       33 with the other variables that name the terminal to claude), and `TMUX`, emptied (see
       decision 2). bash had also changed it on the way (see Findings): it
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
    so sessions need no mark (see Findings; 34 marks servers). 9's reasons against this no longer
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
       `tmux -L cld-A kill-server`, which ends `a` - or, in `kill`, end it (see below). So where the server they reach runs without its session, they ask it for
       the socket it was started on (`#{socket_path}`), and if that names a NAME that differs only
       in case, they refuse the name as clashing with that session, pointing at no kill.
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
    pointing at `tmux -L cld-NAME ls` and `cld kill -n NAME -s SUFFIX`, so that every claude gets
    the environment of the shell that ran `cld new` or `cld resume`; `list` shows nothing for such a
    server, whose session has ended, and `join` refuses the name the same way. `kill` ends the
    server, with `kill-server` alone since there is no session to end first, and prints nothing,
    as for a session: what claude started through tmux ends with it, as `kill`'s help says, and
    as it did where claude failed and its dead pane kept the session. cld 0.8.2 and earlier refused
    the name in `kill` too, and all four pointed at `tmux -L cld-NAME kill-server` (#69). `kill`
    ends only a server that has sessions, none of them `cld-NAME`, and whose `#{socket_path}` is
    `.../cld-NAME`, a format tmux expands (see Findings): a server without a session is one that a
    `cld new` is starting - it answers before `new-session` has made the session - or one exiting,
    and one with `cld-NAME` by then has had it made since the lookup; `kill` leaves them, and all
    four refuse the name, pointing at `tmux -L cld-NAME ls` alone. Its `kill-server` runs under
    the same format, `if -F` in its tmux command, so that a server that has changed since `kill`
    read it - a fresh one a `cld new` started on the socket, with its session - is left, and `kill`
    ends nothing and says nothing, as where it had come first. A session renamed - by hand, or by
    a `tmux rename-session` claude runs, which renames its pane's session - is not the one its
    server is named after (as in cost 4), and `kill` ends its server, claude with it. Cost 6 has
    a server that is another session's. The interactive list's Ctrl+X ends such a server too, on
    the row of the session it outlived (15.3). Since 34 `kill`'s format requires cld's mark too. `new`,
    `resume` and `join` refuse a terminal that is a live pane of any of cld's servers (see 2),
    found through the socket `TMUX` names, and say whose session's server it is; the terminal of any other tmux nests without a check. The
    options stay as they were, the fixed `terminal-features` indexes too: `new` sets them on a
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
       `cld kill` has between its lookup and the kill. `cld kill -n NAME` goes on killing by name.
       A server that has outlived the session on the row (13) has no pane of it left to check: the
       list ends it as `cld kill` does, whichever claude of that name left it - a kill of that
       claude's session would have ended the server all the same;
    4. one kill step: the list and `cld kill` run the same code, `End` in `internal/session` -
       after the name's check, the lookup, then `kill-session -t =cld-NAME` and `kill-server` in
       one tmux command, or `kill-server` alone, under `if -F`, on a server that has outlived the
       session and a refusal of any other server that runs without it (13). `End`
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
    10. A conversation that Claude Code's agent view moved to its background sessions is left to
        claude. `/bg` and "Move to background and exit" in `/exit`'s dialog exit claude with
        status 0, which ends the session (5); after `←` on an empty prompt claude stays in the
        session, and `kill` ends it, not the copy that claude's daemon runs on, whether or not
        `Esc` took claude back to the conversation (see Findings). `resume` by the name finds the
        copy, which claude refuses while it runs: cld neither stops it nor passes
        `--fork-session`, as it guards no conversation open elsewhere (16.6). `list` does not mark
        a session whose conversation moved: that would take `claude agents --json`, and `list`,
        whose way of reading the names completion shares (17), runs no claude. The copy keeps the
        title's hooks (25, 26), which name the server's socket and the session: until the copy
        stops, they set the status of any session of the name that runs, a later `new`'s too, and
        fail while none does; cld leaves that as well. The README and the user guide say how to
        bring the conversation back: `claude attach ID`, or `claude stop ID`, then `kill` where
        the session stays, and `resume`.
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
       servers it takes (19.8), and since 28 `--permissions` its sets. `new -n` offers none: it
       refuses a name a
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
    Since 28 the settings are `$schema`, what `--permissions` allows and `plansDirectory`, and
    `.gitignore` gets `/.claude/settings.local.json`, `/.claude/plans/` and `/.claude/worktrees/`.
    Settled with it:
    1. the settings are those of this repository's `.claude/settings.json`: `--mcp goland` (since
       28 with `--permissions cld`) writes it byte for byte, and its `.mcp.json`, which the tests
       check, so a change to either goes
       with one to `internal/project`. They are Go data there - the allow list and the other keys
       - rather than an embedded copy of the file, so that each server's entries go where the
       file has goland's. They include the maintainer's tools (`dotnet`, `go`, `make`,
       `docker build`, `gh pr merge`) and `theme`, which are nobody else's defaults: a project
       edits them afterwards, and cld adds them back only when it runs there again, since it never
       removes anything (see 4) - until 28, which put the list behind `--permissions cld` and
       dropped `theme`;
    2. `--mcp SERVER`, given again or separated by commas (`--mcp goland,jbcontext`), takes
       `goland`, `jbcontext` or `rider`; any other name, the empty one included (`--mcp goland,`),
       is a usage error. Each server is written once, in that order, whatever the order given. A
       server is its entry in `.mcp.json`, its name in `enabledMcpjsonServers`, without which
       claude asks before it starts the server (see Findings), and `mcp__NAME` in
       `permissions.allow` (since 28 with `--permissions cld`, and with `read-only` its tools that
       only read, 28.5), which Claude Code's docs have match every tool of the server, so that
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
       which claude reads - where their values differ (since 28 only where the file lacks them),
       however they are written (`1.0` is `1`, an
       object's members in any order); adds the entries `permissions.allow` and
       `enabledMcpjsonServers` lack, after theirs; replaces a server's entry in `.mcp.json` that
       differs, whole, since a merge would keep a stdio server's old `args` beside `mcp`, or a
       `command` beside a `url`; and keeps everything else: other keys, entries and servers,
       `permissions.deny`, their order, indentation and values, byte for byte, and the file's
       mode and a symbolic link to it. It removes nothing: a server given before stays when it is
       not given again. `.claude/settings.local.json`, someone's own, is only made, where nothing
       is - a symbolic link to no file counts as something. What is missing is created, `.claude`
       included, 0644 and 0755 less the umask. Running it again changes nothing;
    5. `.gitignore` gets `/.claude/*` and then `!/.claude/settings.json` (until 28, 28.1), where
       they are not there
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
       same (`git check-ignore -v -z --stdin`; since 28 also what else a project shares under
       `.claude`, of which it warns, 28.2): a `.claude/` or `.claude` elsewhere - before cld's
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
       whole name where `NAME` leaves nothing (24.4), and `-n`'s calling its value a name
       (`invalid name 'a.b'`), no longer a session's; then the two together, which are refused
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
       and else `-s` alone - `session 'api-fix' exists; attach to it with cld join -n api -s fix`
       (since 37, `exists in DIR` for a session that records its home).
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
25. The tab's title follows claude's status: `✳ cld-S`, and `◐` and `◑` in turn in place of
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
       SOCKET if -F -t =cld-S: ... "set -t =cld-S: ..."` - and sets `@cld-status` on that
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
       `#{?pane_dead,✳,#{?#{==:#{@cld-status},busy},#{T:@cld-busy},✳}} cld-S`, the marker
       `@cld-busy` and the path `@cld-tmux` go on claude's session, not the server, as 5's options
       go on claude's pane: a session claude makes keeps tmux's. A claude that exited is not
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
    6. cld still prints `✳ cld-S` before tmux starts, to a terminal only (31), and the list
       before it hands the terminal over (14), which tmux replaces on attach with the session's
       title as it stands. A terminal that detaches keeps the title tmux set last - a busy marker,
       if claude was busy - and a session an older cld started keeps `✳ cld-S`, as it has neither
       the hooks nor the title;
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
    ` [w]`, as in `✳ cld-S [w]`, and loses it as claude leaves. It follows claude as 25's
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
28. Project settings a team can share: `cld setup project` writes what a project shares, and none
    of one developer's settings. It wrote this repository's own into any project - a theme, an
    update channel, and an allow list that amounts to running commands without a prompt - and a
    `.gitignore` that kept all of `.claude` but the settings out of git, what claude has a project
    commit among it: a new skill showed in no `git status`, and cld exited 0 (see Findings).
    Settled with it:
    1. `.gitignore` gets `/.claude/settings.local.json`, `/.claude/plans/` and
       `/.claude/worktrees/`, each where it is not there, anywhere in the file, as git reads it:
       without the leading slash too, and with a carriage return or trailing spaces, but not a
       tab. What is missing goes at the end, in the file's line endings, as in 19.5. git ignores
       one developer's settings, the plans that `plansDirectory` keeps in the project, and
       `claude --worktree`'s worktrees, and adds the rest of `.claude` - `commands/`, `agents/`,
       `skills/`, `rules/`, `hooks/`, `CLAUDE.md` - which Claude Code's docs have a project
       commit. claude keeps its other runtime files out of git itself, `worktrees/` among them
       (see Findings); the line is written all the same, as it does not rest on when claude
       writes its own;
    2. cld removes no line: a project with `/.claude/*` and `!/.claude/settings.json`, which cld
       wrote before, keeps them, and gets the new lines beside them. In a git work tree cld then
       asks git (19.7) about each path a project shares under `.claude`, and after the report
       warns of those git ignores, a warning a pattern: `cld: warning: git ignores
       .claude/commands/, ... and .claude/CLAUDE.md, which a project shares through git, by the
       pattern /.claude/* (.gitignore, line 1)`. `.claude/settings.json`, which cld writes to be
       shared, still ends it with status 1. A directory is asked about with its slash, so that git
       takes it for one before it exists, and with `--no-index`, so that git answers where the
       project added files in it with `git add -f`: a new one there is ignored all the same - but
       not a submodule, which the index holds as one entry (mode 160000, `git ls-files --stage`),
       checked out or not, and whose files are its own repository's, out of the project's
       patterns' reach; cld asks the index about the directories `--no-index` answers for. A
       path there as something else - a symbolic link to a directory of skills shared between
       projects, say, which git keeps as a link, and with the slash refuses, answering for no path
       (see Findings) - is asked about as the files are: without the slash, and with the index,
       since a file git tracks is shared whatever pattern matches it. Not taken: rewriting the old
       lines, which a project may have made its own, and a check of each file under the
       directories;
    3. the settings are `$schema`, `permissions.allow` and `plansDirectory`, and with `--mcp`
       `enabledMcpjsonServers`. `theme`, `autoUpdatesChannel`, `autoMemoryEnabled` and
       `autoCompactEnabled` are gone: the first two are a person's, which the project's file would
       override in each developer's `~/.claude/settings.json`, and all four restate claude's
       defaults (see Findings). cld never replaces a value the file has, `$schema` and
       `plansDirectory` included: it adds the keys and entries the file lacks (19.4 otherwise). A
       project that ran an earlier cld keeps the four keys until it removes them;
    4. `--permissions SET` picks what `permissions.allow` gets. `read-only`, the default: `Read`,
       and `Bash` prefix rules for `ls`, `pwd`, `cat`, `head`, `tail`, `wc`, `grep`, `stat`, `du`,
       `which` and `git status` - commands none of whose options runs another command, as
       `find -exec` and `rg --pre` do, or writes a file, as `sort -o` and `tree -o` do. A prefix
       rule admits every option (see Findings), so `git diff`, `git log` and `git show` are left
       out: their `--output FILE` writes any file, `.git/config` among them, whose
       `core.fsmonitor` the next `git status` runs - a claude that a prompt injected would run any
       command without a prompt. claude runs the three without asking with the options it checks,
       and asks for the others. `cld`: this repository's list, one developer's,
       which lets claude edit files and run `git`, `go`, `make`, `docker run` and more without a
       prompt. `none`: nothing, and cld does not read `permissions`. claude asks for no bare
       read-only command anyway (see Findings), so `read-only` saves prompts for their other
       options and for reading outside the project; what matters is that it is safe to share,
       since whoever accepts the folder's workspace trust gives claude what the file allows. Any
       other SET, the empty one included, is a usage error, checked after `--mcp`'s servers;
       completion offers the three, each described;
    5. a server allows what the set does of it: with `cld`, every tool (`mcp__NAME`) and, for
       `jbcontext`, `Bash(jbcontext:*)`; with `read-only`, `mcp__NAME__TOOL` for each tool GoLand
       2026.2.3's server marks `readOnlyHint` - Rider's too, the same platform's server, not
       probed - and `Bash(jbcontext search:*)` and `jbcontext mcp`'s one tool, `code_search`
       (see Findings); with `none`, nothing. An entry for a tool a server lacks allows nothing,
       and a tool it adds later is asked for until cld's list has it;
    6. this repository's `.claude/settings.json` and `.mcp.json` are what `cld setup project --mcp
       goland --permissions cld` writes, byte for byte, which the tests check (19.1); its
       `.gitignore` was changed by hand to the new lines. The README and the guide say what the
       command writes, and to review it before committing;
    7. the tests: each set, with servers and without, where there is nothing; a file with the
       keys already, other values in them, a key given twice, and `none` beside a `permissions`
       that is no object; the new lines as git reads them, with `git status` adding the shared
       files and ignoring the personal ones; and the warnings - `.claude/`, the old lines with
       and without exceptions after them, patterns that ignore some of the paths, git's excludes,
       a symbolic link to a directory of skills, files added with `git add -f`, and submodules,
       checked out and not, beside a repository that is none.

    Out of scope: other MCP servers (19), removing the old lines or keys, and sets of a project's
    own.

29. Notifications (#59): claude's notifications reach the terminal once its setting
    `preferredNotifChannel` names a channel the terminal takes, which cld leaves to the user. Its
    default, `auto`, goes by `TERM_PROGRAM`, which tmux sets to `tmux` in claude's pane, and sends
    nothing there (see Findings): the README's "notifications ... work" held only for a channel
    set by hand. Settled with it:
    1. cld sets no channel. tmux knows the terminal attached (`#{client_termtype}`, C2), and the
       channel could go into cld's `--settings` where the user set none; but `--settings` outranks
       the user's settings, and a terminal that joins the session later, or beside the first
       (23), may take another sequence than the one that started it;
    2. the README and the user guide say so, with the channel for each terminal. The server's
       `allow-passthrough on` passes a channel's sequences on, and tmux's defaults the bell, to
       every terminal attached to the session;
    3. the tests: C5 has the probe notify on the channels `iterm2`, `kitty`, `ghostty` and
       `terminal_bell` as claude 2.1.284 writes them under tmux, and checks that each terminal
       gets the sequences without the passthrough, and the bell. Which channel `auto` picks is
       claude's, read in its bundle: the probe does not pick one.

    Out of scope: a notification while no terminal is attached, which tmux drops (see Findings).

30. Links (#60): claude's links - the file paths and URLs it marks with OSC 8 under tmux 3.4 or
    newer - reach the terminal as links, for the terminal to open with its own click. tmux
    writes a link only to a terminal with the `hyperlinks` feature, and gives that by XTVERSION
    to iTerm2, foot and tmux alone (see Findings): in kitty, Ghostty, WezTerm, Alacritty, VTE's
    terminals, Konsole, Windows Terminal, VS Code and JetBrains' terminal they were text.
    Settled with it:
    1. cld gives the feature by `TERM`, in `terminal-features` entries at fixed indexes past
       tmux's defaults, as it gives `extkeys` (13): `xterm*:extkeys:hyperlinks` at 100, the entry
       that had `extkeys` alone, `wezterm:hyperlinks` at 101 and `alacritty:hyperlinks` at 102.
       `xterm*` covers kitty's `xterm-kitty`, Ghostty's `xterm-ghostty` and the `xterm-256color`
       of the others; `wezterm` is WezTerm's `TERM` where its `term` says so, and `alacritty`
       Alacritty's where that terminfo entry is installed. foot, whose `TERM` is `foot`, tmux
       knows by XTVERSION. An entry's features are separated by `:`: with `,` tmux takes neither;
    2. a terminal that takes no links shows their text, as before: one that parses OSC as
       ECMA-48 ignores a code it does not know, as xterm does. Those that garble links - VTE up
       to 0.48.1, Windows Terminal up to 0.9, Emacs's terminal, screen with long URIs - are old
       releases, or have no `xterm*` `TERM`;
    3. the click that opens a link is the terminal's, not claude's, so it does not wait on
       Ctrl+click reaching claude (35); with `mouse on` the terminal reports clicks to tmux, and
       where it opens a link only with the modifier that keeps a click from the program, that
       modifier it takes. Which click opens a link in each terminal was not checked (see
       Status). A session that an older cld started keeps its links text;
    4. the tests: C2 expects `hyperlinks` on both terminals, and C5 a link the probe writes, as
       claude does, in the terminal's output. The baseline terminal, an outer tmux, has the
       feature from XTVERSION whatever cld sets; JediTerm has it from cld's `xterm*` entry, and
       failed both without it. `TestServerOptions` checks the three entries, one each after a
       second `cld new` set them again.
31. A terminal to attach from (#63): `new`, `resume` and `join` refuse to hand tmux anything but a
    terminal it can draw on. Run from cron, `ssh host cld new` without `-t` or a script whose
    input is not the terminal, they printed the title to stdout and became tmux, which failed with
    a message of its own and status 1, and for `new` and `resume` left the socket of the server it
    had started first, which `list` and every TAB went on asking (see Findings). Settled with it:
    1. what: cld's stdin is a terminal (`term.IsTerminal`), which tmux takes for the client's, and
       `TERM` is set, not empty and not `dumb` - the list's conditions on stdin and `TERM` (14.1),
       without stdout, which tmux does not draw on, and the foreground. Otherwise status 1 and
       `cld: new needs a terminal, and its input is not one`, `..., and TERM is not set`,
       `..., and TERM is empty` or `..., and TERM is dumb`, with `resume` or `join` for `new`. A
       `TERM` that terminfo does not know stays tmux's to refuse, socket and all: cld reads no
       terminfo;
    2. when: once every other check has passed - the name, the tools and their versions, the
       lookup (a session that exists or none, a lingering server), the directory and `-w`'s
       repository - just before the title, so that each of those says what it said without a
       terminal too. `Attach`, the rest of `join`, which the list's Enter takes too, makes it
       there: where the list runs it passes;
    3. the message says what is missing, and no more: most ways into it involve no ssh, so
       `ssh -t` is in the user guide's Troubleshooting instead;
    4. the title goes to stdout only where stdout is a terminal: tmux draws on stdin's terminal,
       and a pipe or a file took the escape in as text (`cld new | tee`, say). `$(cld new)` in a
       shell on a terminal is no refusal: its stdin is the terminal, and claude runs there, while
       the substitution no longer captures the title;
    5. not done: a detached start (`cld new -d`, as `claude --bg` has), a feature of its own, and
       the check of the foreground that the list makes (14.1);
    6. the tests: `TestRefusesWithoutATerminal` runs each command without a terminal, and on one
       with `TERM` `dumb`, empty or unset - status 1, the message, nothing on stdout, no socket
       left by the real tmux for `new` and `resume`, no attach recorded by the fake tmux for
       `join` - and fails before the change; `TestTitleOnlyToATerminal` hands over with stdin a
       terminal and stdout a pipe, with nothing written to either. The tests that hand over to the
       fake tmux run cld on a pseudo-terminal of the test's (`RunCldOnTerminal` in
       `tests/internal/sandbox`), its stdin, stdout and controlling terminal, and read the title
       from it; `TestFailedWriteEndsCld` has `new`, `resume` and `join` write the title to a
       terminal open for reading only. The tests that ran them without a terminal for a refusal
       that comes first still do, and pin the order (2).

32. What a kill does to claude (#66): `kill`, and the list's Ctrl+X (15), end claude as a terminal
    that closes does, and say so. The SIGHUP of its pty hanging up is claude's graceful shutdown:
    it kills the shell commands still running, runs its `SessionEnd` hooks with the reason
    `other` - for 1.5 s, longer where a hook's `timeout` asks, 60 s at most - and exits 129 (see
    Findings). The kill's tmux command does not wait for claude, and claude, orphaned, may still
    run its hooks once `cld kill` returns: those of a script standing in for claude ran 1.5 s
    past its return, and what the script wrote to its terminal on the way out failed. `kill`'s
    help, the user guide and `End`'s comment say so. Settled with it:
    1. `End` does not wait for claude. It could wait for the pids it reads (`#{pane_pid}`, 15.3)
       to exit, bounded only by claude's failsafe, which claude moves as it goes (see Findings):
       some 70 s past the signal where a hook's `timeout` asks for 60 s, and half a minute more
       while its output drains, or more where `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS` sets
       more. But `cld kill`, and the list, would then hang for as long as claude takes to shut
       down, which no terminal that closes does. The overlap that waiting would avoid, a
       `cld kill && cld resume` whose new claude starts beside the old one's hooks, is named in
       the user guide;
    2. no `send-keys /exit` before the kill, for the reason `prompt_input_exit`: the keys land
       wherever claude's input is - a dialog, a prompt half typed - and in a `-w` session `/exit`
       opens the dialog that asks whether to keep the worktree;
    3. the tests: none of claude's shutdown, which the tests' probe does not have; `TestHelpText`
       compares `kill`'s help, with its new sentence, with `testdata/help`.

33. The terminal's variables (#67): `new` and `resume` leave out of the environment they run tmux
    with every variable that claude reads before `TERM_PROGRAM` to tell its terminal, but for
    `TERM` - until then `TERMINAL_EMULATOR` alone (see Starting point): `CURSOR_TRACE_ID`,
    `__CFBundleIdentifier`, `VisualStudioVersion`, `TERMINAL_EMULATOR`, and VS Code's askpass with
    `VSCODE_GIT_ASKPASS_MAIN`. tmux sets `TERM` and `TERM_PROGRAM=tmux` in a pane, but a server
    keeps the rest of the environment of the client that started it, and claude reads each of
    those first (see Findings): a session created in Cursor's terminal, or in a JetBrains IDE's on
    macOS, left its claude - and what claude starts through tmux - taking itself to be in that
    terminal, one it knows no extended keys for: claude asked tmux, which does not answer, and
    left them off, so Shift+Enter submitted from whatever terminal joined. Settled with it:
    1. each goes whatever its value: it names the terminal the session was created in, which any
       other can join. `__CFBundleIdentifier` names every macOS app a shell runs in, iTerm2 too,
       which claude also reads for `/terminal-setup`'s iTerm2 clipboard offer and for computer
       use's terminal app (see Findings): claude no longer takes either from the terminal a
       session was created in;
    2. VS Code's askpass goes as a unit: `VSCODE_GIT_ASKPASS_MAIN` names Cursor, Windsurf or
       Antigravity, and `GIT_ASKPASS`, VS Code's script beside it, runs it with
       `VSCODE_GIT_ASKPASS_NODE` and `VSCODE_GIT_ASKPASS_EXTRA_ARGS` to ask through
       `VSCODE_GIT_IPC_HANDLE`, the socket of the window the session was created in; a part left
       without the rest is an askpass that fails. So every `VSCODE_GIT_ASKPASS_*` goes, with
       `VSCODE_GIT_IPC_HANDLE`, and `GIT_ASKPASS` where its directory is
       `VSCODE_GIT_ASKPASS_MAIN`'s; a `GIT_ASKPASS` elsewhere is the user's own, and stays. VS
       Code's git editor, which `git.terminalGitEditor` gives, asks through the same socket and
       fails at once without it (see Findings), so it goes as a unit too, though it names no
       terminal to claude: every `VSCODE_GIT_EDITOR_*`, and `GIT_EDITOR` where it names, in double
       quotes or not, a script in `VSCODE_GIT_EDITOR_MAIN`'s directory; a `GIT_EDITOR` elsewhere
       stays. git on the server then asks as it would outside VS Code, not in a window that may
       have closed. claude's Bash tool runs git with `GIT_EDITOR=true` whatever it got (see
       Findings): the editor mattered to what else runs on the server, a shell in a window of its
       own;
    3. the rest of the environment stays as the shell that ran `new` or `resume` had it (13), for
       claude's life: `join` gives tmux the `update-environment` variables of its terminal,
       `SSH_AUTH_SOCK` and `DISPLAY` among them, for what starts on the session later, not claude
       (see Findings). The user guide says so, with the ways around it;
    4. not done: leaving out everything claude's own background sessions drop - `LC_TERMINAL`,
       `ITERM_SESSION_ID`, `KITTY_WINDOW_ID`, `WT_SESSION`, `SSH_CONNECTION` and more (see
       Findings). claude reads those after `TERM_PROGRAM`, or for other things than the terminal
       it takes itself to be in (the maintainer chose the variables that change that one);
    5. the tests: `TestNewTmuxCommand` with the variables in cld's environment,
       `TestVSCodeGit` with VS Code's askpass and editor, its askpass and an editor of the user's
       own, and a `GIT_ASKPASS` and `GIT_EDITOR` of the user's own beside them and alone, and
       `TestClaudeNeverSeesTheTerminal` for every one of them in claude's environment and the
       server's (C6); what was not run is in Status.

34. cld's servers are marked (#71): a tmux server named `cld-NAME` that cld did not start - the
    user's own `tmux -L cld-outer`, say - is none of cld's, whatever its sessions are called.
    13 went by the socket's name alone: in a pane of such a server `new`, `resume` and `join`
    refused, pointing at a `C-q d` that does nothing there (see 2), and `list` printed its table;
    from anywhere, `new`, `resume`, `join` and `kill` pointed at `tmux -L cld-NAME kill-server`,
    which ends the user's sessions - and once `kill` ended a server that outlived its session
    (13, #69), `new`, `resume` and `join` pointed at `kill`, which ended them without a word - and
    a session there that spelled out cld's scheme was listed, offered, found and killed (see
    Findings). Settled with it:
    1. `new` and `resume` set the server option `@cld` to `1` in the tmux command that starts the
       server, one `set` before the others: no other process. A format finds a user option in the
       server's options before any other (see Findings), so nothing that claude's tmux sets on a
       session or a window hides it;
    2. cld reads the mark, `#{||:#{@cld},#{==:#{prefix},C-q}}`, in the formats it already runs.
       The `list-sessions` filter for session `cld-NAME` in `lookup` and `Sessions` requires it
       too, so `list` and completion pass over an unmarked server, and `new`, `resume`, `join` and
       `kill` find no session there; `lingering` reads it, with `display-message -p`, before its
       check of a server that has outlived its session (13) and `#{socket_path}`, and refuses the
       name with `tmux server cld-NAME is not one of cld's; use another name`, pointing at no
       `kill`, before it looks for a name that differs only in case (13.6). That check, under
       which `kill`'s `kill-server` runs again, requires the mark too: a server that loses it
       after the read - one of the user's own started on the socket since - is left, as one where
       the session has been made since (13). `OwnPane`'s
       `list-panes` names the live panes' ttys on a marked server only, so `new`, `resume` and
       `join` nest in any other, and `list` is interactive there. `new`'s next index (24) still
       counts an unmarked server, whose name `new` would refuse;
    3. the servers that cld 0.8.2 and earlier started have no `@cld`: the prefix `C-q` they set
       marks them instead, so an upgrade keeps every session, where the user guide would have had
       to say to end them, as for 13. tmux's default prefix is `C-b`; a server of the user's own
       named `cld-NAME` whose prefix is `C-q` counts as cld's. The prefix is a session option: a
       session that sets its own reads that, where cld's sessions keep the global one (see
       Findings);
    4. like 9's, the mark guards against mistakes, not intent: whatever reaches the socket can set
       it. It is not 9's mark: that one told cld's sessions from the others on the one shared
       server, which 13 does by the server's name; this one tells cld's servers from the others;
    5. the tests: `TestLeavesAForeignServerAlone` starts servers `cld-x`, holding `other`, and
       `cld-y`, holding `cld-y`, which `list` leaves out and `new`, `resume`, `join` and `kill`
       refuse, their sessions left running, and one as cld 0.8.2 started them, whose session
       `list` shows, `new` finds and `kill` ends, and one of cld's that outlived its session,
       whose mark a tmux first on the `PATH` takes away before `kill`'s `kill-server`, which
       `kill` leaves running; `TestNestsInsideAnotherTmux` runs `new` and `list` in a pane of
       `cld-outer`; `TestCompleteNames` has an unmarked server `cld-own`; and `TestServerOptions`
       reads `@cld`. The tests that make cld's sessions or servers by hand, `TestLingeringServer`'s
       among them, mark their servers with `set -s @cld 1`.

35. Clicks with a modifier reach claude (#73): cld's server unbinds tmux's `C-MouseDown1Pane` and
    `M-MouseDown3Pane`. Of tmux's mouse bindings on a pane only these two - `swap-pane -s @` and
    the pane menu - take the press without asking whether the pane's program takes the mouse, so
    a Ctrl+click reached claude as its release alone, and an Alt+right-click not at all. claude
    opens a link only on the release of a click whose press it saw (read in its bundle, see
    Findings), so under cld a Ctrl+click would open none. Settled with it:
    1. `new` and `resume` run `unbind -n C-MouseDown1Pane ; unbind -n M-MouseDown3Pane` with the
       server's other options, after `mouse on`: tmux hands a mouse key it has no binding for to
       the pane, which is claude's where claude takes the mouse, and goes nowhere where it does
       not. Unbinding a key that is not bound is no error, so the options set again by a second
       `new` for the name still end in `new-session`'s `duplicate session`;
    2. the session has one pane unless claude splits it, as its agent teams do for teammates in
       tmux panes (claude 2.1.284's `TmuxBackend` runs `split-window -d -t PANE -h -l 70%` on
       claude's pane, read in its bundle), and key tables are the server's: the panes and
       sessions claude makes there lose the two bindings as well, and a teammate that takes the
       mouse gets the clicks. That is accepted: `C-q {` and `C-q }` still swap panes, and `C-q >`
       opens the pane menu, as a right-click does over a program that does not take the mouse.
       Bindings that ask, as tmux's others do
       (`if -F '#{mouse_any_flag}' { send-keys -M } { ... }`), would keep the two for such a
       program, at the cost of a copy of tmux's pane menu in cld;
    3. the rest of what `mouse on` does over a program without the mouse - claude drawing in the
       main screen, a claude that failed - is tmux's own and stays: a drag selects in copy mode, a
       middle-click pastes, a right-click opens the menu. So does what claude makes of a click
       under tmux: claude opens a link on a Ctrl or Alt click alone, as it knows Ghostty's plain
       click and the Cmd+click of Ghostty and Warp on macOS by `TERM_PROGRAM` and XTVERSION,
       which are tmux's there (see Findings); the terminal's own click on the links tmux passes
       it (30) is the terminal's. The user guide says so, and how to select with the terminal
       instead; a session an older cld started keeps tmux's two bindings until it ends;
    4. the tests: `TestServerOptions` finds neither key in the root table, `MouseDown1Pane` still
       there, and the second `new`'s `duplicate session`; C4 (`TestContractClicks`) types a
       Ctrl+click and an Alt+right-click and waits for the press and the release of each in
       claude's input, on each terminal.

36. Scrollback (#77): a server keeps 50000 lines of a pane's history (`history-limit`), where
    tmux keeps 2000. While a terminal is attached, tmux draws in its alternate screen, and the
    terminal's scrollback gets nothing (see Findings): what claude's classic renderer leaves in
    the scrollback without tmux - the whole conversation - is in the pane's history alone, which
    the wheel (C4) and `C-q [` open in copy mode. The fullscreen renderer scrolls its own
    transcript, and fills the history only when `Ctrl+O`, then `[`, writes it out. Settled with
    it:
    1. the option goes with the server's options, before `new-session`: tmux 3.7 gives a pane
       that exists a new limit, but 3.5 and 3.6 keep a pane's limit from when it was made (see
       Findings), so that it holds on the oldest tmux cld runs on (6) too;
    2. 50000 lines, as the issue proposed: the server's memory grows with the history, to
       about 35 MB for 50000 lines of 100 plain characters and 156 MB where each has an RGB colour
       (see Findings), and is freed with the session. The sessions claude makes on its server get
       it too;
    3. the README says which renderer the wheel and the history are for, and the user guide how
       to read the history back, for screen readers too: Claude Code's screen-reader mode, always
       the classic renderer, relies on the terminal's scrollback and on OSC 133 marks, which tmux
       keeps to itself (see Findings);
    4. not done: binding copy mode's `previous-prompt` and `next-prompt`, which would jump between
       the turns that claude marks in screen-reader mode; and keeping tmux out of the terminal's
       alternate screen (`terminal-overrides` without `smcup`), not tried;
    5. the tests: `TestServerOptions` pins the option and the limit of claude's pane, and
       `TestNewTmuxCommand` the option's place in tmux's command.
37. Sessions of another repository (#80): repositories of one name share `NAME` and its indexes
    (24.1, 24.3) - two clones of a project, a fork beside its upstream, generic names such as
    `api` - and so do directories of one name outside a repository. With cld 0.8.0 and tmux 3.7c,
    from `scratch/api`, `cld join -s 0` attached to the claude of `work/api`'s `api-0`, and
    `cld kill -s 0` ended it, printing nothing. Settled with it:
    1. `new` and `resume` record where they made the session, its home, as `@cld-home` on
       claude's session, beside `@cld-tmux` (25.4): the directory whose name `NAME` defaults to -
       the one that holds the repository's common `.git`, or its git directory (24.3), or outside
       a work tree the current directory - by the path that gives the name, with `-n` too, as the
       session belongs where it was made whatever its name. It goes as a word of its own, with a
       `\` before a `;` at its end, as the directory does (see Findings); `set` does not expand
       it, so its `#` stay single;
    2. `join` and `kill` refuse with status 1, before they attach or kill, a session whose
       `@cld-home` is set and is another directory than the current one's home, where `-n` was
       not given and `NAME`'s default is not `""`: `session 'api-0' belongs to /work/api, not to
       this repository; name it with cld kill -n api -s 0`, or `not to this directory` outside a
       repository. Homes whose paths differ are compared as files (`os.SameFile`): git names the
       common `.git` by its real path from a linked worktree, and by a path from `PWD`, which can
       go through a symbolic link, from the main work tree (see Findings), and a file system that
       ignores case finds one directory by paths in other letters (worked out, not checked on
       macOS). `-n` takes a session from anywhere, and so does `-s S` where `NAME` leaves nothing
       (24.5), and `cld list`'s Enter and Ctrl+X, which name a session whole; a session of cld
       0.8.2 or earlier, which records no home, is taken as before. So is a server that has
       outlived its session (13), which `kill` ends: the home went with the session. The
       maintainer chose that `join` refuse, as `kill` does, rather than warn and attach: a warning
       printed before tmux takes the terminal over would be gone as it attaches;
    3. the lookup reads `@cld-home` in the `list-sessions` that finds the session and its pids,
       `#{session_name} PIDS` then a tab and `#{@cld-home}`, which takes the rest: the check costs
       no tmux command. The lookup now runs with `-u`, as `list` does (13.1): under a locale
       without UTF-8, tmux would write the tab, and a home's characters other than ASCII, as `_`
       (see Findings);
    4. `new` and `resume` name the home of the session whose name they refuse:
       `session 'api-2' exists in /work/api; attach to it with cld join -n api -s 2`, and
       `exists in DIR, but its claude exited`; a session without a home, as before;
    5. `join -s`'s completion (24.8) offers only the sessions `join` takes: without `-n`, those
       whose home is the current directory's or unset. `list`'s read of the sessions takes the
       home with the rest: the home and the directory, both paths, which can hold a tab, end the
       line, after the home's length in bytes (`#{n:@cld-home}`), so the directory still takes
       the rest of it. `list` shows no home;
    6. names stay as they are. Making `NAME` unique on a collision - `scratch-api`, or a short
       hash - was the other way: a name would then hang on what else runs, and the same
       repository would get another name once the other's sessions ended; the maintainer kept
       names stable. The remote's name stays out, as 24.3 has it;
    7. the tests: two repositories `api`, one reached through a symbolic link, and a directory
       `api` outside a repository whose path holds a tab, with a session each and one of an older
       cld made by hand; `@cld-home` on each; `list`'s directories; the refusals of `join` and
       `kill`, from the other repository, the directory and a subdirectory, and `new` and
       `resume` naming the home; completion from each place, a subdirectory, a linked worktree,
       with `-n` and in `/`; the refusals and completion again under a locale without UTF-8,
       where they hang on the lookup's `-u` (3); the joins and kills that pass - from a linked
       worktree and a subdirectory of a session made through the link, with `-n`, in `/`, and for
       the older session. The command `new` hands tmux, word for word, has the home, one that
       ends in `;` and holds a `#` among them, and the fake tmux's `list-sessions` lines the
       home's length, `0`, before the directory.

    Out of scope: showing the home in `cld list`, a home for the sessions of older clds, and
    `new`'s index, which the repositories of one name still share.

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
- The baseline terminal types raw xterm input (`CSI 13;2u`, `CSI I`/`CSI O`, SGR wheel and
  clicks) through `send-keys -H`: tmux 3.3a does not know the key name `S-Enter` and types it
  literally, and an outer tmux reports focus changes only to panes of an attached client.
- tmux 3.3a expands `display -p -t =SESSION` to nothing when no client is attached; the tests read
  formats through `list-panes`.
- tmux 3.7 prints nothing for `list-keys -T prefix KEY`, and its `new-session -A` honours `-c`: a
  reattach from another directory moved the session's directory for new windows there. Since 0.2.0
  `join` attaches with `attach-session`, which moves neither claude nor the session.
- The test image builds tmux from source, the release `TMUX_VERSION` (see 6), on `debian:trixie`.
- tmux 3.7's `paste-buffer` writes control characters as `^X` unless given `-S`; the baseline
  terminal passes `-S`, since a terminal pastes them as they are, to a tmux 3.7 or newer, as the
  outer server's `#{version}` says. An older one writes them as they are, and refuses `-S`.
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
  outlives its session, or in `kill` to end it: `lingering` asks such a server, with
  `display-message -p`, for a format (`outlives`) that is 1 where the server has sessions, none
  `cld-NAME`, and its `#{socket_path}` is `.../cld-NAME`, and for that path, which tells a name
  that clashes in case; `End`'s `kill-server` runs under `if -F` with the same format, so that the
  check and the kill see one server. The format keeps to what tmux had before 3.6: `&&` of two
  operands, and `==` with 0 for a `!`. `noServer` takes three of tmux's messages for no server: `no server running on` (a stale
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
- The mark (decision 34) is a format, `mark`, that `only` - the filter of `lookup` and `Sessions` -
  joins to the session's name with `#{&&:...}`, and `outlives` - `kill`'s check - to the rest of
  its own; `lingering` prints it before that check and the socket's path, and `OwnPane` a pane's
  tty only under it. The fake tmux answers `list-sessions` as a marked server would, whatever the
  filter. The tests that hold the lookup match its filter up to the session's name,
  `'#{==:#{session_name},cld-a},'*`, and then its format.
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
  (`waitModes`), and so is mouse reporting before the wheel and a click (`WheelUp` and `Click`:
  the driver's `wheel-up` and `click` send nothing while it is off), as tmux turns every mouse
  mode off and on again after it draws (see Findings). Both races showed only under load; a sleep
  in the emulator thread whenever mouse reporting goes off, and before each read once the process
  has exited, made them fail every time.
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
  (since 28 with `--permissions cld`) writes with the repository's own `.claude/settings.json` and
  `.mcp.json`, which they read from the directory above `tests`. The write that fails, of
  `.gitignore` after the settings, comes from a symbolic link to a file whose path is 4095 bytes
  long, as for `setup telemetry`.
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
- The tests' pseudo-terminal (31), for cld to run on where no terminal emulator is needed, is the
  sandbox's `OpenPty`, without cgo: `/dev/ptmx`, then `TIOCSPTLCK` and `TIOCGPTN` on Linux, and
  `TIOCPTYGRANT`, `TIOCPTYUNLK` and `TIOCPTYGNAME` on macOS, as libc's `unlockpt` and `ptsname`
  do. A goroutine reads its other end from the start, as a terminal emulator would, so that
  nothing written there waits on the test; `Output` closes the test's copy of the terminal and
  returns what was written once no program has it open, when that read ends (with EIO on Linux).
  `RunCldOnTerminal` gives cld the terminal as a shell does: its stdin, stdout and
  controlling terminal, in a session of its own (`Setsid`, `Setctty`).

## What the tests found

JediTerm 3.76 (read from its source, confirmed by the contract):

- it answers no XTVERSION, so tmux records no terminal type and falls back to its defaults for
  `xterm*`: `bpaste`, `clipboard`, `focus`, `title` - but the emulator ignores focus reporting
  (DECSET 1004 is a stub) and does not handle OSC 52. Since cld adds the `extkeys` feature for
  `xterm*` (as Claude Code's tmux docs recommend), tmux also asks it for modifyOtherKeys, which it
  ignores;
- it takes OSC 8 links (`JediEmulator` handles them), which tmux writes to it only with the
  `hyperlinks` feature cld adds for `xterm*` (30), as C5 sees;
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
- since 3.6 the default wheel binding hands the wheel to a program in the alternate screen whether
  or not it asked for the mouse, where earlier versions entered copy mode (see Findings).

The `VT10x` in Findings came from the probing shell, which carried
`TERMINAL_EMULATOR=JetBrains-JediTerm`: claude's `--debug` log read `extendedKeys=no (env:
terminal=pycharm, no answer)` - the leak `new` and `resume` guard against by leaving
`TERMINAL_EMULATOR`, and since 33 the other variables claude reads before `TERM_PROGRAM`, out of
the environment they run tmux with. From a clean environment the
real claude 2.1.281 put the pane in key mode `Ext 2`, and Shift+Enter inserted a newline (checked
by hand in a nested tmux).

## Status

- Baseline and JediTerm contracts run on tmux 3.7c and 3.5a, the newest release and the oldest
  cld runs on (Linux, Docker, built from source), and the baseline on Homebrew's tmux on macOS.
  3.6a passed the whole suite once, for #74 (see Findings), and 3.6 is not run in CI. While the
  minimum was 3.7 (#21 to #74, see 6) only 3.7c ran; before, 3.3a, 3.4 and 3.5a ran too. What
  the real claude makes of 3.5's Shift+A and Shift+Backspace (see Findings) was not checked: the
  floor at 3.5a keeps them from it.
- Links (30) are tested as tmux writes them to the baseline terminal and to JediTerm's emulator,
  and were probed with `TERM` `wezterm` and `alacritty` on a pty that answers nothing (see
  Findings). No real terminal was seen showing them, nor which click opens one.
- `setup telemetry` is tested against a fake docker on Linux, and on macOS only for its refusal;
  it was checked by hand against the real collector image (see Findings), with collectors of the
  debug exporter in the plugin's place: the plugin itself, and claude sending through the
  collector, are still to check.
- `setup project` was checked by hand with git 2.53.0 and `claude mcp list` 2.1.283 (see
  Findings), the ports from their variables included; a claude session calling the servers' tools,
  and whether a session, rather than `claude mcp list`, takes the variables from the project's
  `.claude/settings.local.json`, were not. Nor were 28's permission sets tried in a claude
  session: what claude asks for with each was read from its bundle, not run, and Rider's tools
  were not listed.
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
  git 2.47.3 in the Linux image, and with the git of the macOS runner, and so is the home that
  `join` and `kill` check (37); a home reached by a path in other letters, on a file system that
  ignores case, was not tried.
- A failed claude's options and hook on its pane (5) are tested with a pane split by hand; a
  teammate's pane that claude splits off was read in claude 2.1.284's bundle, not run.
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
  were run by hand without `TMUX` and `TMUX_PANE`, not by a real background worker. `/bg`, the
  `/exit` dialog and `←` make claude run a conversation in the background (16.10), and after `←`
  the worker outlives `cld kill` (see Findings); what made claude run one in a worker two seconds
  after it started, which the claude in the pane showed (see Findings), is not known. A copy's
  hooks after its session has ended were not seen: run by hand, they say that no server runs,
  and set the option on a later session of the name.
- Notifications (29): what each channel writes was read in claude's bundle, and the contract
  checks that the baseline terminal and JediTerm get it; a real claude notifying in a session,
  and iTerm2, kitty and Ghostty showing it, were not seen.
- claude's shutdown on `cld kill` (32) was read from the bundles of 2.1.283 and 2.1.284, and
  `cld kill` probed with a script in claude's place (see Findings); a real claude's `SessionEnd`
  hooks on `cld kill` were not run.
- The variables of 33 were read in claude's bundles, 2.1.282 to 2.1.284, and VS Code's in its
  git extension's source (see Findings), and the pane's environment probed on tmux 3.7c; claude
  was not run in the terminals of Cursor, VS Code or a JetBrains IDE, nor on macOS, where
  `__CFBundleIdentifier` comes from.
- A Ctrl+click on a link under cld was not run with the real `claude`: the tests see the press
  and the release reach the probe, and claude's handling of them was read in its 2.1.284 bundle
  (see Findings), as was where its agent teams put teammates in tmux panes, not run either.
  Selecting with Shift held, Option in iTerm2 and Fn in Terminal.app, as the user guide says, was
  not tried.
- The real `claude` was not run under a locale that names no UTF-8: that it draws its UI in UTF-8
  whatever `LC_ALL`, `LC_CTYPE` and `LANG` say was read in 2.1.284's bundle (see Findings). Only
  a stub claude was run under one, by hand on tmux 3.7c and in the tests through `cld new`,
  `resume`, `join` and the list's Enter. A terminal that does not take UTF-8 was not tried.
- The history of 50000 lines (36) was checked with programs that print lines, on tmux 3.5, 3.6a
  and 3.7c (see Findings); the real claude's classic renderer filling it, and reading it back with
  a screen reader, were not.
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
  `/resume cld-NAME` inside `cld new -n NAME`; whether a session that turned Remote Control on
  at startup, as cld's do, records its Remote Control session in the conversation; and
  `claude stop ID`, then `cld resume`, of a conversation that `/bg` or `←` moved - which of the
  two transcripts of the name claude resumes, or whether it opens its picker - and `Esc` in agent
  view after `←` (16.10). Until then the user guide and decision 16 go by Claude Code's docs, and
  by claude's `--help` and bundle (see Findings), and the user guide says which of it is not
  checked.
