# cld

[![ci](https://github.com/zadykian/cld/actions/workflows/ci.yml/badge.svg)](https://github.com/zadykian/cld/actions/workflows/ci.yml)

Run [Claude Code](https://code.claude.com) in named sessions, each on a private tmux server of its
own: detach, close the terminal, and reattach later - from the same terminal or another one -
without losing the conversation. The server ignores your `~/.tmux.conf` and is set up for claude:
Shift+Enter, the mouse wheel, focus events, links and clipboard copies work, and the prefix is
`C-q`, which claude leaves free. claude's notifications reach the terminal too, once its setting
`preferredNotifChannel` names a channel your terminal takes: see [Notifications](#notifications).

## Install

```sh
curl -fsSL https://github.com/zadykian/cld/releases/latest/download/install.sh | sh
```

The script downloads cld for your system - Linux or macOS, on amd64 or arm64 - from the latest
release, checks it against the release's `cld.sha256` and installs it as `~/.local/bin/cld`, which
has to be on your `PATH`; `cld update` upgrades it later. `CLD_INSTALL_DIR` installs cld
elsewhere, and `CLD_VERSION` another release, 0.4.0 or later:
`... | CLD_INSTALL_DIR=~/bin CLD_VERSION=0.4.0 sh`. See the [guide](docs/guide.md#installing) for
what it checks, and for installing by hand. From a clone, `make install` builds cld into
`~/.local/bin` (`PREFIX=/usr/local` for another prefix); it needs Go 1.26 or newer.

Requirements: tmux 3.5a or newer, and Claude Code 2.1.232 or newer as `claude` on the `PATH`; git
for `cld new -w`, and to name sessions after their repository; Docker, on Linux, for
`cld setup telemetry`.

Debian 13 has tmux 3.5a and Ubuntu 26.04 3.6a; Debian 12 (3.3a, 3.5a in bookworm-backports),
Ubuntu 24.04 (3.4) and RHEL 9 and 10 (3.2a, 3.3a) ship an older one. tmux 3.7 or newer is
recommended: only there does claude draw with synchronized output, which flickers less.
[Homebrew](https://formulae.brew.sh/formula/tmux) has it on macOS and Linux, as do Debian testing
and unstable; or build a [tmux release](https://github.com/tmux/tmux/releases) from source. cld
0.3.0 still runs on tmux 3.3 and 3.4: see [Upgrading](docs/guide.md#upgrading), which also covers
upgrading tmux and cld.

### Shell completion

`cld setup completion SHELL` sets up completion in bash, zsh or fish: TAB then completes the
commands and their options, the sessions of `cld join -n` and `-s` and the servers of
`cld setup project --mcp`. Run it once, then start a new shell:

```sh
cld setup completion zsh   # or bash, or fish
```

It writes the script that `cld completion SHELL` prints where the shell reads it - for zsh, with
the lines that load it at the end of `~/.zshrc` - and `cld update` writes the script anew when a
new release prints another. bash needs bash-completion 2, which Debian's and Ubuntu's `~/.bashrc`
load, and completes with [ble.sh](https://github.com/akinomyoga/ble.sh) too. The
[guide](docs/guide.md#shell-completion) says where each script goes, and how to set it up by hand,
for macOS's bash 3.2 among others.

## Usage

A session is named `NAME-SUFFIX`: it is the tmux session `cld-NAME-SUFFIX` on a tmux server of its
own, `tmux -L cld-NAME-SUFFIX`, running `claude --name cld-NAME-SUFFIX` with
[Remote Control](https://code.claude.com/docs/en/remote-control) on, so that you can also continue
it from claude.ai or the Claude app.

`-n NAME` gives `NAME`, by default the name of the git repository you are in or, outside one, of
the current directory; `-s SUFFIX` gives `SUFFIX`, which `cld new` otherwise makes an index: `0`, or
where sessions `NAME-INDEX` run, the index above the highest of them. In a repository `api`,
`cld new` twice, `cld new -s fix` and `cld new -n web` make the sessions `cld-api-0`, `cld-api-1`,
`cld-api-fix` and `cld-web-0`, which `cld join -s 1` and `cld kill -n web -s 0` then reach; in
`/root`, outside any repository, `cld new` makes `cld-root-0`. `cld join` and `cld kill` need `-s`,
and `cld resume` `-s` or `SESSION`, with which it names its session as `cld new` does. Repositories
of one name share `NAME`: without `-n`, `cld join` and `cld kill` refuse a session that another
repository or directory of the same name made.

| Command | Action |
|---|---|
| `cld new [-n NAME] [-s SUFFIX] [-w]` | create the session in the current directory and attach to it; with `-w`, claude works in the git worktree `cld-NAME-SUFFIX` |
| `cld resume [-n NAME] [-s SUFFIX] [SESSION]` | create the session with claude resuming the conversation `cld-NAME-SUFFIX`, or `SESSION` |
| `cld join [-n NAME] -s SUFFIX [--detach-others]` | attach to the session, beside any other terminal on it; with `--detach-others`, detach those |
| `cld kill [-n NAME] -s SUFFIX` | end the session, its claude and its tmux server |
| `cld list` | list the sessions: name, state (`attached`, `detached` or `exited`) and claude's directory; on a terminal, join or kill one |
| `cld setup project [--mcp SERVER] [--permissions SET]` | set claude up in the project in the current directory |
| `cld setup telemetry [--local URL] [--remote URL]` | send claude's telemetry through a local OpenTelemetry collector |
| `cld setup completion SHELL` | set up completion in `bash`, `zsh` or `fish` |
| `cld update` | update cld to the latest release, replacing the file it runs from, and the completion scripts |
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

`/background` (`/bg`), and "Move to background and exit" where `/exit` offers it, end the session
too: they move the conversation to Claude Code's
[background sessions](https://code.claude.com/docs/en/agent-view), where it goes on without cld.
`←` on an empty prompt moves it there as well, and leaves claude in the session, in agent view,
where `Esc` goes back to the conversation, which goes on in the background all the same.

The terminal's tab shows the session, `✳ cld-NAME-SUFFIX`, and while claude works `◐` and `◑` in
turn in place of the `✳`, as claude's own title does outside tmux; while claude works in a linked
git worktree, the name ends in ` [w]`. claude tells tmux through hooks that cld gives it; the
[guide](docs/guide.md#sessions) says what they miss. Each hook starts a tmux client, and while the
title turns so does each terminal on the session, every second: a tmux slow to start, such as
Ubuntu's snap, holds claude up a moment as it starts, after each tool and as each turn starts and
ends, and costs CPU for as long as the title turns.

While attached, the terminal shows tmux, and its own scrollback and search see nothing of the
session. In claude's classic renderer, which draws in the terminal's main screen, what scrolls off
goes to tmux's history instead, the last 50000 lines, which the mouse wheel or `C-q [` opens in
tmux's copy mode; claude's fullscreen renderer (`/tui fullscreen`) takes the wheel itself and
scrolls its own transcript. See the [guide](docs/guide.md#scrollback), for screen readers too.

### Notifications

claude notifies you when it finishes a task or waits for a permission while you are away - under
tmux, only on the channel its setting `preferredNotifChannel` names. Its default, `"auto"`, goes by
the terminal claude runs in, which in a session is tmux, and sends nothing. Name your terminal's
channel in `~/.claude/settings.json` - `"iterm2"`, `"kitty"` or `"ghostty"`, or elsewhere
`"terminal_bell"`, the bell - or in `/config`, as "Local notifications":

```json
{
  "preferredNotifChannel": "iterm2"
}
```

cld's server passes them on to every terminal on the session. See the
[guide](docs/guide.md#notifications).

### Resuming a conversation

A session's conversation outlives it: after `cld kill`, a reboot or a crash it stays in Claude
Code's history as `cld-NAME-SUFFIX`, and `cld resume -n NAME -s SUFFIX` resumes it in a new
session - `-s SUFFIX` alone where `NAME` is the repository's. Run it in the conversation's
directory, or anywhere in its git repository. `cld resume SESSION` resumes another conversation: a
session ID, a name, or a search term for claude's picker. Do not resume a
conversation that is open elsewhere: two claudes would write to one transcript, their messages
interleaved, as the [Claude Code docs](https://code.claude.com/docs/en/sessions) say. claude
refuses one that runs in its background sessions, naming `claude attach ID`, which opens it
outside cld, and `claude stop ID`, after which `cld resume` brings it back - after `cld kill`,
where the session stays. See the [guide](docs/guide.md#resuming-a-conversation) for what is not
checked yet.

### Worktrees

`cld new -w` runs `claude --worktree cld-NAME-SUFFIX`, named as the session is: claude
[creates the git worktree](https://code.claude.com/docs/en/worktrees)
`.claude/worktrees/cld-NAME-SUFFIX` on the branch `worktree-cld-NAME-SUFFIX` - from your current
`HEAD` - or reopens it, and works there. `cld kill` leaves the worktree, and `cld resume -s SUFFIX`,
run in the repository, takes the conversation back to it. See the
[guide](docs/guide.md#worktrees).

### Project settings

`cld setup project` sets claude up in the project in the current directory:
`.claude/settings.json` shares through git what claude may do without asking, and where it keeps
plans; `.claude/settings.local.json` is for your own settings; and `.gitignore` keeps those, the
plans and claude's worktrees out of git, so that what the project shares in `.claude` - commands,
agents, skills - goes in. `--mcp` adds MCP servers to `.mcp.json`:

```sh
cld setup project --mcp goland,jbcontext   # or --mcp goland --mcp jbcontext
```

| `--mcp` | Server |
|---|---|
| `goland` | GoLand's own MCP server (Settings › Tools › MCP Server) |
| `rider` | Rider's own MCP server (Settings › Tools › MCP Server) |
| `jbcontext` | JetBrains Context's semantic code search, `jbcontext mcp` |

`--permissions` says what claude may do without asking:

| `--permissions` | claude may |
|---|---|
| `read-only` (default) | read files, run commands that only read (`git status`, `ls`, `cat`, `grep`, ...) and use the servers' tools that only read |
| `cld` | as cld's own repository has it: also edit files and run `git`, `go`, `make`, `docker run` and more, and use every tool of the servers |
| `none` | nothing more than claude allows by itself |

The settings are shared: whoever accepts the folder's workspace trust gives claude what they allow,
and `cld` amounts to running commands without a prompt - review the changes before you commit
them. An IDE's port comes from `GOLAND_MCP_PORT` or `RIDER_MCP_PORT` where claude runs, or else is
the IDE's default. cld edits files that exist in place, adding what they lack - a setting the
project has keeps its value, and a server's entry in `.mcp.json` that differs from cld's is
replaced - and removing nothing, so running it again changes nothing. See the
[guide](docs/guide.md#project-settings).

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

## cld and Claude Code's background sessions

Claude Code keeps conversations running without a terminal too, and without tmux: `claude --bg`,
or `/bg` or `←` in a conversation, hands one to a supervisor process; `claude attach ID` opens it
again, and [agent view](https://code.claude.com/docs/en/agent-view), `claude agents`, lists them
all on one screen. cld keeps claude as you run it in a terminal: a session goes by a name, not an
ID, and TAB completes it; claude keeps running until you end it, idle or not; and it renders as
your settings say, where an attached background session is always fullscreen. The
[guide](docs/guide.md#cld-and-claude-codes-background-sessions) compares the two point by point,
and says how they combine.

## cld and `claude --tmux`

Claude Code has its own tmux option, `claude --worktree [name] --tmux`, for a different problem:

| | `cld` | `claude --worktree --tmux` |
|---|---|---|
| Purpose | a named conversation you detach from and come back to | a new session working in an isolated git worktree |
| Working copy | the current directory, or with `-w` a git worktree claude creates | a new git worktree per session (`--tmux` requires `--worktree`) |
| Coming back | `cld join -s SUFFIX`; once the session has ended, `cld resume -s SUFFIX` | not documented |
| tmux configuration | a private server per session that ignores `~/.tmux.conf` and sets what claude needs | not documented; [the docs](https://code.claude.com/docs/en/terminal-config#configure-tmux) advise settings for `~/.tmux.conf` |
| iTerm2 | a regular tmux client | native panes when available; `--tmux=classic` for regular tmux |

The `claude --tmux` column follows the option's row in Claude Code's
[CLI reference](https://code.claude.com/docs/en/cli-reference) and `claude --help` in 2.1.284.
`cld new -w` combines the two: claude's own worktree, in a session of cld's.

## Terminals

The tests check one contract - title, Shift+Enter, Ctrl keys, detach, mouse wheel and clicks,
focus, clipboard, notifications, links, paste, claude exiting, and the keys of the session list -
against each terminal:

| Terminal | How it is tested | Differences |
|---|---|---|
| xterm-compatible (baseline) | a pane of an outer tmux server | none |
| JetBrains IDEs (JediTerm) | JediTerm's emulator, headless, typing through its own key handling | Shift+Enter arrives as ESC CR (the IDE's newline setting); no focus reports; no OSC 52 |
| iTerm2 | not automated yet | |

## Development

```sh
make docker-check                    # as CI: tmux 3.7c built from source, baseline and JediTerm
make docker-check TMUX_VERSION=3.5a  # the same on the oldest tmux cld runs on, as CI does too
make check                           # natively, baseline terminal only
make check TERMINALS=tmux,jediterm   # natively, with JediTerm
```

Natively, the checks need Go, tmux 3.5a or newer, ShellCheck and shfmt; for JediTerm also a JDK,
and its jars, fetched once with `tests/jediterm/fetch-deps tests/jediterm/lib`.

[docs/design.md](docs/design.md) records the tmux and claude behaviour cld relies on, the decisions
taken and how the tests work. Pushing a tag `vX.Y.Z` publishes a release with cld for each
platform, `cld.sha256` and `install.sh`; `make dist VERSION=X.Y.Z` builds the same into `dist/`.

## License

[MIT](LICENSE)
