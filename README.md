# cld

[![ci](https://github.com/zadykian/cld/actions/workflows/ci.yml/badge.svg)](https://github.com/zadykian/cld/actions/workflows/ci.yml)

Run [Claude Code](https://code.claude.com) in named sessions, each on a private tmux server of its
own. Detach, close the terminal, and join again later, from that terminal or another, with the
conversation as you left it. The server ignores your `~/.tmux.conf` and is set up for claude:
Shift+Enter, the mouse wheel, focus events, links and clipboard copies work, with
[gaps in JetBrains IDEs](docs/guide/terminal.md#jetbrains-ides). Its prefix is `C-q`, which claude
leaves free.

## Install

```sh
curl -fsSL https://github.com/zadykian/cld/releases/latest/download/install.sh | sh
```

The script installs cld for Linux or macOS, on amd64 or arm64, as `~/.local/bin/cld`, which has to
be on your `PATH`. It checks the download against the release's `cld.sha256` first.
`CLD_INSTALL_DIR` installs cld elsewhere, and `CLD_VERSION` another release, 0.4.0 or later:
`... | CLD_INSTALL_DIR=~/bin CLD_VERSION=0.4.0 sh`. From a clone, `make install` builds cld into
`~/.local/bin`, or under another `PREFIX`, with Go 1.26 or newer. `cld update` upgrades cld later.
The guide covers [installing](docs/guide/install-and-upgrade.md) and
[upgrading](docs/guide.md#upgrading).

cld needs:

- tmux 3.5a or newer. Debian 13 has 3.5a and Ubuntu 26.04 has 3.6a. Debian 12 ships 3.3a, with
  3.5a in bookworm-backports. Ubuntu 24.04 (3.4) and RHEL 9 and 10 (3.2a, 3.3a) ship older ones.
  [Homebrew](https://formulae.brew.sh/formula/tmux) has a newer one on macOS and Linux, as do
  Debian testing and unstable. Or build a [tmux release](https://github.com/tmux/tmux/releases).
  tmux 3.7 or newer is best: only there does claude draw with synchronized output, which flickers
  less. cld 0.3.0 still runs on tmux 3.3 and 3.4 (see [upgrading](docs/guide.md#upgrading)).
- Claude Code 2.1.232 or newer, as `claude` on the `PATH`.
- git, for `cld join -w` and to name sessions after their repository.
- On Linux, systemd for `cld setup restore`.

## Usage

A session is named `NAME-SUFFIX`, which `-n NAME` and `-s SUFFIX` give. `NAME` is by default the
name of the git repository you are in, or else of the current directory, and `SUFFIX` the next
index, for a new session. In a repository `api`, `cld join` twice makes the sessions `api-0` and
`api-1`, and `cld join -s 1` attaches to the second again.

| Command | What it does |
|---|---|
| `cld join` | attach to a session, bringing it back where it has ended, or creating it |
| `cld detach` | detach the terminals from a session |
| `cld kill` | end a session, its claude and its tmux server |
| `cld list` | list the sessions and claude's status in each; on a terminal, join or kill one |
| `cld restore` | bring back the sessions that ran when the machine stopped |
| `cld setup` | set up claude's settings, shell completion, or a restore at login |
| `cld update` | update cld to the latest release |

`cld help COMMAND` gives each command's options. In a session, `C-q d` detaches, and `C-q s` shows
the session list over it, to move the terminal to another session. `cld help` names the other
keys. Where the terminal keeps `Ctrl+Q`, as VS Code and Rider can, `! cld detach` in claude
detaches it (see [terminals that keep C-q](docs/guide/terminal.md#terminals-that-keep-c-q)).

## Features

- **Sessions.** Repositories of one name share `NAME`: without `-n`, cld refuses a running session
  that another of them made. claude keeps the environment of the shell that started its session.
  See [sessions](docs/guide/sessions.md).
- **Ending.** Leaving claude, with `/exit`, ends its session. A claude that fails keeps its
  session, with its message on screen and a line saying how to end it, until `cld kill`. See
  [ending a session](docs/guide/sessions.md#ending-a-session).
- **The tab's title** names the session, and its mark turns while claude works. `cld list` shows
  whether claude is busy, waiting for you or idle. See [in the terminal](docs/guide/terminal.md).
- **Scrollback.** The terminal shows tmux, and its own scrollback sees nothing of the session. In
  claude's classic renderer, tmux keeps the last 50000 lines, which the wheel or `C-q [` opens in
  copy mode. See [scrollback and screen readers](docs/guide/terminal.md#scrollback).
- **Notifications** reach every terminal on the session, once claude's setting
  `preferredNotifChannel` names your terminal's channel: its default sends none under tmux.
  `cld setup config user --notifications CHANNEL` sets it. See
  [notifications](docs/guide/terminal.md#notifications).
- **Inside your own tmux**, claude gets only what that tmux lets through, and the session's last
  line names the keys it keeps. See [inside your own tmux](docs/guide/inside-your-own-tmux.md).
- **Moving between sessions.** `C-q s`, `C-q (`, `C-q )` and `C-q L` move the terminal, and so does
  `! cld join` in claude. The session left runs on. See
  [the session list](docs/guide/session-list.md#moving-between-sessions).
- **Idle sessions.** A session with no terminal attached and no key typed for over 30 days ends at
  the next `cld list`, or `cld join` without `-s`. `CLD_IDLE_DAYS` sets the days. See
  [idle sessions](docs/guide/idle-sessions.md).
- **Resuming.** cld keeps a record of each session for 30 days. `cld join` brings an ended session
  back, its conversation resumed in the directory it ran in. `--resume` takes another
  conversation, and `--fork` a copy. See [resuming a conversation](docs/guide/resuming.md).
- **After a reboot**, `cld restore` brings back the sessions that ran, detached, and continues a
  turn the reboot cut off. On Linux, `cld setup restore` has your systemd run it. See
  [after a reboot](docs/guide/after-a-reboot.md).
- **claude's options.** The words after `--` go to claude, as in
  `cld join -- --model opus "review the diff"`. `cld join -w` has claude work in a git worktree
  named after the session. See [claude's options and worktrees](docs/guide/claude-options.md).
- **claude's settings.** `cld setup config project` shares through git what claude may do without
  asking, and adds the IDE's MCP server. `cld setup config user` sets up your own settings for
  every project. See [claude's settings](docs/guide/project-settings.md).
- **Shell completion.** `cld setup completion SHELL` has bash, zsh or fish complete the commands,
  their options and the sessions' names. See [shell completion](docs/guide/shell-completion.md).
- **Remote Control** is claude's own setting, as without cld. While connected, the session's
  transcript is stored on Anthropic's servers, as
  [Claude Code's docs](https://code.claude.com/docs/en/remote-control) say. See
  [Remote Control and agent view](docs/guide/sessions.md#remote-control-and-agent-view).

When cld refuses a name or fails, see [troubleshooting](docs/guide/troubleshooting.md).

## cld and Claude Code's background sessions

Claude Code keeps conversations running without a terminal too: `claude --bg`, `/bg` or `←` hands
one to a supervisor, and [agent view](https://code.claude.com/docs/en/agent-view) lists them. You
choose between the two as you start claude: cld turns agent view off in its sessions, and a claude
started without cld keeps it. cld keeps claude as you run it in a terminal, under a name that TAB
completes, until you end it or leave it idle. The guide compares the two
[point by point](docs/guide/background-sessions.md).

Claude Code's own `claude --worktree --tmux` answers another need: a new session in a new git
worktree, in tmux as your `~/.tmux.conf` sets it up. Its
[CLI reference](https://code.claude.com/docs/en/cli-reference) documents no way back to such a
session, as of 2.1.284. `cld join -w` puts claude's own worktree in a session of cld's.

## Development

```sh
make check                           # vet and test natively, on the baseline terminal
make check TERMINALS=tmux,jediterm   # with JetBrains' JediTerm too
make lint                            # every gate of CI's lint job
make docker-check                    # as CI runs it, on tmux built from source
```

The tests check one [terminal contract](docs/design/testing.md) against an xterm-compatible
terminal and JediTerm; iTerm2 is not automated yet. [docs/design.md](docs/design.md) records the
tmux and claude behaviour cld relies on, the decisions taken and how the tests work.
[CLAUDE.md](CLAUDE.md) has the commands and what they need, the constraints and the conventions.

## License

[MIT](LICENSE)
