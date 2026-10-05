---
paths:
  - "tests/**"
---

# tests

The invariants a change to the tests keeps. The terminal contract and the layers of the tests are
in [testing.md](../../docs/design/testing.md); each helper's doc comment says how it works.

## What runs

- Tests build cld and run it against real tmux. The probe, `tests/probe`, stands in for claude,
  and for systemctl and loginctl.
- No test reaches the user's systemd or the user's own cld sessions: each runs in a sandbox of its
  own (`internal/sandbox`), in parallel.
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
  the interactive list.
- `record_expiry_test.go`: the record's indexes and expiry; `record_lock_test.go`: its lock and
  the start mark.
- `restore_test.go`: `restore` after a reboot, and the helpers the `restore_*` files share;
  `restore_marks_test.go` and `restore_unmark_test.go`: the run and busy marks;
  `restore_failures_test.go`: what `restore` leaves or cannot bring back; `restore_race_test.go`
  and `restore_forget_test.go`: the races; `restore_setup_test.go`: `setup restore`.
- `cli_TOPIC_test.go`: arguments, errors, tool and version checks, the help and the completion
  scripts, a file per topic; `cli_fake_tmux_test.go` has the helpers they share.
- `completion_test.go`: `setup completion` where nothing was; `completion_files_test.go`: files
  that exist, and refusals; `completion_shells_test.go`: bash, zsh and fish loading its scripts;
  `completion_blesh_test.go`: bash with ble.sh.
- `project_test.go`: `setup project` against the real git where there is nothing, and what the
  `project_*` files share; `project_edits_test.go`: files that exist; `project_refusals_test.go`:
  what it refuses.
- `project_gitignore_test.go`: the lines `setup project` adds to `.gitignore`;
  `project_ignored_test.go`: the shared files git ignores all the same; `project_settings_test.go`:
  the permission rules it writes, read from the repository's own `.claude/settings.json`.
- `install_test.go` and `update_test.go`: `install.sh` and `cld update`, against releases that an
  HTTP server of the test's serves; `install_release_test.go`: the release and binary `install.sh`
  picks; `install_refusals_test.go`: what it refuses.
- `update_completion_test.go`: the completion scripts `cld update` writes anew;
  `update_refusals_test.go`: what `cld update` refuses. They share `update_test.go`'s helpers.
- The helpers that more than one test file uses, a file per topic with no test in it:
  `terminal_test.go`, `tmux_test.go`, `list_test.go` for the interactive list, `entries_test.go`
  for the record's entries, `hooks_test.go`, `files_test.go`, `setup_test.go` and
  `release_test.go`. A helper of one file's tests stays there.
