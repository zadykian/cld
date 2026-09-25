# cld

[![ci](https://github.com/zadykian/cld/actions/workflows/ci.yml/badge.svg)](https://github.com/zadykian/cld/actions/workflows/ci.yml)

Run [Claude Code](https://code.claude.com) in named sessions, each on a private tmux server of its
own: detach, close the terminal, and reattach later - from the same terminal or another one -
without losing the conversation.

```
cld new             # create the session "cld-main" in the current directory
cld new -n review   # create the session "cld-review"
cld new -n fix -w   # create "cld-fix", with claude in the git worktree "fix"
cld join -n review  # attach to it again, from this terminal or another one
cld list            # list the sessions cld started
cld kill -n review  # end the session and its claude
```

## Install

```sh
os=$(uname -s | tr '[:upper:]' '[:lower:]') arch=$(uname -m); case $arch in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
mkdir -p ~/.local/bin && curl -fsSL "https://github.com/zadykian/cld/releases/latest/download/cld-$os-$arch" -o ~/.local/bin/cld && chmod +x ~/.local/bin/cld
```

`~/.local/bin` has to be on your `PATH`. Each release publishes cld for Linux and macOS, on amd64
(x86_64) and arm64, and `cld.sha256`, which lists their checksums. From a clone, `make install`
builds cld and installs it there (`make install PREFIX=/usr/local` for another prefix); it needs
Go 1.26 or newer.

Requirements: tmux 3.7 or newer, and Claude Code 2.1.222 or newer as `claude` on the `PATH`; git
for `cld new --worktree`.

Most distributions ship an older tmux - Debian 13 has 3.5a, Ubuntu 26.04 3.6a - which cld
refuses, naming the version it found. [Homebrew](https://formulae.brew.sh/formula/tmux) has tmux
3.7 on macOS and Linux, as do Debian testing and unstable; or build a
[tmux release](https://github.com/tmux/tmux/releases) from source. Or stay on cld 0.3.0, the
last release that runs on tmux 3.3 to 3.6:

```sh
curl -fsSL https://github.com/zadykian/cld/releases/download/v0.3.0/cld -o ~/.local/bin/cld && chmod +x ~/.local/bin/cld
```

After upgrading tmux, end the sessions started before (`cld list`, then `cld kill -n NAME`): cld
checks the version of the `tmux` on the `PATH`, and each `cld new` starts a server of its own with
it, but a session's server keeps running the tmux that started it until the session ends.

Before it starts claude, `cld new` runs `claude --version` in the directory claude will start in
and refuses an older claude, naming the version it found; a newer one always passes. It then
starts that same `claude`, by its path: cld skips relative `PATH` entries (`.`, or an empty one).
Update claude the way you installed it: `claude update` for the native installer, or through
Homebrew, npm or your system's package manager.

cld 0.3.0 and earlier were a bash script, downloaded from `releases/latest/download/cld`. Later
releases publish a binary per platform instead, so that download fails once one of them is the
latest; an installed script keeps working until you update it with the lines above or
`make install`.

## Usage

Session `NAME` is the tmux session `cld-NAME` on a tmux server of its own, also named `cld-NAME`
(`tmux -L cld-NAME`), running `claude --name cld-NAME`. A name consists of up to 64 ASCII letters,
digits, `_` and `-`; without `-n` it is `main`. Under a long `TMUX_TMPDIR` fewer fit: the path of
the server's socket, `$TMUX_TMPDIR/tmux-UID/cld-NAME` with its symlinks resolved (on macOS `/tmp`
is `/private/tmp`), has to stay within 103 bytes on macOS and 107 on Linux, and tmux otherwise
fails with `File name too long`.

Each session starts with [Remote Control](https://code.claude.com/docs/en/remote-control) on, so
you can also continue it from claude.ai or the Claude app: `cld new` passes claude
`--settings '{"remoteControlAtStartup":true}'`, which overrides the "Enable Remote Control for all
sessions" setting in `/config`. claude still keeps it off where your organisation's policy does,
or where the project's `.claude/settings.json` or `.claude/settings.local.json` sets
`remoteControlAtStartup` to `false`.

| Command | Action |
|---|---|
| `cld new [-n NAME] [-w]` | create the session in the current directory and attach to it; fails if it exists. With `-w` (`--worktree`), claude works in the git worktree `NAME` (see below) |
| `cld join [-n NAME]` | attach to the session; fails if it does not exist |
| `cld kill [-n NAME]` | end the session and its tmux server; claude exits as when its terminal closes, and what claude started through tmux ends too |
| `cld list` | list the sessions cld started: name, whether a terminal is attached (or claude exited), and the directory claude is in |
| `cld help [COMMAND]` | show the help of cld, or of one command: its options and their defaults. `cld -h` and `cld COMMAND -h` (or `--help`) do the same |
| `cld version` | show the version |

| Keys | Action |
|---|---|
| `C-q d` | detach; claude keeps running |
| `C-q C-q` | send `C-q` to claude |

If claude exits with an error - it could not start, say - its session stays open with claude's
message on screen and a line on how to end it: `C-q d` detaches, `cld kill -n NAME` ends the
session. `cld join -n NAME` shows the line again, also when claude exited with no terminal
attached. `cld list` shows such a session as `exited`. Leaving claude the usual ways (`/exit`,
`Ctrl+C` twice, `Ctrl+D` twice) closes the session.

Joining from a second terminal detaches the first one; claude keeps running in its directory,
wherever you join from. A killed session's conversation stays in Claude Code's history, named
`cld-NAME` in the `claude --resume` picker.

Each session has its tmux server to itself; `tmux -L cld-NAME ls` lists what runs on session
`NAME`'s. Whatever claude runs - its Bash tool, a hook - reaches that server with a plain `tmux`,
and a session made that way has another name there: `cld` sees only `cld-NAME`, and `cld kill`
ends the others with the server. Each claude also gets the environment of the shell that ran
`cld new` - `CLAUDE_CONFIG_DIR`, a virtualenv, `AWS_PROFILE` and the like. If claude exits while
sessions it started through tmux keep its server running, `cld new`, `cld join` and `cld kill`
refuse the name and point at the server; end it with `tmux -L cld-NAME kill-server`. Where tmux's
socket directory ignores case, as it does on macOS's default file system, names that differ only
in case share one socket: while session `a` or its server runs, `cld new -n A`, `cld join -n A`
and `cld kill -n A` refuse the name and say that it clashes with `a`.

Before 0.2.0, `cld [NAME]` attached to the session, creating it if needed; it now fails and names
the two commands.

cld 0.3.0 and earlier ran every session on one tmux server, `tmux -L cld`, where later versions do
not look: end those sessions before upgrading, or afterwards find them with `tmux -L cld ls` and
end them with `tmux -L cld kill-session -t =cld-NAME`, or all of them with
`tmux -L cld kill-server`. Under a locale such as `en_US.UTF-8`, cld 0.3.0 also took names with
non-ASCII letters or digits, such as `café`, which later versions refuse.

### Worktrees

`cld new -n NAME -w` passes `--worktree NAME` to claude, which
[creates the worktree](https://code.claude.com/docs/en/worktrees) `.claude/worktrees/NAME` of the
repository on the branch `worktree-NAME`, or reopens it if it exists, and works there; `cld list`
shows its directory. As with `claude --worktree`:

- gitignored files listed in `.worktreeinclude` are copied into it;
- when claude exits it asks whether to keep the worktree.

A new worktree branches from your current `HEAD`, not from the remote's default branch as
`claude --worktree` does by default: `cld` also puts `"worktree":{"baseRef":"head"}` in the
settings it passes claude, which overrides the `worktree.baseRef` setting.

`cld kill` leaves the worktree where it is, and `cld new -n NAME -w` reopens it.

claude makes a worktree only in a directory whose workspace trust you have accepted: run `claude`
(or `cld new`) there once first; otherwise claude says so and exits, and the session stays open
with the message (see Usage). `cld` itself checks that the current directory is in a git
repository.

### Why a private tmux server per session

`cld` runs tmux with `-L cld-NAME -f /dev/null`: a server for each session, shared with no other
session and no other tmux you use. tmux starts a pane with the environment of the shell that
started its server, apart from `PATH` and a few variables such as `SSH_AUTH_SOCK`; on a server of
its own, each claude gets the environment of the shell that ran `cld new`. What claude runs
through tmux stays on its session's server. cld's options never touch any other tmux you use, and
your `~/.tmux.conf` never touches claude:

- `extended-keys on` lets modified keys such as Shift+Enter reach claude when it asks for them;
- the `extkeys` terminal feature for `xterm*` terminals, as
  [Claude Code's docs](https://code.claude.com/docs/en/terminal-config#configure-tmux) recommend:
  tmux asks the terminal for modified keys only when it knows the terminal supports them, and it
  does not recognise every terminal that does;
- `mouse on` and `focus-events on`: claude hints when either is off. With the mouse on, the wheel
  over a program that draws in the main screen without the mouse - claude outside fullscreen, a
  shell - scrolls the pane's history; claude's fullscreen transcript gets the wheel either way;
- `allow-passthrough on`: claude wraps its notifications and clipboard copies (OSC 52) in tmux
  passthrough;
- `status off`: claude keeps the whole tab;
- prefix `C-q`: claude binds `C-b` and nearly every other Ctrl key, but not `C-q`;
- `remain-on-exit failed` on claude's window: a claude that exits with an error keeps its pane,
  so its message stays readable;
- the tab title is set to `✳ cld-NAME`, and tmux keeps claude's own title changes to its pane;
- `TERMINAL_EMULATOR` is removed from the server's environment: claude trusts it over
  `TERM_PROGRAM=tmux`, and a session created in a JetBrains terminal would otherwise behave as if
  it ran in JediTerm, even when joined from iTerm2.

## cld and `claude --tmux`

Claude Code has its own tmux option, `claude --worktree [name] --tmux`. It solves a different
problem:

| | `cld` | `claude --worktree --tmux` |
|---|---|---|
| Purpose | a named conversation you detach from and come back to | a new session working in an isolated git worktree |
| Working copy | the directory you run it in, or with `-w` a git worktree claude creates | a new git worktree per session (`--tmux` requires `--worktree`) |
| Needs | tmux 3.7 or newer; a git repository for `-w` | a git repository |
| Coming back | `cld join -n NAME` attaches to the session | not documented |
| tmux server and options | a private server per session that ignores `~/.tmux.conf` and sets what claude needs (see above) | not documented; for claude inside tmux, [the docs](https://code.claude.com/docs/en/terminal-config#configure-tmux) advise adding passthrough and extended-keys settings to `~/.tmux.conf` |
| iTerm2 | a regular tmux client | iTerm2 native panes when available; `--tmux=classic` for regular tmux |

The `claude --tmux` column is based on `claude --help` in Claude Code 2.1.281: the Claude Code docs
do not describe the option yet.

`cld new -n NAME -w` combines them: claude's own worktree, in a session of cld's.

## Terminals

The tests check one contract - title, Shift+Enter, Ctrl keys, detach, mouse wheel, focus,
clipboard, paste, claude exiting - against each terminal:

| Terminal | How it is tested | Differences |
|---|---|---|
| xterm-compatible (baseline) | a pane of an outer tmux server | none |
| JetBrains IDEs (JediTerm) | JediTerm's emulator, headless, typing through its own key handling | Shift+Enter arrives as ESC CR (the IDE's newline setting); no focus reports; no OSC 52 |
| iTerm2 | not automated yet | |

## Development

The checks run in Docker, the same way as in CI:

```sh
make docker-check    # tmux 3.7c, built from source on debian:trixie: baseline and JediTerm
```

They run on one tmux, pinned and bumped by hand together with the minimum cld requires;
`make docker-image TMUX_VERSION=X` builds an image with another release, to try it.

Natively, `make check` needs Go, tmux 3.7 or newer, ShellCheck and shfmt, and runs the baseline
terminal only.
For JediTerm add a JDK, fetch its jars once with `tests/jediterm/fetch-deps tests/jediterm/lib`
and run `make check TERMINALS=tmux,jediterm`.

cld is a Go program on [cobra](https://github.com/spf13/cobra): the command line is in `cmd/cld`,
how it uses tmux - and why - in `internal/session`.

How the tests work - the probe that stands in for claude, the sandboxes, the terminal drivers - is
described in [docs/design.md](docs/design.md) and at the top of each package under `tests/`.

## Releasing

Pushing a tag `vX.Y.Z` runs the checks and publishes a GitHub release with cld built for each
platform - `cld-linux-amd64`, `cld-linux-arm64`, `cld-darwin-amd64` and `cld-darwin-arm64`, their
version set to `X.Y.Z` - and `cld.sha256`, which lists them. `make dist VERSION=X.Y.Z` builds the
same into `dist/`.

## License

[MIT](LICENSE)
