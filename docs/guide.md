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

- A session's name is `NAME-SUFFIX`; below, `S` stands for it whole, and tmux knows the session
  as `cld-S`. `NAME` and `SUFFIX` each consist of ASCII letters, digits, `_` and `-`, starting
  with a letter or digit, and `S` has 64 characters at most: a longer one is refused, pointing at
  `-n` and `-s`.
- Without `-n`, `NAME` is the name of the git repository you are in: that of the directory that
  holds its `.git`, so that its worktrees - claude's under `.claude/worktrees` among them - and
  its subdirectories share it; for a submodule, or a worktree of a bare repository, the name of
  the git directory, without `.git`. Outside a repository, and where git is missing, it is the
  name of the current directory as `pwd` shows it - that of a symbolic link, not of where it
  leads: in `/root`, `cld new` makes `cld-root-0`. Each run of the characters a name cannot have
  becomes `-`, and `-` and `_` go from either end: `my.site` gives `my-site-0`, `.dotfiles`
  `dotfiles-0`. Where nothing is left - in the root directory, or for a name in another script -
  `S` is `SUFFIX` alone.
- Without `-s`, `cld new` gives the index above the highest of the sessions `NAME-INDEX` that run,
  those `cld list` shows, and of servers that outlive their session (see
  [Troubleshooting](#troubleshooting)); a gap stays a gap. A session that has ended counts no
  more, so the next `cld new` can give its name again, and claude's history then holds two
  conversations of that name (see [Resuming a conversation](#resuming-a-conversation)). Two
  `cld new` started at the same moment can pick the same name: the second ends with tmux's
  `duplicate session: cld-S`, and run again it takes the next.
- `cld join` and `cld kill` need `-s`: in the session's repository or directory `-s SUFFIX` alone,
  elsewhere `-n NAME -s SUFFIX` too. cld's own messages name a session that way, splitting `S` at
  its last `-`. A name without one, made where `NAME` leaves nothing, takes `-s S` in such a
  directory - `cd / && cld kill -s S` - or `cld list`, whose `Enter` and `Ctrl+X` take any
  session.
- Each claude gets the environment of the shell that ran `cld new` or `cld resume` -
  `CLAUDE_CONFIG_DIR`, a virtualenv, `AWS_PROFILE` and the like.
- Whatever claude runs - its Bash tool, a hook - reaches the session's server with a plain `tmux`,
  and `tmux -L cld-S ls` lists what runs there. cld sees only `cld-S`; `cld kill` ends the
  rest with the server.
- Joining from a second terminal leaves the first attached: both show the same claude, and keys
  from either reach it. The window takes the size of the terminal you used last, and a larger one
  shows the rest of its screen dotted. `cld join --detach-others` detaches the other terminals
  instead; claude keeps running in its directory either way.
- Remote Control is turned on over the "Enable Remote Control for all sessions" setting in
  `/config`. It stays off where your organisation's policy does, or where the project's
  `.claude/settings.json` or `.claude/settings.local.json` sets `remoteControlAtStartup` to
  `false`.
- The tab's title is `✳ cld-NAME`, with `◐` and `◑` in turn in place of the `✳` while claude
  works. Under tmux claude keeps its own marker at `✳`, so cld gives claude hooks with
  `--settings` that tell tmux when a turn starts, when claude asks for a permission and when the
  turn ends. They miss:
  - an interrupt (`Esc`) as claude writes: the title stays busy until the next prompt, or until
    claude, idle for a minute, notifies it; an interrupt in a tool comes through;
  - a prompt that a `UserPromptSubmit` hook of your own blocks: busy until the next one;
  - everything under `disableAllHooks`, or a policy that allows only managed hooks: the title
    stays `✳ cld-NAME`.

  A terminal that detaches keeps the title it had, a busy one too. A session that an older cld
  started keeps `✳ cld-NAME`.
- While claude works in a linked git worktree - one `cld new -w` has it make, one it enters with
  its `EnterWorktree` tool, one you run `cld new` in - the title ends in ` [w]`, and loses it
  when claude leaves. It follows claude's working directory, not where a shell command `cd`s
  to. Without git on your `PATH` when the session starts, or with hooks turned off, there is no
  `[w]`.

## The session list

On a terminal, `cld list` shows the sessions full screen, the first one selected. `Enter` joins
the selected one as `cld join` does, beside any other terminal on it; `Ctrl+X` twice kills it as
`cld kill` does. After the first `Ctrl+X`, `Esc` keeps the session and the list open; so do two
seconds without the second `Ctrl+X`, and any other key, which then does what it does. A `Ctrl+X`
held down does not go on to kill the next session.

- The list reads the sessions when it opens and after a kill: a session made or ended elsewhere
  shows when you run `cld list` again. If the selected session has ended when you press `Enter` or
  the second `Ctrl+X`, the list says so and reads them again.
- The state `attached` counts terminals only: someone on the session through Remote Control does
  not show, and a kill ends the session for them too.
- A kill leaves a `cld new -w` worktree where it is, and the conversation stays: after a kill by
  mistake, `cld resume -n NAME -s SUFFIX` brings it back.
- `cld list` prints the table and exits where its input or output is not a terminal
  (`cld list | cat`), `TERM` is unset or `dumb`, it runs in the background, or it runs in a pane of
  one of cld's servers; with no sessions it prints nothing. A script that leaves it the terminal
  gets the list and waits for a key: pipe it for the table.
- Whether a JetBrains IDE passes `Esc` and `Ctrl+X` on to its terminal depends on its keymap;
  `Ctrl+C` also leaves the list.

## Resuming a conversation

A conversation stays in Claude Code's history until Claude Code removes it, after 30 days by
default. `cld resume -n NAME -s SUFFIX` runs `claude --resume cld-S` in session `S`, made as
`cld new` makes it:

- claude looks the name up in the current directory or, in a git repository, in any checkout of
  it; a session ID it finds from any directory. cld keeps no record of where a session ran, and a
  killed one no longer shows in `cld list`. claude looks in the history of the shell's
  `CLAUDE_CONFIG_DIR`, if you set one.
- When exactly one conversation has the name, claude resumes it. Several can have it: `/clear`
  keeps the name for the conversation it starts, and a later `cld new` of the same `S` presumably
  gives it to a new one as well (not checked yet) - as does `cld new` without `-s`, which gives an
  index again once its session has ended. claude then opens its picker with the name as the search
  term, which may not be an exact filter: for `cld-rev` it may list `cld-review` too (not checked
  yet).
- With `SESSION`, claude still gets `--name cld-S`, meant to give the conversation the session's
  name for the next `cld resume -n NAME -s SUFFIX`; how claude applies it to a resumed
  conversation has not been checked yet. Without `-s`, the session gets the index `cld new` would
  give it.
- For a session ID that matches no conversation, claude prints
  `No conversation found with session ID: ...` and exits with an error; the session stays with the
  message.
- `cld resume` refuses a name that a session holds, one whose claude exited included: end it with
  `cld kill` first. It cannot tell whether the conversation is open elsewhere. When another
  claude has the conversation's Remote Control session, the resumed one leaves Remote Control off
  (`Remote Control not started here`) until `/remote-control` moves it over; whether a cld
  session, which turns Remote Control on as it starts, records that session in its conversation has
  not been checked yet.

## Worktrees

As with `claude --worktree`, gitignored files listed in `.worktreeinclude` are copied into a new
worktree, and when claude exits it asks whether to keep the worktree. Unlike it, a new worktree
branches from your current `HEAD`, not from the remote's default branch, whatever your
`worktree.baseRef` setting says.

- The worktree is named as the session is, `cld-S`, on the branch `worktree-cld-S`: in a
  repository `api`, `cld new -w` makes `.claude/worktrees/cld-api-0`.
- `cld new -s SUFFIX -w` again reopens the worktree with a new conversation; `cld resume -s SUFFIX`,
  run in the repository, resumes the conversation, and claude takes it back to its worktree - or,
  if the worktree is gone, resumes where `cld resume` runs and says so. `cld resume` has no `-w`,
  and a worktree claude makes during a resumed session - for a subagent, say - branches as your
  settings say.
- A worktree outlives its session, and `cld new -w` counts sessions, not worktrees: once session
  `api-0` has ended, the next `cld new -w` is `api-0` again, and reopens its worktree. Give `-s`
  for a new one.
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

`cld join -n <TAB>` offers the `NAME` of the sessions' names, with how many sessions have it;
`cld join -s <TAB>` offers the `SUFFIX` of each session whose `NAME` is `-n`'s, or else that of the
repository or directory you are in, with its state; and `--mcp` the next server after a comma. No
file names are offered, and `cld new`, `cld resume`, `cld kill` and the values of
`cld setup telemetry` offer nothing. The script runs `cld` on every TAB, so the names are always
current. `CLD_COMPLETION_DESCRIPTIONS=0` in the environment leaves out the states and the other
descriptions.

`cld setup completion SHELL` writes the script that `cld completion SHELL` prints where the shell
reads it, making its directories, and says what it wrote; start a new shell for it to take effect.
Run again, it changes nothing but a script that differs. Once `cld update` has installed a release,
it has that release print each script anew, and writes the ones that differ; a script without
descriptions, `cld completion SHELL --no-descriptions` written in its place, stays one. Where it
cannot write one, the update stands, and cld warns: run the `cld setup completion SHELL` the
warning names. The install command leaves the scripts as they are: run
`cld setup completion SHELL` again after it.
cld removes nothing: to undo it, delete the script, and for zsh the lines in `.zshrc`.

- **bash**: `~/.local/share/bash-completion/completions/cld` - with `$XDG_DATA_HOME` in place of
  `~/.local/share` where it is set, or in the first directory of `$BASH_COMPLETION_USER_DIR` where
  that is. bash-completion 2 loads it at the first TAB, where `~/.bashrc` loads bash-completion:
  Debian's and Ubuntu's do; with Homebrew's bash, install `bash-completion@2` and follow its
  caveats. Without bash-completion, every TAB prints `_get_comp_words_by_ref: command not found`.

  With [ble.sh](https://github.com/akinomyoga/ble.sh) 0.4, which edits bash's command line, the
  script completes as in bash, with the descriptions in ble.sh's menu, and offers no file names
  where cld offers nothing - on TAB, and in grey as you type. `-n=NAME`, `--name=NAME` and
  `--mcp=SERVER` complete nothing there, since ble.sh drops what cld offers after the `=`: write
  `-n NAME`. ble.sh 0.3 offers file names wherever cld offers nothing.

  macOS's own `/bin/bash`, 3.2, takes Homebrew's `bash-completion` (1.3), which reads no such
  directory: install it, add the line its caveats show to `~/.bash_profile`, and write the script
  by hand, `cld completion bash > "$(brew --prefix)/etc/bash_completion.d/cld"`, which
  `cld update` leaves as it is; bash 3.2 cannot load it with `source <(cld completion bash)`. It
  puts no space after a completed name, and offers file names where cld offers nothing.
- **zsh**: `~/.local/share/cld/zsh/_cld` (`$XDG_DATA_HOME` in place of `~/.local/share` where
  set), and at the end of `~/.zshrc` - `$ZDOTDIR/.zshrc` where `ZDOTDIR` is in the environment -
  the lines that load it:

  ```zsh
  # cld's completion, from cld setup completion zsh
  if [[ -r ${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh/_cld ]]; then
    fpath=("${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh" $fpath)
    (( $+functions[compdef] )) || { autoload -U compinit && compinit -i; }
    autoload -Uz _cld && compdef _cld cld
  fi
  ```

  They run `compinit` only where nothing before them has - oh-my-zsh, say, or Ubuntu's
  `/etc/zsh/zshrc` - since a second `compinit` drops the completions set up after the first; with
  `-i`, it leaves out directories that other users can write to instead of asking about them as
  zsh starts. A `compinit` after the lines finds the script all the same. Where the script is
  missing, as on another machine that shares the `.zshrc`, they do nothing. cld adds them once, and
  not again while `.zshrc` has their first line. Where a tool writes `.zshrc` for you - a link into
  a read-only store, as home-manager makes, or a dotfiles manager that writes it over - cld cannot
  write it, or its lines do not last: add them where that tool takes them.
- **fish**: `~/.config/fish/completions/cld.fish` (`$XDG_CONFIG_HOME` in place of `~/.config`
  where set), the directory fish looks in first, where the script used to be written by hand:
  cld replaces such a script.

## Troubleshooting

- **claude too old.** `cld new` and `cld resume` name the version they found. Update claude the way
  you installed it: `claude update` for the native installer, or through Homebrew, npm or your
  system's package manager.
- **A server without its session.** If claude exits while what it started through tmux keeps its
  server running, `cld new`, `cld resume`, `cld join` and `cld kill` refuse the name: end the
  server with `tmux -L cld-S kill-server`.
- **Names that differ only in case.** Where tmux's socket directory ignores case, as on macOS's
  default file system, `A` and `a` share one socket: while one of them runs, cld refuses the other.
- **`File name too long`.** The server's socket, `$TMUX_TMPDIR/tmux-UID/cld-S` with its symlinks
  resolved (on macOS `/tmp` is `/private/tmp`), must stay within 103 bytes on macOS and 107 on
  Linux: under a long `TMUX_TMPDIR`, use a shorter name.

## Upgrading

- **cld.** `cld update` replaces cld with the latest release, when there is a newer one: it checks
  the release's binary for your system against `cld.sha256` and that it runs, then replaces the
  file cld runs from - the one a symbolic link leads to - which you need to be able to write.
  Then it writes anew the completion scripts that `cld setup completion` wrote, where the new
  release prints others (see [Shell completion](#shell-completion)). It reaches GitHub through the
  proxy `HTTPS_PROXY` names, if any. cld 0.5.0 and earlier have no `update`: run the
  [install command](../README.md#install) again, with the same `CLD_INSTALL_DIR` if you gave one -
  it also takes `CLD_VERSION` for another release. A cld built from source, whose version is
  `dev`, is not updated.
- **`cld join` and other terminals.** cld 0.7.0 and earlier detached any other terminal from the
  session on `cld join`, and on `Enter` in `cld list`; both now leave it attached. Use
  `cld join --detach-others` to take the session over as before.
- **Session names.** In cld 0.7.1 and earlier, `-n NAME` named the session `NAME` whole,
  defaulting to `main`, and `-w` named the worktree `NAME`. Now a session is `NAME-SUFFIX` (see
  [Sessions](#sessions)), and `cld join` and `cld kill` need `-s`, as `cld resume` does without
  `SESSION`. A session made before, `main` say, has no `SUFFIX`: join or kill it from `cld list`,
  or with `cd / && cld join -s main`. Its conversation comes back with `cld resume cld-main`, in a
  session named as `cld new` names one; a worktree made before, `NAME`, stays where it is, and
  claude takes the conversation back there. `git worktree remove .claude/worktrees/NAME` removes
  it once you are done with it.
- **tmux.** End the sessions started before the upgrade (`cld list`, then `cld kill`): each
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
