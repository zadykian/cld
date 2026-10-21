# 57. Renaming from the list

Status: Accepted (#172).

## Context

A session kept the name `join` gave it. claude renames a conversation with `Ctrl+R` in agent view
and in its `/resume` picker; in claude's prompt, `Ctrl+R` searches the history (claude 2.1.299;
[claude's findings](../findings/claude.md#agent-view-and-background-sessions)).

A session that runs cannot change its name in place:

- tmux sets `socket_path` once, as the server starts. After a `mv` of the socket, `#{socket_path}`
  and `TMUX` keep the old path, and SIGUSR1 makes the socket there again (tmux 3.5a and 3.7c, read;
  3.7c probed; [tmux's findings](../findings/tmux-servers.md#sockets)).
- cld's checks of a server read `socket_path` ([decision 34](0034-servers-are-marked.md)). `C-q s`,
  the moves and a bare `detach` go by the socket `TMUX` names
  ([decision 51](0051-moving-between-sessions.md)).
- claude keeps `--name cld-S` and `--worktree cld-S`, and the hooks in its `--settings` name
  `sessions/S.json` and `=cld-S:` ([decision 40.2](0040-session-record.md),
  [decision 25](0025-title-follows-status.md)).

A session that has ended is its record's files alone ([decision 40](0040-session-record.md)), which
cld can move under the record's lock. `join` brings it back with `--name cld-S` beside
`--resume ID`, so the conversation takes the new name ([decision 16.2](0016-resume.md);
[claude's findings](../findings/claude.md#resuming)).

This amends the keys and footer of [decision 14](0014-the-session-list.md), and Enter's refusal
(14.5). It also amends [decision 40](0040-session-record.md): an `ended` row renames too (40.3), a
rename moves the entry that `kill` keeps (40.5), and the old name is free at once (40.6).

## Decision

### 57.1 Ctrl+R on an ended row

In the list, `Ctrl+R` on an `ended` row opens an editor in the footer, with the row's name typed:
`rename to NAME`, then `enter to rename · esc to cancel` where they fit. Enter renames the session,
and the list reads the sessions again, the selection on the new name. Enter on the same name only
closes the editor. Esc closes it, keeping the list open, and Ctrl+C leaves the list. `Ctrl+R`
follows claude's lists of conversations.

An ended row's hints gain `ctrl+r to rename`. Its arrows' hint goes first where the terminal is
short of cells, so that an 80-column terminal shows the rest whole. Of decision 14's keys, Ctrl+R
no longer does nothing, and an armed kill counts it as any other key: it disarms the kill.

### 57.2 The editor

Each byte typed goes into the name, but for the editor's keys: a printable one, one beyond ASCII and
a control character alike. Backspace takes the last character off, and Ctrl+U the whole name. The
arrows, Ctrl+X, Ctrl+R and the keys sent as a sequence, such as Alt's, do nothing there. The list
turns on no bracketed paste, so a paste is typed, and a CR in the text pasted acts as Enter.

Nothing typed is dropped or changed: Enter checks the name whole, as `-s` is checked
([decision 1](0001-naming.md)). A refusal shows after the name, or alone where both do not fit, and
the editor stays open while the row is still `ended`. A name over 64 characters is refused without
itself, which would fill the footer. The terminal's cursor is hidden, so a cell in inverse video
stands for it. A name too long for the terminal is cut from the left, and a control character
shows as `?`.

### 57.3 A row that runs

`Ctrl+R` on a row that runs says to end the session first with Ctrl+X twice. Rejected: a kill, a
rename and a restore in one step. It interrupts claude mid-turn and detaches its terminals. A kill
does not wait for claude ([decision 32](0032-what-a-kill-does.md)), so the resume would race the
old claude's exit. From `C-q s` over that session, it would end the popup partway
([decision 15.2](0015-killing-from-the-list.md)). That step can follow in an issue of its own.

### 57.4 What the rename refuses

The rename runs under the record's lock, as the forget does
([decision 40.6](0040-session-record.md)). It refuses what the forget refuses: a session that runs
again, that another cld is starting ([decision 50.4](0050-one-command-join.md)), or that has no
entry. It also refuses:

- A server that still runs under the old name, as `join` refuses it
  ([decision 13](0013-a-server-per-session.md)): its claude may still hold the conversation.
  `cld kill` ends such a server, and the refusal says so, as the list shows no advice. Enter's
  refusal of such a row, which named no remedy, takes the same words.
- An entry without a conversation's ID. `join` would resume by the name `cld-NEW`, which no
  conversation has, and claude would open its picker
  ([claude's findings](../findings/claude.md#resuming)).
- A new name that a session or a server has, that another cld is starting, or that has any file in
  the record. A name that has ended stays taken until the list forgets it. What has expired goes
  first, as `join` removes it ([decision 40.6](0040-session-record.md)): the list shows no row for
  it, and so no way to forget it.

### 57.5 Moving the record's files

The entry is written anew with the new name, to a temporary file, then linked under that name: a
link never replaces a file. It takes the old entry's time, so its expiry stays. Each file beside the
entry is linked under the new name, keeping its time. Among them are the run mark, for `restore`,
and `S.dir`, claude's last directory, where the session comes back
([decision 56.4](0056-following-claudes-cd.md)). The old names then go. A failure before the old
entry has gone removes the links made, and the old name stays whole.

The entry kept the old name's index counted, which `join` gave at the session's start, perhaps
long before its last use. So the rename gives the old index again, as of the rename, and the new
name's too. `indexes.json` keeps the highest index given alone, so a higher one is given again in
its place. The indexes go before the files: a failure there moves nothing.

A file system may ignore case, as macOS's does by default. There a rename that changes only the case
finds the old files under the new name, and refuses the name as taken. That is not checked on macOS.

### 57.6 The old name is free at once

Decision 40.6 kept a name until claude no longer kept its conversation. The old name is now free at
once, though its index is given again (57.5): `join` without `-s` gives it to no session for 30
days. A `-w` session's worktree keeps `cld-OLD`, and a later `join -s OLD -w` opens it again
([decision 4](0004-worktrees.md)). Rejected: keeping the old name until expiry, which needs a kind
of record file of its own for a rare case.

The list renames a whole name. Renaming `api-3` to `auth` takes it out of the sessions that `-s`
names in repository `api` ([decision 24](0024-names-from-the-repository.md)).

### 57.7 No command

Rejected: a `cld rename` command beside the key. The list names a session whole, and 40.5 kept the
forget out of the command line. A command would need its help, completion and `-n` and `-s`.

### 57.8 Tests

`record_rename_test.go` covers the rename, the footers in 80 and 30 columns, the editor, a restore
under the new name and the indexes. There the list's Enter and `restore` after a rename come back
where the moved `S.dir` says. `record_rename_refusals_test.go` covers the refusals,
`record_rename_failures_test.go` a link and a read that fail, and `restore_rename_test.go` a rename
racing a restore and a join. `session_list_states_test.go` checks Enter's refusal of a server
that outlived its session. C10's `rename` types the keys in each terminal
([testing](../testing.md)).

## Consequences

- A session that ran under a name it no longer suits takes another in two keys and the name.
- A session that runs is renamed by ending it first, which ends its claude.
- The forget's files and the rename's are one list in `internal/session`: a file added beside the
  entry moves with it.
- Out of scope: renaming from the command line, and a session that runs.
