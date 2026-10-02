# cld: user guide

What you may need beyond the [README](../README.md) and `cld help`: caveats, what is not checked
yet, troubleshooting and upgrading. How cld works, and why, is in [design.md](design.md).

- [Installing and upgrading](guide/install-and-upgrade.md): `install.sh`, and each release's changes
- [Sessions](guide/sessions.md): names, claude's environment, a kill, Remote Control, agent view
- [In the terminal](guide/terminal.md): title and status, mouse, scrollback, JetBrains IDEs,
  `C-q`, notifications
- [Inside your own tmux](guide/inside-your-own-tmux.md): the keys it keeps, and how to free them
- [The session list and moving between sessions](guide/session-list.md): `cld list`, `C-q s`
- [Idle sessions](guide/idle-sessions.md): what counts as idle, and keeping a session
- [Resuming a conversation](guide/resuming.md): the record, names, copies, agent view's moves
- [After a reboot](guide/after-a-reboot.md): `cld restore` and `cld setup restore`
- [claude's options and worktrees](guide/claude-options.md): the words after `--`, and `-w`
- [Project settings](guide/project-settings.md): permission sets, the IDE's port, existing files
- [Telemetry](guide/telemetry.md): what each endpoint gets, the settings, your collector config
- [Shell completion](guide/shell-completion.md): what TAB offers, and each shell's caveats
- [cld and Claude Code's background sessions](guide/background-sessions.md): the two compared
- [Troubleshooting](guide/troubleshooting.md): cld's refusals and errors, and a slow cld

## Upgrading

<!-- Released copies of install.sh link to this heading: keep it. -->

- **cld.** `cld update` installs the latest release, through the proxy `HTTPS_PROXY` names. You
  need to be able to write the file cld runs from. cld 0.5.0 and earlier have no `update`: run the
  [install command](../README.md) again, with the same `CLD_INSTALL_DIR`.
- **Running sessions** keep the cld and the tmux that started them until they end. To move one to
  the new release or tmux, end it with `cld kill`, and bring it back with `cld join`.
- **What changed** in each release: [installing and upgrading](guide/install-and-upgrade.md).
- **From a script.** cld 0.3.0 and earlier were a bash script, which keeps working until you install
  cld as the [README](../README.md) says. Its sessions ran on one server, which later releases do
  not see: `tmux -L cld ls` lists them, and `tmux -L cld kill-server` ends them.
- Names with non-ASCII letters or digits, such as `café`, which 0.3.0 took, are refused now.
- **Staying on tmux 3.3 or 3.4.** cld 0.3.0 is the last release that runs on them:

  ```sh
  curl -fsSL https://github.com/zadykian/cld/releases/download/v0.3.0/cld -o ~/.local/bin/cld && chmod +x ~/.local/bin/cld
  ```
