---
paths:
  - "tests/**"
---

# tests

The invariants a change to the tests keeps. The layers of the tests are in
[testing.md](../../docs/design/testing.md), which links the pages of the terminal contract and the
harness. Each helper's doc comment says how it works.

## What runs

- Tests build cld and run it against real tmux. The probe, `tests/probe`, stands in for claude,
  and for systemctl and loginctl. It stands in for osascript, ps and ssh too, on a model Mac
  (`internal/mac`), and sh fakes in `tests/fakes` for open and ghostty.
- No test reaches the user's systemd, Ghostty or own cld sessions, a real host or `open`. Each
  runs in a sandbox of its own (`internal/sandbox`), in parallel, whose PATH holds the fakes first.
- The sandbox's `TMUX_TMPDIR` stays short, as socket paths hit the `sun_path` limit of about 108
  bytes ([decision 13](../../docs/design/decisions/0013-a-server-per-session.md)).
- The sandbox's work directory is `_`, of which nothing is left in a name: there `-s x` names the
  session `x`.
- `TestMain` unsets every `GIT_*` variable, which git sets for hooks and `rebase --exec`;
  `TestGitVariables` pins that.
- The probe enters the terminal modes claude enters; it changes with any behaviour of claude's
  that cld comes to rely on.
- The probe answers `claude --version` with `99.0.0 (Claude Code)` by default, so that raising the
  oldest claude leaves the tests alone ([decision 6](../../docs/design/decisions/0006-versions.md)).

## Terminals

- The terminal contract, C1 to C10, runs per terminal: a difference between terminals is an
  expectation, not a skip.
- A terminal that cannot do something skips with a reason.
- JediTerm emulates on a thread of its own: wait with `waitModes` for modes that change while
  cld runs. Once `Running` is false, what the terminal shows is final.
- The ghostty driver builds only with `-tags ghostty`, and copies the Ghostty app faithfully. A
  test that types a prefix just after a client attaches calls `waitAnswered` first: Ghostty's
  focus reports to tmux's queries would cancel the prefix
  ([decision 54.5](../../docs/design/decisions/0054-ghostty.md)).
- The completion tests skip a shell that is not installed; `CLD_BLESH` makes the ble.sh test fail
  where it would skip, as `make docker-blesh-check` runs it.

## Where a test goes

- `contract_test.go`: the terminal contract, its start and C1 and C2; `contract_keys_test.go`:
  the keys and the paste (C3, C8); `contract_mouse_test.go`: C4; `contract_passthrough_test.go`:
  C5; `contract_detach_test.go`: the detach and claude's exit (C7, C9); `contract_list_test.go`:
  the session list (C10). The helpers that only these files use stay among them.
- `session_TOPIC_test.go`: sessions and servers, a file per feature. They cover names, `join`,
  `detach`, `kill`, moves, other repositories and servers, and the server's options and hooks.
  They cover claude's exit and status, completion, the keys another tmux keeps, the idle sweep and
  the interactive list (`session_list_*`). The helpers that only these files use stay among them,
  such as `session_cells_test.go` for the screen's attributes.
- `record_test.go`: the record's entries, their hooks and place, and the helpers the `record_*`
  files share; `record_ended_test.go` and `record_list_test.go`: `ended` sessions, in `join` and
  the interactive list; `record_existing_test.go`: `join --existing` on sessions that do not run.
- `record_moved_test.go`: claude's last directory, the hooks and the kill that write it;
  `record_moved_join_test.go`: where an ended session shows and comes back by it.
- `record_rename_test.go`: the list's rename; `record_rename_refusals_test.go` and
  `record_rename_failures_test.go`: what it refuses, and its failures.
- `record_expiry_test.go`: the record's indexes and expiry; `record_lock_test.go`: its lock and
  the start mark.
- `restore_test.go`: `restore sessions` after a reboot, and the helpers the `restore_*` files
  share; `restore_marks_test.go` and `restore_unmark_test.go`: the run and busy marks;
  `restore_failures_test.go`: what `restore sessions` leaves or cannot bring back;
  `restore_race_test.go`, `restore_forget_test.go` and `restore_rename_test.go`: the races;
  `restore_setup_test.go`: `setup restore`.
- `tabs_fakes_test.go`: the probe's osascript against the model Ghostty in `internal/mac`, its
  snapshot, and the helpers the `tabs_fakes_*` files share. `tabs_fakes_failures_test.go`: its
  failure modes, its delay file and the arguments it refuses; `tabs_fakes_apply_test.go`: its
  apply. `tabs_fakes_ssh_test.go`: the probe's ssh, on real tmux; `tabs_fakes_tools_test.go`: its
  ps, the sh fakes of open and ghostty, a pty's size, and the pty as cld's input.
- `tabs_test.go`: `restore tabs` against the fake Ghostty, its record and its dry-run, and the
  helpers the other `tabs_*` files share; `tabs_mac_test.go` builds their model Mac.
  `tabs_classify_test.go`: what each pane shows; `tabs_merge_test.go`, `tabs_last_test.go` and
  `tabs_quit_test.go`: the record's merges and closes, each pane's `last`, and Ghostty's quits.
- `tabs_watch_test.go`: the watcher's life; `tabs_watch_ticks_test.go`: its ticks, its failures,
  the record's lock and its restart; `tabs_watch_runs_test.go`: the handover, a relaunch and the
  last tick; `tabs_refusals_test.go`: what `restore tabs` refuses.
- `tabs_scripts_test.go`: the scripts' terms against Ghostty's dictionary, their syntax on macOS,
  and the real ps; `tabs_scripts_mocked_test.go`: the scripts against a mock Ghostty on macOS.
- `cli_restore_test.go`: the word after `restore`.
- `cli_host_test.go`: `--host`'s ssh, its words, `__host` and completion, and the helpers the host
  tests share; `cli_host_refusals_test.go`: what `--host` refuses; `session_host_test.go`: sessions
  on the host through the fake ssh.
- `cli_TOPIC_test.go`: arguments, errors, tool and version checks, the help and the completion
  scripts, a file per topic; `cli_fake_tmux_test.go` has the helpers they share.
- `completion_test.go`: `setup completion` where nothing was; `completion_files_test.go`: files
  that exist, and refusals; `completion_shells_test.go`: bash, zsh and fish loading its scripts;
  `completion_blesh_test.go`: bash with ble.sh.
- `project_test.go`: `setup config project` against the real git where there is nothing, and what
  the `project_*` files share; `project_edits_test.go`: files that exist;
  `project_refusals_test.go`: what it refuses.
- `project_gitignore_test.go`: the lines `setup config project` adds to `.gitignore`;
  `project_ignored_test.go`: the shared files git ignores all the same; `project_settings_test.go`:
  the permission rules it writes, read from the repository's own `.claude/settings.json`.
- `user_test.go`: `setup config user` against the sandbox's `HOME` and `CLAUDE_CONFIG_DIR`;
  `user_refusals_test.go`: what it refuses.
- `install_test.go` and `update_test.go`: `install.sh` and `cld update`, against releases that an
  HTTP server of the test's serves; `install_release_test.go`: the release and binary `install.sh`
  picks; `install_refusals_test.go`: what it refuses.
- `update_completion_test.go`: the completion scripts `cld update` writes anew;
  `update_refusals_test.go`: what `cld update` refuses. They share `update_test.go`'s helpers.
- `pins_test.go`: pins that move together, Ghostty's commit and its bindings
  ([decision 54.2](../../docs/design/decisions/0054-ghostty.md)). The ghostty driver's own tests
  are in `internal/terminal/ghostty_test.go`, behind its tag.
- The helpers that more than one test file uses, a file per topic with no test in it:
  `terminal_test.go`, `tmux_test.go`, `list_test.go` for the interactive list, `entries_test.go`
  for the record's entries, `hooks_test.go`, `files_test.go`, `setup_test.go` and
  `release_test.go`. A helper of one file's tests stays there.
