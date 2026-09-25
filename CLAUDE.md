# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`cld` runs Claude Code in named sessions on a private tmux server (`tmux -L cld -f /dev/null`), so
a conversation can be detached and rejoined from any terminal. The product is a Go program on
cobra: `cmd/cld` is the command line (commands, argument errors, usage text), `internal/session`
the tmux side, and `internal/fail` carries exit statuses up to `main`. Everything else is its test
harness (Go, under `tests/`), docs and CI.

## Commands

```sh
make check                              # lint + test, natively (baseline terminal only)
make lint                               # gofmt, go vet; shellcheck, shfmt -i 4 on fetch-deps
make test                               # cd tests && go test -count=1 ./...
cd tests && go test -count=1 -run 'TestList$' .   # a single test
make check TERMINALS=tmux,jediterm      # add JediTerm: needs a JDK and, once,
                                        #   tests/jediterm/fetch-deps tests/jediterm/lib
make docker-check                       # same as CI: tmux 3.3a (debian:bookworm), tmux + jediterm
make docker-check BASE=ubuntu:24.04     # tmux 3.4; debian:trixie has 3.5a
make docker-check BASE=debian:trixie TMUX_VERSION=3.7c   # tmux built from source
make dist VERSION=X.Y.Z                 # dist/cld-OS-ARCH, linux/darwin x amd64/arm64, + cld.sha256
make install PREFIX=DIR                 # build cld for the host into DIR/bin (VERSION stamps it)
```

Native runs need Go, tmux, ShellCheck and shfmt. CI (`.github/workflows/ci.yml`) runs the Docker
image on tmux 3.3a, 3.4, 3.5a and 3.7c, and `make check` on macOS with Homebrew tmux. Pushing a
tag `vX.Y.Z` runs the checks and publishes a release: a binary per platform and `cld.sha256`.

## Constraints on cld

- Must run on **tmux 3.3 or newer**; behaviour differs by tmux version (e.g. `remain-on-exit
  failed` only from 3.5, because 3.3/3.4 crash over a dead pane that had focus reporting on).
  Version gating is done from `tmux -V` at startup.
- Builds with `CGO_ENABLED=0` for linux and darwin on amd64 and arm64 (so no `ttyname`: cld runs
  `tty`). gofmt and go vet must pass; ShellCheck and `shfmt -i 4` for `tests/jediterm/fetch-deps`.
- Only `main` exits: errors carry their exit status up (`internal/fail`); `new` and `join` end in
  `syscall.Exec` of tmux. cobra's defaults are overridden to keep cld's command line - the
  first argument checked before cobra, options read up to the first argument, one usage text,
  no completion command (see docs/design.md, Implementation notes).
- Sessions are always addressed as `=cld-NAME` (exact match); a bare target would prefix-match
  `cld-rev` to `cld-review`. `set` targets use `=cld-NAME:` because `set` takes a pane.
- Names are validated (`^[A-Za-z0-9][A-Za-z0-9_-]*$`), never sanitised.
- claude is passed to tmux as separate argv words so tmux execs it directly, not via `sh -c`.
- cld only sees sessions it started: `new` sets the user option `@cld` to the session's id in the
  same tmux command as `new-session`, and every lookup filters on
  `#{==:#{@cld},#{session_id}}`. A plain flag would not work, because tmux resolves `@cld` from
  server/pane/window options before session options. Anything claude runs inherits `TMUX` and can
  reach cld's server, which is why the mark exists.
- Per-session settings (`remain-on-exit`, its empty format, the `pane-died` hook) go on claude's
  window, not the server, so sessions cld did not start behave as plain tmux would.
- `TERMINAL_EMULATOR` is removed from the environment `new` execs tmux with, so from the server's;
  claude trusts it over `TERM_PROGRAM=tmux`.

The package comment of `internal/session` explains why each tmux option is set; keep it accurate
when changing options.

## Test architecture (`tests/`)

Tests build cld (`cmd/cld`) and run it against real tmux; only `claude` is faked. Read the
package doc comments at the top of each file for details.

- `main_test.go` — `TestMain` builds cld and `probe/` into a temp dir, the probe as `claude` (and
  symlinks it as a fake `tmux` for version/tool checks and the commands `new` and `join` exec),
  and compiles the JediTerm driver when `CLD_TERMINALS` includes `jediterm`. `forEachTerminal`
  runs a body as a parallel subtest per terminal.
- `probe/` — stands in for claude: enters the same terminal modes claude does, logs argv/cwd/env
  (`PID.json`) and raw input bytes (`PID.in`) to `$CLD_PROBE_DIR`, and takes commands through a
  FIFO (`PID.ctl`: `title`, `osc52`, `loadbuffer`, `rekey`, `inline`, `cd`, `tmux`, `exit`).
  `CLD_PROBE_FAIL` makes it fail at startup. Invoked as `tmux`, it fakes `tmux -V` via
  `CLD_FAKE_TMUX_VERSION` and `list-sessions` via `CLD_FAKE_TMUX_SESSIONS`, and records any other
  command in `tmux.json`.
- `internal/sandbox` — an isolated world per test: its own short `TMUX_TMPDIR` (socket paths hit
  the ~108-byte `sun_path` limit), `HOME`, `PATH` with the probe first, `TMUX` unset. Tests are
  parallel and never touch the user's own cld sessions.
- `internal/terminal` — the `Terminal` interface with one driver per outer terminal: `tmux.go`
  (an outer tmux server provides the pty; input as raw xterm bytes via `send-keys -H`) and
  `jediterm.go`, which talks line-by-line to `jediterm/JediTermDriver.java` (headless JediTerm
  3.76, pinned in `jediterm/deps.txt`). A terminal that cannot do something skips with a reason.
- `contract_test.go` — the terminal contract (C1–C9 in `docs/design.md`: title, client features,
  Shift+Enter, Ctrl keys, detach, wheel, focus, clipboard, paste, claude exiting), run per terminal.
  Legitimate per-terminal differences are encoded as expectations, not skips.
- `session_test.go` — session lifecycle and server behaviour; `cli_test.go` — argument parsing,
  errors, tool/version checks.

## Documentation conventions

`docs/design.md` is the project's record of tmux/claude behaviour: **Findings** (probed behaviour,
with the tmux versions checked), **Decisions** (numbered) and **Implementation notes**. Behaviour
changes are made together across the code (`internal/session`'s package comment, inline comments,
the usage text in `cmd/cld/usage.go`), `README.md` and `docs/design.md`, with tests. When a change
rests on observed tmux or claude behaviour, record the probe and the versions in Findings.

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
