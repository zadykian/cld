# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`cld` runs Claude Code in named sessions, each on a private tmux server of its own
(`tmux -L cld-NAME -f /dev/null`), so a conversation can be detached and rejoined from any
terminal. The product is a Go program on cobra: `cmd/cld` is the command line (commands, their
help texts, argument errors), `internal/session` the tmux side, `internal/picker` the interactive
`cld list` on a terminal, `internal/project` `cld setup project` (a project's
`.claude/settings.json`, `.claude/settings.local.json`, `.mcp.json` and `.gitignore`),
`internal/telemetry` `cld setup telemetry` (a local OpenTelemetry Collector in Docker, and
claude's settings pointed at it), `internal/configfile` edits the files the two write in place,
`internal/update` `cld update` (cld replacing itself with the latest release),
`internal/completion` `cld setup completion` (the completion script where bash, zsh or fish reads
it, and `cld update` writing it anew), `internal/tool` finds the programs cld runs on the `PATH`
(and ends cld as a shell would when one cannot run), `internal/fail` carries exit statuses up to
`main`, and `internal/output` prints cld's own output, a write that fails being one of those ends,
and its warnings. `install.sh`, published with each release,
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
make docker-image TMUX_VERSION=X        # an image with another tmux release, to try it by hand
make dist VERSION=X.Y.Z                 # dist/cld-OS-ARCH, linux/darwin x amd64/arm64, cld.sha256,
                                        #   install.sh
make install PREFIX=DIR                 # build cld for the host into DIR/bin (VERSION stamps it)
```

Native runs need Go, tmux 3.7 or newer, ShellCheck and shfmt. CI (`.github/workflows/ci.yml`)
runs the Docker image in one job, `linux`, on the pinned tmux 3.7c, and `make check` on macOS with
Homebrew tmux. Pushing a tag `vX.Y.Z` runs the checks and publishes a release: a binary per
platform, `cld.sha256` and `install.sh`.

Pull requests land on `main` by fast-forward only, as the commits CI checked; GitHub's merge
methods are all refused (`gh pr merge` included): the repository allows merge commits alone, and
the ruleset on `main` rebase alone. A comment `/fast-forward` on the pull request pushes its head
to `main` (`.github/workflows/fast-forward.yml`), which the ruleset lets through once the checks
`linux` and `macos` pass and every conversation is resolved. A pull request that changes
`.github/workflows` is pushed by hand, `git push origin SHA:main`, since the workflow's token may
not push such a change. Rebase onto `main` before either.

## Constraints on cld

- Requires **tmux 3.7 or newer**, the release the tests run on (3.7c, pinned in
  `tests/Dockerfile` and the `Makefile`); there is no behaviour per tmux version. The check reads
  `tmux -V` at startup. Raising the minimum is one change: the pin, the check, the docs.
- `new` and `resume` require **claude 2.1.232 or newer**, the first release that takes what cld
  passes and does what it relies on, `resume`'s documented behaviour included (the tests never
  run the real claude): they run `claude --version` before starting that claude; `join`, `kill`,
  `list`, `setup project`, `setup telemetry`, `setup completion`, `update` and completion do not. Re-derive the
  minimum when cld starts to pass or rely on something newer (docs/design.md, decision 6).
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
- Shell completion is cobra's (`cld completion SHELL`, `__complete`): `join -n` offers the NAME,
  and `join -s` the SUFFIX, of the names `list` shows, read as `list` reads them, `help` the
  commands it takes, `setup project` and
  `setup telemetry` among them, `setup project --mcp` its MCP servers, nothing offers file names
  (`--collector-config`'s `FILE` neither), and completion makes none of the startup checks,
  `setup telemetry`'s included, and never starts the interactive list (decisions 17 to 19 in
  docs/design.md). The `Short`s are also what `cld <TAB>` shows.
- `setup completion SHELL` (bash, zsh, fish) writes the script cobra generates, byte for byte
  what `completion SHELL` prints, where the shell reads it, following `BASH_COMPLETION_USER_DIR`,
  `XDG_DATA_HOME`, `XDG_CONFIG_HOME` and `ZDOTDIR`: bash-completion 2's user directory,
  `~/.config/fish/completions`, and for zsh `~/.local/share/cld/zsh/_cld` with lines at the end
  of `.zshrc` that load it, running `compinit -i` only where nothing has before them (decision
  22). It reads both files before it writes either, writes through `internal/configfile`, and
  removes nothing. Outside the tests, which set `HOME` to the sandbox's, run it only with `HOME`
  (and those variables) pointing at a scratch directory.
- Sessions are always addressed as `=cld-NAME` (exact match); a bare target would prefix-match
  `cld-rev` to `cld-review`. `set` targets use `=cld-NAME:` because `set` takes a pane.
- Names are validated (`^[A-Za-z0-9][A-Za-z0-9_-]*$`, at most 64 characters so that the socket
  path fits in `sun_path`), never sanitised. On the command line a session's name is
  `NAME-SUFFIX`, from `-n NAME` and `-s SUFFIX`, which `new`, `resume`, `join` and `kill` take
  together: `NAME` is by default the name of the directory holding the git repository's common
  `.git` (`git rev-parse --git-common-dir`) or, outside a repository, of the current directory
  (`os.Getwd`, which keeps `PWD`'s), made a NAME - the one name cld changes, as nobody typed it -
  and `SUFFIX`, for `new` and for `resume` with SESSION, the index one above the highest of the
  sessions `NAME-INDEX` whose servers run; `join` and `kill` need `-s`, and `resume` `-s` or
  SESSION. Where nothing is left of the default, as in `/`, the name is `SUFFIX` alone. cld's
  messages and the `pane-died` hint name a session back as `-n NAME -s SUFFIX`, split at its last
  `-` (`session.Options`; decision 24). Elsewhere, and in `internal/session`, NAME is a session's
  whole name, all tmux sees. `-w` gives claude `--worktree cld-NAME-SUFFIX`.
- claude is passed to tmux as separate argv words so tmux execs it directly, not via `sh -c`,
  and by the path of the claude `new` or `resume` checked, so tmux does not look `claude` up in
  the `PATH`; a word of cld's ending in `;` (resume's SESSION or the directory given with `-c`
  can) goes with a `\` before the `;`, since tmux would end its command there. The directory also
  goes with every `#` doubled: tmux expands `-c` as a format, where `#(...)` runs a shell command.
- A server per session: session `cld-NAME` lives on server `cld-NAME` (`tmux -L cld-NAME`), and
  cld looks for that one session there, filtering on `#{==:#{session_name},cld-NAME}`. Anything
  claude runs inherits `TMUX` and reaches claude's own server, where a session it makes has another
  name; there is no mark. `list` reads the sockets `cld-*` in `${TMUX_TMPDIR:-/tmp}/tmux-UID` and
  asks each server; stale sockets answer "no server running" and are passed over, never removed.
  `kill` runs `kill-session`, then `kill-server`, in one tmux command; `new`, `resume`, `join` and
  `kill` refuse a name whose server runs without its session, and where that server's
  `#{socket_path}` names another NAME that differs only in case (a socket directory that ignores
  case, as on macOS), they name that session instead of pointing at `kill-server`.
- Per-session settings (`remain-on-exit`, its empty format, the `pane-died` hook) go on claude's
  window, and the tab's title (`set-titles`, `set-titles-string`, `@cld-busy`, `@cld-tmux`) on
  claude's session, not the server, so the sessions claude makes on its server behave as plain
  tmux would.
- The tab's title is `✳ cld-NAME`, with `◐` and `◑` in turn while claude is busy (claude keeps its
  own at `✳` under tmux), and ` [w]` after it in a linked git worktree. `new` and `resume` give
  claude hooks in `--settings` that set `@cld-status` on its session - `tmux if -F -t
  "$TMUX_PANE"`, by the path cld checked, only where it changes, printing nothing - and tmux sets
  the title from it; a `#()` job in `@cld-busy` refreshes the terminal a second later to turn the
  marker, since `status off` leaves tmux no timer (decision 25). `SessionStart` and `CwdChanged`
  hooks keep `@cld-worktree`, 1 while claude's directory is in a linked git worktree; they run git
  by the path cld found, and are left out where it finds none (decision 26). The hook events must
  exist in the minimum claude.
- `TERMINAL_EMULATOR` is removed from the environment `new` and `resume` exec tmux with, so from
  the server's; claude trusts it over `TERM_PROGRAM=tmux`.
- `setup telemetry` needs Docker, and Linux (`--network host`): it replaces the container
  `cld-telemetry`, one per Docker daemon, and rewrites the `env` of claude's user settings,
  `$CLAUDE_CONFIG_DIR/settings.json` (default `~/.claude/settings.json`), keeping every other key.
  Outside the tests - whose fake docker comes first on the `PATH` - run it only with `HOME` and
  `CLAUDE_CONFIG_DIR` pointing at a scratch directory. The collector image is pinned
  (`otel/opentelemetry-collector:0.161.0`) and bumped deliberately.
- `setup project` writes in the current directory, as this repository has them: `--mcp goland`
  writes its `.claude/settings.json` and `.mcp.json` byte for byte, which `project_test.go`
  checks, so change those files and `internal/project` together. It edits files that exist in
  place and removes nothing; `.claude/settings.local.json` is only created. In a git work tree it
  then checks with `git check-ignore` that git does not ignore `.claude/settings.json`
  (decision 19).
- `setup` has commands of its own: `run` checks the argument after it before cobra, as it checks
  the first, and after `setup completion` the shell, and `help` takes `setup project`,
  `setup telemetry` and `setup completion SHELL`. Their checks (Linux, docker)
  stay in their `Args` and `RunE`, never in a root hook, so completion (`__complete setup ...`),
  which `run` lets through, runs none of them.

The package comments of `internal/session`, `internal/telemetry`, `internal/project`,
`internal/update` and `internal/completion` explain why each tmux option is set and each step of
`setup telemetry`, `setup project`, `update` and `setup completion` is taken; keep them accurate
when changing any of them.

## Test architecture (`tests/`)

Tests build cld (`cmd/cld`) and run it against real tmux; only `claude` is faked, and `docker`
for `setup telemetry`. Read the package doc comments at the top of each file for details.

- `main_test.go` — `TestMain` unsets every `GIT_*` variable (git sets them for hooks and
  `rebase --exec`; `TestGitVariables` pins it), builds cld and `probe/` into a temp dir, the probe
  as `claude` (and symlinks it as a fake `tmux` for version/tool checks and the commands `new`,
  `resume` and `join` exec, and as `docker` beside `claude`, so that no test reaches the real
  Docker), and compiles the JediTerm driver when `CLD_TERMINALS` includes `jediterm`.
  `forEachTerminal` runs a body as a parallel subtest per terminal.
- `probe/` — stands in for claude: enters the same terminal modes claude does, logs argv/cwd/env
  (`PID.json`) and raw input bytes (`PID.in`) to `$CLD_PROBE_DIR`, and takes commands through a
  FIFO (`PID.ctl`: `title`, `osc52`, `loadbuffer`, `rekey`, `inline`, `cd`, `tmux`, `hook`,
  `exit`); `hook EVENT JSON` runs the hooks of its `--settings` as claude would.
  `CLD_PROBE_FAIL` makes it fail at startup. `claude --version` answers first, writing nothing,
  with `CLD_FAKE_CLAUDE_VERSION` (`99.0.0 (Claude Code)` when unset). Invoked as `tmux`, it fakes
  `tmux -V` via `CLD_FAKE_TMUX_VERSION` and `list-sessions` via `CLD_FAKE_TMUX_SESSIONS` (unset:
  no server running; `CLD_FAKE_TMUX_EXITED` names servers that exit as they are asked), and
  records any other command in `tmux.json`, or runs the real tmux `CLD_FAKE_TMUX_REAL` names.
  Invoked as `docker`, it records each call in `docker.jsonl`, keeps the state of the container
  `cld-telemetry` in `docker.container`, takes connections on the port of a container it starts
  running (the probe again, as the collector's receiver, until `rm -f`; one that takes none still
  holds the port), and takes `CLD_FAKE_DOCKER_*` variables: the container before, the state one
  starts in and its restart count, whether it takes connections, the collector's log, a call that
  fails, a file that `run -d` writes.
- `internal/sandbox` — an isolated world per test: its own short `TMUX_TMPDIR` (socket paths hit
  the ~108-byte `sun_path` limit), `HOME`, `PATH` with the probe first, `TMUX` unset, and a work
  directory named `_`, of which nothing is left in a session's name, so that `-s x` names a
  session `x` there. Tests are
  parallel and never touch the user's own cld sessions. `Tmux(server, ...)` runs tmux against one
  server (`cld-NAME` for session NAME); `Sessions()` and `Clients()` span every `cld-*` server,
  naming a session that is not on its own server `SERVER/SESSION`.
- `internal/terminal` — the `Terminal` interface with one driver per outer terminal: `tmux.go`
  (an outer tmux server provides the pty; input as raw xterm bytes via `send-keys -H`) and
  `jediterm.go`, which talks line-by-line to `jediterm/JediTermDriver.java` (headless JediTerm
  3.76, pinned in `jediterm/deps.txt`). A terminal that cannot do something skips with a reason.
  JediTerm emulates on a thread of its own, so a test waits for modes that change while cld runs
  (`waitModes`); once `Running` is false, what the terminal shows is final.
- `contract_test.go` — the terminal contract (C1–C10 in `docs/design.md`: title, client features,
  Shift+Enter, Ctrl keys, detach, wheel, focus, clipboard, paste, claude exiting, the session
  list's keys), run per terminal.
  Legitimate per-terminal differences are encoded as expectations, not skips.
- `session_test.go` — session lifecycle and server behaviour, the names `new` gives from the
  repository and the index, the hooks that keep claude's status and its worktree for the title,
  and the names completion offers;
  `cli_test.go` — argument parsing, errors, tool/version checks, the help, compared byte for
  byte with `testdata/help`, and the completion scripts; `telemetry_test.go` — `setup telemetry`
  against the fake docker: its calls, the collector config, the port, the settings file, failures
  (Linux only; macOS checks the refusal); `project_test.go` — `setup project` against the real
  git: the files as the repository has them, edits of files that exist, `.gitignore`'s lines,
  patterns that ignore the settings all the same, refusals; `install_test.go` — `install.sh`
  piped into `sh` and `bash`, against releases an HTTP server of the test's serves
  (`CLD_RELEASES_URL`) and a fake `uname`: platforms, versions, directories, refusals, and the
  script cut short at every line; `update_test.go` — `cld update` of a cld built as release 0.4.0
  and copied into the sandbox, against the same kind of releases, whose binaries also print a
  script for `completion SHELL`: updates and none, a symbolic link, a build from source,
  refusals, a directory it cannot write, signals, the completion scripts written anew and the
  warning for one it cannot write; `completion_test.go` — `setup completion` in the sandbox's
  home directory: each script and `.zshrc`'s lines where the variables say, files that exist,
  refusals, and bash (with bash-completion 2), zsh and fish loading them, each skipped where it
  is not installed (`tests/Dockerfile` installs all three).

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
checked yet, troubleshooting, upgrading. Neither explains how cld works inside; that is for
`docs/design.md` and the package comments.

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
