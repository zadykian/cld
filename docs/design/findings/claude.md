# Findings: claude

What Claude Code does that cld relies on or works around, probed on Linux. "Read" means read in
claude's JavaScript bundle, not run; "run" means the real claude ran. What tmux does around claude
is in [tmux-sessions.md](tmux-sessions.md) and [tmux-terminal.md](tmux-terminal.md).

## Versions and the command line

- **What the oldest claude rests on** (#21). Read in the changelog and in the npm bundles of
  2.1.118, 2.1.119, 2.1.133, 2.1.221 and 2.1.222. The changelog has `--worktree` from 2.1.49,
  `--name` from 2.1.76 and `worktree.baseRef` from 2.1.133. In 2.1.119 `remoteControlAtStartup`
  moved into the settings, within reach of `--settings`, and from 2.1.222 a project's `false` beats
  a flag's `true`. On 25 September 2026 npm's `latest` tag was at 2.1.282. The `stable` channels of
  npm, Homebrew, apt, dnf and apk served 2.1.274 ([decision 6](../decisions/0006-versions.md)).
- **`claude --version`** (2.1.282, native installer, run). It prints `2.1.282 (Claude Code)` and
  exits 0 in about 20 ms, leaving nothing that holds its output. In a removed directory it exits 1.
- **`claude --help`** (2.1.284, run with a scratch `HOME`). The short options are `-c`, `-d`, `-h`,
  `-n`, `-p`, `-r`, `-v` and `-w`. `-p`, `--bg`, `--tmux` (with `--worktree`), `--teleport` and the
  hidden `--init-only` and `--rewind-files` take claude out of the session, and `--from-pr` resumes
  ([decision 41](../decisions/0041-claude-options.md)).
- **How claude reads its command line** (2.1.284, read). commander reads `-xyz` as `-x yz` where
  `-x` takes a value, else as `-x -yz`, and the last of a repeated option wins. Before it, claude
  looks through every word, past `--` too, for `--bg` and for `--tmux` beside `-w`, and leaves the
  session on them ([decision 41](../decisions/0041-claude-options.md)).
- **A command after cld's options** (`claude --name cld-x --settings '{}' mcp --help`, 2.1.284,
  run). The first word that is no option's value names a command, which claude runs instead of a
  conversation.
- **`claude --help` on resuming** (2.1.282, run). `--resume [value]` takes a session ID or opens a
  picker, and the help says nothing of names. As the value is optional, a word starting with `-`
  after it counts as the next option.
- **`claude --resume ID PROMPT`** (2.1.232 from npm and 2.1.285, read). The word after the ID is the
  prompt, which claude runs as a turn after the resumed messages
  ([decision 48](../decisions/0048-restore-after-reboot.md)).

## Exiting

- **How claude exits** (2.1.281, run). Status 0 for `/exit` and for `Ctrl+C` or `Ctrl+D` twice, so
  `remain-on-exit failed` keeps only a claude that failed
  ([decision 5](../decisions/0005-failures-stay-on-screen.md)).
- **SIGHUP** (2.1.283 and 2.1.284, read). claude shuts down as for SIGTERM but exits 129, running
  its cleanups, then its `SessionEnd` hooks with the reason `other`. A failsafe forces the exit
  6.5 s after the signal, or up to 71 s where writes, an OAuth refresh and a 60 s hook `timeout`
  delay it. A background conversation ignores SIGHUP unless it owns its terminal
  ([decision 32](../decisions/0032-what-a-kill-does.md)).
- **`SessionEnd`'s reason** (2.1.285, read). `prompt_input_exit` for `/exit` and the other ways out
  of the prompt, the move to the background among them, and `other` for a signal or a kill
  ([decision 48](../decisions/0048-restore-after-reboot.md)).
- **How long claude waits for `SessionEnd`'s hooks** (2.1.284, read; the changelog, read
  2026-09-30). claude gives a `SessionEnd` hook without a `timeout` of its own that of
  `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS`, 1.5 s where unset. It ends the event's hooks together
  at the variable's or the longest `timeout` among them, 1.5 to 60 s, on exit, `/clear` and
  `/resume`. The limit was 1.5 s whatever the `timeout` until 2.1.74, and for hooks without one
  until 2.1.268 ([decision 40](../decisions/0040-session-record.md)).

## Worktrees

- **`--worktree NAME` on a branch ahead of the remote's default** (2.1.281, run). It branches
  `worktree-NAME` from `origin/master`. With `"worktree": {"baseRef": "head"}` in `--settings` it
  branches from the current commit, even where the project's settings say `"fresh"`
  ([decision 4](../decisions/0004-worktrees.md)).
- **`--worktree` in a repository without a remote** (2.1.281, run). claude makes
  `.claude/worktrees/NAME` from `HEAD` and moves there, and `#{pane_current_path}` follows. The
  worktree outlives the session, uncommitted files and claude's lock included, and the same
  `--worktree` reopens it.
- **`--worktree` where workspace trust was never accepted** (2.1.281, run). claude prints
  `Error creating worktree: Workspace trust not yet accepted.` and exits 1, which
  `remain-on-exit failed` (tmux 3.5 or newer) keeps on screen
  ([decision 5](../decisions/0005-failures-stay-on-screen.md)).
- **Where claude sets its directory** (2.1.283, read). claude moves, firing `CwdChanged`, for
  `--worktree`, `EnterWorktree`, `ExitWorktree` and a resumed conversation that recorded a worktree,
  and runs hooks there unless a launch sets a project root. A `WorktreeCreate` hook replaces
  claude's own making of a worktree, so it gives no event to listen to
  ([decision 26](../decisions/0026-title-marks-worktree.md)).
- **What `SessionStart` and `CwdChanged` see** (2.1.283, run in a trusted linked worktree).
  `SessionStart` ran in claude's directory, and `!cd /tmp` fired `CwdChanged` with `/tmp`. claude
  then reset its shell to the worktree with no event, and `#{pane_current_path}` stayed there
  ([decision 26](../decisions/0026-title-marks-worktree.md)).

## Resuming

- **How claude resumes** (2.1.282, read). `--resume ID` without such a conversation exits 1, and one
  running as a background session is refused unless `--fork-session` is given. Where another process
  holds the conversation's Remote Control session, claude leaves Remote Control off
  ([decision 42](../decisions/0042-remote-control-is-claudes.md)).
- **How a resumed conversation is named and found** (#75: 2.1.283 and 2.1.284, read; the changelog).
  `--name` sets the title before the resume, which keeps it. A `--resume VALUE` that is no ID looks
  among the conversations of the directory's git worktrees. It compares each one's custom title,
  else its AI title, lower-cased and trimmed, with VALUE's, and resumes the one that matches. None
  or several open the picker, which matches any part, so `cld-rev` finds `cld-review`. A name a
  running claude has already gets a variant, `NAME-WORD-WORD`, as claude starts. Any other is taken
  unchanged, and `/clear` keeps it ([decision 45](../decisions/0045-resuming-a-copy.md)).
- **`--fork-session`** (the same, in the changelog since 2.0.73). It resumes under a new ID, leaving
  the original untouched, without its worktree or Remote Control session. It also skips the refusal
  of a background conversation ([decision 45](../decisions/0045-resuming-a-copy.md)).
- **What a resumed conversation keeps** (the [sessions
  docs](https://code.claude.com/docs/en/sessions), read 29 September 2026). `--mcp-config`,
  `--settings`, `--plugin-dir`, `--fallback-model` and `--add-dir` must be passed again, and the
  model comes back unless one is picked ([decision 16](../decisions/0016-resume.md)).
- **How long claude keeps a conversation** (2.1.232 from npm, 2.1.283 and 2.1.284, read). Its
  cleanup removes transcripts unmodified for `cleanupPeriodDays`, 30 by default and at least 1
  ([decision 40](../decisions/0040-session-record.md),
  [decision 46](../decisions/0046-idle-sessions.md)).

## Settings

- **`remoteControlAtStartup`** (2.1.282, read: a live check would connect to claude.ai). The first
  of the policy, flag and user settings that has it wins, but a project's `false` beats them all and
  its `true` is ignored ([decision 10](../decisions/0010-remote-control.md),
  [decision 42](../decisions/0042-remote-control-is-claudes.md)).
- **Remote Control where no setting names it** (2.1.284, read; the [Remote Control
  docs](https://code.claude.com/docs/en/remote-control), read 29 September 2026). claude takes the
  organisation's default, else a feature flag of Anthropic's, off where unset. The docs: while
  connected, the transcript is stored on Anthropic's servers, and it needs a claude.ai subscription
  and the Anthropic API ([decision 42](../decisions/0042-remote-control-is-claudes.md)).
- **The defaults of the settings cld wrote before decision 28** (2.1.284, read). Unset,
  `autoMemoryEnabled` and `autoCompactEnabled` are `true`, `theme` `"dark"` and `autoUpdatesChannel`
  `"latest"`, the values cld wrote. A project's settings override the user's
  ([decision 28](../decisions/0028-shared-project-settings.md)).
- **Read-only commands and permission rules** (2.1.284, read; git 2.53.0, run). claude runs bare
  read-only commands unasked, checking the options of `git log` and the like against safe lists.
  `Bash(git log *)` or `Bash(git log:*)` is a prefix rule: it matches `git log` followed by
  anything, with no flag-level analysis. So it admits `--output=FILE`, which claude's own
  safe-option lists refuse: a `git log` writing `.git/config`, then `git status`, ran a command
  ([decision 28](../decisions/0028-shared-project-settings.md)).
- **What claude keeps out of git** (2.1.284, read). It adds its runtime files, `worktrees/` among
  them, to `.git/info/exclude`, but nothing covers plans, kept in `~/.claude/plans/` by default. The
  bundle marks `.claude/settings.json` "Commit" and `settings.local.json` "Gitignore"
  ([decision 28](../decisions/0028-shared-project-settings.md)).
- **MCP servers from `.mcp.json`** (2.1.283 with a scratch `CLAUDE_CONFIG_DIR`, GoLand and Rider
  2026.2 serving). `claude mcp list` shows them `Pending approval` until the folder is trusted and
  `enabledMcpjsonServers` names them, then `Connected`
  ([decision 19](../decisions/0019-project-settings.md)).
- **`${VAR:-DEFAULT}` in a server's URL** (the same). claude expands it from its environment or the
  user's `settings.json`, not the project's `settings.local.json`. `${VAR}` unset fails the server
  with `Missing environment variables: VAR` ([decision 19](../decisions/0019-project-settings.md)).
- **When claude reads its telemetry settings** (2.1.282 with GoLand 2026.2.3 and the JetBrains
  OpenTelemetry plugin 2.1.5, run). At startup only, from its environment
  ([decision 18](../decisions/0018-telemetry.md)).
- **The user settings `setup config user` writes** (2.1.232 from npm and 2.1.289, read). Both
  bundles' settings schemas have `model`, `effortLevel` (`low` to `xhigh`), `theme` (`dark` among
  others), `editorMode` (`normal` or `vim`), `autoCompactEnabled` and `autoUpdatesChannel`
  (`latest`, `stable` or `rc`). `preferredNotifChannel` takes `auto`, `iterm2`, `terminal_bell`,
  `iterm2_with_bell`, `kitty`, `ghostty` or `notifications_disabled`. A value the schema does not
  know for `effortLevel`, `theme`, `editorMode` or `preferredNotifChannel` reads as unset, with no
  error ([decision 53](../decisions/0053-project-and-user-settings.md)).
- **The name of their scope** (the same, run). The `--help` of `claude mcp add` and
  `claude plugin install` names it `user`, beside `project` and `local`, in `--scope`
  ([decision 53.1](../decisions/0053-project-and-user-settings.md)).

## Hooks

- **The hook events** (2.1.232 and 2.1.283, read). Both have `UserPromptSubmit`, `PostToolUse`,
  `PostToolUseFailure` with `is_interrupt`, `PermissionRequest`, `Elicitation`, `ElicitationResult`,
  `Notification` with `idle_prompt`, `Stop` and `StopFailure`. No event comes when the user
  interrupts claude as it writes ([decision 25](../decisions/0025-title-follows-status.md)).
- **The events around a permission** (2.1.285, read). `PermissionRequest` comes as the dialog shows,
  within the tool's call. None of the 33 events comes with the user's answer: an allowed tool runs
  on to its `PostToolUse` ([decision 49](../decisions/0049-status-in-the-list.md)).
- **The input of `SessionStart` and `SessionEnd`** (2.1.232 from npm, 2.1.283 and 2.1.284, read).
  Every hook gets `session_id`, the ID `--resume` takes, and `cwd`; `SessionStart` adds `source`,
  `SessionEnd` `reason`. From 2.1.283 a hook run for another process's call gets `served:` and that
  caller's ID ([decision 40](../decisions/0040-session-record.md)).
- **Hooks given with `--settings`** (2.1.283, run alone on a scratch tmux server). claude ran
  `SessionStart` and `UserPromptSubmit` with its own `TMUX` and `TMUX_PANE`, and their `tmux` set
  the option on claude's session. No `Stop` followed a prompt that a hook blocked, so the status
  stayed `busy` ([decision 25](../decisions/0025-title-follows-status.md)).
- **Hooks of a background conversation** (2.1.284 under cld 0.8.1 and tmux 3.7c, seen in `ps` and
  the daemon's log). A daemon started earlier from the default tmux server ran the conversation with
  the pane's `--settings`, but without `TMUX` and `TMUX_PANE`. Every hook's `tmux` went to the
  default server and failed, 141 times in two hours, where one naming the socket and `=cld-NAME:`
  works anywhere ([decision 25](../decisions/0025-title-follows-status.md)). Run by hand, the hooks'
  command says no server runs once the session's server is gone. Once a new server on that socket
  has session `cld-NAME`, the command sets the option there (tmux 3.7c).
- **`async` and `timeout` of a command hook** (2.1.284, read; the hooks reference and changelog,
  read 2026-09-29). With `async: true` claude goes on at once, with no timeout. A hook claude waits
  for it kills at `timeout`, 600 s by default and 30 s for `UserPromptSubmit`. Both predate 2.1.232,
  the oldest claude cld runs ([decision 39](../decisions/0039-hook-cost.md)).
- **The order a background hook lands in** (2.1.284, read). The hooks cld writes ran on Ubuntu's
  tmux 3.7c snap under load and on 3.7c built from source. With no model call between one answer's
  tools, a `PermissionRequest` can follow a `PostToolUse` at once. A `busy` sent to the background
  landed after a later `waiting` in 10 of 20 runs at a 0 ms gap on the snap, and in 1 at 50 ms.
  Built from source it did so in 4 of 40 at 0 ms, and in none from 5 ms
  ([decision 39](../decisions/0039-hook-cost.md)).

## Agent view and background sessions

- **How claude moves a conversation to the background** (#70; 2.1.283 and 2.1.284, read; seen in
  cld's sessions on tmux 3.7c). With agent view on, `/bg`, the `/exit` dialog's "Move to background
  and exit" and `←` on an empty prompt hand it to a daemon, as a copy with a new ID. `/bg` and the
  dialog exit 0, ending cld's session, and after `←` `cld kill` leaves the copy running. `--resume`
  by the name then finds the copy and refuses it unless `--fork-session` is given
  ([decision 47](../decisions/0047-agent-view-off.md)).
- **Background sessions beside cld's** (2.1.283 and 2.1.284, read; the [agent
  view](https://code.claude.com/docs/en/agent-view) and
  [fullscreen](https://code.claude.com/docs/en/fullscreen) docs, read 2026-09-29). The docs: a
  supervisor runs them without a terminal, stops one unattached for about an hour unless pinned, and
  renders an attached one fullscreen. `attach`, `logs` and `stop` take only a short ID, never a
  name, and `claude agents --json` listed cld's sessions as `"kind": "interactive"`
  ([decision 47](../decisions/0047-agent-view-off.md)).
- **Agent view turned off** (#111: 2.1.285's commands, run by the maintainer). Under
  `CLAUDE_CODE_DISABLE_AGENT_VIEW=1` each of `agents`, `attach`, `logs`, `stop`, `rm` and `respawn`
  exited 1. `"disableAgentView": true` in `--settings` or any settings file did the same, and a
  `false` in `--settings` beat a `true` in the files
  ([decision 47](../decisions/0047-agent-view-off.md)).
- **`disableAgentView` in claude's code** (2.1.232 from npm and 2.1.285, read; the docs, read
  2026-09-30). In 2.1.285 the setting removes `/background`, `/stop`, the background `/fork`, the
  `/exit` dialog's move, `←` and the daemon. The refusal to resume a background conversation does
  not ask it ([decision 16.10](../decisions/0016-resume.md),
  [decision 47](../decisions/0047-agent-view-off.md)).
- **Ctrl+X in agent view** (the docs, read 2026-09-25; 2.1.282's hints, read). `Ctrl+X` stops a
  session, and a second press within two seconds deletes it with its worktree, uncommitted changes
  included. The hints read `ctrl+x to stop` and `ctrl+x again to delete · esc to keep`
  ([decision 15](../decisions/0015-killing-from-the-list.md)).
- **Hint strings** (2.1.282, read). Hints are lower case but for Enter and Esc in some, dim, and
  joined by ` · `, as in `↑/↓ to navigate · Esc to cancel`
  ([decision 14](../decisions/0014-the-session-list.md)).

## In a terminal

claude's OSC 8 links under tmux and its reading of the locale are in
[tmux-terminal.md](tmux-terminal.md), beside what tmux makes of them.

- **What claude asks of the terminal** (the real claude under tmux, its first 12 s). It turns on
  bracketed paste, colour-scheme and focus reports, the alternate screen and all-motion mouse
  reporting. It queries XTVERSION, the kitty keyboard protocol, DA1 and DECRQM `?2026`, and sets the
  title `✳ NAME`.
- **Which terminal claude takes itself to be in** (2.1.282, 2.1.283 and 2.1.284, read). IDE markers
  come first: `CURSOR_TRACE_ID`, `VSCODE_GIT_ASKPASS_MAIN`, `__CFBundleIdentifier`,
  `VisualStudioVersion` and `TERMINAL_EMULATOR`; then `TERM` and `TERM_PROGRAM`, `tmux` in a pane.
  Extended keys are on from the start for iTerm2, kitty, WezTerm, Ghostty, tmux, Windows Terminal
  and Warp, and elsewhere only if the terminal answers. claude's own background sessions drop these
  variables but `VisualStudioVersion`, and keep `GIT_EDITOR`
  ([decision 33](../decisions/0033-terminal-variables.md)).
- **claude's title** (2.1.283, read; a session's `#{pane_title}` sampled for 45 s). `◐` and `◑` in
  turn while busy and `✳` otherwise, but under `TMUX`, `STY` or `ZELLIJ` a flag, on by default,
  keeps `✳`. claude sends no OSC 9;4 progress
  ([decision 25](../decisions/0025-title-follows-status.md)).
- **How claude notifies** (2.1.283 and 2.1.284, read; the settings reference, read 2026-09-29).
  `preferredNotifChannel` is `auto` by default, which serves iTerm2, kitty, Ghostty and Apple
  Terminal's bell, and sends nothing under tmux. A channel set by hand goes in tmux passthrough
  where `TMUX` is set, but `terminal_bell` is a bare BEL
  ([decision 29](../decisions/0029-notifications.md)).
- **How claude opens a clicked link** (2.1.284, read). A click that selects nothing opens the OSC 8
  link under it 500 ms after release, where Ctrl or Alt is held, or in Ghostty or Warp. Under tmux
  only Ctrl or Alt works, as `TERM_PROGRAM` and XTVERSION are tmux's
  ([decision 35](../decisions/0035-modifier-clicks.md)).
- **A new line without Shift+Enter** (2.1.284, read). `ctrl+j` makes one, and the hint reads
  `\⏎ for newline` where Shift+Enter does not work.
- **The renderers and scrollback** (2.1.284, read; the
  [fullscreen](https://code.claude.com/docs/en/fullscreen) and
  [accessibility](https://code.claude.com/docs/en/accessibility) docs, read 2026-09-29). The classic
  renderer keeps the conversation in the terminal's scrollback, and the fullscreen one scrolls and
  searches its own in the alternate screen. Screen-reader mode is always classic, and background
  sessions are always fullscreen ([decision 36](../decisions/0036-scrollback.md)).

## Inside a session

- **`TMUX` in the Bash tool** (#79: 2.1.284, run). The tool inherits the session's `TMUX`, so a
  `cld` that claude runs sees the session's own server
  ([decision 46](../decisions/0046-idle-sessions.md)).
- **How claude runs a shell command** (2.1.284, read). `!` and the Bash tool run
  `eval 'COMMAND' < /dev/null`, so `tty` fails, with `TMUX` and `TMUX_PANE` kept
  ([decision 44](../decisions/0044-detach-command.md),
  [decision 51](../decisions/0051-moving-between-sessions.md)).
- **Agent teams in tmux** (2.1.284, read). Where `teammateMode` picks tmux, the first teammate
  splits claude's own pane, so claude's window can hold more panes than claude's
  ([decision 5](../decisions/0005-failures-stay-on-screen.md)).
- **claude's memory** (#79: 2.1.284, `ps -o rss` on the maintainer's host). Each of a conversation's
  four processes held 225 to 540 MB, where a session's tmux server holds 4 to 5 MB
  ([decision 46](../decisions/0046-idle-sessions.md)).
