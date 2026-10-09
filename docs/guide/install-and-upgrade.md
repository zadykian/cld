# Installing and upgrading

## install.sh

- `install.sh` runs in `sh` or `bash`, and needs `curl`, and `sha256sum` or `shasum`. Under Rosetta
  2 on Apple silicon, it still installs the arm64 binary.
- It replaces `cld` only once the download matches `cld.sha256` and runs. On any failure it exits
  with status 1, leaving the directory unchanged.
- It edits no shell profile: where it says its directory is not on your `PATH`, or another `cld`
  comes first, fix `PATH` yourself.

By hand, download your system's binary and `cld.sha256` from a
[release](https://github.com/zadykian/cld/releases), and check the binary:
`grep ' cld-linux-amd64$' cld.sha256 | sha256sum -c` (`shasum -a 256 -c` on macOS). Install it as
`cld`, executable, on your `PATH`.

## What changed in each release

[Upgrading](../guide.md) says how to upgrade; here is what changed, newest first.

### From cld 0.12.0 and earlier

- **Notifications** reach a terminal that shows another window of the session, and no longer go
  missing while tmux redraws it. A session made before keeps missing them, until you end it and
  `cld join` makes it anew
  ([decision 55](../design/decisions/0055-passthrough-from-claudes-pane.md)).

### From cld 0.11.0 and earlier

- **`cld setup project`** is now `cld setup config project`, with the same options, and
  `cld setup project` fails as an unknown command
  ([decision 53](../design/decisions/0053-project-and-user-settings.md)). The new
  `cld setup config user` sets up your own settings: see [claude's settings](project-settings.md).
- **`cld setup telemetry`** is gone
  ([decision 52](../design/decisions/0052-telemetry-outside-cld.md)). What it set up stays and
  keeps working: Docker starts the collector after a reboot, and claude sends to it. To send
  claude's telemetry elsewhere, edit the keys in claude's settings, as
  [Claude Code's docs](https://code.claude.com/docs/en/monitoring-usage) say.
- To remove the collector, run `docker rm -f cld-telemetry`. Then remove the keys cld set from the
  `env` of claude's settings: `CLAUDE_CODE_ENABLE_TELEMETRY`, `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA`,
  `OTEL_LOG_TOOL_DETAILS`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_EXPORTER_OTLP_ENDPOINT`,
  `OTEL_METRICS_EXPORTER`, `OTEL_TRACES_EXPORTER` and `OTEL_LOGS_EXPORTER`.

### From cld 0.10.0 and earlier

- **`cld new` and `cld resume`** are gone: `cld join` does all three, by the session's state
  ([decision 50](../design/decisions/0050-one-command-join.md)).

  | Before | Now |
  |---|---|
  | `cld new` | `cld join`; over an ended session, `cld join --new` |
  | `cld resume -s SUFFIX` | `cld join -s SUFFIX` |
  | `cld resume SESSION` | `cld join --resume SESSION` |
  | `cld resume --fork SESSION` | `cld join --resume SESSION --fork` |

- `cld join` no longer refuses an ended or missing session, and without `-s` creates one. A script
  that relied on those refusals checks `cld list` first.
- **Agent view** is off in new sessions. A conversation an older session moved out comes back as
  [resuming a conversation](resuming.md) says.
- **Moving between sessions.** `cld join` and `cld list` in a session's pane move the terminal. In
  new sessions, `C-q s`, `C-q (`, `C-q )` and `C-q L` are cld's.
- **`STATE`** in `cld list`'s table has claude's status after a comma, as in `detached, waiting`. A
  script that splits rows at spaces finds more words there.
- **After a reboot.** `cld restore` brings back only sessions that a cld with it started. On Linux,
  run `cld setup restore` once.

### From cld 0.9.0 and earlier

- **Idle sessions** end after 30 days, older ones too. Set `CLD_IDLE_DAYS` before the first run to
  keep them. `cld list` gains `LAST ACTIVE` before `DIRECTORY`, which a script finds fourth, after
  a header of five words.
- **Ended sessions.** These releases kept no record, so `cld join` resumes their sessions only by
  name (see [resuming a conversation](resuming.md)).
- **Remote Control.** cld 0.3.0 to 0.9.0 turned it on over a `false` in `/config`. Now claude's own
  setting decides, and its `true` connects every claude, in cld or not.

### From cld 0.8.2 and earlier

- **A session made in an IDE's terminal**, such as Cursor's, or on macOS a JetBrains IDE's, takes
  Shift+Enter for Enter from any terminal.
- **A pane split off in claude's window** stays on screen once its program fails, and the message
  line says `claude exited`.
- **Project settings.** `cld setup project` of 0.4.0 to 0.8.2 wrote `/.claude/*` and
  `!/.claude/settings.json` to `.gitignore`, which keep shared files out of git. It also wrote
  `theme`, `autoUpdatesChannel`, `autoMemoryEnabled`, `autoCompactEnabled` and a long allow list.
- Run `cld setup config project` again, then remove the two lines, the four keys and the entries
  you do not want to share.

### From cld 0.8.0 and 0.8.1

- **The title's hooks** failed after every tool in a conversation claude ran in the background,
  with `no current session`. `cld kill` leaves that conversation running, and claude refuses to
  resume it: `claude stop ID` first (`claude agents` lists the IDs), then `cld kill` and
  `cld join`.

### From cld 0.7.1 and earlier

- **Session names.** `-n NAME` named the session `NAME` whole, by default `main`. Reach one from
  `cld list`, or as `cd / && cld join -s main`.
- `cld join --resume cld-main` brings its conversation into a session named the new way. The old
  worktree `NAME` stays: `git worktree remove .claude/worktrees/NAME` removes it.

### From cld 0.7.0 and earlier

`cld join` detached any other terminal; `cld join --detach-others` still does.

### From before cld 0.2.0

`cld [NAME]` fails as an unknown command: `cld join` attaches to a session `NAME-SUFFIX`.
