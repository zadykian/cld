# cld: design

The research behind cld and the reasons for what it does. What cld offers its users is in the
[README](../README.md) and the [user guide](guide.md).

- [Overview](design/overview.md): where cld started, the server's options, the packages, cld's
  record of its sessions, a session's life and how releases are built.
- [Testing](design/testing.md): the test layers, the terminal contract C1-C10, the drivers, the
  harness, CI, what the tests found, and what is not checked yet.

## Findings

Behaviour probed by hand, each finding with the versions checked. A decision links the
findings it rests on, and a finding links back.

- [tmux: servers and sessions](design/findings/tmux-sessions.md): releases, running a program,
  command parsing, targets, clients, options and formats, a pane's environment, dead panes,
  history, sockets, servers ending and what they cost.
- [tmux: the terminal's side](design/findings/tmux-terminal.md): a client starting, the locale, an
  outer tmux, keys, focus, the mouse, notifications, links, titles, messages, popups and
  `detach-client`.
- [claude](design/findings/claude.md): its versions and command line, exiting, worktrees, resuming,
  settings, hooks, agent view and background sessions, and what it does in a terminal.
- [Terminals](design/findings/terminals.md): JediTerm, VS Code, iTerm2 and the other terminals.
- [Environment](design/findings/environment.md): shells, completion, cobra, `install.sh` and
  GitHub, git, Go and Linux, telemetry, JetBrains IDEs and systemd.

## Decisions

A record per decision, numbered in the order taken. Each gives its Context, the Decision in parts
N.M, and the Consequences. A record's Status names the later decisions that amend or supersede it.
The `adr` skill in `.claude/skills` adds a record.

- [1. Naming](design/decisions/0001-naming.md): Accepted. Amended by 13 and 24.
- [2. Inside another tmux](design/decisions/0002-inside-another-tmux.md): Accepted. Amended by 13,
  34, 43, 44, 50 and 51.
- [3. Commands](design/decisions/0003-commands.md): Accepted (0.2.0). Amended by 13, 17, 18, 24 and
  40. Its `new` and `join` superseded by 50.
- [4. Worktrees](design/decisions/0004-worktrees.md): Accepted. Amended by 24, 42 and 50.
- [5. Failures stay on screen](design/decisions/0005-failures-stay-on-screen.md): Accepted. Amended
  by #64, #78, 24, 44, 48 and 50.
- [6. Versions](design/decisions/0006-versions.md): Accepted (#21, #74). Amended by 13, 42, 43, 48,
  50 and 51.
- [7. JediTerm pin](design/decisions/0007-jediterm-pin.md): Accepted.
- [8. iTerm2](design/decisions/0008-iterm2.md): Accepted.
- [9. Only cld's own sessions](design/decisions/0009-only-clds-own-sessions.md): Accepted (0.2.1).
  Superseded by 13.
- [10. Remote Control](design/decisions/0010-remote-control.md): Accepted. Superseded by 42.
- [11. Go and cobra](design/decisions/0011-go-and-cobra.md): Accepted (#20). Amended by 12, 13 and
  33.
- [12. Help from cobra](design/decisions/0012-help-from-cobra.md): Accepted (#28). Amended by 17,
  18, 22, 24, 42, 50, 52 and 53.
- [13. A server per session](design/decisions/0013-a-server-per-session.md): Accepted (#22).
  Replaces 9. Amended by #69, 15, 24, 34, 38, 40 and 51.
- [14. The session list](design/decisions/0014-the-session-list.md): Accepted (#23). Amended by 15,
  23, 40, 49, 50 and 51.
- [15. Killing from the session list](design/decisions/0015-killing-from-the-list.md): Accepted
  (#24). Amended by 23, 40 and 51.
- [16. Resume](design/decisions/0016-resume.md): Accepted (#26). Amended by 24, 40, 41, 45 and 47.
  Superseded by 50.
- [17. Shell completion](design/decisions/0017-shell-completion.md): Accepted (#25). Amended by 18,
  19, 22, 24, 27, 28, 37, 40, 44, 49, 50, 52 and 53.
- [18. Telemetry](design/decisions/0018-telemetry.md): Accepted (#30). Amended by 19, 22 and 48.
  Superseded by 52.
- [19. Project settings](design/decisions/0019-project-settings.md): Accepted. Amended by 28, 52
  and 53.
- [20. An install script](design/decisions/0020-install-script.md): Accepted.
- [21. Self-update](design/decisions/0021-self-update.md): Accepted. Amended by 22.
- [22. Setting completion up](design/decisions/0022-setting-completion-up.md): Accepted. Amended by
  27 and 52.
- [23. Joining beside other terminals](design/decisions/0023-joining-beside-other-terminals.md):
  Accepted. Amended by 50.
- [24. Names from the repository](design/decisions/0024-names-from-the-repository.md): Accepted.
  Amended by 37, 38, 40, 49 and 50.
- [25. The tab's title follows claude's status](design/decisions/0025-title-follows-status.md):
  Accepted. Amended by 26, 39 and 49.
- [26. The tab marks a worktree](design/decisions/0026-title-marks-worktree.md): Accepted. Amended
  by 39.
- [27. Completion in bash with ble.sh](design/decisions/0027-completion-with-blesh.md): Accepted.
- [28. Project settings a team can share](design/decisions/0028-shared-project-settings.md):
  Accepted. Amended by 53.
- [29. Notifications](design/decisions/0029-notifications.md): Accepted (#59). Amended by 53.
- [30. Links](design/decisions/0030-links.md): Accepted (#60).
- [31. A terminal to attach from](design/decisions/0031-a-terminal-to-attach-from.md): Accepted
  (#63). Amended by 41, 48, 50 and 51.
- [32. What a kill does to claude](design/decisions/0032-what-a-kill-does.md): Accepted (#66).
- [33. The terminal's variables](design/decisions/0033-terminal-variables.md): Accepted (#67).
- [34. cld's servers are marked](design/decisions/0034-servers-are-marked.md): Accepted (#71).
- [35. Clicks with a modifier reach claude](design/decisions/0035-modifier-clicks.md): Accepted
  (#73).
- [36. Scrollback](design/decisions/0036-scrollback.md): Accepted (#77).
- [37. Sessions of another repository](design/decisions/0037-sessions-of-another-repository.md):
  Accepted (#80). Amended by 44 and 50.
- [38. Stale sockets cost no tmux](design/decisions/0038-stale-sockets.md): Accepted (#65). Amended
  by 46.
- [39. What the title's hooks cost](design/decisions/0039-hook-cost.md): Accepted (#81).
- [40. A record of the sessions](design/decisions/0040-session-record.md): Accepted (#57). Amended
  by 44, 48 and 50.
- [41. claude's options](design/decisions/0041-claude-options.md): Accepted (#61). Amended by 42,
  45, 47, 48, 50 and 51.
- [42. Remote Control is claude's own setting](design/decisions/0042-remote-control-is-claudes.md):
  Accepted (#62). Amended by 47.
- [43. Inside your own tmux](design/decisions/0043-inside-your-own-tmux.md): Accepted (#68). Amended
  by 50 and 51.
- [44. Detaching without the key](design/decisions/0044-detach-command.md): Accepted (#72). Amended
  by 50 and 51.
- [45. Resuming a copy](design/decisions/0045-resuming-a-copy.md): Accepted (#75). Amended by 50.
- [46. Idle sessions end](design/decisions/0046-idle-sessions.md): Accepted (#79). Amended by 48, 50
  and 51.
- [47. Agent view is off in cld's sessions](design/decisions/0047-agent-view-off.md): Accepted
  (#111). Amended by 50.
- [48. Sessions come back after a reboot](design/decisions/0048-restore-after-reboot.md): Accepted
  (#115). Amended by 50 and 52.
- [49. claude's status in the list](design/decisions/0049-status-in-the-list.md): Accepted (#113).
  Amended by 50.
- [50. One command for a session](design/decisions/0050-one-command-join.md): Accepted (#114).
  Replaces `new` and `join` of 3 and `resume` of 16. Amended by 51.
- [51. Moving between sessions](design/decisions/0051-moving-between-sessions.md): Accepted (#112).
- [52. Telemetry outside cld](design/decisions/0052-telemetry-outside-cld.md): Accepted (#158).
  Replaces 18. Amended by 53.
- [53. A project's settings and the user's](design/decisions/0053-project-and-user-settings.md):
  Accepted (#155).
- [54. Ghostty](design/decisions/0054-ghostty.md): Accepted (#162).
