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
  those `cld list` shows, and of servers that outlive their session or are your own (see
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
- Repositories of one name share `NAME` and its indexes - two clones of a project, a fork beside
  its upstream, `api` in two places - and so do directories of one name outside a repository.
  `cld new` and `cld resume` record where they made a session: the repository's directory, or
  outside one the current directory. Without `-n`, `cld join` and `cld kill` refuse a session made
  in another - the repository's worktrees and subdirectories are its own - and say where:

  ```
  cld: session 'api-0' belongs to /work/api, not to this repository; name it with cld kill -n api -s 0
  ```

  With `-n`, and with `-s S` where `NAME` leaves nothing, they take the session from anywhere, as
  `cld list` does; they take a session that cld 0.8.2 or earlier made, which records nothing,
  from anywhere too. `cld new` and `cld resume` name the directory of a session whose name they
  refuse, and `cld join -s` completes only the sessions `cld join` takes.
- Each claude gets the environment of the shell that ran `cld new` or `cld resume` -
  `CLAUDE_CONFIG_DIR`, a virtualenv, `AWS_PROFILE` and the like - but for the variables that
  would have claude take itself to be in that shell's terminal, whichever terminal joins, and
  Shift+Enter for Enter: those of JetBrains IDEs, Cursor and Visual Studio, macOS's
  `__CFBundleIdentifier`, and the askpass and editor that VS Code and its forks give git, which
  would ask in a window that may have closed. A `GIT_ASKPASS` or `GIT_EDITOR` of your own stays.
- claude keeps that environment for its life: `cld join` from another ssh connection gives tmux
  its `SSH_AUTH_SOCK` and `DISPLAY`, for what starts on the session later, not claude, so after a
  reconnect claude's `git push` finds no agent. An agent socket at a path that stays -
  `SSH_AUTH_SOCK` naming a link that `~/.ssh/rc` points at each login's socket, say - or
  `cld kill` then `cld resume` is the way around it.
- cld takes the terminal for UTF-8, as claude does, whatever `LC_ALL`, `LC_CTYPE` and `LANG` say:
  tmux would draw what is not ASCII - most of claude's UI - as `_` where they name no UTF-8, as
  over ssh to a host whose sshd takes no `LANG`, in a container or from cron. A terminal that
  does not take UTF-8 shows claude garbled, under cld or not.
- Whatever claude runs - its Bash tool, a hook - reaches the session's server with a plain `tmux`,
  and `tmux -L cld-S ls` lists what runs there. cld sees only `cld-S`; `cld kill` ends the
  rest with the server, also once claude has exited (see [Troubleshooting](#troubleshooting)).
- `cld kill`, and `Ctrl+X` in `cld list`, end claude as a terminal that closes does: claude
  kills the shell commands it still runs, runs its `SessionEnd` hooks with the reason `other`
  (`/exit` gives `prompt_input_exit`), and exits. It gives the hooks 1.5 s, longer where one has
  a longer `timeout`, 60 s at most; `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS` sets it outright.
  What it prints on the way out, its resume hint among it, is lost. `cld kill` returns once tmux
  has ended the session, without waiting for claude, whose hooks may still run for a moment:
  `cld kill -s 1 && cld resume -s 1` can briefly run the old claude beside the new one.
- Only claude's own pane stays on screen when it fails: a pane split off in its window - by
  claude for a teammate, or with `C-q %` - closes when its program ends, as in plain tmux,
  whatever its exit status.
- Joining from a second terminal leaves the first attached: both show the same claude, and keys
  from either reach it. The window takes the size of the terminal you used last, and a larger one
  shows the rest of its screen dotted. `cld join --detach-others` detaches the other terminals
  instead; claude keeps running in its directory either way.
- The mouse goes to claude where claude takes it, in its fullscreen view: the wheel and clicks
  reach claude as they would without cld. claude opens a link on a Ctrl+click or an Alt+click;
  not on a plain click in Ghostty, nor on a Cmd+click in Ghostty or Warp on macOS, as it does
  without cld, since claude sees tmux there and not your terminal - which may still open the
  link with its own click, as below. Where claude does not take the mouse - drawing in the main
  screen, or after it has failed - tmux takes it: the wheel scrolls the pane's history in tmux's
  copy mode, which `q` leaves, a drag selects there and copies as you let go, a middle-click
  pastes tmux's last copy, and a right-click opens tmux's menu for the pane. For your
  terminal's own selection, drag with Shift held - Option in iTerm2, Fn in Terminal.app. A
  session that an older cld started keeps tmux's own Ctrl+click and Alt+right-click until it
  ends: there claude opens a link on an Alt+click alone.
- Remote Control is turned on over the "Enable Remote Control for all sessions" setting in
  `/config`. It stays off where your organisation's policy does, or where the project's
  `.claude/settings.json` or `.claude/settings.local.json` sets `remoteControlAtStartup` to
  `false`.
- The tab's title is `✳ cld-S`, with `◐` and `◑` in turn in place of the `✳` while claude
  works. Under tmux claude keeps its own marker at `✳`, so cld gives claude hooks with
  `--settings` that tell tmux when a turn starts, when claude asks for a permission and when the
  turn ends. They miss:
  - an interrupt (`Esc`) as claude writes: the title stays busy until the next prompt, or until
    claude, idle for a minute, notifies it; an interrupt in a tool comes through;
  - a prompt that a `UserPromptSubmit` hook of your own blocks: busy until the next one;
  - everything under `disableAllHooks`, or a policy that allows only managed hooks: the title
    stays `✳ cld-S`.

  A terminal that detaches keeps the title it had, a busy one too. A session that an older cld
  started keeps `✳ cld-S`.
- While claude works in a linked git worktree - one `cld new -w` has it make, one it enters with
  its `EnterWorktree` tool, one you run `cld new` in - the title ends in ` [w]`, and loses it
  when claude leaves. It follows claude's working directory, not where a shell command `cd`s
  to. Without git on your `PATH` when the session starts, or with hooks turned off, there is no
  `[w]`.
- claude's links - the file paths and URLs it marks - reach the terminal as links, which it opens
  with its own click; as tmux has the terminal report clicks, some terminals want the key that
  keeps a click from the program with it. tmux passes links on where it knows the terminal takes
  them: cld tells it so for a `TERM` that starts with `xterm`, as most terminals set, and for
  `wezterm` and `alacritty`; iTerm2, foot and tmux it recognises. Inside another tmux, links go on
  to that tmux, which - from 3.4 - passes them to its own terminal only where it knows that
  terminal takes them. Other terminals under another `TERM`, and a session an older cld started,
  show them as plain text.

## Scrollback

While a terminal is attached, tmux draws the session in the terminal's alternate screen: the
terminal's own scrollback, scroll bar and search get nothing from it. What scrolls off claude's
screen goes to tmux's history of the pane instead, the last 50000 lines of it.

- The mouse wheel, or `C-q [`, shows the history in tmux's copy mode: `PgUp` and the arrows move,
  `C-r` searches up and `C-s` down - `?` and `/` where `VISUAL` or `EDITOR`, in the shell that
  ran `cld new`, names vi - and `q` leaves.
- That is claude's classic renderer, which draws in the main screen as a shell does: without
  tmux the conversation would be in the terminal's scrollback. claude's fullscreen renderer draws
  in the alternate screen and scrolls its own transcript: the wheel goes to it, and `Ctrl+O`,
  then `/`, searches it; `Ctrl+O`, then `[`, writes the conversation out for copy mode. `/tui`
  says which renderer runs, and `/tui default` and `/tui fullscreen` switch.
- A session an older cld started keeps tmux's 2000 lines.
- With a screen reader: Claude Code's screen-reader mode, which always runs the classic renderer,
  leaves the conversation in the terminal's scrollback for your screen reader's review commands
  and the terminal's search, and marks each turn (OSC 133) for the terminal's jump between
  prompts. Under cld the scrollback holds nothing, and tmux passes no marks on. Read back in copy
  mode, `C-q [`, a screen at a time, or run `claude` itself, without cld.

## The session list

On a terminal, `cld list` shows the sessions full screen, the first one selected. `Enter` joins
the selected one as `cld join` does, beside any other terminal on it; `Ctrl+X` twice kills it as
`cld kill` does. After the first `Ctrl+X`, `Esc` keeps the session and the list open; so do two
seconds without the second `Ctrl+X`, and any other key, which then does what it does. A `Ctrl+X`
held down does not go on to kill the next session.

- The list reads the sessions when it opens and after a kill: a session made or ended elsewhere
  shows when you run `cld list` again. If the selected session has ended when you press `Enter` or
  the second `Ctrl+X`, the list says so and reads them again; where what claude started keeps its
  server running, `Ctrl+X` ends that server, as `cld kill` does.
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

## Notifications

claude's setting `preferredNotifChannel` - "Local notifications" in `/config` - picks how claude
notifies you. Its default, `"auto"`, sends a desktop notification in iTerm2, kitty and Ghostty,
rings the bell in Terminal.app where the profile's audible bell is off, and sends nothing in other
terminals - nor in a session of cld's: claude goes by `TERM_PROGRAM`, which tmux sets to `tmux` in
its panes, whatever terminal is attached. Set your terminal's channel in claude's user settings,
`~/.claude/settings.json` (`$CLAUDE_CONFIG_DIR/settings.json` if you set that variable):

| Terminal | `preferredNotifChannel` | claude sends |
|---|---|---|
| iTerm2 | `"iterm2"`, or `"iterm2_with_bell"` to ring the bell too | OSC 9 |
| kitty | `"kitty"` | OSC 99 |
| Ghostty | `"ghostty"` | OSC 777 |
| Terminal.app, any other | `"terminal_bell"` | the bell, which the terminal shows as it is set to |

- claude wraps OSC 9, 99 and 777 in tmux's passthrough, and cld's server passes them on, as it
  passes the bell, to every terminal attached to the session. A session with no terminal attached
  shows none, then or when one attaches. The tests check what reaches the terminal; a real claude
  notifying through cld, and iTerm2, kitty and Ghostty showing it, have not been checked yet.
- The setting applies to every claude you run, outside cld too. There `"iterm2"`, `"kitty"` and
  `"ghostty"` change nothing in the terminal they name, but `"terminal_bell"` rings the bell where
  `"auto"` sends nothing: in other terminals, and in Terminal.app with its audible bell on. One
  channel serves every terminal that joins a session. cld does not set it for you: its
  `--settings` would override yours, and the terminal that joins later may be another.
- iTerm2 hands its notifications to macOS once "Notification Center Alerts" is on, and "Send
  escape sequence-generated alerts" under "Filter Alerts" (Settings › Profiles › Terminal), as
  Claude Code's
  [docs](https://code.claude.com/docs/en/terminal-config#get-a-terminal-bell-or-notification) say.
- A `Notification` hook of your own - a sound, `notify-send` - runs whatever the setting, in any
  terminal.

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
- Claude Code's [agent view](https://code.claude.com/docs/en/agent-view) moves a conversation to
  its background sessions, where claude's supervisor process runs it on as a copy, with a new
  session ID and the same name, which Claude Code's docs say it numbers, as in `cld-S (2)`, where a
  background session has it already. `/background` (`/bg`), and "Move to background and exit" in
  the dialog `/exit` shows while background work runs, exit claude without an error, which ends
  the session and its server. `←` on an empty prompt leaves claude in the session, in agent view,
  where `Esc`, or `Enter` on the conversation's row, goes back to the conversation (not checked
  yet), which the supervisor goes on running: `cld kill` ends the claude in the session but not
  the copy. `cld list` does not tell that a conversation moved; `claude agents` lists the
  background sessions. While the copy runs, `cld resume -n NAME -s SUFFIX` finds it by the name,
  and claude refuses it and exits with an error, which leaves the session `exited`:

  ```text
  Session UUID is running as a background session (ID). Run `claude attach ID` to open it, or `claude stop ID` first to resume it here. Add --fork-session to branch off a copy instead.
  ```

  `claude attach ID` opens the copy in the terminal you run it in, outside cld. The copy keeps
  the hooks for the tab's title, which name cld's session `S`: while a session `S` runs - the one
  it left, or a later `cld new` that gives the index again - its tab shows the copy's status
  until the copy stops, and while none runs the hooks fail after each tool, with a hook error (not
  checked yet). To bring the conversation back into cld, run `claude stop ID`, then
  `cld kill -n NAME -s SUFFIX` where the session stays, and `cld resume -n NAME -s SUFFIX` (not
  checked yet: the old transcript has the name too). cld does not pass `--fork-session`.
  claude's `disableAgentView` setting turns agent view off, `/bg` with it; `/config` has
  `← opens agents` to turn off the key alone.

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

- `.claude/settings.json` gets `$schema`, `permissions.allow` as `--permissions` says, and
  `plansDirectory`, `.claude/plans`. Settings of a person's, such as a theme or an update channel,
  belong in your own `~/.claude/settings.json`: the project's file would override everyone's.
- `--permissions read-only`, the default, allows `Read` and `ls`, `pwd`, `cat`, `head`, `tail`,
  `wc`, `grep`, `stat`, `du`, `which` and `git status`, with any options, none of which runs a
  command or writes a file. It leaves out `git diff`, `git log` and `git show`, whose
  `--output FILE` writes any file - `.git/config` too, where git reads commands to run. claude
  runs the three without asking with the options it knows to be safe, and asks for the others.
  `--permissions cld` allows what cld's own repository does: reading, editing and writing files,
  web search and fetch, and shell commands such as `ls`, `grep`, `git`, `go`, `dotnet`, `make`,
  `docker run` and `gh pr merge` - enough for a prompt-injected claude to run anything.
  `--permissions none` adds nothing: claude asks for all it does not allow by itself.
- `.claude/settings.local.json` is only created, holding its `$schema`.
- `.gitignore` gets `/.claude/settings.local.json`, `/.claude/plans/` and `/.claude/worktrees/`;
  the lines without the leading slash count as there. The rest of `.claude` - `commands/`,
  `agents/`, `skills/`, `rules/`, `hooks/`, `CLAUDE.md` - is the project's to share. claude keeps
  its other files there out of git itself, in `.git/info/exclude`.
- With `--mcp`, each server gets its entry in `.mcp.json` and its name in `enabledMcpjsonServers`,
  and `permissions.allow` gets its tools: with `read-only`, the IDE's tools that only read - as
  GoLand 2026.2.3 marks them: reading and searching files, symbols, problems, `git_status` - and
  `jbcontext search`; with `cld`, every tool, `mcp__NAME` - GoLand's `execute_terminal_command`
  among them - and `jbcontext` whatever its command. claude asks nothing about what they allow
  once you have accepted the folder's workspace trust.

The IDEs' servers are `http://127.0.0.1:${GOLAND_MCP_PORT:-64422}/stream` and
`http://127.0.0.1:${RIDER_MCP_PORT:-64482}/stream`: `.mcp.json` is shared, and each developer's IDE
listens on a port of its own. Where yours is not the default (the IDE's "Copy HTTP Stream Config"
shows its URL), set the variable in your shell's profile, or in the `env` of your own
`~/.claude/settings.json`, which serves every project. Not in the project's
`.claude/settings.local.json`: claude 2.1.283 does not expand `.mcp.json` from it.

Where the files exist, cld adds the keys, entries and `.gitignore` lines that are missing, and
replaces a server's entry that differs from its own, whole. A key the file has keeps its value,
and keys, entries and servers of your own stay, in their order and indentation; cld removes
nothing, so what an earlier cld wrote - its `theme`, `autoUpdatesChannel`, `autoMemoryEnabled` and
`autoCompactEnabled`, a longer allow list - stays until you remove it. A file it cannot edit - not
valid JSON, say - stops it with nothing changed. In a git work tree cld then checks that git does
not ignore `.claude/settings.json` all the same - through `.claude/`, `*.json` or git's own
excludes - and otherwise names the pattern and exits with status 1. It warns of the other files a
project shares under `.claude` that git ignores, naming the pattern - of a directory even where
you added files in it with `git add -f`, as git ignores a new one there, but not of a submodule,
whose files are its own repository's. An earlier cld wrote `/.claude/*` and
`!/.claude/settings.json`, which ignore them, and which you replace with the new lines. Review the
changes, `git diff` and `git status`, before you commit them: whoever accepts the folder's
workspace trust gives claude what the settings allow.

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
repository or directory you are in and that was made there (or by cld 0.8.2 or earlier), with its
state; `--mcp` the next server after a comma; and `--permissions` its sets. No file names are
offered, and `cld new`, `cld resume`, `cld kill` and the values of `cld setup telemetry` offer
nothing. The script runs `cld` on every TAB, so the names are always current.
`CLD_COMPLETION_DESCRIPTIONS=0` in the environment leaves out the states and the other
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

## cld and Claude Code's background sessions

Claude Code runs conversations without a terminal itself: `claude --bg PROMPT` starts one, and
`/bg` (`/background`) or `←` on an empty prompt hands the one you are in to a supervisor process.
`claude attach ID` opens such a background session in any terminal, and
[agent view](https://code.claude.com/docs/en/agent-view), `claude agents`, lists them. As of
Claude Code 2.1.284 - agent view is a research preview, and its docs say what changed since:

| | cld | Background sessions |
|---|---|---|
| Needs | cld and tmux, of the version the [README](../README.md#install) names | nothing but claude; the `disableAgentView` setting, or `CLAUDE_CODE_DISABLE_AGENT_VIEW`, turns agent view off, and `--bg` and `/bg` with it |
| Address | a name - `-s SUFFIX` in its repository - which TAB completes | an ID of 8 hex digits, or its start: `claude attach 7c5d`; for a name, claude says `No job matching 'NAME'` |
| Coming back | claude as you left it, in the renderer you chose with `/tui`; in the classic one, the wheel scrolls the pane's history in tmux's copy mode | always fullscreen, whatever `/tui` chose; the terminal's scrollback and tmux's copy mode see only the screen |
| Idle | claude keeps running until you end it | the supervisor stops claude once it is done, or waits for your next message, and has been unattached for about an hour, unless the session is pinned (`Ctrl+T` in agent view); attaching resumes the conversation |
| claude crashes | the session stays, with claude's last screen and how it exited, `exited` in `cld list` | the supervisor starts claude again; `claude logs ID` shows its recent output |
| Reboot | claude stops; `cld resume` resumes the conversation in a new session | claude stops; the session shows failed - stopped after 48 hours - and attaching resumes the conversation |
| Listing | `cld list`: name, state and directory; join or kill | `claude agents`: state, activity and age; attach, peek, reply, dispatch, stop |

A cld session is not one of them: `claude agents --json` lists it as `"kind": "interactive"`, named
`cld-NAME-SUFFIX` and without the `id` that `claude attach`, `claude logs` and `claude stop` take,
and agent view does not show it; nor does `cld list` show background sessions. The two combine: in
a cld session, `/bg` or `←` on an empty prompt moves the conversation to a background session, and
`/fork` copies it into one while the original stays in the session.
[Resuming a conversation](#resuming-a-conversation) says what a move does to the session, and how
to bring the conversation back into cld.

## Troubleshooting

- **claude too old.** `cld new` and `cld resume` name the version they found. Update claude the way
  you installed it: `claude update` for the native installer, or through Homebrew, npm or your
  system's package manager.
- **`needs a terminal`.** `cld new`, `cld resume` and `cld join` attach the terminal their input
  comes from, and refuse without one - from cron, `ssh host cld new` or a script whose input is
  not the terminal - or with `TERM` unset, empty or `dumb`. Over ssh, `ssh -t host cld new` gives
  them one.
- **A server without its session.** If claude exits while what it started through tmux keeps its
  server running, `cld list` does not show the session, and `cld new`, `cld resume` and `cld join`
  refuse the name: `tmux -L cld-S ls` shows what runs there, and `cld kill` ends it with the
  server. So it goes where the session was renamed (`tmux rename-session`): claude may still run
  there, and `cld kill` ends it too.
- **A tmux server of your own named `cld-S`.** cld marks the servers it starts and leaves any other
  alone: `cld list` does not show it, cld runs in its panes as in any other tmux, and `cld new`,
  `cld resume`, `cld join` and `cld kill` refuse the name `S` - use another. One whose prefix is
  `C-q` counts as cld's, as the servers of cld 0.8.2 and earlier do.
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
- **The tab's title in a background conversation.** A session's claude keeps the hooks it started
  with. In cld 0.8.0 and 0.8.1 they found the session through claude's `TMUX` and `TMUX_PANE`,
  which claude does not give a conversation it runs in the background, in a worker of its daemon
  that the claude in the pane shows: there every hook failed, after each tool, with
  `PostToolUse:Bash hook error` and `no current session`, and the title stayed `✳`. `cld kill`
  leaves such a worker running, and claude refuses to resume its conversation: stop it with
  `claude stop ID` (`claude agents` lists the IDs), then end the session with `cld kill` and
  bring the conversation back with `cld resume`, as
  [Resuming a conversation](#resuming-a-conversation) says.
- **Shift+Enter in a session made in an IDE.** In cld 0.8.2 and earlier, a session made in the
  terminal of Cursor, Windsurf, Antigravity or Visual Studio, or on macOS of a JetBrains IDE or
  VSCodium, handed that terminal's variables to its claude, which then took Shift+Enter for Enter
  from any terminal. End such a session with `cld kill` and bring its conversation back with
  `cld resume`.
- **A pane split off in claude's window.** A session keeps what cld set for a failed claude when
  it started. In cld 0.8.2 and earlier that went to claude's window, so a pane split off there -
  by claude for a teammate, or with `C-q %` - whose program fails stays on screen, and the message
  line says `claude exited` while claude runs on. End such a session with `cld kill` and bring
  its conversation back with `cld resume`.
- **Project settings.** `cld setup project` of cld 0.4.0 to 0.8.2 wrote `/.claude/*` and
  `!/.claude/settings.json` to `.gitignore`, which keep what a project shares under `.claude` -
  commands, agents, skills - out of git, and to `.claude/settings.json` `theme`,
  `autoUpdatesChannel`, `autoMemoryEnabled`, `autoCompactEnabled` and the allow list that is now
  `--permissions cld`, which override every developer's own settings. cld removes none of them:
  run `cld setup project` again, which adds the new lines and warns of what the old ones ignore,
  then remove the two lines, the four keys and the entries you do not want to share.
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
