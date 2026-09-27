# cld: user guide

The [README](../README.md) gives the overview and `cld help COMMAND` each command's options; this
guide has the details beyond them. How cld works, and why, is in [design.md](design.md).

## Installing

The command in the [README](../README.md#install) runs `install.sh`, which each release publishes
beside its binaries: the latest release's. It runs in any POSIX shell, `sh` or `bash`, and needs
`curl`, and `sha256sum` or `shasum`. It:

- picks the binary from `uname`: `cld-linux-amd64`, `cld-linux-arm64`, `cld-darwin-amd64` or
  `cld-darwin-arm64`, the last also in a shell that runs under Rosetta 2 on Apple silicon;
- takes it from the latest release, or from the one `CLD_VERSION` names, as `0.4.0` or `v0.4.0`;
  the releases before 0.4.0 published a script, which [Upgrading](#upgrading) installs;
- downloads it into `CLD_INSTALL_DIR`, `~/.local/bin` by default, made where missing, checks it
  against the release's `cld.sha256`, runs it for its version, and only then replaces the `cld`
  there. When anything fails it says so and exits with status 1, leaving the directory as it was;
- says when the directory is not on your `PATH`, or another `cld` comes first there. It edits no
  shell profile: add the directory to `PATH` yourself, in `~/.profile`, `~/.zshrc` or the like.

To install cld by hand, download the binary for your system and `cld.sha256` from a
[release](https://github.com/zadykian/cld/releases), check the binary -
`grep ' cld-linux-amd64$' cld.sha256 | sha256sum -c`, with `shasum -a 256 -c` on macOS - and
install it as `cld`, executable, in a directory on your `PATH`.

## Sessions

- A name consists of up to 64 ASCII letters, digits, `_` and `-`, starting with a letter or digit.
- Each claude gets the environment of the shell that ran `cld new` or `cld resume` -
  `CLAUDE_CONFIG_DIR`, a virtualenv, `AWS_PROFILE` and the like.
- Whatever claude runs - its Bash tool, a hook - reaches the session's server with a plain `tmux`,
  and `tmux -L cld-NAME ls` lists what runs there. cld sees only `cld-NAME`; `cld kill` ends the
  rest with the server.
- Joining from a second terminal detaches the first; claude keeps running in its directory.
- Remote Control is turned on over the "Enable Remote Control for all sessions" setting in
  `/config`. It stays off where your organisation's policy does, or where the project's
  `.claude/settings.json` or `.claude/settings.local.json` sets `remoteControlAtStartup` to
  `false`.

## The session list

On a terminal, `cld list` shows the sessions full screen, the first one selected. `Enter` joins
the selected one as `cld join` does, detaching another terminal from it; `Ctrl+X` twice kills it as
`cld kill` does. After the first `Ctrl+X`, `Esc` keeps the session and the list open; so do two
seconds without the second `Ctrl+X`, and any other key, which then does what it does. A `Ctrl+X`
held down does not go on to kill the next session.

- The list reads the sessions when it opens and after a kill: a session made or ended elsewhere
  shows when you run `cld list` again. If the selected session has ended when you press `Enter` or
  the second `Ctrl+X`, the list says so and reads them again.
- The state `attached` counts terminals only: someone on the session through Remote Control does
  not show, and a kill ends the session for them too.
- A kill leaves a `cld new -w` worktree where it is, and the conversation stays: after a kill by
  mistake, `cld resume -n NAME` brings it back.
- `cld list` prints the table and exits where its input or output is not a terminal
  (`cld list | cat`), `TERM` is unset or `dumb`, it runs in the background, or it runs in a pane of
  one of cld's servers; with no sessions it prints nothing. A script that leaves it the terminal
  gets the list and waits for a key: pipe it for the table.
- Whether a JetBrains IDE passes `Esc` and `Ctrl+X` on to its terminal depends on its keymap;
  `Ctrl+C` also leaves the list.

## Resuming a conversation

A conversation stays in Claude Code's history until Claude Code removes it, after 30 days by
default. `cld resume -n NAME` runs `claude --resume cld-NAME` in a session made as `cld new` makes
it:

- claude looks the name up in the current directory or, in a git repository, in any checkout of
  it; a session ID it finds from any directory. cld keeps no record of where a session ran, and a
  killed one no longer shows in `cld list`. claude looks in the history of the shell's
  `CLAUDE_CONFIG_DIR`, if you set one.
- When exactly one conversation has the name, claude resumes it. Several can have it: `/clear`
  keeps the name for the conversation it starts, and a later `cld new -n NAME` presumably gives it
  to a new one as well (not checked yet). claude then opens its picker with the name as the search
  term, which may not be an exact filter: for `cld-rev` it may list `cld-review` too (not checked
  yet).
- With `SESSION`, claude still gets `--name cld-NAME`, meant to give the conversation the session's
  name for the next `cld resume -n NAME`; how claude applies it to a resumed conversation has not
  been checked yet.
- For a session ID that matches no conversation, claude prints
  `No conversation found with session ID: ...` and exits with an error; the session stays with the
  message.
- `cld resume` refuses a name that a session holds, one whose claude exited included: end it with
  `cld kill -n NAME` first. It cannot tell whether the conversation is open elsewhere. When another
  claude has the conversation's Remote Control session, the resumed one leaves Remote Control off
  (`Remote Control not started here`) until `/remote-control` moves it over; whether a cld
  session, which turns Remote Control on as it starts, records that session in its conversation has
  not been checked yet.

## Worktrees

As with `claude --worktree`, gitignored files listed in `.worktreeinclude` are copied into a new
worktree, and when claude exits it asks whether to keep the worktree. Unlike it, a new worktree
branches from your current `HEAD`, not from the remote's default branch, whatever your
`worktree.baseRef` setting says.

- `cld new -n NAME -w` again reopens the worktree with a new conversation; `cld resume -n NAME`,
  run in the repository, resumes the conversation, and claude takes it back to its worktree - or,
  if the worktree is gone, resumes where `cld resume` runs and says so. `cld resume` has no `-w`,
  and a worktree claude makes during a resumed session - for a subagent, say - branches as your
  settings say.
- claude makes a worktree only in a directory whose workspace trust you have accepted: run `claude`
  (or `cld new`) there once first; otherwise claude says so and exits, and the session stays with
  the message.

## Project settings

- `.claude/settings.json` gets `$schema`; in `permissions.allow`, reading, editing and writing
  files, web search and fetch, and shell commands such as `ls`, `grep`, `git`, `go`, `dotnet`,
  `make`, `docker build` and `gh pr view`; and `autoUpdatesChannel`, `plansDirectory`,
  `autoMemoryEnabled`, `theme` and `autoCompactEnabled`, with the values of cld's own.
- `.claude/settings.local.json` is only created, holding its `$schema`.
- `.gitignore` gets `/.claude/*` and `!/.claude/settings.json`; the lines without the leading slash
  count as there.
- With `--mcp`, each server gets its entry in `.mcp.json`, its name in `enabledMcpjsonServers` and
  `mcp__NAME` in `permissions.allow`; `jbcontext` also `Bash(jbcontext:*)`, for `jbcontext search`.
  claude asks nothing about them once you have accepted the folder's workspace trust.

The IDEs' servers are `http://127.0.0.1:${GOLAND_MCP_PORT:-64422}/stream` and
`http://127.0.0.1:${RIDER_MCP_PORT:-64482}/stream`: `.mcp.json` is shared, and each developer's IDE
listens on a port of its own. Where yours is not the default (the IDE's "Copy HTTP Stream Config"
shows its URL), set the variable in your shell's profile, or in the `env` of your own
`~/.claude/settings.json`, which serves every project. Not in the project's
`.claude/settings.local.json`: claude 2.1.283 does not expand `.mcp.json` from it.

Where the files exist, cld sets the keys whose values differ, adds the entries and `.gitignore`
lines that are missing, and replaces a server's entry that differs from its own, whole. Keys,
entries and servers of your own stay, in their order and indentation. A file it cannot edit - not
valid JSON, say - stops it with nothing changed. In a git work tree cld then checks that git does
not ignore `.claude/settings.json` all the same - through `.claude/`, `*.json` or git's own
excludes - and otherwise names the pattern and exits with status 1.

## Telemetry

- `--local` gets spans for model requests, tool calls, MCP calls and hooks, per agent, which show
  where a long run spends its time; they name the Bash commands and MCP tools claude runs, and only
  the local endpoint gets them. For the JetBrains OpenTelemetry plugin, fix its port in Settings ›
  OpenTelemetry › Common, "Use fixed OTLP server port".
- `--remote` keeps getting metrics while the IDE is closed; the local endpoint's data is dropped
  after 30 s.
- A URL is `http://HOST:PORT` (gRPC without TLS) or `https://HOST:PORT` (TLS).
- The collector listens on `127.0.0.1`, on a port the kernel picks the first time and later runs
  keep, so that running sessions keep reaching it; `--port PORT` picks one yourself.
- In the `env` of `settings.json`, cld sets `CLAUDE_CODE_ENABLE_TELEMETRY`,
  `OTEL_METRICS_EXPORTER`, `OTEL_EXPORTER_OTLP_PROTOCOL` and `OTEL_EXPORTER_OTLP_ENDPOINT`, with
  `--local` also `OTEL_TRACES_EXPORTER`, `OTEL_LOGS_EXPORTER`,
  `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA` and `OTEL_LOG_TOOL_DETAILS`, and removes the per-signal
  `OTEL_EXPORTER_OTLP_*_ENDPOINT` and `_PROTOCOL` keys; every other key stays. claude reads its
  settings as a session starts: sessions running then keep theirs.
- Run `cld setup telemetry` again to change anything. cld writes the settings only once the new
  collector takes connections; if it stops first, cld shows the end of its log, and the stopped
  container stays, for `docker logs cld-telemetry`.
- Docker starts the collector again after a reboot. To turn telemetry off, remove the container
  (`docker rm -f cld-telemetry`) and the keys above from `settings.json`.

`--collector-config FILE` is merged over cld's config: maps merge, lists are replaced, so to add an
exporter to a pipeline, repeat the pipeline's whole `exporters` list. cld's config names the
receiver `otlp`, the exporters `otlp_grpc/local` and `otlp_grpc/remote`, and the pipelines
`traces`, `metrics` and `logs`. An auth header for the remote endpoint, say:

```yaml
exporters:
  otlp_grpc/remote:
    headers:
      authorization: Bearer <token>
    compression: gzip
```

cld reads the file when it runs: edits apply when you run it again. The container gets a copy that
`docker inspect cld-telemetry` shows, secrets included.

## Shell completion

`cld join -n <TAB>` offers each session with its state, and `--mcp` the next server after a comma.
No file names are offered, and `cld new -n`, `cld resume` and the values of `cld setup telemetry`
offer nothing. The script runs `cld` on every TAB, so the names are always current.
`cld completion SHELL --no-descriptions`, or `CLD_COMPLETION_DESCRIPTIONS=0` in the environment,
leaves out the states and the other descriptions. Start a new shell for the script to take effect.

- **bash** needs the bash-completion package. With bash-completion 2 (Linux, or Homebrew's
  `bash-completion@2` for Homebrew's bash):

  ```sh
  mkdir -p ~/.local/share/bash-completion/completions
  cld completion bash > ~/.local/share/bash-completion/completions/cld
  ```

  For macOS's own `/bin/bash`, 3.2, install Homebrew's `bash-completion` (1.3), add the line its
  caveats show to `~/.bash_profile`, and run
  `cld completion bash > "$(brew --prefix)/etc/bash_completion.d/cld"`; bash 3.2 cannot load it
  with `source <(cld completion bash)`. It puts no space after a completed name, and offers file
  names where cld offers nothing. Without bash-completion, every TAB prints
  `_get_comp_words_by_ref: command not found`.
- **zsh** needs `compinit`, and the script as `_cld` in a directory on `$fpath`:

  ```sh
  mkdir -p ~/.zfunc
  cld completion zsh > ~/.zfunc/_cld
  ```

  with, in `~/.zshrc`:

  ```sh
  fpath=(~/.zfunc $fpath)
  autoload -U compinit; compinit
  ```

  On macOS it can go in Homebrew's directory instead:
  `cld completion zsh > "$(brew --prefix)/share/zsh/site-functions/_cld"`.
- **fish**: `cld completion fish > ~/.config/fish/completions/cld.fish`.

## Troubleshooting

- **claude too old.** `cld new` and `cld resume` name the version they found. Update claude the way
  you installed it: `claude update` for the native installer, or through Homebrew, npm or your
  system's package manager.
- **A server without its session.** If claude exits while what it started through tmux keeps its
  server running, `cld new`, `cld resume`, `cld join` and `cld kill` refuse the name: end the
  server with `tmux -L cld-NAME kill-server`.
- **Names that differ only in case.** Where tmux's socket directory ignores case, as on macOS's
  default file system, `A` and `a` share one socket: while one of them runs, cld refuses the other.
- **`File name too long`.** The server's socket, `$TMUX_TMPDIR/tmux-UID/cld-NAME` with its symlinks
  resolved (on macOS `/tmp` is `/private/tmp`), must stay within 103 bytes on macOS and 107 on
  Linux: under a long `TMUX_TMPDIR`, use a shorter name.

## Upgrading

- **cld.** Run the [install command](../README.md#install) again, with the same `CLD_INSTALL_DIR`
  if you gave one: it replaces cld with the latest release, or with the one `CLD_VERSION` names.
- **tmux.** End the sessions started before the upgrade (`cld list`, then `cld kill -n NAME`): each
  session's server keeps running the tmux that started it until the session ends.
- **To cld 0.4.0 or later.** cld 0.3.0 and earlier were a bash script, downloaded from
  `releases/latest/download/cld`, which now fails; an installed script keeps working until you
  install cld [as the README says](../README.md#install). Those versions ran every session on one
  server, `tmux -L cld`, where later ones do not look: end those sessions before upgrading, or
  afterwards find them with `tmux -L cld ls` and end them with
  `tmux -L cld kill-session -t =cld-NAME`, or all of them with `tmux -L cld kill-server`. Names
  with non-ASCII letters or digits, such as `café`, which 0.3.0 took under a UTF-8 locale, are now
  refused.
- **Staying on tmux 3.3 to 3.6.** cld 0.3.0 is the last release that runs on them:

  ```sh
  curl -fsSL https://github.com/zadykian/cld/releases/download/v0.3.0/cld -o ~/.local/bin/cld && chmod +x ~/.local/bin/cld
  ```

- **From before 0.2.0.** `cld [NAME]` attached to the session, creating it if needed; it now fails
  and names the two commands.
