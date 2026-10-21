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
