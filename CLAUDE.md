# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`cld` runs Claude Code in named sessions, each on a private tmux server of its own
(`tmux -L cld-NAME -f /dev/null`), so a conversation can be detached and rejoined from any
terminal. The product is a Go program on cobra: `cmd/cld` is the command line (commands, their
help texts, argument errors), `internal/session` the tmux side and cld's record of its sessions,
`internal/picker` the interactive
`cld list` on a terminal, `internal/project` `cld setup project` (a project's
`.claude/settings.json`, `.claude/settings.local.json`, `.mcp.json` and `.gitignore`),
`internal/telemetry` `cld setup telemetry` (a local OpenTelemetry Collector in Docker, and
claude's settings pointed at it), `internal/configfile` edits the files the two write in place,
`internal/update` `cld update` (cld replacing itself with the latest release),
`internal/completion` `cld setup completion` (the completion script where bash, zsh or fish reads
it, and `cld update` writing it anew), `internal/restore` `cld setup restore` (a systemd user
unit that runs `cld restore`, which is `internal/session`'s), `internal/tool` finds the programs
cld runs on the `PATH` (and ends cld as a shell would when one cannot run), `internal/fail`
carries exit statuses up to
`main`, and `internal/output` prints cld's own output, a write that fails being one of those ends,
and its warnings and notes. `install.sh`, published with each release,
installs cld from a release. Everything else is its test harness (Go, under `tests/`),
docs and CI.

## Commands

```sh
make check                              # lint + test, natively (baseline terminal only)
make lint                               # gofmt, go vet; shellcheck, shfmt -i 4 on the scripts
make test                               # cd tests && go test -count=1 ./...
cd tests && go test -count=1 -run 'TestList$' .   # a single test
cd tests && go test -count=1 -run 'TestHelpText$' . -update   # rewrite testdata/help from cld
make check TERMINALS=tmux,jediterm      # add JediTerm: needs a JDK and, once,
                                        #   tests/jediterm/fetch-deps tests/jediterm/lib
make docker-check                       # same as CI: tmux 3.7c built from source, tmux + jediterm
make docker-check TMUX_VERSION=3.5a     # the same on the oldest tmux cld runs on, as CI does too
make docker-blesh-check                 # the completion tests in bash with ble.sh, in that image
                                        #   built on Ubuntu 26.04 with its package ble.sh
make docker-image TMUX_VERSION=X        # an image with another tmux release, to try it by hand
make dist VERSION=X.Y.Z                 # dist/cld-OS-ARCH, linux/darwin x amd64/arm64, cld.sha256,
                                        #   install.sh
make install PREFIX=DIR                 # build cld for the host into DIR/bin (VERSION stamps it)
```

Native runs need Go, tmux 3.5a or newer, ShellCheck and shfmt. CI (`.github/workflows/ci.yml`)
runs the Docker image in one job, `linux`, on the pinned tmux 3.7c, and in another,
`linux-oldest`, on tmux 3.5a, the oldest cld runs on, the completion tests in the image built on
Ubuntu with ble.sh in a third, `blesh`, and `make check` on macOS with Homebrew tmux. Pushing a
tag `vX.Y.Z` runs the checks and publishes a release: a binary per platform, `cld.sha256` and
`install.sh`.

Pull requests land on `main` by fast-forward only, as the commits CI checked; GitHub's merge
methods are all refused (`gh pr merge` included): the repository allows merge commits alone, and
the ruleset on `main` rebase alone. A comment `/fast-forward` on the pull request pushes its head
to `main` (`.github/workflows/fast-forward.yml`), which the ruleset lets through once the checks
`linux` and `macos` pass and every conversation is resolved. A pull request that changes
`.github/workflows` is pushed by hand, `git push origin SHA:main`, since the workflow's token may
not push such a change. Rebase onto `main` before either.

## Constraints on cld

- Requires **tmux 3.5a or newer**, the oldest release the tests run on (in CI's `linux-oldest`),
  beside the newest (3.7c, pinned in `tests/Dockerfile` and the `Makefile`); there is no
  behaviour per tmux version, but for the tests' `paste-buffer -S` (3.7 and newer) and the keys
  a tmux cld runs inside keeps (below). The check reads `tmux -V` at startup, and compares the
  letter of a bug-fix release: 3.5 is refused. Raising the minimum is one change:
  `linux-oldest`'s pin, the check, the docs.
- `new` and `resume` require **claude 2.1.232 or newer**, the first release that takes what cld
  passes and does what it relies on, `resume`'s documented behaviour included (the tests never
  run the real claude): they run `claude --version` before starting that claude, and so does the
  list's Enter on a session that has ended, which resumes it, and `restore` for each session it
  brings back, with the claude, the directory and the environment the session started with;
  `join`, `detach`, `kill`, `list` otherwise, `setup project`, `setup telemetry`,
  `setup completion`, `setup restore`, `update` and completion do not. Re-derive the minimum
  when cld starts to pass or rely on something newer (docs/design.md, decision 6).
- Builds with `CGO_ENABLED=0` for linux and darwin on amd64 and arm64 (so no `ttyname`: cld runs
  `tty`). gofmt and go vet must pass; ShellCheck and `shfmt -i 4` for `install.sh` and
  `tests/jediterm/fetch-deps`.
- `install.sh` is POSIX sh, run as `curl -fsSL .../releases/latest/download/install.sh | sh`:
  everything stays in functions that its last line calls, so that a download cut short runs
  nothing. It relies on the names `make dist` publishes, `cld-OS-ARCH` and `cld.sha256` with its
  `HASH  NAME` lines, as `internal/update` does, so a change to them goes with one to both
  (decisions 20 and 21).
- `update` makes none of the tmux and claude checks: it finds the latest release where GitHub's
  `releases/latest` redirects, and refuses a version that is no X.Y.Z (`dev`). It replaces the
  file cld runs from (symbolic links resolved) by a rename, once the new binary matches its
  checksum and prints the release's version; an interrupt before that leaves cld as it was and
  ends with 128 plus the signal's number. Once cld is replaced, it has the new cld print anew
  (`completion SHELL`, which every release has) each script where `setup completion` writes one,
  and writes those that differ; one it cannot write is a warning (`output.Warn`) naming
  `cld setup completion SHELL`, never a failure. `CLD_RELEASES_URL` points it, and `install.sh`,
  at the tests' releases.
- Only `main` exits: errors carry their exit status up (`internal/fail`); `new`, `resume`, `join`
  and the list's Enter end in `syscall.Exec` of tmux. cobra's defaults are overridden to keep
  cld's command line - the first argument checked before cobra, and the one after `setup`,
  options read up to the first argument, a `help [COMMAND]` that takes one of cld's commands
  (and after `setup` or `completion`, one of theirs, and after `setup completion`, a shell) and
  refuses anything else, the help and
  cobra's other output printed through `output.Print`, the commands unsorted (see docs/design.md,
  Implementation notes).
- The help is cobra's, generated with its default templates from each command's `Use`, `Short`
  and `Long` and its option usages (value names in backquotes: `` `NAME` ``). cobra wraps
  nothing: break the texts by hand within 80 columns, which `TestHelpText` checks - all but
  cobra's own last line, which names the command (81 columns for `setup completion`).
- Shell completion is cobra's (`cld completion SHELL`, `__complete`), bash's script with lines of
  cld's at the end of `__start_cld` that turn off file names, and ble.sh's own completions, where
  cld offers none under ble.sh (`bashScript`, decision 27): `join -n` and `detach -n` offer the
  NAME, and their `-s` the SUFFIX, of the names `list` shows that run, read as `list` reads them,
  and `resume -n` and `-s` those that have ended, `help` the
  commands it takes, `setup project` and `setup telemetry` among them, `setup project --mcp` its
  MCP servers and `--permissions` its sets, nothing offers file names (`--collector-config`'s
  `FILE` neither), and completion
  makes none of the startup checks, `setup telemetry`'s and `setup restore`'s included, and never
  starts the interactive list (decisions 17 to 19 in docs/design.md). The `Short`s are also what
  `cld <TAB>` shows.
- `setup completion SHELL` (bash, zsh, fish) writes the script cobra generates (bash's with cld's
  lines), byte for byte what `completion SHELL` prints, where the shell reads it, following
  `BASH_COMPLETION_USER_DIR`, `XDG_DATA_HOME`, `XDG_CONFIG_HOME` and `ZDOTDIR`: bash-completion 2's
  user directory,
  `~/.config/fish/completions`, and for zsh `~/.local/share/cld/zsh/_cld` with lines at the end
  of `.zshrc` that load it, running `compinit -i` only where nothing has before them (decision
  22). It reads both files before it writes either, writes through `internal/configfile`, and
  removes nothing. Outside the tests, which set `HOME` to the sandbox's, run it only with `HOME`
  (and those variables) pointing at a scratch directory.
- Sessions are always addressed as `=cld-NAME` (exact match); a bare target would prefix-match
  `cld-rev` to `cld-review`. `set` targets use `=cld-NAME:` because `set` takes a pane.
- Names are validated (`^[A-Za-z0-9][A-Za-z0-9_-]*$`, at most 64 characters so that the socket
  path fits in `sun_path`), never sanitised. On the command line a session's name is `NAME-SUFFIX`,
  from `-n NAME` and `-s SUFFIX`, which `new`, `resume`, `join`, `detach` and `kill` take
  together: `NAME` is by default the name of the directory holding the git repository's common
  `.git` (`git rev-parse --git-common-dir`) or, outside a repository, of the current directory
  (`os.Getwd`, which keeps `PWD`'s), made a NAME - the one name cld changes, as nobody typed it -
  and `SUFFIX`, for `new` and for `resume` with SESSION, the index one above the highest of the
  sessions `NAME-INDEX` whose servers run; `join` and `kill` need `-s`, and `resume` `-s` or
  SESSION. Where nothing is left of the default, as in `/`, the name is `SUFFIX` alone. cld's
  messages and the `pane-died` hint and border line name a session back as `-n NAME -s SUFFIX`,
  split at its last `-` (`session.Options`; decision 24). Elsewhere, and in `internal/session`,
  NAME is a session's whole name, all tmux sees. `-w` gives claude `--worktree cld-NAME-SUFFIX`,
  and `resume --fork`, which needs a SESSION other than the session's own name, `--fork-session`
  (decision 45). `new` and `resume` record where they made a session, the repository's directory
  or the current one, as `@cld-home` on claude's session; `join`, `detach` and `kill` without
  `-n`, where NAME's default is not empty, refuse a session whose `@cld-home` is another directory
  (compared as files), and `join -s` and `detach -s` offer only the sessions they take
  (`session.Home`; decision 37).
- cld keeps a **record of its sessions** in `$XDG_STATE_HOME/cld`, by default `~/.local/state/cld`
  (`internal/session/record.go`; decision 40): `sessions/NAME.json`, a line of JSON per session -
  its name, the directory claude started in and its conversation's ID - whose file time is the
  entry's; beside it, and gone with it, `NAME.env`, the claude and the environment (after
  `withoutTerminal`, 0600) the session's server started with, `NAME.run`, the run mark, and
  `NAME.busy`, the busy mark; `indexes.json`, the highest index given each `NAME-`; and `lock`,
  which `new` and `resume` hold (`flock`) from the name to tmux, `restore` for each session it
  brings back, and the list's forget. They write the entry and the environment before
  tmux - once the terminal is checked, so that a refusal there writes nothing - and
  tmux, once `new-session` has made the
  session, makes the run mark (but for `restore`, which keeps its time) and removes the busy mark,
  in a `run-shell` after it in the same command, which a failed `new-session` cuts short; they
  give claude a `SessionStart` hook that writes the entry again with `session_id` from its input,
  `Stop` and `SessionEnd` hooks that touch it, and hooks that keep the busy mark -
  `UserPromptSubmit` makes it and touches the run mark; `Stop`, `StopFailure`,
  `PostToolUseFailure` with `is_interrupt` and `idle_prompt` remove it - with decision 39's
  timeout, but for `SessionEnd`'s, which claude ends at 1.5 s itself; cld reads none of claude's
  transcripts. `list` shows an entry without its session as `ended`; `resume` without SESSION
  passes `--resume ID` from the entry (or else the name), in the entry's directory; `join`,
  `detach` and `kill` refuse an `ended` session, pointing at `resume`; `new`'s index counts the
  entries and the indexes given. An entry or index older than 30 days (claude's default
  `cleanupPeriodDays`) counts no more and goes, but for the entry of a session whose server runs;
  the list's Ctrl+X twice on an `ended` row forgets its entry, and `kill` does not. `kill`, the
  list's Ctrl+X and the idle sweep remove the run mark in their tmux command, between
  `kill-session` and `kill-server`, and the `pane-died` hook for claude's exit with status 0; a
  reboot leaves it. Where cld cannot write the record it warns and makes the session all the same.
  The tests' record is in the sandbox's `HOME`.
- `restore` (decision 48) brings back each entry with a run mark and no server, as
  `resume -n NAME -s SUFFIX` would but detached (`new-session -d`, no terminal check, no title,
  no exec: `create` with `launch.detached`), in the entry's directory, with the claude and the
  environment of `NAME.env` but cld's `TMUX_TMPDIR`, and for a busy one `ContinuePrompt` after
  `--resume`; the words after `--` are not kept. A session whose run mark is older than
  `CLD_IDLE_DAYS` (not started nor given a prompt since) it leaves ended, removing the mark, with
  a note: tmux's idle times start again at a restore. A session it cannot bring back is a
  warning, and status 1.
  `setup restore`, Linux with systemd only (a refusal elsewhere; `systemctl --user
  show-environment` must answer), writes `~/.config/systemd/user/cld-restore.service`
  (`Type=oneshot`, `RemainAfterExit=yes`, `KillMode=process`, `WantedBy=default.target`,
  `Environment` for `PATH`, and `TMUX_TMPDIR`, `XDG_STATE_HOME` and `CLD_IDLE_DAYS` where set,
  refusing a `CLD_IDLE_DAYS` that `restore` would refuse) through
  `internal/configfile`, runs `systemctl --user daemon-reload` where it changed and `enable`, and
  names `loginctl enable-linger` where lingering is off. Outside the tests - whose fake
  `systemctl` and `loginctl` come first on the `PATH` - run it only with `HOME` pointing at a
  scratch directory, and never against the user's own systemd.
- claude is passed to tmux as separate argv words so tmux execs it directly, not via `sh -c`,
  and by the path of the claude `new` or `resume` checked, so tmux does not look `claude` up in
  the `PATH`; a word of cld's ending in `;` (resume's SESSION, a word after `--`, the directory
  given with `-c` or the session's home can) goes with a `\` before the `;`, since tmux would end
  its command there. The directory also goes with every `#` doubled: tmux expands `-c` as a
  format, where `#(...)` runs a shell command.
- `new` and `resume` give claude the words after `--`, after cld's own arguments, and refuse
  (status 2, naming why) a word that starts with an option cld gives claude itself (`-n`, `-w`,
  `--settings`), one that resumes a conversation (`-r`, `-c`, `--from-pr`) or one with which
  claude leaves the session (`-p`, `--bg`, `--tmux`, `--teleport`, `--init-only`,
  `--rewind-files`, `-h`, `-v`): `claudeOptions` in `cmd/cld`, decision 41. They also refuse
  words that make tmux's command longer than the 16364 bytes tmux takes (`commandLimit` in
  `internal/session`), counted with the record's hooks before the session's entry is written.
- The clients that attach - `new`'s, `resume`'s, `join`'s and the list's Enter's - and the reads
  of the sessions, `list`'s and the lookup of one (for its `@cld-home`), run `tmux -u`: under a
  locale that names no UTF-8 (`LC_ALL`, `LC_CTYPE`, `LANG`), tmux would otherwise write what is
  not ASCII, most of claude's UI, as `_`.
- A server per session: session `cld-NAME` lives on server `cld-NAME` (`tmux -L cld-NAME`), and
  cld looks for that one session there, filtering on `#{==:#{session_name},cld-NAME}` and cld's
  mark. Anything claude runs inherits `TMUX` and reaches claude's own server, where a session it
  makes has another name. `list` reads the sockets `cld-*` in `${TMUX_TMPDIR:-/tmp}/tmux-UID` and
  asks each server, eight at a time; stale sockets are passed over, never removed. Before cld runs
  tmux on a socket it connects to it, and a refused connection or no socket is no server, with no
  tmux run - only where tmux would get as far, in a `tmux-UID` it takes (decision 38), and not on
  the socket `TMUX` names, which `OwnPane`, `keptKeys` and a bare `detach` (decision 44) run tmux
  on at once.
  `kill` runs `kill-session`, then `kill-server`, in one tmux command, and does not wait for
  claude, whose `SessionEnd` hooks (reason `other`) may still run after it returns (decision 32).
  Where the server runs without its session, `kill` ends it with `kill-server` alone if it has
  outlived the session - it has cld's mark and sessions, none `cld-NAME`, and its
  `#{socket_path}` is `.../cld-NAME`, a format checked before the kill and again under `if -F` in
  its command - and refuses the name otherwise; `new`, `resume`, `join` and `detach` refuse the
  name, pointing at `kill` where it would end the server and at `tmux -L cld-NAME ls` alone
  otherwise. Where that path names another NAME that differs only in case (a socket directory that
  ignores case, as on macOS), all five name that session instead.
- cld marks the servers it starts, not their sessions (`set -s @cld 1`), and reads the mark,
  `#{||:#{@cld},#{==:#{prefix},C-q}}` - the prefix for the servers of cld 0.8.2 and earlier - in
  the formats it runs anyway: the filter for session `cld-NAME`, `lingering`'s, `kill`'s `if -F`,
  `OwnPane`'s, `keptKeys`' and a bare `detach`'s. A server `cld-NAME` without it, the user's own,
  is none of cld's: `list` and completion pass over it, `new`, `resume` and `join` nest in its
  panes, where `detach` without `-s` refuses, and they, `detach` and `kill` refuse its name,
  pointing at another name, never at `kill` (decision 34).
- `detach` is `C-q d` for a terminal that keeps `C-q` from tmux (VS Code, Rider's keymap), and the
  one command that acts in a pane of cld's servers, where `new`, `resume` and `join` refuse: with
  `-s` it looks the session up as `kill` does and runs `detach-client -s =cld-NAME`, and without
  `-n` and `-s`, where `TMUX` names one of cld's servers (`! cld detach` in claude), a bare
  `detach-client` by that socket, which detaches the terminal used last on the pane's session - a
  key, a mouse report (claude asks for every motion) or a focus event makes it that, so mostly the
  one it was typed in; elsewhere it needs `-s`. claude runs shell commands without a terminal, so
  it does not go by `tty`. Both run under `if -F '#{session_attached}'` (the bare one with cld's
  mark too): with no terminal on the session, tmux would fail with `no current client`, or detach
  another session's terminal (decision 44).
- `list` first, and `new` without `-s` once it has taken its index (reading every server as
  `list` does, a failed read there only a warning), end each session idle for longer than
  `CLD_IDLE_DAYS` days (decimal, 30 when unset or empty, `0` none; anything else refused): no
  terminal attached, and the later of `#{session_activity}` and `#{session_last_attached}`, which
  only an attach and a terminal's keys move, older than that. The kill is one tmux command,
  `if -F` with that check again, then `kill-session` and `kill-server`, so that a terminal
  attaching meanwhile keeps the session, and a note (`output.Note`) on stderr names each session
  ended, which `list` then shows as `ended` where the record keeps its entry. `LAST ACTIVE` in
  `list` and its interactive list is the same time, `-` for a session that has ended. Completion
  never ends one, nor does the sweep end the session whose server cld runs on, the socket `TMUX`
  names (compared as a file: tmux resolves symbolic links in its path), lest a claude that runs
  cld end itself (decision 46).
- Per-session settings (`remain-on-exit on`, its empty format, the `pane-died` hook, which for
  claude's exit with status 0 removes the run mark and closes the pane, as `failed` would - a
  pane's `pane-exited` hook never runs - and otherwise keeps the failure on screen) go on claude's
  pane (`set -p`, `set-hook -p`), and the tab's title (`set-titles`, `set-titles-string`,
  `@cld-busy`, `@cld-tmux`) on claude's session, not the window or the server, so the other panes
  of claude's window and the sessions claude makes on its server behave as plain tmux would.
  Once claude has failed, the hook sets `pane-border-status bottom` on claude's window, which
  tmux reads from the window alone, and `pane-border-format` on claude's pane, so that another
  pane there keeps tmux's own line.
- The tab's title is `✳ cld-NAME`, with `◐` and `◑` in turn while claude is busy (claude keeps its
  own at `✳` under tmux), and ` [w]` after it in a linked git worktree. `new` and `resume` give
  claude hooks in `--settings` that set `@cld-status` on its session - `tmux -S SOCKET if -F -t
  =cld-NAME:`, by the path cld checked, with the server's socket and the session written in (a
  conversation claude runs in the background runs them without `TMUX` and `TMUX_PANE`), only
  where it changes, printing nothing - and tmux sets the title from it; a `#()` job in
  `@cld-busy` refreshes the terminal a second later to turn the marker, since `status off` leaves
  tmux no timer (decision 25). `SessionStart` and `CwdChanged`
  hooks keep `@cld-worktree`, 1 while claude's directory is in a linked git worktree; they run git
  by the path cld found, and are left out where it finds none (decision 26). claude waits 5 s at
  most (`timeout`) for each hook but `CwdChanged`'s, which it runs in the background (`async`;
  decision 39), and the record's `SessionEnd` one (above). The hook events, `async` and `timeout`
  must exist in the minimum claude.
- `--settings` carries those hooks and the record's, `"disableAgentView": true` in every session
  and, with `-w`, `worktree.baseRef`, nothing more: flag settings outrank the user's, so what is
  theirs to choose - Remote Control (`remoteControlAtStartup`) among it - stays claude's own
  setting (decision 42). Agent view is not theirs to choose in a session: `/bg`, `/exit`'s "Move to
  background and exit" and `←` hand the conversation to claude's daemon, out of cld, so it is off
  in every session, with no option to turn it on, and a claude started without cld keeps it
  (decision 47). The key must exist in the minimum claude, as the hook events must.
- The variables that name the terminal to claude, which it trusts over `TERM_PROGRAM=tmux` -
  `TERMINAL_EMULATOR`, `__CFBundleIdentifier`, `CURSOR_TRACE_ID`, `VisualStudioVersion` and
  `VSCODE_GIT_ASKPASS_MAIN` - are removed from the environment `new` and `resume` exec tmux with,
  so from the server's; the rest of VS Code's askpass goes with `VSCODE_GIT_ASKPASS_MAIN`, and
  its git editor with it: `VSCODE_GIT_ASKPASS_*`, `VSCODE_GIT_EDITOR_*`, `VSCODE_GIT_IPC_HANDLE`,
  and a `GIT_ASKPASS` or `GIT_EDITOR` naming a script beside its `MAIN` (decision 33).
- claude's notifications go through the server's `allow-passthrough on`, or as a bell under
  tmux's default `bell-action`, on the channel its `preferredNotifChannel` names; its default,
  `auto`, sends none under tmux. cld sets no channel in `--settings`, which would override the
  user's, for whichever terminal joins (decision 29).
- `new`, `resume` and `join` need a terminal - their stdin one, `TERM` set and not `dumb` -
  checked last, just before the title: tmux would fail without it, and `new-session` leave its
  socket behind (decision 31). The title goes to stdout only where stdout is a terminal.
- Inside a tmux that is not one of cld's (`TMUX` names another socket, or an unmarked
  `cld-NAME`), `new`, `resume`, `join` and the list's Enter read its `extended-keys`, `prefix`
  and `prefix2` for the pane whose tty is cld's, and its client's `client_termfeatures`
  (`display -p` through that socket): the prefixes are kept, and Shift+Enter unless
  `extended-keys` is `always`, or `on` where cld's tmux is 3.7 or newer, and the features name
  `extkeys`. Where a key is kept, they name the keys on cld's message line once attached
  (`display -l -C -d 0`), with the terminal redrawn a second and three seconds later, since
  claude's startup draws over the line; never with `output.Warn`, which would show only after
  detach. cld's tmux 3.5 refuses `-C`, new in 3.6, and without it would hold back what claude
  draws until the key: there cld reads and names nothing (`Tmux.older`, decision 43).
- `setup telemetry` needs Docker, and Linux (`--network host`): it replaces the container
  `cld-telemetry`, one per Docker daemon, and rewrites the `env` of claude's user settings,
  `$CLAUDE_CONFIG_DIR/settings.json` (default `~/.claude/settings.json`), keeping every other key.
  Outside the tests - whose fake docker comes first on the `PATH` - run it only with `HOME` and
  `CLAUDE_CONFIG_DIR` pointing at a scratch directory. The collector image is pinned
  (`otel/opentelemetry-collector:0.161.0`) and bumped deliberately.
- `setup project` writes in the current directory, as this repository has them:
  `--mcp goland --permissions cld` writes its `.claude/settings.json` and `.mcp.json` byte for
  byte, which `project_test.go` checks, so change those files and `internal/project` together;
  the default `--permissions` is `read-only`. The settings hold nothing of one person's (no
  `theme`), and `.gitignore` gets `/.claude/settings.local.json`, `/.claude/plans/` and
  `/.claude/worktrees/`, so that git adds what a project shares in `.claude`. It edits files
  that exist in place, never replaces a value the settings have - but it replaces a server's
  entry in `.mcp.json` that differs, whole - and removes nothing;
  `.claude/settings.local.json` is only created. In a git work tree it then checks with
  `git check-ignore` that git does not ignore `.claude/settings.json`, and warns of the other
  files a project shares under `.claude` that git ignores (decisions 19 and 28).
- `setup` has commands of its own: `run` checks the argument after it before cobra, as it checks
  the first, and after `setup completion` the shell, and `help` takes `setup project`,
  `setup telemetry`, `setup completion SHELL` and `setup restore`. Their checks (Linux, docker,
  systemd) stay in their `Args` and `RunE`, never in a root hook, so completion
  (`__complete setup ...`), which `run` lets through, runs none of them.

The package comments of `internal/session`, `internal/telemetry`, `internal/project`,
`internal/update`, `internal/completion` and `internal/restore` explain why each tmux option is
set and each step of `setup telemetry`, `setup project`, `update`, `setup completion` and
`setup restore` is taken; keep them accurate when changing any of them.

## Test architecture (`tests/`)

Tests build cld (`cmd/cld`) and run it against real tmux; only `claude` is faked, `docker` for
`setup telemetry`, and `systemctl` and `loginctl` for `setup restore`. Read the package doc
comments at the top of each file for details.

- `main_test.go` — `TestMain` unsets every `GIT_*` variable (git sets them for hooks and
  `rebase --exec`; `TestGitVariables` pins it), builds cld and `probe/` into a temp dir, the probe
  as `claude` (and symlinks it as a fake `tmux` for version/tool checks and the commands `new`,
  `resume` and `join` exec, and as `docker`, `systemctl` and `loginctl` beside `claude`, so that
  no test reaches the real Docker or the user's systemd), and compiles the JediTerm driver when
  `CLD_TERMINALS` includes `jediterm`.
  `forEachTerminal` runs a body as a parallel subtest per terminal.
- `probe/` — stands in for claude: enters the same terminal modes claude does, logs argv/cwd/env
  (`PID.json`) and raw input bytes (`PID.in`) to `$CLD_PROBE_DIR`, and takes commands through a
  FIFO (`PID.ctl`: `title`, `osc52`, `loadbuffer`, `notify`, `link`, `rekey`, `inline`, `cd`,
  `tmux`, `hook`, `unsetenv`, `exit`); `hook EVENT JSON` runs the hooks of its `--settings` as
  claude would, and `notify CHANNEL TEXT` writes what claude writes on a notification channel.
  `CLD_PROBE_FAIL` makes it fail at startup. `claude --version` answers first, writing nothing,
  with `CLD_FAKE_CLAUDE_VERSION` (`99.0.0 (Claude Code)` when unset). Invoked as `tmux`, it fakes
  `tmux -V` via `CLD_FAKE_TMUX_VERSION` and `list-sessions` via `CLD_FAKE_TMUX_SESSIONS` (unset:
  no server running; `CLD_FAKE_TMUX_EXITED` names servers that exit as they are asked, and
  `CLD_FAKE_TMUX_DENIED` those it may not connect to; where `tmux-UID` exists, cld asks it only
  on a socket that takes connections, as `socket` in the tests makes), and
  records any other command in `tmux.json`, or runs the real tmux `CLD_FAKE_TMUX_REAL` names.
  Invoked as `docker`, it records each call in `docker.jsonl`, keeps the state of the container
  `cld-telemetry` in `docker.container`, takes connections on the port of a container it starts
  running (the probe again, as the collector's receiver, until `rm -f`; one that takes none still
  holds the port), and takes `CLD_FAKE_DOCKER_*` variables: the container before, the state one
  starts in and its restart count, whether it takes connections, the collector's log, a call that
  fails, a file that `run -d` writes. Invoked as `systemctl` or `loginctl`, it records each call
  in `systemd.jsonl`, answers `loginctl show-user` with `CLD_FAKE_LINGER` (`no` unset), and fails
  a call with the argument `CLD_FAKE_SYSTEMD_FAIL` names.
- `internal/sandbox` — an isolated world per test: its own short `TMUX_TMPDIR` (socket paths hit
  the ~108-byte `sun_path` limit), `HOME`, `PATH` with the probe first, `TMUX` unset, and a work
  directory named `_`, of which nothing is left in a session's name, so that `-s x` names a
  session `x` there. Tests are
  parallel and never touch the user's own cld sessions. `Tmux(server, ...)` runs tmux against one
  server (`cld-NAME` for session NAME); `Sessions()` and `Clients()` span every `cld-*` server,
  naming a session that is not on its own server `SERVER/SESSION`. `RunCld` runs cld without a
  terminal, `RunCldOnTerminal` on a pseudo-terminal of the test's (`OpenPty`), as the handover
  of `new`, `resume` and `join` needs, returning what cld wrote there.
- `internal/terminal` — the `Terminal` interface with one driver per outer terminal: `tmux.go`
  (an outer tmux server provides the pty; input as raw xterm bytes via `send-keys -H`) and
  `jediterm.go`, which talks line-by-line to `jediterm/JediTermDriver.java` (headless JediTerm
  3.76, pinned in `jediterm/deps.txt`). A terminal that cannot do something skips with a reason.
  JediTerm emulates on a thread of its own, so a test waits for modes that change while cld runs
  (`waitModes`); once `Running` is false, what the terminal shows is final.
- `contract_test.go` — the terminal contract (C1–C10 in `docs/design.md`: title, client features,
  Shift+Enter, Ctrl keys, detach, wheel, clicks, focus, clipboard, notifications, links, paste,
  claude exiting, the session list's keys), run per terminal.
  Legitimate per-terminal differences are encoded as expectations, not skips.
- `session_test.go` — session lifecycle and server behaviour, `detach` as claude runs it, the
  names `new` gives from the repository and the index, the sessions of another repository of the
  same name, the hooks that keep claude's status and its worktree for the title, the names
  completion offers, the keys a tmux cld runs inside keeps from claude, and the idle sessions
  `list` and `new` end (`CLD_IDLE_DAYS` of a few seconds);
  `restore_test.go` — the run mark, the busy mark and the environment beside an entry, no mark
  where tmux made no session, `restore` of sessions whose servers `kill-server` ended (a reboot)
  and of none ended on purpose or idle past `CLD_IDLE_DAYS` by the mark, its failures as warnings
  with status 1 (a failed tmux keeping the busy mark), the servers without their session or not
  cld's that it leaves alone, two `restore` at once and a `restore` racing a `resume`
  (the record's lock), and `setup restore` against the fake `systemctl` and `loginctl`, its
  refusal off Linux and completion running neither;
  `record_test.go` — cld's record of its sessions: the entry, the hooks that give it the
  conversation's ID, `ended` sessions in `list`, `join`, `kill` and the interactive list,
  `resume` by the ID (or the name) in the entry's directory, the copy's after `resume --fork`, the
  indexes, expiry, the lock of two `new` at once, `XDG_STATE_HOME` and a record cld cannot write;
  `cli_test.go` — argument parsing, errors, tool/version checks, the help, compared byte for
  byte with `testdata/help`, the completion scripts, and `LAST ACTIVE`, the kill of an idle
  session, the session cld runs in kept and `new`'s sweep that cannot read against the fake tmux;
  `telemetry_test.go` — `setup telemetry`
  against the fake docker: its calls, the collector config, the port, the settings file, failures
  (Linux only; macOS checks the refusal); `project_test.go` — `setup project` against the real
  git: the files as the repository has them, edits of files that exist, `.gitignore`'s lines,
  patterns that ignore the settings, or the files a project shares under `.claude`, all the same
  (old lines, a symbolic link, `git add -f`, submodules), refusals; `install_test.go` — `install.sh`
  piped into `sh` and `bash`, against releases an HTTP server of the test's serves
  (`CLD_RELEASES_URL`) and a fake `uname`: platforms, versions, directories, refusals, and the
  script cut short at every line; `update_test.go` — `cld update` of a cld built as release 0.4.0
  and copied into the sandbox, against the same kind of releases, whose binaries also print a
  script for `completion SHELL`: updates and none, a symbolic link, a build from source,
  refusals, a directory it cannot write, signals, the completion scripts written anew and the
  warning for one it cannot write; `completion_test.go` — `setup completion` in the sandbox's
  home directory: each script and `.zshrc`'s lines where the variables say, files that exist,
  refusals, and bash (with bash-completion 2), zsh and fish loading them, each skipped where it
  is not installed (`tests/Dockerfile` installs all three); and bash with ble.sh, where it is
  installed, completing lines typed into a tmux pane - TAB writes the line to a file once ble.sh
  is done - which `make docker-blesh-check` runs in the image built on Ubuntu with `PACKAGES`
  `ble.sh`, `CLD_BLESH` making the test fail where it would skip.

## Documentation conventions

`docs/design.md` is the project's record of tmux/claude behaviour: **Findings** (probed behaviour,
with the tmux versions checked), **Decisions** (numbered) and **Implementation notes**. Behaviour
changes are made together across the code (the package comments of `internal/session`,
`internal/picker` and `internal/telemetry`, inline comments, the commands' `Short`, `Long` and
option usages in `cmd/cld`, which the help is generated from, and `tests/testdata/help`),
`README.md`, `docs/guide.md` and `docs/design.md`, with tests. When a change rests on observed tmux
or claude behaviour, record the probe and the versions in Findings.

`README.md` is the overview: what cld does, installing it, the commands, a paragraph per feature.
`docs/guide.md` has the details a user may need beyond it and `cld help`: caveats, what is not
checked yet, the comparison with Claude Code's background sessions (dated by the claude release it
was checked on; the README keeps a paragraph of it), troubleshooting, upgrading. Neither explains
how cld works inside; that is for `docs/design.md` and the package comments.

Commit messages follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/):
a header `type(scope): description`, then a body, then optional footers, each separated by a blank
line.

- **type**: `feat` for a new feature, `fix` for a bug fix; otherwise `build`, `chore`, `ci`,
  `docs`, `perf`, `refactor`, `style` or `test`.
- **scope** (optional): the part of the project changed, e.g. `cli` (`cmd/cld`), `session`
  (`internal/session`), `tests`, `jediterm`, `docs`.
- **description**: short and imperative, lower case, no trailing period, e.g.
  `fix(session): read the pty name without tty's newline`.
- **body**: a bullet list saying what was wrong or missing, what changed, which tmux/claude
  versions it was checked on, and what the tests cover.
- **breaking changes**: `!` before the colon (`feat(cli)!: ...`) and a `BREAKING CHANGE:` footer
  describing what users must change.
