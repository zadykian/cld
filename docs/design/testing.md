# Testing

The tests build cld and run it against real tmux, which is local and cheap, so never faked. A
probe stands in for claude, and for the programs that would reach outside the sandbox. The
[overview](overview.md) maps the rest of the design record.

## Layers

1. **Static.** `make vet` runs gofmt, go vet, ShellCheck and shfmt. `make lint`, CI's `lint` job,
   adds golangci-lint, the size caps, Vale, govulncheck, the workflow linters and lychee.
2. **Behaviour against real tmux.** The probe replaces claude, `tmux -V` for the version check,
   docker for `setup telemetry` ([decision 18](decisions/0018-telemetry.md)), and `systemctl`
   and `loginctl` for `setup restore` ([decision 48](decisions/0048-restore-after-reboot.md)).
   As claude it sets the terminal modes claude sets, logs its arguments, directory, environment
   and raw input, and writes what claude writes on command.
3. **Terminal contract.** The same checks run against several outer terminals, through drivers.

Isolation needs no seams in cld. `TMUX_TMPDIR` moves the sockets into a sandbox, `HOME` is a
temporary directory, `TMUX` is unset, and the probe comes first on the `PATH`. `cld join`
attaches, so it needs a pty: a driver's, or the sandbox's own where a test hands over to the fake
tmux ([decision 31.6](decisions/0031-a-terminal-to-attach-from.md)).

## The terminal contract

A result that rightly differs per terminal is an expectation of that terminal, never a skip, so
a terminal gaining or losing support flips a test. Tests cite the checks by number.

- **C1, title.** The tab's title is `✳ cld-NAME`, with `◐` and `◑` in turn while claude is busy,
  and ` [w]` after it while claude works in a linked git worktree. It survives claude's own title
  changes. Evidence: terminal, probe.
- **C2, the client.** tmux's view of the client: `#{client_termtype}` and
  `#{client_termfeatures}` (`extkeys`, `focus`, `mouse`, `clipboard`, `hyperlinks`). Evidence:
  tmux.
- **C3, keys.** tmux asks the terminal for modified keys, and takes the request back on detach.
  Shift+Enter reaches claude apart from Enter, and the Ctrl keys claude binds (`C-b`, `C-_`) pass
  through. `C-q d` detaches, and `C-q C-q` sends `C-q`. Evidence: probe input log, terminal
  output.
- **C4, mouse and focus.** The wheel and focus changes reach claude, and so do a Ctrl+click and an
  Alt+right-click whole, press and release. Over a main-screen program without mouse reporting,
  the wheel scrolls the pane's history. Evidence: probe input log, tmux.
- **C5, passthrough.** OSC 52, claude's notifications (OSC 9, 99 and 777) in tmux passthrough, the
  bell, copies through `tmux load-buffer -w` and claude's OSC 8 links reach the outer terminal.
  Evidence: terminal, terminal output.
- **C6, environment.** Neither claude nor what it starts through tmux sees a variable that names
  the terminal the session was made in ([decision 33](decisions/0033-terminal-variables.md)).
  Evidence: probe environment dump, tmux.
- **C7, detach.** After detach the terminal is clean: no mouse reporting, no alternate screen.
  Evidence: terminal.
- **C8, paste.** A paste reaches claude bracketed and whole, and a prefix key inside it stays text.
  Evidence: probe input log.
- **C9, exit.** claude exiting ends its session, and its server unless tmux sessions claude made
  keep it. The terminal is left clean. A claude that fails, by a status other than 0 or a signal,
  keeps its session, with its message and how to end it. Evidence: terminal, tmux.
- **C10, the session list.** `cld list` on a terminal reads the terminal's own keys. Down and Enter
  join the second session, titled `✳ cld-NAME`. Ctrl+X twice kills the selected session, whose row
  stays, selected, as `ended`. Esc puts the terminal back: the main screen, no mouse reporting,
  the cursor visible and the same `stty -g`. In `C-q s`'s popup, Esc closes the list, and Down and
  Enter move the terminal to the second session
  ([decision 51](decisions/0051-moving-between-sessions.md)).
  Evidence: terminal, probe, tmux.

## Drivers

### tmux, the baseline

An outer tmux server on its own socket provides the pty, on every push. Keys go by name through
`send-keys`, and Shift+Enter, focus and the mouse as raw xterm bytes through `send-keys -H`. A
paste goes through the outer paste buffer, and formats give the title and the modes. Freezing
stops the outer server with SIGSTOP, so it neither reads the pane nor answers it. A key held down
is one command list that the outer server times. The driver's comments give the workarounds for
tmux 3.3a to 3.7 and uutils.

### JediTerm

`jediterm-core`, pinned at 3.76 ([decision 7](decisions/0007-jediterm-pin.md)), runs headless on
every push. JetBrains publishes it to its Maven repository; it needs only slf4j and annotations.
The driver is Java, as JediTerm is a JVM library, and the Go side talks to it a line per command.
It spawns cld through pty4j with `TERMINAL_EMULATOR=JetBrains-JediTerm`, feeds JediTerm's emulator
with a recording display, and types keys as key events through JediTerm's own encoder. It covers
the emulator, not the IDE around it: what the IDE's keymap takes stays a manual check.

The emulator reads the pty on a thread of its own, even after pty4j reports the process gone.
Once `Running` is false the screen and modes are final, which C7 and C9 read. Modes that change
while cld runs are waited for. So is mouse reporting before a wheel or a click, as tmux turns the
mouse modes off and on again after it draws. Both races showed only under load.

### iTerm2, planned

Not built ([decision 8](decisions/0008-iterm2.md)). It would run the real app on a hosted macOS
runner, nightly and on tags, in levels:

- Level 0 launches cld from a Dynamic Profile, and tmux and the probe check C2 and C6.
- Level 1 reads the title, the screen and the detach state through the Python API (C1, C5, C7).
  An outside client needs an AppleScript cookie, unless "Allow all apps to connect" is on; its
  `defaults` key is undocumented.
- Level 2 posts real key and mouse events (C3, C4) as CGEvents, since `send_text` cannot express
  modifiers. That needs Accessibility permission on the runner. The fallback is a run on a Mac
  before releases.

A spike comes first. Does the pinned iTerm2 start on a hosted runner without a dialog in the way?
Does the Python API connect with "allow all apps" set through `defaults`? Can one Shift+Enter
event be posted?

## The harness

The tests' own comments give each case's reasons; these shape the harness as a whole.

- The probe answers `claude --version` before anything else, `99.0.0` unless a test says
  otherwise, so raising cld's floor ([decision 6](decisions/0006-versions.md)) changes no test.
  It writes no record then: only the claude in a session counts as one.
- The fake tmux's servers are sockets the test holds open, as cld connects before it asks tmux
  ([decision 38](decisions/0038-stale-sockets.md)). A stale socket is one nothing listens on, not a
  plain file: macOS reports a file as no socket, and tmux fails (#51).
- The sandbox opens its pty without cgo. A goroutine reads it from the start, as a terminal would,
  so nothing written there waits on the test.
- A race holds tmux as it would make a session, by a wrapper first on the `PATH`. A reboot is
  `kill-server` on each server, which runs no `pane-died` hook.
- `install.sh` and `cld update` run against releases that an HTTP server of the test's serves
  (`CLD_RELEASES_URL`), `install.sh` under a fake `uname`.
- `setup project`'s tests run the real git. A failed write comes from a path of 4095 bytes, the
  most Linux takes, as in the telemetry tests.
- The telemetry tests take ports outside the kernel's ephemeral range. The kernel hands a port that
  a listener on port 0 let go to the next such listener, now and then another test's.
- The fake docker's collector holds its port until `rm -f`. One that takes no connections binds the
  port without listening and without `SO_REUSEADDR`: connections are refused, but no other socket
  can have the port ([findings](findings/environment.md)). Left free, the port could go to another
  test's cld, whose listener would then take the waiting cld's connections as if the collector were
  ready.

The session list's timing tests ([decision 15](decisions/0015-killing-from-the-list.md)) never
rely on the time between keys that separate tmux clients type. Under load such keys came over two
seconds apart, failing 7 runs in 96. The tests count the list's frames in the output log instead,
and paste the keys meant to come together. To line a key up with a timer, a test stops cld with
SIGSTOP. Without cld's check for a waiting byte, that read-late case failed 4 runs in 4.

## CI

- Every push and pull request runs the `lint` job, and the contract on tmux and JediTerm in the
  Docker image, tmux built from source on `debian:trixie`. `blesh` runs the completion tests in
  bash with ble.sh, in the image built on Ubuntu
  ([decision 27](decisions/0027-completion-with-blesh.md)). `macos` runs `make check` with
  Homebrew's tmux.
- The image runs 3.7c (`linux`) and 3.5a, the oldest cld runs on (`linux-oldest`, advisory). A
  developer runs the same image locally.
- A tag `vX.Y.Z` runs the checks and publishes a release.
- iTerm2, nightly and on tags, waits on its driver, and stays advisory until it proves stable.

Pull requests land on `main` by fast-forward, so `main` holds the commits CI checked. GitHub's
merge methods all write commits of their own, so the repository allows merge commits alone, and
the ruleset on `main` rebase alone, leaving none. A comment `/fast-forward` pushes the head once
the required checks pass; a change to `.github/workflows` is pushed by hand, as the workflow's
token may not push it.

## What the tests found

JediTerm 3.76, read from its source and confirmed by the contract
([terminals findings](findings/terminals.md)):

- It answers no XTVERSION, so tmux falls back to its `xterm*` defaults. The emulator ignores focus
  reporting, OSC 52, and the modifyOtherKeys request that cld's `extkeys` has tmux send.
- It takes OSC 8 links, which tmux writes only with the `hyperlinks` feature
  ([decision 30](decisions/0030-links.md)).
- Shift+Enter is CR, or ESC CR with its `shiftEnterSendsEscCR` setting, which tmux passes on as
  Meta+Enter.
- Its wheel constants are named the wrong way round, but its UI maps them right.
- tmux sends claude a focus-in when a client attaches, whatever the terminal supports.

tmux 3.3a to 3.7c ([tmux and the terminal](findings/tmux-terminal.md)):

- A control character in a bracketed paste reaches claude unchanged, and the prefix there binds
  nothing.
- Under `mouse off` tmux passes a pane's mouse modes on, and claude 2.1.281 scrolled its
  transcript. `mouse on` adds the wheel scrolling the history over a main-screen program (C4).
- From 3.6 the wheel goes to a program in the alternate screen, asked for or not; earlier releases
  entered copy mode.

A `VT10x` once seen came from a probing shell that carried `TERMINAL_EMULATOR`: the leak that
[decision 33](decisions/0033-terminal-variables.md) closes. From a clean environment claude 2.1.281
put the pane in key mode `Ext 2`, and Shift+Enter inserted a newline, checked by hand.

## Status

The contracts run on tmux 3.7c and 3.5a in Docker, and the baseline on Homebrew's tmux on macOS.
3.6a passed the whole suite once (#74); 3.6 is not in CI. What the real claude makes of 3.5's
Shift+A and Shift+Backspace was not checked: the floor at 3.5a keeps them from it.

Checked with the probe or by hand, with what is left:

- Stale sockets ([38](decisions/0038-stale-sockets.md)): probed on Linux with tmux 3.5a and 3.7c;
  on macOS run in CI only.
- The command limit ([41](decisions/0041-claude-options.md)): probed on 3.5a and 3.7c, read in
  the 3.4 and 3.7c sources; on macOS it rests on CI.
- Scrollback ([36](decisions/0036-scrollback.md)): checked on 3.5, 3.6a and 3.7c with programs
  that print lines, not with claude's classic renderer or a screen reader.
- Links ([30](decisions/0030-links.md)): probed with `TERM` `wezterm` and `alacritty`. No real
  terminal was seen showing one, nor which click opens it.
- A failed pane ([5](decisions/0005-failures-stay-on-screen.md)): tested beside a pane split by
  hand; a teammate's pane was read in claude 2.1.284.
- `install.sh` ([20](decisions/0020-install-script.md)) and `cld update`
  ([21](decisions/0021-self-update.md)): run by hand on Linux, against v0.4.0 and from 0.4.0 to
  0.5.0. Not an update on macOS, nor Rosetta 2's `sysctl.proc_translated`.
- `setup completion` ([22](decisions/0022-setting-completion-up.md)): bash 5.2.37 with
  bash-completion 2.16, zsh 5.9 and fish 4.0.2 in the image, and zsh on macOS. Not Homebrew's
  `bash-completion@2`, nor `compinit -i` under group-writable directories.
- `setup telemetry` ([18](decisions/0018-telemetry.md)): run by hand against the real collector
  image. Not the JetBrains plugin, nor claude sending through it.
- `setup project` ([19](decisions/0019-project-settings.md),
  [28](decisions/0028-shared-project-settings.md)): checked with git 2.53.0 and
  `claude mcp list` 2.1.283, not in a claude session.
- Names and homes ([24](decisions/0024-names-from-the-repository.md),
  [37](decisions/0037-sessions-of-another-repository.md)): git 2.47.3 and macOS's. Not a home in
  other letters where case is ignored, nor `-w`'s worktree with the real claude.
- Detach ([44](decisions/0044-detach-command.md)) and moving
  ([51](decisions/0051-moving-between-sessions.md)): run as claude runs a shell command, not by
  its `!`. Moving ran under dash, bash, zsh and fish, not the other shells cld takes, nor a popup
  in a real terminal. Nor were IDEs seen taking `Ctrl+Q` (#72).
- Inside another tmux ([43](decisions/0043-inside-your-own-tmux.md)): probed with 3.7c on both
  sides, and 3.5a and 3.6a for the message. Not another release outside, nor a real terminal.
- `join` ([50](decisions/0050-one-command-join.md)): the start mark's races ran without the mark on
  3.7c only. For the maintainer: an ended row's footer, and `-w` with `--new` over an ended session.
- Idle sessions ([46](decisions/0046-idle-sessions.md)): limits of seconds, not days.
- Restore ([48](decisions/0048-restore-after-reboot.md)): a transient unit stood for the user's
  systemd. Not a real reboot, lingering off, or a resumed claude asking a permission. launchd is not
  looked into.
- The list's status ([49](decisions/0049-status-in-the-list.md)): format probed on 3.5a and 3.7c;
  bold `waiting` seen only in tmux's rendering.

Read in claude's bundle or docs rather than run. A run that would reach the API or claude.ai is
the maintainer's to run or allow.

- Title hooks ([25](decisions/0025-title-follows-status.md),
  [26](decisions/0026-title-marks-worktree.md)): claude 2.1.283 ran them up to a prompt a hook
  blocked, and the worktree's in a worktree and on `cd`. Not a real turn, nor a background worker,
  nor the title and the empty OSC 7 tmux sends it in iTerm2.
- Hook costs ([39](decisions/0039-hook-cost.md)) and notifications
  ([29](decisions/0029-notifications.md)): read in 2.1.284. No real notification was seen. Still to
  see: a `CwdChanged` hook in the background, a hook that reaches its timeout, and a
  `PermissionRequest` right after another tool's `PostToolUse`.
- Kill ([32](decisions/0032-what-a-kill-does.md)): probed with a script in claude's place. Not a
  real claude's `SessionEnd` hooks.
- Terminal variables ([33](decisions/0033-terminal-variables.md)): read in 2.1.282 to 2.1.284 and
  VS Code's source. claude was not run in Cursor, VS Code, a JetBrains IDE or on macOS.
- Clicks ([35](decisions/0035-modifier-clicks.md)): the probe gets press and release; claude's
  handling was read in 2.1.284. Selecting with Shift held, Option in iTerm2 and Fn in Terminal.app,
  was not tried.
- A locale without UTF-8: claude 2.1.284's bundle draws in UTF-8 regardless. Only a stub claude
  ran under such a locale.
- The record ([40](decisions/0040-session-record.md)): hook input read in 2.1.232 to 2.1.284. Not
  `/resume`, `/rename` or background conversations keeping the ID, nor macOS's file system.
- Words after `--` ([41](decisions/0041-claude-options.md)): `--help` 2.1.284 was run, the rest
  read.
- Agent view ([47](decisions/0047-agent-view-off.md)): the maintainer saw 2.1.285's
  `claude agents --json` refused under the key. Not `/bg`, `←`, `/exit`'s dialog or `/fork` in a
  session, `ListAgents` and `SendMessage` between cld's sessions, and whatever else of claude needs
  the daemon.
- Remote Control ([42](decisions/0042-remote-control-is-claudes.md)): read in 2.1.284 and the docs.
  Not known: whether a session that turned Remote Control on at startup records that session in its
  conversation.
- Resuming ([16](decisions/0016-resume.md), [45](decisions/0045-resuming-a-copy.md)): the probes of
  #26 and `--fork-session` (#75) never ran. Among them: `claude stop ID` then `join` of a moved
  conversation, `Esc` in agent view after `←`, and `--fork-session` on a conversation open elsewhere
  or in the background. Until they run, the guide and those decisions go by claude's docs, `--help`
  and bundle.
