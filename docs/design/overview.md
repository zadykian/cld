# Architecture overview

cld runs Claude Code in named sessions, each on a private tmux server of its own, so a
conversation can be detached and rejoined from any terminal. What it offers its users is in the
[README](../../README.md) and the [user guide](../guide.md). This file is the map of how it does
that, and of where the rest of the design record lives:

- the numbered decisions, a file each under `decisions/`, which the [index](../design.md) lists;
- the findings: probed behaviour, with the versions checked, of
  [tmux's sessions](findings/tmux-sessions.md), [tmux and the terminal](findings/tmux-terminal.md),
  [claude](findings/claude.md), [the terminals](findings/terminals.md) and
  [shells, tools and the system](findings/environment.md);
- [testing](testing.md): the test layers, the terminal contract C1-C10, CI and what is checked.

## Where it started

cld began as a function in `~/.bashrc`. It printed the title `✳ cld-NAME`, then ran
`tmux -L cld -f /dev/null` with a few options and `new-session -AD`, starting `claude --name`.
It changed nothing in the calling shell, so it did not need to be a function. It became an
executable that can be tested, versioned and installed: a bash script up to 0.3.0, then a Go
program ([decision 11](decisions/0011-go-and-cobra.md)). Like the function, `join` ends by
replacing itself with tmux (`execve`), so the terminal talks to tmux alone.

Two of the function's choices still shape cld. The server is private and reads no
`~/.tmux.conf`, which keeps cld's options away from the user's own tmux; since
[decision 13](decisions/0013-a-server-per-session.md) each session has such a server. And a
server keeps the environment of the client that started it, while claude trusts
`TERMINAL_EMULATOR` over `TERM_PROGRAM=tmux`. A server started from the JetBrains terminal made
every claude on it act as in JediTerm, Shift+Enter submitting, even when attached from iTerm2. So
tmux runs without the variables that name a terminal
([decision 33](decisions/0033-terminal-variables.md)).

## The server's options

A server gets these as `join` makes it, the reasons here unless a decision holds them:

- `@cld 1` marks the server as cld's ([decision 34](decisions/0034-servers-are-marked.md)).
- `extended-keys on`: tmux answers no kitty keyboard query, so claude falls back to
  modifyOtherKeys, which tmux forwards only with this on. That carries Shift+Enter.
- The `extkeys` feature for `xterm*`: tmux asks a terminal for modified keys only where it knows
  the terminal sends them, and it does not recognise every terminal that does. Claude Code's docs
  recommend it. It goes at a fixed index with `hyperlinks`
  ([decision 30](decisions/0030-links.md)): `set -a` would add a copy at each set.
- `mouse on` and `focus-events on`: claude probes both and hints when they are off. With the
  mouse on, the wheel over a program drawing in the main screen scrolls the pane's history.
  claude's fullscreen transcript gets the wheel either way.
- Ctrl+click and Alt+right-click unbound on a pane
  ([decision 35](decisions/0035-modifier-clicks.md)).
- `history-limit 50000` ([decision 36](decisions/0036-scrollback.md)), set before
  `new-session`: tmux 3.7 applies a new limit to existing panes, earlier releases only to new
  ones.
- `allow-passthrough on`: claude wraps its OSC 52 copies and notifications in tmux passthrough
  ([decision 29](decisions/0029-notifications.md)).
- `status off`: claude keeps the whole tab.
- `prefix C-q`: claude binds `C-b` and nearly every other Ctrl key, but not `C-q`. `C-q d`
  detaches, `C-q C-q` sends a `C-q`, and `cld detach` serves a terminal that keeps `C-q`
  ([decision 44](decisions/0044-detach-command.md)).
- `C-q s`, `C-q (`, `C-q )` and `C-q L` run cld to move the terminal between servers
  ([decision 51](decisions/0051-moving-between-sessions.md)).
- `remain-on-exit on` and a `pane-died` hook on claude's pane, so a failure stays on screen
  ([decision 5](decisions/0005-failures-stay-on-screen.md)).
- The title options on claude's session ([decision 25](decisions/0025-title-follows-status.md)).

What concerns claude alone, the failure's options and hook and the title's, goes on claude's pane
or session, never its window or the server. The other panes of its window, and the sessions claude
makes on its server, then behave as plain tmux.

## The packages

- `cmd/cld`: the command line on cobra, the commands' help texts, argument errors, and which
  checks run in which order.
- `internal/session`: the tmux side, and cld's record of its sessions. `switch.go` moves a
  terminal between servers, `record.go` keeps the record and `restore.go` is `cld restore`.
- `internal/picker`: `cld list` on a terminal. `cmd/cld` decides when it runs, and hands it
  join's checks and kill's steps.
- `internal/project`: `cld setup project`, a project's claude settings, `.mcp.json` and
  `.gitignore` ([decision 19](decisions/0019-project-settings.md)).
- `internal/telemetry`: `cld setup telemetry`, a local OpenTelemetry Collector in Docker and
  claude's settings pointed at it ([decision 18](decisions/0018-telemetry.md)).
- `internal/configfile`: edits in place the files the `setup` commands write. A file keeps what
  cld does not change, byte for byte, so one Claude Code wrote keeps its look.
- `internal/update`: `cld update` ([decision 21](decisions/0021-self-update.md)).
- `internal/completion`: `cld setup completion`, and the scripts `cld update` writes anew
  ([decision 22](decisions/0022-setting-completion-up.md)).
- `internal/restore`: `cld setup restore`, a systemd user unit that runs `cld restore`
  ([decision 48](decisions/0048-restore-after-reboot.md)).
- `internal/tool`: finds the programs cld runs on the `PATH`, and ends cld as a shell would when
  one cannot run ([decision 11.5 and 11.9](decisions/0011-go-and-cobra.md)).
- `internal/fail`: carries exit statuses up to `main`, the only place that exits.
- `internal/output`: prints cld's own output, its warnings and its notes.

Everything else is the test harness under `tests/`, `install.sh`, docs and CI.

### Ending and exit statuses

Only `main` exits. An error carries its exit status up, with the message to print, unless tmux
has printed its own. A tmux command that fails ends cld with tmux's status; a session lookup that
fails ends it with status 1. A program that cannot run ends cld with 127 or 126, as a failed
`execve` does. A failed write of cld's output is one of these ends too, so output cut short never
passes for whole. An error keeps the advice for the command line, such as ` (see cld help)`,
apart from its message: the list's footer shows the message alone.

### The command line on cobra

cld overrides cobra's defaults (cobra 1.10.2, pflag 1.0.9) where they would change its command
line ([decision 12](decisions/0012-help-from-cobra.md)). The comments in `cmd/cld` give each
reason.

- cld checks the first argument before cobra, which takes an unknown command for an argument of
  the root and runs `cld -n x join` as `join -n x`.
- Each command reads options only up to its first argument, where pflag would read them all.
- cobra's output goes to a buffer that cld prints, as cobra drops the error of a failed write.
- `version` is a command, `completion` is cld's own, and completion offers no file names
  ([decision 17](decisions/0017-shell-completion.md)).

## cld's record of its sessions

tmux forgets a session with its server, so cld keeps a record of its own in `$XDG_STATE_HOME/cld`,
by default `~/.local/state/cld` ([decision 40](decisions/0040-session-record.md)). A session's
entry holds its name, the directory claude started in and its conversation's ID; the file's time
is the entry's. Beside the entry are the claude and environment the server started with, the run
mark, the busy mark and the start mark ([decision 48](decisions/0048-restore-after-reboot.md),
[decision 50.4](decisions/0050-one-command-join.md)). Also kept: the highest index given each
`NAME-`, and a lock.

- `join` writes the entry and the environment once nothing is left to refuse, before tmux. A
  `run-shell` after `new-session`, in the same tmux command, makes the run mark and removes the
  busy and the start mark, so a failed `new-session` leaves no mark.
- claude's hooks write the conversation's ID into the entry and keep the busy mark. cld reads none
  of claude's transcripts.
- An entry or index older than 30 days, claude's default `cleanupPeriodDays`, goes, but for a
  session whose server runs.
- Where cld cannot write the record, it warns and makes the session all the same.

## A session's life

### Joining

`join` is the one command for a session ([decision 50](decisions/0050-one-command-join.md)). It
refuses bad combinations with status 2 before looking for any tool. In a pane of cld's servers it
moves the terminal instead ([decision 51](decisions/0051-moving-between-sessions.md)). Otherwise
it takes the record's lock and looks the session up on server `cld-NAME`, by its exact name. It
runs `claude --version` only where claude starts ([decision 6](decisions/0006-versions.md)), and
checks the terminal last ([decision 31](decisions/0031-a-terminal-to-attach-from.md)).

- A session that runs is attached with `attach-session`, beside the other terminals
  ([decision 23](decisions/0023-joining-beside-other-terminals.md)). It moves neither claude nor
  the session, where the bash function's `new-session -A` honours `-c` on tmux 3.7: a reattach
  from another directory moved the session's directory for new windows.
- A session that has ended comes back, claude resuming the entry's conversation in the entry's
  directory ([decision 16](decisions/0016-resume.md)).
- Otherwise `join` makes the session. claude goes to tmux as separate words, by the path cld
  checked, so tmux runs it directly, not through `sh -c`. claude's `--settings` carry cld's hooks
  and turn agent view off ([decision 47](decisions/0047-agent-view-off.md)).
- tmux takes a command of 16364 bytes at most, which the words after `--` count against
  ([decision 41](decisions/0041-claude-options.md)).

Every client that attaches, and every read of the sessions, runs `tmux -u`. Without it, under a
locale that names no UTF-8, tmux writes most of claude's interface as `_`.

The lock goes with cld's `execve`, before tmux has made the session. So an attached create leaves
a start mark, which a second `join` or `restore` waits on
([decision 50.4](decisions/0050-one-command-join.md)). `join` without `-s` reads no start mark. Its
index is above that of every entry of the NAME, so above that of any session another cld is
starting, whose entry is written before its mark.

### Running

claude's hooks set its status and worktree as options of its session, from which tmux sets the
tab's title ([decision 25](decisions/0025-title-follows-status.md),
[decision 26](decisions/0026-title-marks-worktree.md)). Each hook costs claude a wait
([decision 39](decisions/0039-hook-cost.md)). `list` connects to each socket of the socket
directory, then asks the servers that answer, eight at a time
([decision 38](decisions/0038-stale-sockets.md)).

### Ending

- claude exiting with status 0 ends its session; a failure stays on screen
  ([decision 5](decisions/0005-failures-stay-on-screen.md)).
- In one tmux command, `kill` removes the run mark, then ends the session and its server. It does
  not wait for claude's `SessionEnd` hooks ([decision 32](decisions/0032-what-a-kill-does.md)).
- A server that outlives its session is refused, or ended by `kill`
  ([decision 13](decisions/0013-a-server-per-session.md)).
- `list`, and `join` without `-s`, end sessions idle past `CLD_IDLE_DAYS`
  ([decision 46](decisions/0046-idle-sessions.md)).

The entry stays after each of these. `list` shows the session as `ended`, and `join` brings it
back, until its entry expires or the list forgets it
([decision 15](decisions/0015-killing-from-the-list.md)).

### After a reboot

A reboot ends every server without running a hook, so the run marks stay. `cld restore`, run by
the user's systemd at its start, brings back each entry with a run mark and no server, detached,
with the claude and environment it started with. A busy one gets a prompt to continue. A session
whose run mark is older than `CLD_IDLE_DAYS` stays ended, since tmux's idle times start again at a
restore ([decision 48](decisions/0048-restore-after-reboot.md)).

## Distribution

- A tag `vX.Y.Z` publishes a binary per platform, `cld-OS-ARCH` for Linux and macOS on amd64 and
  arm64, `cld.sha256`, and `install.sh` ([decision 20](decisions/0020-install-script.md)).
  `cld update` replaces cld with the latest release's binary
  ([decision 21](decisions/0021-self-update.md)), and `make install` builds one from a clone.
- The binaries are built with cgo off on the Linux runner. The Linux ones are static; the darwin
  ones link only system libraries. Go's linker signs the darwin/arm64 one ad hoc, which Apple
  silicon requires; none is notarized.
- Without cgo Go has no `ttyname`, so cld runs `tty` to name its terminal. It trims the newline
  after the name rather than cut it, as uutils' `tty` 0.8.0 (Ubuntu 26.04) prints none.
- Releases up to 0.3.0 published the script as `cld`, a download that fails now that a Go release
  is the latest.
- Completion comes from the binary, so the script always matches it: cobra's scripts ask
  `cld __complete` at each TAB. Releases publish no completion files, and `make install` installs
  none. A Homebrew tap is possible later; its formula would generate them with
  `generate_completions_from_executable(bin/"cld", shell_parameter_format: :cobra)`.
