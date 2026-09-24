# cld

[![ci](https://github.com/zadykian/cld/actions/workflows/ci.yml/badge.svg)](https://github.com/zadykian/cld/actions/workflows/ci.yml)

Run [Claude Code](https://code.claude.com) in named sessions on a private tmux server: detach,
close the terminal, and reattach later - from the same terminal or another one - without losing
the conversation.

```
cld          # attach to (or create) the session "cld-main"
cld review   # attach to (or create) the session "cld-review"
```

## Install

```sh
mkdir -p ~/.local/bin && curl -fsSL https://github.com/zadykian/cld/releases/latest/download/cld -o ~/.local/bin/cld && chmod +x ~/.local/bin/cld
```

`~/.local/bin` has to be on your `PATH`. Each release also publishes `cld.sha256`. From a clone,
`make install` does the same (`make install PREFIX=/usr/local` for another prefix).

Requirements: bash (3.2 or newer), tmux 3.3 or newer, and `claude` on the `PATH`.

## Usage

`cld [NAME]` attaches to the tmux session `cld-NAME`, or creates it running `claude --name cld-NAME`
in the current directory. A name consists of letters, digits, `_` and `-`.

| Keys | Action |
|---|---|
| `C-q d` | detach; claude keeps running |
| `C-q C-q` | send `C-q` to claude |

Attaching from a second terminal detaches the first one; claude keeps running in the directory the
session was created in.

### Why a private tmux server

`cld` runs tmux with `-L cld -f /dev/null`, so its options never touch any other tmux you use, and
your `~/.tmux.conf` never touches claude:

- `extended-keys on` lets modified keys such as Shift+Enter reach claude when it asks for them;
- the `extkeys` terminal feature for `xterm*` terminals, as
  [Claude Code's docs](https://code.claude.com/docs/en/terminal-config#configure-tmux) recommend:
  tmux asks the terminal for modified keys only when it knows the terminal supports them, and it
  does not recognise every terminal that does;
- `mouse on` and `focus-events on`: claude scrolls its fullscreen transcript with the wheel and
  hints when either is off;
- `allow-passthrough on`: claude wraps its notifications and clipboard copies (OSC 52) in tmux
  passthrough;
- `status off`: claude keeps the whole tab;
- prefix `C-q`: claude binds `C-b` and nearly every other Ctrl key, but not `C-q`;
- the tab title is set to `✳ cld-NAME`, and tmux keeps claude's own title changes to its pane;
- `TERMINAL_EMULATOR` is removed from the server's environment: claude trusts it over
  `TERM_PROGRAM=tmux`, and a server started from a JetBrains terminal would otherwise make every
  session on it behave as if it ran in JediTerm, even when attached from iTerm2.

## cld and `claude --tmux`

Claude Code has its own tmux option, `claude --worktree [name] --tmux`. It solves a different
problem:

| | `cld` | `claude --worktree --tmux` |
|---|---|---|
| Purpose | a named conversation you detach from and come back to | a new session working in an isolated git worktree |
| Working copy | the directory you run it in | a new git worktree per session (`--tmux` requires `--worktree`) |
| Needs | bash and tmux 3.3 or newer | a git repository |
| Coming back | `cld NAME` attaches to the session if it exists | not documented |
| tmux server and options | a private server that ignores `~/.tmux.conf` and sets what claude needs (see above) | not documented; for claude inside tmux, [the docs](https://code.claude.com/docs/en/terminal-config#configure-tmux) advise adding passthrough and extended-keys settings to `~/.tmux.conf` |
| iTerm2 | a regular tmux client | iTerm2 native panes when available; `--tmux=classic` for regular tmux |

The `claude --tmux` column is based on `claude --help` in Claude Code 2.1.281: the Claude Code docs
do not describe the option yet.

To get both, create the worktree yourself (`git worktree add`), then run `cld NAME` inside it.

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
make docker-check                      # tmux 3.3a (debian:bookworm): baseline and JediTerm
make docker-check BASE=ubuntu:24.04    # tmux 3.4; debian:trixie has 3.5a
make docker-check BASE=debian:trixie TMUX_VERSION=3.7c   # a tmux release built from source
```

Natively, `make check` needs Go, tmux, ShellCheck and shfmt, and runs the baseline terminal only.
For JediTerm add a JDK, fetch its jars once with `tests/jediterm/fetch-deps tests/jediterm/lib`
and run `make check TERMINALS=tmux,jediterm`. `CLD_BASH=/bin/bash make test` runs cld under a
specific bash.

How the tests work - the probe that stands in for claude, the sandboxes, the terminal drivers - is
described in [docs/design.md](docs/design.md) and at the top of each package under `tests/`.

## Releasing

Pushing a tag `vX.Y.Z` runs the checks and publishes a GitHub release with `cld`, its version set
to `X.Y.Z`, and `cld.sha256`.

## License

[MIT](LICENSE)
