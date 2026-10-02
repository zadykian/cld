# 49. claude's status in the list

Status: Accepted (#113). Amended by [50](0050-one-command-join.md).

## Context

`list` said whether a terminal is attached, not whether claude works or waits for the user. The
title's hooks already keep that in `@cld-status` ([decision 25](0025-title-follows-status.md)).

## Decision

`list`, its interactive list and the completion of `join -s` and `detach -s` show claude's status,
`busy`, `waiting` or `idle`, after the state.

### 49.1 Folded into STATE

As the maintainer chose, the status follows the state after a comma, `detached, waiting`, not in a
column of its own. A session with no status, `exited` or `ended` among them, shows its state alone.
Like the title ([decision 25.4](0025-title-follows-status.md)), it goes by the active pane, so a
pane split off beside claude's shows what claude left. STATE is as wide as its longest, at least
eight cells. A script that splits rows at spaces now finds two words; the [guide](../../guide.md)'s
Upgrading says so.

### 49.2 Order and emphasis

The rows keep the order of the names. The interactive list draws `waiting` in bold; the table has no
attributes, as scripts read it. Rejected, as the maintainer chose: `waiting` rows first, which would
move a row between two reads ([decision 14.6](0014-the-session-list.md)).

### 49.3 The read

`list` reads the option in the `list-sessions` it runs anyway, after a space in the state's field,
since an empty field would vanish. tmux writes only the three words there, and nothing for a dead
pane (tmux 3.5a and 3.7c; [tmux findings](../findings/tmux-sessions.md)). Not the busy mark
([decision 48.3](0048-restore-after-reboot.md)): it does not tell `waiting` from `busy`.

### 49.4 As of the read

A row's status, like its LAST ACTIVE, is as of the list's last read, so it does not change under a
key.

### 49.5 The hooks' limits

The title's limits ([decision 25.2](0025-title-follows-status.md)) now show. An interrupt as claude
writes, or a prompt a hook blocks, leaves `busy`. Nothing is set for a cld before 0.8.0, under
`disableAllHooks`, or before the first prompt. Nor under a policy that allows only managed hooks. No
event comes with the user's answer to a permission (claude 2.1.285;
[claude findings](../findings/claude.md)), so an allowed tool shows `waiting` until it has run. What
a refused permission leaves until the next event was not traced. A copy moved to the background from
a session of cld 0.10.0 or earlier keeps the hooks. It sets the status of any running session of its
name ([decision 16.10](0016-resume.md)). The idle sweep goes by LAST ACTIVE, not by the status
([decision 46.1](0046-idle-sessions.md)).

### 49.6 Completion

A running SUFFIX is described by the same words, an ended one `ended in DIR`
([decision 50.7](0050-one-command-join.md)).

### 49.7 Tests

The tests show sessions in each status, with none, and exited in a turn ([testing](../testing.md)).
They check the table, completion, and the list at two widths.

## Consequences

Out of scope: a status as claude starts, which a `SessionStart` hook could set, and reading the
sessions again on a timer, which [decision 14.6](0014-the-session-list.md) rules out.
