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
  leads: in `/root`, `cld join` makes `cld-root-0`. Each run of the characters a name cannot have
  becomes `-`, and `-` and `_` go from either end: `my.site` gives `my-site-0`, `.dotfiles`
  `dotfiles-0`. Where nothing is left - in the root directory, or for a name in another script -
  `S` is `SUFFIX` alone.
- `cld join` is the one command for a session. Where session `S` runs - attached, detached, or
  with its claude exited - it attaches to it; where `S` has ended, it brings it back first (see
  [Resuming a conversation](#resuming-a-conversation)); and where there is no `S`, it creates it
  first, in the current directory. Two `cld join -s SUFFIX` of one session at the same moment -
  or one and a `cld restore` - make it once: a second `cld join` waits for the first and attaches
  to the session it made, and `cld restore` leaves a session that a `cld join` is starting to it.
- Without `-s`, `cld join` creates a new session, under the index above the highest of the
  sessions `NAME-INDEX` that `cld list` shows - those that run, and those that have ended within
  30 days - of servers that outlive their session or are your own (see
  [Troubleshooting](#troubleshooting)), and of the indexes it gave `NAME` within 30 days, a
  forgotten session's too; a gap stays a gap. So a name comes back only once claude has removed
  the conversation of that name, 30 days after it was last written by default. Two `cld join`
  started at the same moment take two names: the second waits for the first.
- `cld kill` needs `-s`, as `cld detach` does outside claude (see
  [Terminals that take C-q](#terminals-that-take-c-q)), and `cld join` for a session that exists: in
  the session's repository or directory `-s SUFFIX` alone, elsewhere `-n NAME -s SUFFIX` too. cld's
  own messages name a session that way, splitting `S` at its last `-`. A name without one, made
  where `NAME` leaves nothing, takes `-s S` in such a directory - `cd / && cld kill -s S` - or
  `cld list`, whose `Enter` and `Ctrl+X` take any session.
- Repositories of one name share `NAME` and its indexes - two clones of a project, a fork beside
  its upstream, `api` in two places - and so do directories of one name outside a repository.
  `cld join` records where it made a session: the repository's directory, or outside one the
  current directory. Without `-n`, `cld join`, `cld detach` and `cld kill` refuse a running
  session made in another - the repository's worktrees and subdirectories are its own - and say
  where:

  ```
  cld: session 'api-0' belongs to /work/api, not to this repository; name it with cld kill -n api -s 0
  ```

  With `-n`, and with `-s S` where `NAME` leaves nothing, they take the session from anywhere, as
  `cld list` does; they take a session that cld 0.8.2 or earlier made, which records nothing,
  from anywhere too. `cld join -s` and `cld detach -s` complete only the sessions they take. cld's
  record of a session that has ended keeps no such home: from a repository of the same name,
  `cld join -s SUFFIX` brings it back in the directory it ran in, which `cld list` shows.
- Each claude gets the environment of the shell that ran the `cld join` that created or brought
  back its session - `CLAUDE_CONFIG_DIR`, a virtualenv, `AWS_PROFILE` and the like - but for the
  variables that would have claude take itself to be in that shell's terminal, whichever terminal
  joins, and Shift+Enter for Enter: those of JetBrains IDEs, Cursor and Visual Studio, macOS's
  `__CFBundleIdentifier`, and the askpass and editor that VS Code and its forks give git, which
  would ask in a window that may have closed. A `GIT_ASKPASS` or `GIT_EDITOR` of your own stays.
- claude keeps that environment for its life: a `cld join` that attaches from another ssh connection
  gives tmux its `SSH_AUTH_SOCK` and `DISPLAY`, for what starts on the session later, not claude, so
  after a reconnect claude's `git push` finds no agent. An agent socket at a path that stays -
  `SSH_AUTH_SOCK` naming a link that `~/.ssh/rc` points at each login's socket, say - or `cld kill`
  then `cld join` is the way around it.
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
  `cld kill -s 1 && cld join -s 1` can briefly run the old claude beside the new one.
- A session whose claude failed keeps what claude printed on screen, with a line below it that
  says how claude exited - `status 1`, say, or `signal 15` - how to end the session, and how to
  detach from it. The line stays through keys, `C-q d` and `cld join`; until the first key, the
  message line at the bottom says the same. It takes the screen's last row: where claude's output
  fills the screen, what was on that row goes if claude's cursor was above it, and otherwise the
  top line moves up out of sight. In a narrow terminal the line leaves out how to detach, and then
  the command that ends the session, rather than show them cut short. A session that an older cld
  started has the message line only.
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
- cld leaves [Remote Control](https://code.claude.com/docs/en/remote-control) to claude, as
  without cld: a session connects as it starts where `/config`'s "Enable Remote Control for all
  sessions" is `true` - `remoteControlAtStartup` in claude's user settings - or, left at
  `default`, where your organisation's default or Claude Code's is on. A `false` for
  `remoteControlAtStartup` in the project's `.claude/settings.json` or
  `.claude/settings.local.json` keeps it off there. `/remote-control` connects a session later,
  and claude's `--remote-control`, given after `--` as in `cld join -- --remote-control`, as it
  starts (see [claude's options](#claudes-options)). While a session is connected, its
  transcript - your messages, claude's responses, tool activity - is stored on Anthropic's
  servers; see
  [Connection and security](https://code.claude.com/docs/en/remote-control#connection-and-security).
- cld turns Claude Code's [agent view](https://code.claude.com/docs/en/agent-view) off in a
  session: `disableAgentView` in the settings it gives claude, which outrank yours. With agent
  view on, `/background` (`/bg`), "Move to background and exit" in `/exit`'s dialog and `←` on an
  empty prompt move the conversation to claude's background sessions, out of cld: the session
  ends, or keeps claude in agent view, while the conversation goes on as a copy with the hooks cld
  gave claude, which name the session (see [Resuming a conversation](#resuming-a-conversation)).
  What `/bg`, `←`, the dialog and `/fork` do under the setting has not been checked yet with
  claude running - claude's code has another `/fork` then, "Spawn a background agent that
  inherits the full conversation" - nor have `ListAgents` and `SendMessage` between cld's
  sessions, or what else of claude needs its daemon. No option of cld's turns agent view on in a
  session, and `--settings` after `--` is refused (see [claude's options](#claudes-options)); a
  claude you start without cld keeps it (see
  [cld and Claude Code's background sessions](#cld-and-claude-codes-background-sessions)).
- The tab's title is `✳ cld-S`, with `◐` and `◑` in turn in place of the `✳` while claude
  works. Under tmux claude keeps its own marker at `✳`, so cld gives claude hooks with
  `--settings` that tell tmux when a turn starts, when claude asks for a permission and when the
  turn ends; `cld list` shows the same status (see [The session list](#the-session-list)). They
  miss:
  - an interrupt (`Esc`) as claude writes: the title stays busy until the next prompt, or until
    claude, idle for a minute, notifies it; an interrupt in a tool comes through;
  - a prompt that a `UserPromptSubmit` hook of your own blocks: busy until the next one;
  - your answer to a permission, of which claude tells nothing: until the tool it asked for has
    run - a long command you allow - the title stays `✳` and `cld list` says `waiting`; after you
    refuse one, until the turn ends or your next prompt (not checked yet);
  - everything under `disableAllHooks`, or a policy that allows only managed hooks: the title
    stays `✳ cld-S`, and `cld list` shows no status.

  A terminal that detaches keeps the title it had, a busy one too. A session that a cld before
  0.8.0 started keeps `✳ cld-S`, and shows no status in `cld list`.

  Each hook starts a tmux client, and while the title is busy each terminal on the session starts
  `sh`, `sleep` and a tmux client every second to turn the marker: each takes as long, and as
  much CPU, as tmux takes to start (see [Troubleshooting](#troubleshooting)). Through Ubuntu's
  snap, claude waits a tenth of a second and more at each event - as it starts, after every
  tool, as a turn starts and ends, as it asks you something - and the turning takes some 14% of
  a core for each terminal, also where one of the misses above leaves the title busy. claude
  waits for a hook 5 s at most, and not at all for the one on a change of its directory, which
  it runs in the background.
- While claude works in a linked git worktree - one `cld join -w` has it make, one it enters with
  its `EnterWorktree` tool, one you run `cld join` in - the title ends in ` [w]`, and loses it
  when claude leaves. It follows claude's working directory, not where a shell command `cd`s
  to. Without git on your `PATH` when the session starts, or with hooks turned off, there is no
  `[w]`. claude does not wait for tmux as it changes directory: a worktree it enters and leaves
  again within one answer can leave the title as the first change had it, until claude next
  changes directory, and a tmux that fails there shows only in claude's transcript view
  (`Ctrl+O`).
- claude's links - the file paths and URLs it marks - reach the terminal as links, which it opens
  with its own click; as tmux has the terminal report clicks, some terminals want the key that
  keeps a click from the program with it. tmux passes links on where it knows the terminal takes
  them: cld tells it so for a `TERM` that starts with `xterm`, as most terminals set, and for
  `wezterm` and `alacritty`; iTerm2 and tmux it recognises, and foot from tmux 3.7. Inside another
  tmux, links go on to that tmux, which - from 3.4 - passes them to its own terminal only where it
  knows that terminal takes them. Other terminals under another `TERM`, and a session an older cld
  started, show them as plain text.

## Scrollback

While a terminal is attached, tmux draws the session in the terminal's alternate screen: the
terminal's own scrollback, scroll bar and search get nothing from it. What scrolls off claude's
screen goes to tmux's history of the pane instead, the last 50000 lines of it.

- The mouse wheel, or `C-q [`, shows the history in tmux's copy mode: `PgUp` and the arrows move,
  `C-r` searches up and `C-s` down - `?` and `/` where `VISUAL` or `EDITOR`, in the shell that
  ran the `cld join` that created the session, names vi - and `q` leaves.
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

## Terminals that take C-q

`C-q d` detaches only where the terminal hands `Ctrl+Q` on to tmux. Some keep it, and the key does
nothing there:

- VS Code on macOS and Windows - and the Remote-SSH, WSL and container windows opened from them -
  where `Ctrl+Q` is Quick Open View, one of the commands its terminal leaves to VS Code. Give the
  key to the terminal in your `settings.json` (not checked yet):

  ```json
  "terminal.integrated.commandsToSkipShell": ["-workbench.action.quickOpenView"]
  ```

- JetBrains IDEs with the Visual Studio 2022 keymap, which Rider bundles, where `Ctrl+Q` is Find
  Action: keep "Override IDE shortcuts" on in Settings › Tools › Terminal, and remove `Ctrl+Q`
  from Find Action in Settings › Keymap (not checked yet).

Without the key:

- `! cld detach` in claude detaches the terminal you typed it in, and leaves any other terminal on
  the session attached. `cld detach` alone does so wherever the session's tmux server runs it -
  claude, or a shell in a window you open there - as it detaches the terminal used last on the
  session; where no terminal is attached, it does nothing. With the session open in two terminals,
  a click, the focus or the mouse moving over the other one counts as using it too: move the
  mouse over the other terminal before claude runs the command, and that one is detached.
- `cld detach -n NAME -s SUFFIX`, from another terminal or in claude, detaches every terminal on
  the session; `cld join --detach-others` detaches them too, as it attaches the terminal it runs
  in.
- Closing the terminal's tab detaches it as well. claude keeps running either way.

When claude has exited with an error, the line on screen names `cld detach -n NAME -s SUFFIX`, where
the terminal is wide enough, for another terminal: claude no longer runs `!`.

## The session list

On a terminal, `cld list` shows the sessions full screen, the first one selected. `Enter` joins
the selected one as `cld join` does, beside any other terminal on it; `Ctrl+X` twice kills it as
`cld kill` does. After the first `Ctrl+X`, `Esc` keeps the session and the list open; so do two
seconds without the second `Ctrl+X`, and any other key, which then does what it does. A `Ctrl+X`
held down does not go on to kill the next session.

- The list reads the sessions when it opens and after a kill: a session made or ended elsewhere
  shows when you run `cld list` again. If the selected session has ended when you press `Enter`,
  `Enter` brings it back, as on an `ended` row; if it has gone - forgotten meanwhile - or has ended
  when you press the second `Ctrl+X`, the list says so and reads them again. Where what claude
  started keeps its server running, `Ctrl+X` ends that server, as `cld kill` does.
- A session that has ended shows as `ended`, with the directory it ran in (see
  [Resuming a conversation](#resuming-a-conversation)); a session you kill stays on its row as one.
  On such a row, `Enter` resumes it as `cld join -n NAME -s SUFFIX` does, and `Ctrl+X` twice
  forgets it: cld's record of it goes, and its conversation stays in claude's history. cld
  checks claude for the resume once the list has closed.
- `STATE` has claude's status after the state, as the tab's title follows it: `busy` while claude
  works, `waiting` while it asks you something - a permission, an MCP server's question - drawn in
  bold, and `idle` once its turn is done, as in `detached, waiting`. The rows stay in the order of
  their names. There is none until claude's first prompt in the session, none once claude has
  exited (`exited`), and none where claude runs no hooks; the hooks miss some changes (see
  [Sessions](#sessions)). A pane split off in claude's window that keeps the session after claude
  has gone, or that you are in beside claude's `exited` one, shows the status claude left, as the
  title does. The status is as of when the list read the sessions.
- The state `attached` counts terminals only: someone on the session through Remote Control does
  not show, and a kill ends the session for them too.
- A kill leaves a `cld join -w` worktree where it is, and the conversation stays: after a kill by
  mistake, `Enter` on its row, or `cld join -n NAME -s SUFFIX`, brings it back.
- `cld list` prints the table and exits where its input or output is not a terminal
  (`cld list | cat`), `TERM` is unset or `dumb`, it runs in the background, or it runs in a pane of
  one of cld's servers; with no sessions, running or ended, it prints nothing. A script that
  leaves it the terminal gets the list and waits for a key: pipe it for the table.
- Whether a JetBrains IDE passes `Esc` and `Ctrl+X` on to its terminal depends on its keymap;
  `Ctrl+C` also leaves the list.

## Idle sessions

A claude holds some 0.2 to 0.5 GB of memory for as long as its session runs, used or not.
`cld list`, and `cld join` without `-s`, end each session idle for longer than 30 days, as
`cld kill` ends it, and say so on stderr: `cld: ended session 'api-0', idle for 31 days`. The
`LAST ACTIVE` column of `cld list` shows how long ago each session was active - `now`, `5m`, `2h`,
`31d` - as of when the list read the sessions, and `-` for a session that has ended.

- A session is idle while no terminal is attached to it, from the last key typed into a terminal on
  it or the last terminal attaching, whichever came later: tmux counts nothing else. claude working
  on its own - a long task, `/loop` - does not count, nor does a conversation continued through
  Remote Control, nor claude's status in `STATE`: a `detached, busy` session is ended too. A session
  with a terminal attached is never idle. `C-q d` is a key; a terminal that closes without it, or
  that `cld join --detach-others` or `cld detach` detaches, leaves the session idle from its last
  key, or from when it attached.
- To keep a session, join it now and then (`cld join`, then `C-q d`), or set `CLD_IDLE_DAYS` in
  your shell's profile: the number of days, such as `90` or `0.5`, or `0` to end none. `cld list`
  and `cld join` without `-s` refuse a value that is no number of days. Completion ends no
  session, and neither does `cld join -s SUFFIX`.
- cld never ends the session it runs in: a session's claude running `cld list`, say, or a shell in
  a pane on its tmux server. Ending it would end cld too, and that claude in the middle of its
  work. A session driven through Remote Control alone ends at the next `cld list` run elsewhere.
- The kill checks again that the session is idle: a terminal that attaches as `cld list` ends it,
  or a key typed, keeps it. The session goes with its server, as with `cld kill`: whatever claude
  started through tmux ends too.
- `cld join` without `-s` ends the idle sessions once it has taken its index, so the session it
  makes does not take the name of one it has just ended: in a repository `api` whose one session,
  `api-0`, was idle, it makes `api-1`. As after `cld kill`, the name is not given again while cld
  keeps the session (see [Sessions](#sessions)). Where a server answers the read with an error,
  `cld join` warns and ends none; `cld list` fails with it.
- A session ended for being idle shows in `cld list` as `ended`, as after `cld kill`, and `Enter`
  on its row, or `cld join -n NAME -s SUFFIX`, brings the conversation back (see
  [Resuming a conversation](#resuming-a-conversation)) for as long as Claude Code keeps it: it
  removes a transcript last written longer ago than its `cleanupPeriodDays` setting, 30 days by
  default, so the conversation of a session idle for 30 days can go soon after, and cld forgets
  the session about then. Raise `cleanupPeriodDays` in `~/.claude/settings.json`, or set
  `CLD_IDLE_DAYS` lower, to keep the conversations for longer than the sessions.
- A session that `cld restore` brings back after a reboot is new to tmux: its `LAST ACTIVE`, and
  the time it is idle from, start at the restore. So `cld restore` leaves ended a session not
  started or given a prompt for longer than `CLD_IDLE_DAYS` days, saying so - it cannot tell
  your keys and attaches before the reboot - which would otherwise come back at every reboot (see
  [After a reboot](#after-a-reboot)).

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

## Inside your own tmux

cld runs inside your own tmux as in any terminal: the session is attached in a pane of your tmux,
which gets the terminal's keys before the session does. Without the lines below, your tmux keeps
from claude:

- its prefix, `C-b` by default, which is claude's key to background a task: `C-b C-b` sends one
  `C-b` on. Another prefix, and a second one (`prefix2`), is kept the same way;
- Shift+Enter, which arrives as Enter and submits the prompt - Ctrl+Enter and other such keys lose
  their modifier too: `Ctrl+J`, or `\` and then Enter, starts a new line;
- clipboard copies, focus events, and claude's links, which show as plain text where your tmux
  does not know that the terminal takes them;
- claude's notifications but the bell: OSC 9, 99 and 777 (see [Notifications](#notifications)),
  which your tmux drops whatever its settings. The bell, `"terminal_bell"`, gets through.

Where your tmux keeps a key, `cld join` and `Enter` in `cld list` name the keys on the session's
last line once it is attached, until you press a key, which reaches claude:
`your tmux keeps C-b and Shift+Enter: see "Inside your own tmux" in cld's guide`. Shift+Enter is
named unless your tmux asks your terminal for modified keys and passes them on, which needs the
first line below and, for most terminals, the second. As claude starts, it can draw over the line:
tmux draws it again a second and three seconds after attaching, and a claude that draws over it
later hides it until the key; `C-q ~` lists the messages tmux has shown, the line among them (`q`
leaves the list). Your tmux has a prefix unless it is set to `None`, so the line comes back on
every attach inside it. Where cld's own tmux is 3.5, which shows such a line only by holding back
what claude draws until the key, cld shows none. Nor can it tell where `TMUX` does not name your
tmux: run over ssh from a pane of it, cld says nothing, though the same keys are kept.

These lines in `~/.tmux.conf` bring back all but the prefix and those notifications:

```
set -s extended-keys on
set -as terminal-features 'xterm*:extkeys:hyperlinks'
set -s set-clipboard on
set -s focus-events on
```

Where cld's own tmux is 3.5 or 3.6, the first line has to be `set -s extended-keys always`: only
from 3.7 does it take your tmux for a terminal that sends modified keys, and ask it for them, and
with `on` your tmux passes them only to a program that asks. With `always` it passes them to every
program in it, asked or not. The second line tells tmux that your terminal sends modified keys
when asked, and takes links, as cld's own tmux assumes: tmux knows it only for the terminals it
recognises - iTerm2, mintty and XTerm for the keys, foot too from tmux 3.6 and WezTerm from 3.7,
and iTerm2 for links, foot too from 3.7 - and the line names every terminal whose `TERM` begins
with `xterm`; where yours is another, put it in place of `xterm*`. With `set-clipboard on`, any
program in your tmux can set the clipboard, not claude alone. A terminal attached before the lines
keeps what it had: after `tmux source ~/.tmux.conf`, detach from your tmux and attach again, and
there detach from the session (`C-q d`) and `cld join` it again.

## Resuming a conversation

A conversation stays in Claude Code's history until Claude Code removes it, 30 days after it was
last written by default. cld keeps a record of its sessions as long, in
`$XDG_STATE_HOME/cld`, by default `~/.local/state/cld`: for each, the directory claude started in
and the ID of its conversation, which a hook cld gives claude writes as claude starts the
conversation, and again after `/clear` or `/resume`. Where session `S` has ended,
`cld join -n NAME -s SUFFIX` makes it again, as it makes a new session, with claude resuming that
conversation, `claude --resume ID`, in that directory, from wherever you run `cld join`. The ID
finds the conversation whatever its name has become - after `/rename`, also from claude.ai or the
app - and whichever others share it.

- A reboot, or a server that crashes, ends the sessions and their claudes, but not the
  conversations, cld's record or a `cld join -w` worktree: `cld list` then shows the sessions as
  `ended`, with the directories they ran in, and `Enter` there, or `cld join -n NAME -s SUFFIX`,
  brings each back, as `cld restore` brings back all that ran (see
  [After a reboot](#after-a-reboot)). So does `cld kill`, and claude's own `/exit`, whose sessions
  `cld restore` leaves ended.
- cld forgets a session 30 days after claude last started, answered in or ended its conversation
  there - claude's default `cleanupPeriodDays`: a `cleanupPeriodDays` of your own is not read -
  but not while its server runs, and when you forget it in `cld list`. `cld kill` does not forget
  it.
- `cld join --new -n NAME -s SUFFIX` makes a session that has ended anew, in the current
  directory, with a new conversation; the old one stays in claude's history, under the same name.
- Where the directory the session ran in no longer exists, `cld join` refuses to bring it back,
  naming the command that resumes the conversation by its ID (without one, by its name) from the
  current directory: `cld join -n NAME -s SUFFIX --resume ID`.
- Without a record of the session - one an older cld started, or one forgotten -
  `cld join -n NAME -s SUFFIX` makes a new session under the name, with a new conversation:
  `cld join -n NAME -s SUFFIX --resume cld-S` resumes the old one by its name, in the current
  directory. Where the record has no ID - the session's claude failed before it started its
  conversation, or tmux failed to make the session, which `cld list` shows as `ended` all the
  same - `cld join` runs `claude --resume cld-S` in the directory the session ran in. claude looks
  the name up there or, in a git repository, in any checkout of it; a session ID it finds from any
  directory. claude looks in the history of the shell's `CLAUDE_CONFIG_DIR`, if you set one.
- By name, claude resumes the conversation when exactly one has the name - whatever the case of
  its letters. Several can have it: `/clear` keeps the name for the conversation it starts, and a
  later `cld join --new -s SUFFIX` of the same `S`, or a `cld join -s SUFFIX` once cld has
  forgotten the session, gives it to a new one as well - whose ID cld's record then holds in place
  of the old one's; claude keeps a name unique only among the claudes running. With several, or
  none, claude opens its picker with the name as the search term, which lists the conversations
  whose name, git branch or tag contains it: for `cld-rev`, `cld-review` too. Where the repository
  has more than one worktree - `cld join -w` makes one - the picker starts with the conversations
  of the checkout claude runs in, although claude looked the name up in every checkout. `Enter` or
  `↓` leaves the search box for the list, where `Ctrl+W` shows every worktree's conversations,
  `Ctrl+A` every project's, and `Ctrl+R` renames the one selected. Pick one there, or resume it by
  its session ID: `cld join -s SUFFIX --resume ID`.
- `cld join --resume SESSION` makes the session with claude resuming `SESSION`, in the current
  directory - a session that has ended too, in place of its own conversation - and claude still
  gets `--name cld-S`: the conversation takes the session's name for good, as cld's record gets
  its ID - claude sets the name before it restores the conversation's own, which it keeps only
  where none is set. Without `-s`, the session gets the next index, as any `cld join` without `-s`.
- `cld join --resume SESSION --fork` passes `--fork-session`: claude resumes a copy of the
  conversation under a new session ID, named `cld-S`, and leaves the conversation as it was, its
  name included. The copy starts where `cld join` runs - claude does not take it back to a
  worktree - and without the conversation's Remote Control session; cld's record gets the copy's
  ID, so that `cld join -n NAME -s SUFFIX` brings the copy back once the session has ended. It can
  run beside a session that has the conversation open, as `cld join -s b --resume cld-api-0 --fork`
  beside `api-0`, and claude resumes a copy of a conversation that runs as one of its background
  sessions, which it otherwise refuses to resume - both read from claude's code, not checked yet
  with claude running. `--fork` needs `--resume SESSION`, and refuses `cld-S` itself, whatever the
  case of its letters: the copy would take the name of the conversation it copies, and a
  `cld join --resume` by that name would open the picker. Without `-s`, the index can make it so -
  `cld join --resume cld-api-0 --fork` in repository `api`, where no session of that name runs or
  is recorded, would be session `api-0` again - so give another `-s`. A `SESSION` that names the
  conversation another way, by its session ID or as the one you pick in the picker, cld cannot
  tell, and it checks none of this for a `--fork-session` given after `--`.
- For a session ID that matches no conversation, claude prints
  `No conversation found with session ID: ...` and exits with an error; the session stays with the
  message.
- A resumed conversation keeps its model, which `--model` overrides, but not `--mcp-config`,
  `--plugin-dir`, `--add-dir` or `--fallback-model`: give them again after `--` (see
  [claude's options](#claudes-options)), with absolute paths - where `cld join` brings back a
  session that has ended, claude starts in the directory the session ran in, and reads a relative
  path from there, not from where you run `cld join`. `Enter` in `cld list` resumes a session
  without them.
- Where the session runs, one whose claude exited included, `cld join` attaches to it, and refuses
  `--resume`, `--new`, `-w` and the words after `--`, which would be lost: end it with `cld kill`
  first, or give another `-s`. It cannot tell whether the conversation is open elsewhere. When
  another claude has the conversation's Remote Control session, the resumed one leaves Remote
  Control off (`Remote Control not started here`) until `/remote-control` moves it over; whether a
  session that Remote Control connected as it started records that session in its conversation
  has not been checked yet.
- Claude Code's [agent view](https://code.claude.com/docs/en/agent-view) is off in cld's sessions
  (see [Sessions](#sessions)), but not in one that cld 0.10.0 or earlier started, until it ends
  (see [Upgrading](#upgrading)). There, as in every session before the upgrade, agent view moves a
  conversation to its background sessions, where claude's supervisor process runs it on as a copy,
  with a new session ID and the same name, which Claude Code's docs say it numbers, as in
  `cld-S (2)`, where a background session has it already. `/background` (`/bg`), and "Move to
  background and exit" in the dialog `/exit` shows while background work runs, exit claude without
  an error, which ends the session and its server. `←` on an empty prompt leaves claude in the
  session, in agent view, where `Esc`, or `Enter` on the conversation's row, goes back to the
  conversation (not checked yet), which the supervisor goes on running: `cld kill` ends the claude
  in the session but not the copy. `cld list` does not tell that a conversation moved;
  `claude agents` lists the background sessions. While the copy runs, once the session has
  ended, `cld join -n NAME -s SUFFIX` finds it by the name, and claude refuses it and exits with an
  error, which leaves the session `exited` - agent view off in the new session changes none of
  that (read in claude's code, not run). claude 2.1.285 says what follows, where 2.1.283 and
  2.1.284 began `Session UUID is running as a background session (ID).`:

  ```text
  "cld-S" is running in the background (ID). Run `claude attach ID` to open it, or `claude stop ID` first to resume it here. Add --fork-session to branch off a copy instead.
  ```

  `claude attach ID` opens the copy in the terminal you run it in, outside cld. The copy keeps the
  hooks for the tab's title, which name cld's session `S`: while a session `S` runs - the one it
  left, or a later `cld join` that gives the index again - its tab and its row in `cld list` show
  the copy's status until the copy stops, and while none runs the hooks fail after each tool, with a
  hook error (not checked yet). To bring the conversation back into cld, run `claude stop ID`, then
  `cld kill -n NAME -s SUFFIX` where the session stays, and `cld join -n NAME -s SUFFIX` (not
  checked yet: the old transcript has the name too). With `--fork`, cld passes `--fork-session`:
  `cld join -s OTHER --resume cld-S --fork` resumes a copy in another session, as the message
  offers, and leaves the background session running (not checked yet). In a session that keeps agent
  view, `/config`'s `← opens agents` turns off the key alone.

## After a reboot

A reboot, or a crash of the machine, ends every session and its claude, but not the conversations
or cld's record of them, which also keeps whether each session runs. `cld restore` brings back each
session that ran when the machine stopped, as `cld join -n NAME -s SUFFIX` would bring it back, but
detached: claude resumes the session's conversation, by its ID, in the directory it ran in, and
`cld join` attaches to it. It prints a line for each session it brings back:

```text
Restored session 'api-0' in /work/api
Restored session 'api-1' in /work/api, continuing its turn
```

- It leaves ended the sessions you ended - with `cld kill`, `Ctrl+X` in `cld list`, claude's
  `/exit` or another of its ways out - and those ended for being idle. A session whose claude
  failed, which stays until you end it, counts as one that ran. A session that an older cld
  started is left too: `cld join` brings it back.
- Closing claude's pane or its session with tmux's own keys - `C-q x`, `C-q &` - or
  `tmux kill-server` counts as a crash: `cld restore` brings the session back. End it with
  claude's `/exit` or `cld kill` for it to stay ended.
- It also leaves ended a session that nobody started or gave a prompt to for longer than
  `CLD_IDLE_DAYS` days, 30 by default, with a note: `cld: left session 'api-2' ended, idle for
  31 days`. Joining a session, or typing into it without a prompt, does not count here, where it
  does for [Idle sessions](#idle-sessions); `cld join` brings such a session back.
- A claude that was in the middle of a turn - from a prompt to the end of its answer, or an
  interrupt - resumes with the prompt `The machine restarted while you were working; continue where
  you left off.`, and goes on. It asks for permissions as in any turn: a turn that needs one waits
  until you join and answer.
- Each claude gets the environment its session started with - that of the shell that ran the
  `cld join` that created or brought back the session, without the variables that name the
  terminal (see [Sessions](#sessions)) - which cld keeps beside the session's entry, readable by
  you alone, and forgets with it. The words given to claude after `--` do not come back, as with
  `cld join`, nor do panes split in the session. What belonged to the login before the reboot
  comes back as it was, stale: claude's `git push` finds no ssh agent at the old `SSH_AUTH_SOCK`.
  An agent socket at a path that stays, or `cld kill` then `cld join` from a new login, is the way
  around it, as after a reconnect (see [Sessions](#sessions)).
- cld checks each claude as `cld join` checks it, in the directory the session ran in. A session it
  cannot bring back - its directory gone, a claude too old - is a warning, and the others come
  back; `cld restore` then exits with status 1.
- `cld restore` takes the sessions one at a time under the lock that `cld join` takes, so that two
  at once, or a `cld join` of the same session, make one session; it leaves a session that a
  `cld join` is starting to that `cld join`.

On Linux, `cld setup restore` has your systemd run `cld restore` as it starts:

```sh
cld setup restore
```

- It writes `~/.config/systemd/user/cld-restore.service`, a unit that runs this `cld` as
  `cld restore` with the `PATH` you run `cld setup restore` with, where it finds tmux, and your
  `TMUX_TMPDIR`, `XDG_STATE_HOME` and `CLD_IDLE_DAYS` where you set them, which your systemd has
  none of; then it enables the unit, `systemctl --user enable cld-restore.service`. Run it again
  after moving cld, or when one of those changes. `journalctl --user -u cld-restore` shows what
  `cld restore` said.
- Your systemd starts at your first login, and at your last logout ends what it started, the
  sessions `cld restore` brought back among them. With lingering on - `loginctl enable-linger`,
  which `cld setup restore` names where it is off - it starts at boot, and runs on without you, and
  so do the sessions.
- It needs your systemd: where `systemctl --user` fails, as in most containers, it writes nothing.
  It refuses to run on macOS. To turn it off, `systemctl --user disable cld-restore.service`, and
  remove the unit.
- Not checked yet: a real reboot. The tests end each session's server in its place, and a transient
  unit stood for the one your systemd starts.

Inside your own tmux, tmux-continuum takes cld's servers for other tmux servers of yours: while
one runs, it saves nothing, and restores nothing as your tmux starts. tmux-resurrect would not
bring back a session of cld's either: it saves the program that a pane's program runs, not claude,
and restores it by typing it into a shell.

## Worktrees

As with `claude --worktree`, gitignored files listed in `.worktreeinclude` are copied into a new
worktree, and when claude exits it asks whether to keep the worktree. Unlike it, a new worktree
branches from your current `HEAD`, not from the remote's default branch, whatever your
`worktree.baseRef` setting says.

- The worktree is named as the session is, `cld-S`, on the branch `worktree-cld-S`: in a
  repository `api`, `cld join -w` makes `.claude/worktrees/cld-api-0`.
- Once the session has ended, `cld join -s SUFFIX`, run in the repository, resumes the
  conversation, and claude takes it back to its worktree - or, if the worktree is gone, resumes
  where `cld join -w` ran and says so; `cld join -s SUFFIX -w --new` reopens the worktree with a
  new conversation. `-w` goes only to a new conversation: `cld join` refuses it where it resumes
  one - a session that has ended, without `--new`, and `--resume` - as claude takes a conversation
  back to its worktree itself. A copy that `--fork` makes stays where `cld join` runs, and a
  worktree claude makes during a resumed session - for a subagent, say - branches as your settings
  say.
- A worktree outlives its session, and `cld join -w` counts sessions, not worktrees: 30 days after
  session `api-0` last ran, the next `cld join -w` can be `api-0` again, and reopen its worktree.
  Give `-s` for a new one.
- claude makes a worktree only in a directory whose workspace trust you have accepted: run `claude`
  (or `cld join`) there once first; otherwise claude says so and exits, and the session stays with
  the message.

## claude's options

`cld join ... -- ARGS` starts claude with `ARGS` after cld's own arguments: `--name cld-S` and
`--settings`, then `--worktree cld-S` for `-w`, and `--resume` where it resumes a conversation,
with `--fork-session` for `--fork`. `ARGS` go to the claude that `cld join` starts, as it creates
a session or brings back one that has ended; where the session runs, its claude has started, and
`cld join` refuses them, as they would be lost.

- Each word goes to claude as it is, an empty one too. claude reads its options and a prompt to
  start with, and reports what it does not take: a claude that fails at startup stays with its
  message until `cld kill` ends the session.
- cld refuses, naming why, the options it gives claude itself, of which claude would keep the last:
  `-n` and `--name`, as `-n` and `-s` name the session and claude; `-w` and `--worktree` -
  `cld join -w` gives claude the worktree for a new conversation, and none for one it resumes, as
  claude takes a conversation back to its worktree itself; and `--settings`, which would replace
  cld's - agent view off, the worktree's base, and the hooks of the tab's title and of cld's record:
  your own go in a settings file, such as `.claude/settings.local.json`.
- It refuses those that resume a conversation, `-r`, `--resume`, `-c`, `--continue` and
  `--from-pr`: `cld join` resumes the conversation of a session that has ended, and
  `cld join --resume SESSION` another.
- It refuses those with which claude would not stay in the session: `-p`, `--print`, `--bg`,
  `--background`, `-h`, `--help`, `-v` and `--version` print and exit, and the hidden
  `--init-only` and `--rewind-files` run the startup hooks or restore files and exit, which ends
  the session before you can read what they print; `--tmux` moves claude to a tmux session of its
  own; and `--teleport` resumes a session from Claude Code on the web.
- A short option counts at the start of a word, as claude reads it: `-pc` is `-p` then `-c`, and
  `-nX` is `-n X`. Every word counts, after a second `--` too, where claude still looks for
  `--tmux`, `--bg` and `--background`: a value that looks like one of these goes after `=`, as in
  `--append-system-prompt='-n means a dry run'`, and a prompt that starts with one cannot be
  given - start it with another word.
- claude's commands, such as `mcp` or `update`, are words like any other: cld passes them on, and
  claude runs the command instead of a conversation, which ends the session when it exits, as
  with `cld join -- mcp list`.
- `--bare` and `--safe-mode` leave out the hooks of settings, cld's among them: the tab's title then
  keeps its `✳` while claude works, and shows no ` [w]`; and cld's record gets neither the
  conversation's ID - `cld join` then resumes by the session's name, or by the ID it resumed - nor
  the times claude answers, so that cld forgets the session 30 days after it started.
- tmux takes a command of 16364 bytes at most, of which cld's own words take some 7 KB, and
  cld refuses words that would make it longer, saying so. Long text goes to claude in a file:
  `--append-system-prompt-file`, `--system-prompt-file`, or a prompt that names a file for claude
  to read.

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

`cld join -n <TAB>` offers the `NAME` of the names of the sessions `cld list` shows, those that
run and those that have ended, with how many sessions have it; `cld join -s <TAB>` offers the
`SUFFIX` of each session whose `NAME` is `-n`'s, or else that of the repository or directory you
are in: of one that runs and was made there (or by cld 0.8.2 or earlier), with its state and
claude's status as `cld list` shows them, `detached, waiting` say, and of one that has ended, as
`ended in DIR`, the directory it ran in; `--mcp` the next server after a comma; and
`--permissions` its sets. `cld detach -n` and `-s` complete as `cld join`'s, the sessions that run
alone. No file names are offered, and `cld kill`, `--resume` and the values of
`cld setup telemetry` offer nothing. The script runs
`cld` on every TAB, so the names are always current. `CLD_COMPLETION_DESCRIPTIONS=0` in the
environment leaves out the states and the other descriptions.

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
[agent view](https://code.claude.com/docs/en/agent-view), `claude agents`, lists them. The two
answer the same need, and a conversation runs in one or the other, as you start claude: cld turns
agent view off in its sessions (see [Sessions](#sessions)), and a claude you start without cld
keeps it. To choose, as of Claude Code 2.1.285 - agent view is a research preview, and its docs
say what changed since:

| | cld | Background sessions |
|---|---|---|
| Needs | cld and tmux, of the version the [README](../README.md#install) names; agent view is off in a session | nothing but claude; the `disableAgentView` setting, or `CLAUDE_CODE_DISABLE_AGENT_VIEW`, turns agent view off, and `--bg` and `/bg` with it |
| Address | a name - `-s SUFFIX` in its repository - which TAB completes | an ID of 8 hex digits, or its start: `claude attach 7c5d`; for a name, claude says `No job matching 'NAME'` |
| Coming back | claude as you left it, in the renderer you chose with `/tui`; in the classic one, the wheel scrolls the pane's history in tmux's copy mode | always fullscreen, whatever `/tui` chose; the terminal's scrollback and tmux's copy mode see only the screen |
| Idle | claude keeps running, working or waiting, until you end it or the session has had no terminal attached and no key typed for 30 days: then the next `cld list`, or `cld join` without `-s`, ends it (see [Idle sessions](#idle-sessions)), and `cld join` resumes the conversation | the supervisor stops claude once it is done, or waits for your next message, and has been unattached for about an hour, unless the session is pinned (`Ctrl+T` in agent view); attaching resumes the conversation |
| claude crashes | the session stays, with claude's last screen and how it exited, `exited` in `cld list` | the supervisor starts claude again; `claude logs ID` shows its recent output |
| Reboot | claude stops; `cld restore`, which `cld setup restore` has your systemd run, resumes each conversation in a new session and continues a turn the reboot cut off | claude stops; the session shows failed - stopped after 48 hours - and attaching resumes the conversation |
| Listing | `cld list`: name, state and whether claude is busy, waiting or idle, when last active and directory; join or kill | `claude agents`: state, activity and age; attach, peek, reply, dispatch, stop |

A cld session is not one of them, and agent view does not show it; nor does `cld list` show
background sessions. With agent view on in the session, `claude agents --json` of 2.1.284 listed it
as `"kind": "interactive"`, named `cld-NAME-SUFFIX` and without the `id` that `claude attach`,
`claude logs` and `claude stop` take; whether it still does has not been checked yet. In a session
that cld 0.10.0 or earlier started, which keeps agent view until it ends, `/bg` or `←` on an empty
prompt still moves the conversation to a background session, and `/fork` copies it into one while
the original stays in the session: [Resuming a conversation](#resuming-a-conversation) says what a
move does to the session, and how to bring the conversation back into cld.

## Troubleshooting

- **claude too old.** `cld join`, where it starts claude, and `cld restore` name the version they
  found. Update claude the way you installed it: `claude update` for the native installer, or
  through Homebrew, npm or your system's package manager.
- **`needs a terminal`.** `cld join` attaches the terminal its input comes from, and refuses
  without one - from cron, `ssh host cld join` or a script whose input is not the terminal - or
  with `TERM` unset, empty or `dumb`. Over ssh, `ssh -t host cld join` gives it one.
- **`would be lost`.** `cld join` refuses `-w`, `--new`, `--resume` and the words after `--` for a
  session that runs, one whose claude has exited too: its claude has started, and would take none
  of them. Attach with `cld join -s SUFFIX` alone, end the session with `cld kill` first, or give
  another `-s`. It refuses `-w` for a session that has ended, which it resumes, unless `--new` asks
  for a new conversation.
- **A server without its session.** If claude exits while what it started through tmux keeps its
  server running, `cld list` does not show the session, and `cld join` and `cld detach` refuse
  the name: `tmux -L cld-S ls` shows what runs there, and `cld kill` ends
  it with the server. So it goes where the session was renamed (`tmux rename-session`): claude may
  still run there, and `cld kill` ends it too.
- **A tmux server of your own named `cld-S`.** cld marks the servers it starts and leaves any other
  alone: `cld list` does not show it, cld runs in its panes as in any other tmux, where
  `cld detach` needs `-s`, and `cld join`, `cld detach` and `cld kill` refuse the name `S` - use
  another. One whose prefix is `C-q` counts as cld's, as the servers of
  cld 0.8.2 and earlier do.
- **`C-q d` does nothing.** The terminal keeps `Ctrl+Q` from tmux: see
  [Terminals that take C-q](#terminals-that-take-c-q).
- **Names that differ only in case.** Where tmux's socket directory ignores case, as on macOS's
  default file system, `A` and `a` share one socket: while one of them runs, cld refuses the other.
  They share one file of cld's record too, which holds the one that started last.
- **`cannot record session`.** Where cld cannot write its record - `~/.local/state` read-only,
  say - `cld join` warns and makes the session all the same, without a record:
  `cld list` will not show it once it has ended, and `cld restore` cannot bring it back. Set
  `XDG_STATE_HOME` to a directory you can write.
- **`File name too long`.** The server's socket, `$TMUX_TMPDIR/tmux-UID/cld-S` with its symlinks
  resolved (on macOS `/tmp` is `/private/tmp`), must stay within 103 bytes on macOS and 107 on
  Linux: under a long `TMUX_TMPDIR`, use a shorter name.
- **A slow cld.** Each command runs tmux a few times - `tmux -V`, but for completion, a command
  for each running server it asks, and then the tmux that starts, joins, detaches or kills the
  session - and each run takes as long as tmux takes to start: 6-20 ms built from source or from
  most packages, 100-200 ms from Ubuntu's tmux snap. `cld list`, a TAB and `cld join` without `-s`,
  which ends the idle sessions (see [Idle sessions](#idle-sessions); `CLD_IDLE_DAYS=0` spares it
  that), ask every server of cld's that runs, eight at a time: over 10 sessions, under half a
  second through the snap. The sockets that ended sessions leave in `tmux-UID`, which neither tmux
  nor cld removes, cost no tmux. In a session, each hook of the tab's title runs tmux too, and
  claude waits for all of them but the one on a change of its directory: after every tool among
  others, through the snap a tenth of a second and more each time (see [Sessions](#sessions)).
  Inside your own tmux, where cld's own tmux is 3.6 or newer, `cld join` and `Enter` in
  `cld list` run one tmux more, which asks yours what it keeps from claude (see
  [Inside your own tmux](#inside-your-own-tmux)).

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
- **Session names.** In cld 0.7.1 and earlier, `-n NAME` named the session `NAME` whole, defaulting
  to `main`, and `-w` named the worktree `NAME`. Now a session is `NAME-SUFFIX` (see
  [Sessions](#sessions)), and `cld kill` needs `-s`, as `cld join` does for a session that exists. A
  session made before, `main` say, has no `SUFFIX`: join or kill it from `cld list`, or with
  `cd / && cld join -s main`. Its conversation comes back with `cld join --resume cld-main`, in a
  session named as `cld join` names one; a worktree made before, `NAME`, stays where it is, and
  claude takes the conversation back there. `git worktree remove .claude/worktrees/NAME` removes it
  once you are done with it.
- **The tab's title in a background conversation.** A session's claude keeps the hooks it started
  with. In cld 0.8.0 and 0.8.1 they found the session through claude's `TMUX` and `TMUX_PANE`,
  which claude does not give a conversation it runs in the background, in a worker of its daemon
  that the claude in the pane shows: there every hook failed, after each tool, with
  `PostToolUse:Bash hook error` and `no current session`, and the title stayed `✳`. `cld kill`
  leaves such a worker running, and claude refuses to resume its conversation: stop it with
  `claude stop ID` (`claude agents` lists the IDs), then end the session with `cld kill` and
  bring the conversation back with `cld join`, as
  [Resuming a conversation](#resuming-a-conversation) says.
- **Shift+Enter in a session made in an IDE.** In cld 0.8.2 and earlier, a session made in the
  terminal of Cursor, Windsurf, Antigravity or Visual Studio, or on macOS of a JetBrains IDE or
  VSCodium, handed that terminal's variables to its claude, which then took Shift+Enter for Enter
  from any terminal. End such a session with `cld kill` and bring its conversation back with
  `cld join`.
- **A pane split off in claude's window.** A session keeps what cld set for a failed claude when
  it started. In cld 0.8.2 and earlier that went to claude's window, so a pane split off there -
  by claude for a teammate, or with `C-q %` - whose program fails stays on screen, and the message
  line says `claude exited` while claude runs on. End such a session with `cld kill` and bring
  its conversation back with `cld join`.
- **Project settings.** `cld setup project` of cld 0.4.0 to 0.8.2 wrote `/.claude/*` and
  `!/.claude/settings.json` to `.gitignore`, which keep what a project shares under `.claude` -
  commands, agents, skills - out of git, and to `.claude/settings.json` `theme`,
  `autoUpdatesChannel`, `autoMemoryEnabled`, `autoCompactEnabled` and the allow list that is now
  `--permissions cld`, which override every developer's own settings. cld removes none of them:
  run `cld setup project` again, which adds the new lines and warns of what the old ones ignore,
  then remove the two lines, the four keys and the entries you do not want to share.
- **Remote Control.** Earlier releases of cld turned Remote Control on in their sessions, over a
  `false` in `/config`; now a session follows claude's own setting (see [Sessions](#sessions)).
  `/remote-control` connects one session. Setting "Enable Remote Control for all sessions" to
  `true` in `/config` connects cld's sessions as before, and every claude you start without cld
  too: the setting is claude's, in its user settings. A session started before the upgrade keeps
  Remote Control until it ends.
- **Idle sessions.** In cld 0.9.0 and earlier a session ran until claude exited or `cld kill`
  ended it. Now `cld list`, and `cld join` without `-s`, end each session idle for longer than 30
  days, those left from before the upgrade too, among them sessions kept on purpose or driven
  through Remote Control alone: to keep them, set `CLD_IDLE_DAYS` (see
  [Idle sessions](#idle-sessions)) before the first run. The table of `cld list` gains
  `LAST ACTIVE` before `DIRECTORY`, which a script now finds fourth, after a header of five words.
- **Agent view.** cld 0.10.0 and earlier left claude's agent view on in their sessions, where
  `/bg`, "Move to background and exit" and `←` moved the conversation out of cld; now it is off in
  a session, and a claude you start without cld keeps it (see [Sessions](#sessions)). A session
  started before the upgrade keeps it until it ends: end it with `cld kill` and bring its
  conversation back with `cld join` to have it off. A conversation moved out of such a session
  comes back into cld as [Resuming a conversation](#resuming-a-conversation) says.
- **After a reboot.** `cld restore` brings back only the sessions that a cld with it started, which
  record whether they run: one started before the upgrade ends at the next reboot, and `cld join`
  brings it back, as `cld resume` did. On Linux, run `cld setup restore` once for your systemd to
  run `cld restore` as it starts (see [After a reboot](#after-a-reboot)).
- **claude's status in `cld list`.** In cld 0.10.0 and earlier, `STATE` in the table of
  `cld list` was one word. Now it has claude's status after a comma where claude has one, as in
  `detached, waiting`, and is as wide as the longest: a script that splits the table at spaces
  finds more words on such a row, and its first word with the comma.
- **`cld new` and `cld resume`.** In cld 0.10.0 and earlier, `cld new` created a session and
  refused one that ran, `cld resume` brought one back and refused one that ran, and `cld join`
  attached to one that ran and refused the others, needing `-s`. `cld join` now does all three, by
  the session's state (see [Sessions](#sessions)), and `cld new` and `cld resume` fail as unknown
  commands:

  | cld 0.10.0 and earlier | Now |
  |---|---|
  | `cld new [-n NAME] [-w] [-- ARGS...]` | `cld join [-n NAME] [-w] [-- ARGS...]` |
  | `cld new [-n NAME] -s SUFFIX [-w] [-- ARGS...]` | the same with `cld join`; where session `NAME-SUFFIX` has ended, with `--new`, which `cld join` otherwise resumes |
  | `cld resume [-n NAME] -s SUFFIX [-- ARGS...]` | `cld join [-n NAME] -s SUFFIX [-- ARGS...]`; for a session cld keeps no record of, with `--resume cld-NAME-SUFFIX` |
  | `cld resume [-n NAME] [-s SUFFIX] SESSION [-- ARGS...]` | `cld join [-n NAME] [-s SUFFIX] --resume SESSION [-- ARGS...]` |
  | `cld resume [-n NAME] [-s SUFFIX] --fork SESSION [-- ARGS...]` | `cld join [-n NAME] [-s SUFFIX] --resume SESSION --fork [-- ARGS...]` |
  | `cld join [-n NAME] -s SUFFIX [--detach-others]` | the same |

  `cld join` no longer refuses a session that has ended, which it brings back, nor one that does not
  exist, which it creates, and without `-s` it creates a new session where it asked for `-s`: a
  script that relied on those refusals checks `cld list` first. Where the session runs, it refuses
  `-w`, `--new`, `--resume` and the words after `--`, which would be lost, where `cld new` refused
  the name. `Enter` in `cld list` brings back a session that has ended since the list read it, where
  it said so.
- **tmux.** End the sessions started before the upgrade (`cld list`, then `cld kill`): each
  session's server keeps running the tmux that started it until the session ends.
- **Ended sessions.** cld 0.9.0 and earlier kept no record of the sessions: a session they started
  leaves `cld list` as it ends, and `cld join -n NAME -s SUFFIX --resume cld-NAME-SUFFIX` resumes it
  by name, in the current directory, as `cld resume -n NAME -s SUFFIX` did. The sessions started
  since stay as `ended`.
- **To cld 0.4.0 or later.** cld 0.3.0 and earlier were a bash script, downloaded from
  `releases/latest/download/cld`, which now fails; an installed script keeps working until you
  install cld [as the README says](../README.md#install). Those versions ran every session on one
  server, `tmux -L cld`, where later ones do not look: end those sessions before upgrading, or
  afterwards find them with `tmux -L cld ls` and end them with
  `tmux -L cld kill-session -t =cld-NAME`, or all of them with `tmux -L cld kill-server`. Names
  with non-ASCII letters or digits, such as `café`, which 0.3.0 took under a UTF-8 locale, are now
  refused.
- **Staying on tmux 3.3 or 3.4.** cld 0.3.0 is the last release that runs on them:

  ```sh
  curl -fsSL https://github.com/zadykian/cld/releases/download/v0.3.0/cld -o ~/.local/bin/cld && chmod +x ~/.local/bin/cld
  ```

- **From before 0.2.0.** `cld [NAME]` attached to the session, creating it if needed; it now fails
  as an unknown command. `cld join` does the same for a session `NAME-SUFFIX` (see **Session names**
  above).
