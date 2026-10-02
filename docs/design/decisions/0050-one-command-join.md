# 50. One command for a session

Status: Accepted (#114). Replaces `new` and `join` of [3](0003-commands.md) and `resume` of
[16](0016-resume.md). Amended by [51](0051-moving-between-sessions.md).

## Context

`new`, `resume` and `join` were three verbs for one intent, and picking one took knowing the
session's state. `join` refused a session that had ended, pointing at `resume`, and `new` and
`resume` refused one that ran. The list's Enter already went by the row's state
([decision 40.3](0040-session-record.md)).

This amends decisions [2](0002-inside-another-tmux.md), [4](0004-worktrees.md),
[5](0005-failures-stay-on-screen.md), [6](0006-versions.md), [12](0012-help-from-cobra.md),
[14](0014-the-session-list.md), [17](0017-shell-completion.md),
[23](0023-joining-beside-other-terminals.md), [24](0024-names-from-the-repository.md),
[31](0031-a-terminal-to-attach-from.md), [37](0037-sessions-of-another-repository.md),
[40](0040-session-record.md), [41](0041-claude-options.md), [43](0043-inside-your-own-tmux.md),
[44](0044-detach-command.md), [45](0045-resuming-a-copy.md), [46](0046-idle-sessions.md),
[47](0047-agent-view-off.md), [48](0048-restore-after-reboot.md) and
[49](0049-status-in-the-list.md).

## Decision

`join` attaches to a session where it runs, brings it back where it has ended, and creates it
otherwise.

### 50.1 By the session's state

- Where it runs, attached, detached or with claude exited, `join` attaches beside the other
  terminals ([decision 23](0023-joining-beside-other-terminals.md)) and starts no claude.
- Where it has ended, `join` brings it back as `resume` did: claude resumes the entry's conversation
  by ID, or else by name, in the entry's directory ([decision 40.4](0040-session-record.md)).
- Where there is none, or without `-s`, it creates the session here.

`--resume SESSION` creates it with claude resuming SESSION here, over an ended entry too, and
`--fork` makes a copy ([decision 45](0045-resuming-a-copy.md)). `--new` makes an ended session anew,
with a new conversation. A `join` of a session whose claude exited means to see it, so it attaches
and shows the `pane-died` hint ([decision 5](0005-failures-stay-on-screen.md)). SESSION is now an
option's value: `--resume` takes the next word, even one starting with `-`, so such a word is
refused, as is an empty one.

### 50.2 What would be lost is refused

`join` refuses an option it would drop, naming it, with status 1. On a session that runs, that is
`-w`, `--new`, `--resume` or the words after `--`. On one that has ended, `-w` is refused, since
claude takes a conversation back to its worktree itself ([decision 16.3](0016-resume.md)). With
`--new`, `-w` goes to the session made anew. The issue refused `-w` on any ended session, which
would leave no way to reopen a worktree with a new conversation; the maintainer's call stays open.
`--detach-others` is never lost.

A session of another repository, checked without `-n`
([decision 37](0037-sessions-of-another-repository.md)), is refused as such whatever the options. So
are a server without its session ([decision 13](0013-a-server-per-session.md)) and one cld did not
start ([decision 34](0034-servers-are-marked.md)).

The command line alone refuses what cannot go together, with status 2 before any tool. That is
`--fork` without `--resume`, `--new` or `-w` with `--resume`, a bad SESSION, and a copy given
SESSION's own name ([decision 45.1](0045-resuming-a-copy.md)). Among the words after `--`, claude's
options for what `join` itself does are refused ([decision 41.2](0041-claude-options.md)), each
saying which option of cld's to use.

### 50.3 The order

`join` refuses a live pane of cld's servers ([decision 2](0002-inside-another-tmux.md)) before any
lookup, which there would name a session only to refuse the terminal. Since
[decision 51.5](0051-moving-between-sessions.md) it moves that pane's terminal instead. `TMUX` stays
set until just before tmux runs, so the sweep keeps the session cld runs in
([decision 46.2](0046-idle-sessions.md)). `claude --version` ([decision 6](0006-versions.md)) runs
only where `join` starts claude, once the lookup has said so.

The command line comes first. With `-s`: tools and tmux, the pane, the name's default, then the
lookup under the lock. Without: `CLD_IDLE_DAYS`, tools (git with `-w`) and tmux, the pane,
`claude --version`, the lock, the index, 45.1's own name, the sweep. Tests pin this order. After the
pane check `join` reads what an outer tmux keeps ([decision 43](0043-inside-your-own-tmux.md)). The
terminal is checked last, before the title ([decision 31.2](0031-a-terminal-to-attach-from.md)),
whatever `join` was about to do.

### 50.4 One session for two joins at once

The record's lock ([decision 40.6](0040-session-record.md)) covers the lookup of `join -s`, so two
joins of one name make one session. The lock alone does not do it. Go opens the lock file
close-on-exec, and `flock` holds only while a descriptor is open, so the lock goes with cld's `exec`
before tmux has made the session. A second `join` then made it too, as did a `restore`, and the
list's forget removed what the first had written (tmux 3.7c;
[environment findings](../findings/environment.md)).

So an attached create leaves a start mark beside the entry: cld's process ID, which the tmux client
keeps, written under the lock just before the `exec`. tmux removes it once it has made the session
([decision 48.1](0048-restore-after-reboot.md)). A `join` that finds no session but a live mark
younger than 10 s lets the lock go, looks again every 50 ms for 10 s at most, and attaches.
`restore` leaves such a session to the `join`, and the list's forget refuses it.

- A mark is live where signal 0 gets any answer but `ESRCH`, `EPERM` included. A snap's tmux answers
  `EACCES` as it starts (Ubuntu's tmux 3.7c snap;
  [environment findings](../findings/environment.md)).
- All three read the mark before the lookup. Read after it, a mark removed meanwhile let a server
  tmux had started, without the session yet, pass for one that had outlived it
  ([decision 13](0013-a-server-per-session.md)).
- A mark left by a failed `new-session` names a client that has exited. One older than 10 s may name
  another process. Neither counts.
- A detached create, `restore`'s, holds the lock until tmux has made the session, and leaves no
  mark.
- The forget and the expiry remove the mark with the entry
  ([decision 40.6](0040-session-record.md)).

Not taken: keeping the lock across the `exec`, which would hand it to the tmux client while its
terminal stays attached. Nor the server's socket as the mark, which tmux makes before the session
and leaves after a failure.

### 50.5 The sweep

`join` without `-s` ends idle sessions once it has its index, under the lock, as `new` did. A failed
read there is only a warning. `join -s` sweeps none ([decision 46.2](0046-idle-sessions.md)).

### 50.6 `new` and `resume` dropped

They go with no pointer to `join`, and so does the hint of [decision 3](0003-commands.md) for an
unknown word that is a name, which would answer `cld new` with `cld new -n new`. Every unknown word
gets `unknown command`, and a bare `cld` names `join`. `join`'s help says what it does by state,
what it refuses as lost and what goes to claude, within 80 columns
([decision 12.4](0012-help-from-cobra.md)).

### 50.7 Completion

`join -n` and `-s` offer the sessions `list` shows, running and ended
([decision 24.8](0024-names-from-the-repository.md)). An ended SUFFIX is described `ended in DIR`, a
running one by its state and status ([decision 49.6](0049-status-in-the-list.md)). `detach` offers
those that run ([decision 44.4](0044-detach-command.md)), and `--resume` nothing, as SESSION is
claude's to find ([decision 16.4](0016-resume.md)).

### 50.8 The list's Enter

The list's Enter runs `join` for every row, and checks claude once it has handed the terminal over.
A row that has ended since the list's read comes back; one forgotten meanwhile is refused.

### 50.9 `restore`

`restore` brings a session back through `join`'s path, detached, keeping its run mark's time
([decision 48.5](0048-restore-after-reboot.md)). `join` writes the entry, the environment and the
marks where `new` and `resume` did. A `join` racing `restore` is held by the lock where `restore`
goes first, and by the start mark (50.4) where `join` does.

### 50.10 Messages

Messages and help that pointed at `resume` or `new` point at `join`. `create`'s refusal of a running
session now comes only from a race past the lock and the start mark.

### 50.11 Breaking

`new` and `resume` fail as unknown commands. `join` no longer refuses a session that has ended,
none, or a missing `-s`. The [guide](../../guide.md)'s Upgrading maps each old command line to the
new one.

### 50.12 Tests

They cover `join` in each state, the refusals of 50.2, and the races of 50.4
([testing](../testing.md)). Two joins of one session, or a `join` and `restore`, make one session
with one claude. Without the start mark they make two, but for `restore` first.

## Consequences

Joining no longer takes knowing the session's state, at the cost of a breaking change (50.11). Out
of scope: a key in the list for `--new` or `--resume`, and other footer words on an ended row.
