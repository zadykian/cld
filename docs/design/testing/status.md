# Testing: status

What the tests leave to a run by hand, a reading of the source, or a later check.

## Where the tests run

The contracts run on tmux 3.7c and 3.5a in Docker, and the baseline on Homebrew's tmux on macOS.
3.6a passed the whole suite once (#74); 3.6 is not in CI. What the real claude makes of 3.5's
Shift+A and Shift+Backspace was not checked: the floor at 3.5a keeps them from it.

## Checked with the probe or by hand

Each entry says what is left.

- Stale sockets ([38](../decisions/0038-stale-sockets.md)): probed on Linux with tmux 3.5a and 3.7c;
  on macOS run in CI only.
- The command limit ([41](../decisions/0041-claude-options.md)): probed on 3.5a and 3.7c, read in
  the 3.4 and 3.7c sources; on macOS it rests on CI.
- Scrollback ([36](../decisions/0036-scrollback.md)): checked on 3.5, 3.6a and 3.7c with programs
  that print lines, not with claude's classic renderer or a screen reader.
- Links ([30](../decisions/0030-links.md)): probed with `TERM` `wezterm` and `alacritty`. No real
  terminal was seen showing one, nor which click opens it.
- A failed pane ([5](../decisions/0005-failures-stay-on-screen.md)): tested beside a pane split by
  hand; a teammate's pane was read in claude 2.1.284.
- `install.sh` ([20](../decisions/0020-install-script.md)) and `cld update`
  ([21](../decisions/0021-self-update.md)): run by hand on Linux, against v0.4.0 and from 0.4.0 to
  0.5.0. Not an update on macOS, nor Rosetta 2's `sysctl.proc_translated`.
- `setup completion` ([22](../decisions/0022-setting-completion-up.md)): bash 5.2.37 with
  bash-completion 2.16, zsh 5.9 and fish 4.0.2 in the image, and zsh on macOS. Not Homebrew's
  `bash-completion@2`, nor `compinit -i` under group-writable directories.
- `setup config project` ([19](../decisions/0019-project-settings.md),
  [28](../decisions/0028-shared-project-settings.md)): checked with git 2.53.0 and
  `claude mcp list` 2.1.283, not in a claude session.
- Names and homes ([24](../decisions/0024-names-from-the-repository.md),
  [37](../decisions/0037-sessions-of-another-repository.md)): git 2.47.3 and macOS's. Not a home in
  other letters where case is ignored, nor `-w`'s worktree with the real claude.
- Detach ([44](../decisions/0044-detach-command.md)) and moving
  ([51](../decisions/0051-moving-between-sessions.md)): run as claude runs a shell command, not by
  its `!`. Moving ran under dash, bash, zsh and fish, not the other shells cld takes, nor a popup
  in a real terminal. Nor were IDEs seen taking `Ctrl+Q` (#72).
- Inside another tmux ([43](../decisions/0043-inside-your-own-tmux.md)): probed with 3.7c on both
  sides, and 3.5a and 3.6a for the message. Not another release outside, nor a real terminal.
- `join` ([50](../decisions/0050-one-command-join.md)): the start mark's races ran without the mark
  on 3.7c only. For the maintainer: an ended row's footer, and `-w` with `--new` over an ended
  session.
- Idle sessions ([46](../decisions/0046-idle-sessions.md)): limits of seconds, not days.
- Restore ([48](../decisions/0048-restore-after-reboot.md)): a transient unit stood for the user's
  systemd. Not a real reboot, lingering off, or a resumed claude asking a permission. launchd is not
  looked into.
- The list's status ([49](../decisions/0049-status-in-the-list.md)): format probed on 3.5a and 3.7c;
  bold `waiting` seen only in tmux's rendering.
- `restore tabs` ([59](../decisions/0059-ghostty-tabs.md)): run against fakes of osascript, ps,
  ssh, open and ghostty, and its scripts' own logic against a mock Ghostty, on the macos job only.
  Ghostty's real answers come from #177's spike, on 1.4.0 and macOS 27.0.1
  ([findings](../findings/terminals.md#the-mac-spikes-results)). It skipped these checks, or parts
  of them:
  1. Check 3, 1.4's terms against 1.3.1's `Ghostty.sdef`, and check 5, no relaunch.
  2. Check 7: ids through undo and drag; minimized, hidden, other-Space and fullscreen windows; the
     quick terminal. Check 16: Cmd+Q in the middle of a read, and the exec of `cld update`.
  3. Check 4: `macos-applescript = false`, and a denial from another app. Part of check 1: the
     Automation list in System Settings, and `tccutil reset`.
  4. Check 8: restoration under `default`, after kill -9 and after a reboot.
  5. Check 10: non-default padding, `window-padding-balance` and a display at scale 1. Check 11: a
     slow `.zshrc` and a Dvorak layout.
  6. Checks 9 and 6: an `undo-timeout` of 20 s, and the cost at 15 and 30 minutes.
  7. Check 14, the line on ws and cld on the host, and check 15, the pace under load.
- `--host` ([62](../decisions/0062-commands-on-another-host.md)): run against the fake ssh, whose
  host is the sandbox itself, with bash as the login shell. Not a real sshd, ssh's own `-t`,
  `LogLevel` and `RemoteCommand=none`, nor a login shell other than bash.

## Read rather than run

These were read in claude's bundle or docs. A run that would reach the API or claude.ai is the
maintainer's to run or allow.

- Title hooks ([25](../decisions/0025-title-follows-status.md),
  [26](../decisions/0026-title-marks-worktree.md)): claude 2.1.283 ran them up to a prompt a hook
  blocked, and the worktree's in a worktree and on `cd`. Not a real turn, nor a background worker,
  nor the title and the empty OSC 7 tmux sends it in iTerm2.
- Hook costs ([39](../decisions/0039-hook-cost.md)) and notifications
  ([29](../decisions/0029-notifications.md)): read in 2.1.284. No real notification was seen. Still
  to see: a `CwdChanged` hook in the background, a hook that reaches its timeout, and a
  `PermissionRequest` right after another tool's `PostToolUse`.
- Kill ([32](../decisions/0032-what-a-kill-does.md)): probed with a script in claude's place. Not a
  real claude's `SessionEnd` hooks.
- Terminal variables ([33](../decisions/0033-terminal-variables.md)): read in 2.1.282 to 2.1.284 and
  VS Code's source. claude was not run in Cursor, VS Code, a JetBrains IDE or on macOS.
- Clicks ([35](../decisions/0035-modifier-clicks.md)): the probe gets press and release; claude's
  handling was read in 2.1.284. Selecting with Shift held, Option in iTerm2 and Fn in Terminal.app,
  was not tried.
- A locale without UTF-8: claude 2.1.284's bundle draws in UTF-8 regardless. Only a stub claude
  ran under such a locale.
- The record ([40](../decisions/0040-session-record.md)): hook input read in 2.1.232 to 2.1.284. Not
  `/resume`, `/rename` or background conversations keeping the ID, nor macOS's file system.
- Following `/cd` ([56](../decisions/0056-following-claudes-cd.md)): read in 2.1.299, the probe's
  `cd` standing in, and a probe for a teammate. A real `/cd` in a session, then `kill` and `join`,
  has not run, nor a real teammate's hooks.
- Words after `--` ([41](../decisions/0041-claude-options.md)): `--help` 2.1.284 was run, the rest
  read.
- User settings ([53](../decisions/0053-project-and-user-settings.md)): the keys and values
  `setup config user` writes, read in 2.1.232 and 2.1.289. No claude ran on the file it writes.
- Agent view ([47](../decisions/0047-agent-view-off.md)): the maintainer saw 2.1.285's
  `claude agents --json` refused under the key. Not `/bg`, `←`, `/exit`'s dialog or `/fork` in a
  session, `ListAgents` and `SendMessage` between cld's sessions, and whatever else of claude needs
  the daemon.
- Remote Control ([42](../decisions/0042-remote-control-is-claudes.md)): read in 2.1.284 and the
  docs. Not known: whether a session that turned Remote Control on at startup records that session
  in its conversation.
- Resuming ([16](../decisions/0016-resume.md), [45](../decisions/0045-resuming-a-copy.md)): the
  probes of #26 and `--fork-session` (#75) never ran. Among them: `claude stop ID` then `join` of a
  moved conversation, `Esc` in agent view after `←`, and `--fork-session` on a conversation open
  elsewhere or in the background. Until they run, the guide and those decisions go by claude's docs,
  `--help` and bundle.
