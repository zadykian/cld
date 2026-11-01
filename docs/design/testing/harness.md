# Testing: the harness

The tests' own comments give each case's reasons; these shape the harness as a whole. How the tests
keep cld apart from the machine is in [testing](../testing.md#layers).

## The probe and the sandbox

- The probe answers `claude --version` before anything else, `99.0.0` unless a test says
  otherwise, so raising cld's floor ([decision 6](../decisions/0006-versions.md)) changes no test.
  It writes no record then: only the claude in a session counts as one.
- The fake tmux's servers are sockets the test holds open, as cld connects before it asks tmux
  ([decision 38](../decisions/0038-stale-sockets.md)). A stale socket is one nothing listens on, not
  a plain file: macOS reports a file as no socket, and tmux fails (#51).
- The sandbox opens its pty without cgo. A goroutine reads it from the start, as a terminal would,
  so nothing written there waits on the test.
- A race holds tmux as it would make a session, by a wrapper first on the `PATH`. A reboot is
  `kill-server` on each server, which runs no `pane-died` hook.
- `install.sh` and `cld update` run against releases that an HTTP server of the test's serves
  (`CLD_RELEASES_URL`), `install.sh` under a fake `uname`.
- `setup config project`'s tests run the real git. `setup config user`'s write the sandbox's
  `HOME`, or a `CLAUDE_CONFIG_DIR` in it. A failed write, for either, comes from a path of 4095
  bytes, the most Linux takes.

## Timing

The session list's timing tests ([decision 15](../decisions/0015-killing-from-the-list.md)) never
rely on the time between keys that separate tmux clients type. Under load such keys came over two
seconds apart, failing 7 runs in 96. The tests count the list's frames in the output log instead,
and paste the keys meant to come together. To line a key up with a timer, a test stops cld with
SIGSTOP. Without cld's check for a waiting byte, that read-late case failed 4 runs in 4.

## The Mac's fakes

`cld restore tabs` (#176) drives Ghostty on a Mac, and no runner automates Ghostty, as none
automates iTerm2 ([decision 8](../decisions/0008-iterm2.md)). The tests give cld a model Mac
instead: `tests/internal/mac` holds Ghostty's windows, tabs, split trees and terminals, and their
processes, in a file of the probe directory.

- The probe plays osascript against the model. A script's first line, its marker, picks a snapshot
  or an apply, so the fake reads no JavaScript. It applies a plan in Go, putting what it builds
  where Ghostty would: the tests check cld's plans, not apply.js. It fails where apply.js would
  throw, so that a plan the script cannot run fails the tests too.
- The model can have Ghostty fail to make one terminal of an apply. Its build then fails partway
  and keeps the keys of what it made, as apply.js reports it. The fake's texts for such errors
  stand in for JXA's until #177's spike, so cld must not rely on them. The model can also have
  Ghostty drop a tab's first moves.
- A failure mode comes from a file in the probe directory before the environment, as a watcher's
  environment is fixed once it starts. Each mode answers as the script would against such a
  Ghostty: the error documents cld tells apart, osascript's own failure, and a hang. An apply may
  also fail midway, or answer short. Where Ghostty does not run, the scripts answer before any
  Apple Event.
- A delay file holds the next snapshot back once it has read the model. So a test can keep a stale
  snapshot in flight while it changes the model. Another holds the next apply back once it has
  built, while the restore holds the record's lock. Tests write these files by a rename: a watcher
  that read one half written would miss the failure, or fail on the delay.
- The probe plays ps from the model's processes, and ssh to a host that is the sandbox itself, for
  `restore tabs` and `--host`. The fake ssh runs the remote command with the real bash in `HOME`,
  `SHELL` set to that bash, as sshd sets the login shell. So a line cld types runs against the
  sandbox's real tmux, as on the host.
- `CLD_FAKE_SSH_EXIT` plays a dropped connection only where ssh would connect. `ssh -G` never
  connects, and cld's check of Ghostty's terminfo cache reads it whatever the connection does. A
  file of the probe directory gives a host the hostname and port that `ssh -G` prints.
- The sandbox's `~/.profile` puts the tools under test first on a login shell's PATH. The login
  shell reads `/etc/profile` before it, where macOS's `path_helper` reorders the PATH. The fake ssh
  refuses a remote shell without it, which would run the developer's cld, or the real ssh.
- open and ghostty are sh fakes on the shell's builtins alone. Tests run cld under a PATH of the
  named tools only, where `cat` would exit 127, and cld would go on without what the fake prints.
- A pane's size comes from a real pty whose winsize the test sets, as cld reads it by the pane's
  tty. A model pane with no pty has a tty that no device backs.
