# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`cld` runs Claude Code in named sessions, each on a private tmux server of its own
(`tmux -L cld-NAME -f /dev/null`), so a conversation can be detached and rejoined from any
terminal. The product is a Go program on cobra: `cmd/cld` is the command line (commands, their
help texts, argument errors), `internal/session` the tmux side, `internal/picker` the interactive
`cld list` on a terminal, and `internal/fail` carries exit statuses up to `main`. Everything else
is its test harness (Go, under `tests/`), docs and CI.

## Commands

```sh
make check                              # lint + test, natively (baseline terminal only)
make lint                               # gofmt, go vet; shellcheck, shfmt -i 4 on fetch-deps
make test                               # cd tests && go test -count=1 ./...
cd tests && go test -count=1 -run 'TestList$' .   # a single test
cd tests && go test -count=1 -run 'TestHelpText$' . -update   # rewrite testdata/help from cld
make check TERMINALS=tmux,jediterm      # add JediTerm: needs a JDK and, once,
                                        #   tests/jediterm/fetch-deps tests/jediterm/lib
make docker-check                       # same as CI: tmux 3.7c built from source, tmux + jediterm
make docker-image TMUX_VERSION=X        # an image with another tmux release, to try it by hand
make dist VERSION=X.Y.Z                 # dist/cld-OS-ARCH, linux/darwin x amd64/arm64, + cld.sha256
make install PREFIX=DIR                 # build cld for the host into DIR/bin (VERSION stamps it)
```

Native runs need Go, tmux 3.7 or newer, ShellCheck and shfmt. CI (`.github/workflows/ci.yml`)
runs the Docker image in one job, `linux`, on the pinned tmux 3.7c, and `make check` on macOS with
Homebrew tmux. Pushing a tag `vX.Y.Z` runs the checks and publishes a release: a binary per
platform and `cld.sha256`.

## Constraints on cld

- Requires **tmux 3.7 or newer**, the release the tests run on (3.7c, pinned in
  `tests/Dockerfile` and the `Makefile`); there is no behaviour per tmux version. The check reads
  `tmux -V` at startup. Raising the minimum is one change: the pin, the check, the docs.
- `new` and `resume` require **claude 2.1.222 or newer**, the first release that takes what cld
  passes and does what it relies on (the tests never run the real claude): they run
  `claude --version` before starting that claude; `join`, `kill` and `list` do not. Re-derive
  the minimum when cld starts to pass or rely on something newer (docs/design.md, decision 6).
- Builds with `CGO_ENABLED=0` for linux and darwin on amd64 and arm64 (so no `ttyname`: cld runs
  `tty`). gofmt and go vet must pass; ShellCheck and `shfmt -i 4` for `tests/jediterm/fetch-deps`.
- Only `main` exits: errors carry their exit status up (`internal/fail`); `new`, `resume`, `join`
  and the list's Enter end in `syscall.Exec` of tmux. cobra's defaults are overridden to keep
  cld's command line - the first argument checked before cobra, options read up to the first
  argument, a `help [COMMAND]` that refuses anything but one of cld's commands, the help printed
  through `fail.Print` with the commands unsorted, no completion command (see docs/design.md,
  Implementation notes).
- The help is cobra's, generated with its default templates from each command's `Use`, `Short`
  and `Long` and its option usages (value names in backquotes: `` `NAME` ``). cobra wraps
  nothing: break the texts by hand within 80 columns, which `TestHelpText` checks.
- Sessions are always addressed as `=cld-NAME` (exact match); a bare target would prefix-match
  `cld-rev` to `cld-review`. `set` targets use `=cld-NAME:` because `set` takes a pane.
- Names are validated (`^[A-Za-z0-9][A-Za-z0-9_-]*$`, at most 64 characters so that the socket
  path fits in `sun_path`), never sanitised.
- claude is passed to tmux as separate argv words so tmux execs it directly, not via `sh -c`,
  and by the path of the claude `new` or `resume` checked, so tmux does not look `claude` up in
  the `PATH`; a word ending in `;` (resume's SESSION can) goes with a `\` before the `;`, since
  tmux would end its command there.
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
  window, not the server, so the sessions claude makes on its server behave as plain tmux would.
- `TERMINAL_EMULATOR` is removed from the environment `new` and `resume` exec tmux with, so from
  the server's; claude trusts it over `TERM_PROGRAM=tmux`.

The package comment of `internal/session` explains why each tmux option is set; keep it accurate
when changing options.

## Test architecture (`tests/`)

Tests build cld (`cmd/cld`) and run it against real tmux; only `claude` is faked. Read the
package doc comments at the top of each file for details.

- `main_test.go` — `TestMain` builds cld and `probe/` into a temp dir, the probe as `claude` (and
  symlinks it as a fake `tmux` for version/tool checks and the commands `new`, `resume` and
  `join` exec), and compiles the JediTerm driver when `CLD_TERMINALS` includes `jediterm`.
  `forEachTerminal` runs a body as a parallel subtest per terminal.
- `probe/` — stands in for claude: enters the same terminal modes claude does, logs argv/cwd/env
  (`PID.json`) and raw input bytes (`PID.in`) to `$CLD_PROBE_DIR`, and takes commands through a
  FIFO (`PID.ctl`: `title`, `osc52`, `loadbuffer`, `rekey`, `inline`, `cd`, `tmux`, `exit`).
  `CLD_PROBE_FAIL` makes it fail at startup. `claude --version` answers first, writing nothing,
  with `CLD_FAKE_CLAUDE_VERSION` (`99.0.0 (Claude Code)` when unset). Invoked as `tmux`, it fakes
  `tmux -V` via `CLD_FAKE_TMUX_VERSION` and `list-sessions` via `CLD_FAKE_TMUX_SESSIONS` (unset:
  no server running; `CLD_FAKE_TMUX_EXITED` names servers that exit as they are asked), and
  records any other command in `tmux.json`, or runs the real tmux `CLD_FAKE_TMUX_REAL` names.
- `internal/sandbox` — an isolated world per test: its own short `TMUX_TMPDIR` (socket paths hit
  the ~108-byte `sun_path` limit), `HOME`, `PATH` with the probe first, `TMUX` unset. Tests are
  parallel and never touch the user's own cld sessions. `Tmux(server, ...)` runs tmux against one
  server (`cld-NAME` for session NAME); `Sessions()` and `Clients()` span every `cld-*` server,
  naming a session that is not on its own server `SERVER/SESSION`.
- `internal/terminal` — the `Terminal` interface with one driver per outer terminal: `tmux.go`
  (an outer tmux server provides the pty; input as raw xterm bytes via `send-keys -H`) and
  `jediterm.go`, which talks line-by-line to `jediterm/JediTermDriver.java` (headless JediTerm
  3.76, pinned in `jediterm/deps.txt`). A terminal that cannot do something skips with a reason.
- `contract_test.go` — the terminal contract (C1–C10 in `docs/design.md`: title, client features,
  Shift+Enter, Ctrl keys, detach, wheel, focus, clipboard, paste, claude exiting, the session
  list's keys), run per terminal.
  Legitimate per-terminal differences are encoded as expectations, not skips.
- `session_test.go` — session lifecycle and server behaviour; `cli_test.go` — argument parsing,
  errors, tool/version checks, and the help, compared byte for byte with `testdata/help`.

## Documentation conventions

`docs/design.md` is the project's record of tmux/claude behaviour: **Findings** (probed behaviour,
with the tmux versions checked), **Decisions** (numbered) and **Implementation notes**. Behaviour
changes are made together across the code (`internal/session`'s and `internal/picker`'s package
comments, inline comments, the commands' `Short`, `Long` and option usages in `cmd/cld`, which the
help is generated from, and `tests/testdata/help`), `README.md` and `docs/design.md`, with tests.
When a change rests on observed tmux or claude behaviour, record the probe and the versions in
Findings.

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
