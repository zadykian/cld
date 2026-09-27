# cld

[![ci](https://github.com/zadykian/cld/actions/workflows/ci.yml/badge.svg)](https://github.com/zadykian/cld/actions/workflows/ci.yml)

Run [Claude Code](https://code.claude.com) in named sessions, each on a private tmux server of its
own: detach, close the terminal, and reattach later - from the same terminal or another one -
without losing the conversation. The server ignores your `~/.tmux.conf` and is set up for claude:
Shift+Enter, the mouse wheel, focus events, notifications and clipboard copies work, and the prefix
is `C-q`, which claude leaves free.

## Install

```sh
os=$(uname -s | tr '[:upper:]' '[:lower:]') arch=$(uname -m); case $arch in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
mkdir -p ~/.local/bin && curl -fsSL "https://github.com/zadykian/cld/releases/latest/download/cld-$os-$arch" -o ~/.local/bin/cld && chmod +x ~/.local/bin/cld
```

`~/.local/bin` has to be on your `PATH`. Releases publish cld for Linux and macOS on amd64 and
arm64, with their checksums in `cld.sha256`. From a clone, `make install` builds cld into
`~/.local/bin` (`PREFIX=/usr/local` for another prefix); it needs Go 1.26 or newer.

Requirements: tmux 3.7 or newer, and Claude Code 2.1.232 or newer as `claude` on the `PATH`; git
for `cld new -w`; Docker, on Linux, for `cld setup telemetry`.

Most distributions ship an older tmux - Debian 13 has 3.5a, Ubuntu 26.04 3.6a.
[Homebrew](https://formulae.brew.sh/formula/tmux) has 3.7 on macOS and Linux, as do Debian testing
and unstable; or build a [tmux release](https://github.com/tmux/tmux/releases) from source. cld
0.3.0 still runs on tmux 3.3 to 3.6: see [Upgrading](docs/guide.md#upgrading), which also covers
upgrading tmux and cld.

### Shell completion

`cld completion SHELL` prints a completion script for bash, zsh or fish: TAB then completes the
commands and their options, the sessions of `cld join -n` and the servers of
`cld setup project --mcp`. `cld completion SHELL --help` says where the script goes; for fish:

```sh
cld completion fish > ~/.config/fish/completions/cld.fish
```

The [guide](docs/guide.md#shell-completion) has bash and zsh, macOS's bash 3.2 included.

## Usage

Session `NAME` - `main` without `-n` - is the tmux session `cld-NAME` on a tmux server of its own,
`tmux -L cld-NAME`, running `claude --name cld-NAME` with
[Remote Control](https://code.claude.com/docs/en/remote-control) on, so that you can also continue
it from claude.ai or the Claude app.

| Command | Action |
|---|---|
| `cld new [-n NAME] [-w]` | create the session in the current directory and attach to it; with `-w`, claude works in the git worktree `NAME` |
| `cld resume [-n NAME] [SESSION]` | create the session with claude resuming the conversation `cld-NAME`, or `SESSION` |
| `cld join [-n NAME]` | attach to the session, detaching any other terminal from it |
| `cld kill [-n NAME]` | end the session, its claude and its tmux server |
| `cld list` | list the sessions: name, state (`attached`, `detached` or `exited`) and claude's directory; on a terminal, join or kill one |
| `cld setup project [--mcp SERVER]` | set claude up in the project in the current directory |
| `cld setup telemetry [--local URL] [--remote URL]` | send claude's telemetry through a local OpenTelemetry collector |
| `cld completion SHELL` | print the completion script for `bash`, `zsh` or `fish` |
| `cld help [COMMAND]` | show the help of cld, or of a command; `-h` and `--help` do the same |
| `cld version` | show the version |

| Keys | Action |
|---|---|
| `C-q d` | detach; claude keeps running |
| `C-q C-q` | send `C-q` to claude |

| Keys in `cld list` | Action |
|---|---|
| `↑` / `↓` | select a session |
| `Enter` | join it |
| `Ctrl+X` twice within two seconds | kill it; `Esc` after the first keeps it |
| `Esc`, `Ctrl+C` | leave, printing the table |

Leaving claude (`/exit`, `Ctrl+C` twice) ends its session. If claude exits with an error, the
session stays with its message on screen, as `exited` in `cld list`, until `cld kill` ends it.
The [guide](docs/guide.md) has more on sessions, the list and what to do when cld refuses a name.

### Resuming a conversation

A session's conversation outlives it: after `cld kill`, a reboot or a crash it stays in Claude
Code's history as `cld-NAME`, and `cld resume -n NAME` resumes it in a new session. Run it in the
conversation's directory, or anywhere in its git repository. `cld resume -n NAME SESSION` resumes
another conversation: a session ID, a name, or a search term for claude's picker. Do not resume a
conversation that is open elsewhere: two claudes would write to one transcript, their messages
interleaved, as the [Claude Code docs](https://code.claude.com/docs/en/sessions) say. See the
[guide](docs/guide.md#resuming-a-conversation) for what is not checked yet.

### Worktrees

`cld new -n NAME -w` runs `claude --worktree NAME`: claude
[creates the git worktree](https://code.claude.com/docs/en/worktrees) `.claude/worktrees/NAME` on
the branch `worktree-NAME` - from your current `HEAD` - or reopens it, and works there. `cld kill`
leaves the worktree, and `cld resume -n NAME`, run in the repository, takes the conversation back
to it. See the [guide](docs/guide.md#worktrees).

### Project settings

`cld setup project` sets claude up in the project in the current directory, as cld's own
repository has it: `.claude/settings.json` shares through git what claude may do without asking,
and a few settings more; `.claude/settings.local.json` is for your own; and `.gitignore` keeps the
rest of `.claude` out of git. `--mcp` adds MCP servers to `.mcp.json`, which claude may then use
without asking:

```sh
cld setup project --mcp goland,jbcontext   # or --mcp goland --mcp jbcontext
```

| `--mcp` | Server |
|---|---|
| `goland` | GoLand's own MCP server (Settings › Tools › MCP Server) |
| `rider` | Rider's own MCP server (Settings › Tools › MCP Server) |
| `jbcontext` | JetBrains Context's semantic code search, `jbcontext mcp` |

An IDE's port comes from `GOLAND_MCP_PORT` or `RIDER_MCP_PORT` where claude runs, or else is the
IDE's default. cld edits files that exist in place, adding what they lack and removing nothing, so
running it again changes nothing. See the [guide](docs/guide.md#project-settings).

### Telemetry

`cld setup telemetry` sends Claude Code's
[telemetry](https://code.claude.com/docs/en/monitoring-usage) through an
[OpenTelemetry Collector](https://opentelemetry.io/docs/collector/) that it runs in Docker, as the
container `cld-telemetry`, and points claude's user settings at it:

```sh
cld setup telemetry --local http://127.0.0.1:4319 --remote http://otel.example.com:4317
```

`--local` gets traces, metrics and logs - the JetBrains OpenTelemetry plugin in your IDE, say -
and `--remote` metrics only, such as a team's collector; give either, or both.
`--collector-config FILE` merges your YAML over the collector's config, for headers or TLS. Linux
only. See the [guide](docs/guide.md#telemetry), turning it off included.

## cld and `claude --tmux`

Claude Code has its own tmux option, `claude --worktree [name] --tmux`, for a different problem:

| | `cld` | `claude --worktree --tmux` |
|---|---|---|
| Purpose | a named conversation you detach from and come back to | a new session working in an isolated git worktree |
| Working copy | the current directory, or with `-w` a git worktree claude creates | a new git worktree per session (`--tmux` requires `--worktree`) |
| Coming back | `cld join -n NAME`; once the session has ended, `cld resume -n NAME` | not documented |
| tmux configuration | a private server per session that ignores `~/.tmux.conf` and sets what claude needs | not documented; [the docs](https://code.claude.com/docs/en/terminal-config#configure-tmux) advise settings for `~/.tmux.conf` |
| iTerm2 | a regular tmux client | native panes when available; `--tmux=classic` for regular tmux |

The `claude --tmux` column follows `claude --help` in Claude Code 2.1.281, as the docs do not
describe the option yet. `cld new -n NAME -w` combines the two: claude's own worktree, in a session
of cld's.

## Terminals

The tests check one contract - title, Shift+Enter, Ctrl keys, detach, mouse wheel, focus,
clipboard, paste, claude exiting, and the keys of the session list - against each terminal:

| Terminal | How it is tested | Differences |
|---|---|---|
| xterm-compatible (baseline) | a pane of an outer tmux server | none |
| JetBrains IDEs (JediTerm) | JediTerm's emulator, headless, typing through its own key handling | Shift+Enter arrives as ESC CR (the IDE's newline setting); no focus reports; no OSC 52 |
| iTerm2 | not automated yet | |

## Development

```sh
make docker-check                   # as CI: tmux 3.7c built from source, baseline and JediTerm
make check                          # natively, baseline terminal only
make check TERMINALS=tmux,jediterm  # natively, with JediTerm
```

Natively, the checks need Go, tmux 3.7 or newer, ShellCheck and shfmt; for JediTerm also a JDK, and
its jars, fetched once with `tests/jediterm/fetch-deps tests/jediterm/lib`.

[docs/design.md](docs/design.md) records the tmux and claude behaviour cld relies on, the decisions
taken and how the tests work. Pushing a tag `vX.Y.Z` publishes a release with cld for each
platform and `cld.sha256`; `make dist VERSION=X.Y.Z` builds the same into `dist/`.

## License

[MIT](LICENSE)
