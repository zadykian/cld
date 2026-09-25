# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`cld` runs Claude Code in named sessions on a private tmux server (`tmux -L cld -f /dev/null`), so
a conversation can be detached and rejoined from any terminal. The whole product is one bash
script, `bin/cld`; everything else is its test harness (Go, under `tests/`), docs and CI.

## Commands

```sh
make check                              # lint + test, natively (baseline terminal only)
make lint                               # shellcheck, shfmt -i 4, gofmt, go vet
make test                               # cd tests && go test -count=1 ./...
cd tests && go test -count=1 -run 'TestList$' .   # a single test
CLD_BASH=/bin/bash make test            # run cld under a specific bash (CI: macOS bash 3.2)
make check TERMINALS=tmux,jediterm      # add JediTerm: needs a JDK and, once,
                                        #   tests/jediterm/fetch-deps tests/jediterm/lib
make docker-check                       # same as CI: tmux 3.3a (debian:bookworm), tmux + jediterm
make docker-check BASE=ubuntu:24.04     # tmux 3.4; debian:trixie has 3.5a
make docker-check BASE=debian:trixie TMUX_VERSION=3.7c   # tmux built from source
make dist VERSION=X.Y.Z                 # dist/cld with CLD_VERSION stamped in, plus cld.sha256
```

Native runs need Go, tmux, ShellCheck and shfmt. CI (`.github/workflows/ci.yml`) runs the Docker
image on tmux 3.3a, 3.4, 3.5a and 3.7c, and `make check` on macOS with Homebrew tmux and
`/bin/bash` 3.2. Pushing a tag `vX.Y.Z` runs the checks and publishes a release.

## Constraints on `bin/cld`

- Must run on **bash 3.2** (macOS) and **tmux 3.3 or newer**; behaviour differs by tmux version
  (e.g. `remain-on-exit failed` only from 3.5, because 3.3/3.4 crash over a dead pane that had
  focus reporting on). Version gating is done from `tmux -V` at startup.
- Formatting is `shfmt -i 4`; ShellCheck must pass.
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
- `TERMINAL_EMULATOR` is stripped (`env -u`) from the server's environment; claude trusts it over
  `TERM_PROGRAM=tmux`.

The long comment at the top of `bin/cld` explains why each tmux option is set; keep it accurate
when changing options.

## Test architecture (`tests/`)

Tests run the real `bin/cld` against real tmux; only `claude` is faked. Read the package doc
comments at the top of each file for details.

- `main_test.go` — `TestMain` builds `probe/` into a temp dir as `claude` (and symlinks it as a
  fake `tmux` for version/tool checks), and compiles the JediTerm driver when `CLD_TERMINALS`
  includes `jediterm`. `forEachTerminal` runs a body as a parallel subtest per terminal.
- `probe/` — stands in for claude: enters the same terminal modes claude does, logs argv/cwd/env
  (`PID.json`) and raw input bytes (`PID.in`) to `$CLD_PROBE_DIR`, and takes commands through a
  FIFO (`PID.ctl`: `title`, `osc52`, `loadbuffer`, `rekey`, `inline`, `cd`, `tmux`, `exit`).
  `CLD_PROBE_FAIL` makes it fail at startup. Invoked as `tmux`, it fakes `tmux -V` via
  `CLD_FAKE_TMUX_VERSION`.
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
changes are made together across `bin/cld` (header comment, inline comments, `usage`), `README.md`
and `docs/design.md`, with tests. When a change rests on observed tmux or claude behaviour, record
the probe and the versions in Findings.

Commit messages: a short imperative subject in sentence case, then a bullet list saying what was
wrong or missing, what changed, which tmux/claude versions it was checked on, and what the tests
cover.
