# CLAUDE.md

`cld` runs Claude Code in named sessions, each on a private tmux server of its own
(`tmux -L cld-NAME -f /dev/null`), so that a conversation can be detached and rejoined from any
terminal. cld is a Go program on cobra, which `install.sh`, published with each release,
installs. The rest of the repository is its test harness, the lint gates' tools, docs and CI.

## Packages

- `cmd/cld`: the command line, a file per command with its help text.
- `cmd/cld/internal`: what the commands share. `cmdline` holds the checks of arguments and
  options and their errors, `naming` the `-n` and `-s`, and `idle` `CLD_IDLE_DAYS` and the sweep.
- `internal/session`: the tmux side, cld's record of its sessions, and `cld restore`.
- `internal/picker`: the interactive `cld list` on a terminal.
- `internal/project`: `cld setup config project` and `user`: a project's `.claude` settings,
  `.mcp.json` and `.gitignore`, and claude's user settings.
- `internal/completion`: `cld setup completion`, the script where bash, zsh or fish reads it.
- `internal/restore`: `cld setup restore`, a systemd user unit that runs `cld restore`.
- `internal/update`: `cld update`, which replaces cld with the latest release.
- `internal/configfile`: edits in place the files the `setup` commands write.
- `internal/tool`: finds the programs cld runs on the `PATH`, and ends cld when one cannot run.
- `internal/fail`: carries exit statuses up to `main`.
- `internal/output`: prints cld's output, warnings and notes; a failed write is an exit.
- `tests/`: the test harness, in Go. `tools/`: the lint gates' tools.
- `docs/design.md`: the index of `docs/design/`, the overview, testing, the findings and a record
  per decision. `docs/guide.md`: the index of the user guide, a page per topic in `docs/guide/`.

## Commands

```sh
make check                              # vet + test, natively (baseline terminal only)
make vet                                # gofmt, go vet; shellcheck, shfmt -i 4 on the scripts
make lint                               # vet + gates, as CI's lint job; tools/run pins the tools
make vale FILES=README.md               # one gate of lint's alone; sizecheck and vale take FILES
make test                               # cd tests && go test -count=1 ./...
cd tests && go test -count=1 -run 'TestList$' .   # a single test
cd tests && go test -count=1 -run 'TestHelpText$' . -update   # rewrite testdata/help from cld
make check TERMINALS=tmux,jediterm      # add JediTerm: needs a JDK and, once,
                                        #   tests/jediterm/fetch-deps tests/jediterm/lib
make docker-check                       # same as CI: tmux 3.7c built from source, tmux + jediterm
make docker-check TMUX_VERSION=3.5a     # the same on the oldest tmux cld runs on, as CI does too
make docker-blesh-check                 # the completion tests in bash with Ubuntu 26.04's ble.sh
make docker-image TMUX_VERSION=X        # an image with another tmux release, to try it by hand
make dist VERSION=X.Y.Z                 # dist/cld-OS-ARCH (4 platforms), cld.sha256, install.sh
make install PREFIX=DIR                 # build cld for the host into DIR/bin (VERSION stamps it)
```

Native runs need Go, tmux 3.5a or newer, ShellCheck and shfmt; `make lint` fetches the tools it
pins. CI (`.github/workflows/ci.yml`) runs `linux` on tmux 3.7c and `linux-oldest` on 3.5a, both
in Docker, `blesh` with ble.sh, `macos` with Homebrew tmux, and `lint`. A tag `vX.Y.Z` publishes
a release: a binary per platform, `cld.sha256` and `install.sh`.

## How changes land

- Pull requests land on `main` by fast-forward only, as the commits CI checked.
- Never run `gh pr merge`: the repository and the ruleset on `main` refuse every merge method.
- Rebase onto `main`, then comment `/fast-forward` on the pull request
  (`.github/workflows/fast-forward.yml`). It pushes once `linux`, `macos` and `lint` pass and
  every conversation is resolved.
- The maintainer pushes a pull request that changes `.github/workflows` by hand,
  `git push origin SHA:main`, as the workflow's token may not push such a change.
- `.claude/settings.json` denies `gh pr merge` and the usual forms of a push to `main`. A rule
  matches the command as written, so the ruleset on `main` stays the guard.

## Hard rules

Each rule links the decision record in `docs/design/decisions/` that gives its reasons.

- tmux 3.5a or newer, checked at startup; raising it changes `linux-oldest`'s pin, the check and the
  docs together ([decision 6](docs/design/decisions/0006-versions.md)).
- claude 2.1.232 or newer, checked only where cld starts claude; re-derive it when cld passes or
  relies on something newer ([decision 6](docs/design/decisions/0006-versions.md)).
- cld builds with `CGO_ENABLED=0` for linux and darwin on amd64 and arm64, so it runs `tty`
  where C would call `ttyname` ([overview](docs/design/overview.md#distribution)).
- Only `main` exits: every error carries its status up through `internal/fail`
  ([decision 11](docs/design/decisions/0011-go-and-cobra.md)).
- Session names are validated, never sanitised, and 64 characters at most
  ([decision 1](docs/design/decisions/0001-naming.md)).
- Address a session as `=cld-NAME`: a bare target prefix-matches, `cld-rev` finding `cld-review`
  ([decision 3](docs/design/decisions/0003-commands.md)).
- One tmux server per session, marked as cld's; an unmarked server `cld-NAME` is the user's, and cld
  never ends it (decisions [13](docs/design/decisions/0013-a-server-per-session.md) and
  [34](docs/design/decisions/0034-servers-are-marked.md)).
- `--settings` carries only cld's hooks, `disableAgentView` and `worktree.baseRef`: what is the
  user's to choose stays claude's own setting (decisions
  [42](docs/design/decisions/0042-remote-control-is-claudes.md) and
  [47](docs/design/decisions/0047-agent-view-off.md)).
- `make dist`'s names `cld-OS-ARCH` and `cld.sha256` change together with `install.sh` and
  `internal/update`, which rely on them (decisions
  [20](docs/design/decisions/0020-install-script.md) and
  [21](docs/design/decisions/0021-self-update.md)).
- `setup config project --mcp goland --permissions cld` writes this repository's `.mcp.json` and
  `.claude/settings.json` byte for byte, but for the hooks, which stay the settings' last key.
  Change them with `internal/project` (decisions
  [19](docs/design/decisions/0019-project-settings.md) and
  [28](docs/design/decisions/0028-shared-project-settings.md)).
- Run `setup config user`, `setup completion` and `setup restore` only with `HOME` and the
  variables they follow pointing at a scratch directory, and `setup restore` never against the
  user's own systemd (decisions [22](docs/design/decisions/0022-setting-completion-up.md),
  [48](docs/design/decisions/0048-restore-after-reboot.md) and
  [53](docs/design/decisions/0053-project-and-user-settings.md)).
- `make lint` fails on any finding, warnings included, in any line of any file
  ([testing](docs/design/testing.md#layers); #121).
- Fix a finding, or justify it in place (`//nolint:LINTER // reason`); never lower a severity or
  add an exclusion.
- The baselines, `tools/sizecheck/baseline.txt` and `tools/valecheck.txt`, are empty and stay so:
  every file passes every gate outright (#125).

## Commits

Messages follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/), as
`git log` on `main` shows them.

- The header is `type(scope): description`. The type is `feat`, `fix`, `build`, `chore`, `ci`,
  `docs`, `perf`, `refactor`, `style` or `test`.
- The scope, optional, is the part changed: `cli` (`cmd/cld`), `session`, `tests`, `jediterm` or
  `docs`. The description is short, imperative and lower case, with no trailing period.
- The body is a list of bullets wrapped at 72 columns: what was wrong or missing, what changed,
  the tmux and claude versions checked, and what the tests cover.
- A breaking change takes `!` before the colon and a `BREAKING CHANGE:` footer saying what users
  must change.

## Where else to look

- `.claude/rules/`: each area's invariants, what each document holds in
  [docs.md](.claude/rules/docs.md), and the writing policy in
  [writing.md](.claude/rules/writing.md), loaded with the files they cover.
- Skills in `.claude/skills/`: `adr`, `split-file`, `trim`, `issue`, and `land`, run by hand only.
- Agents in `.claude/agents/`: `go-reviewer`, which reviews Go and edits nothing, and `doc-editor`.
- Hooks in `.claude/hooks/`: the file's gates after each Edit or Write, `make lint` as a turn ends.
