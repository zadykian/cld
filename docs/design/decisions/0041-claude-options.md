# 41. claude's options

Status: Accepted (#61). Amended by [42](0042-remote-control-is-claudes.md),
[45](0045-resuming-a-copy.md), [47](0047-agent-view-off.md), [48](0048-restore-after-reboot.md),
[50](0050-one-command-join.md) and [51](0051-moving-between-sessions.md).

## Context

cld refused `--` ([decision 16.1](0016-resume.md)), so claude got no option but cld's. Some have no
other way in: `--mcp-config`, `--plugin-dir`, `--append-system-prompt`, a first prompt. Claude
Code's docs say to give `--mcp-config`, `--plugin-dir`, `--add-dir` and `--fallback-model` again on
resume ([claude findings](../findings/claude.md)).

## Decision

### 41.1 The words after --

`join` gives claude the words after `--` where it starts claude, and refuses them as lost where the
session runs ([decision 50.2](0050-one-command-join.md)). They go after cld's own arguments, so that
an option among them that takes the words after it, `--add-dir` say, takes none of cld's. Each goes
as one argv word, an empty one too. A session brought back starts in its entry's directory
([decision 40.4](0040-session-record.md)), so the guide says to give absolute paths. The other
commands refuse a `--`.

### 41.2 Refused options

cld refuses these with status 2, naming the word and why, before any tool is looked for. Their long
forms count too.

First, the options cld gives claude itself, of which claude keeps the last: `-n`, `-w` and
`--settings`. That one would replace cld's: the hooks of
[decisions 25](0025-title-follows-status.md), [26](0026-title-marks-worktree.md) and
[40.2](0040-session-record.md), the worktree's base ([4](0004-worktrees.md)) and agent view off
([47.2](0047-agent-view-off.md)).

Then those that resume a conversation, `-r`, `-c` and `--from-pr`, as the maintainer chose: claude's
most recent conversation, or a pull request's, need not be the session's.

Then those with which claude leaves the session. `-p`, `--bg`, `-h` and `-v` print and exit, as do
the hidden `--init-only` and `--rewind-files`. claude then exits with status 0, which ends the
session before anyone reads what it printed ([decision 5](0005-failures-stay-on-screen.md)).
`--tmux` goes to a tmux session of claude's own, and `--teleport` checks out a web session's branch.

### 41.3 What gives an option

A short option at the start of a word, with a value or more options after it, as claude reads
`-xyz`; a long one alone or with `=VALUE`. Every word counts, after a second `--` too. cld does not
track which of claude's options take a value, and claude's scans for `--tmux` and `--bg` read every
word. Rejected: letting a prompt start with `-p` after a second `--`, which would rest on claude's
other scans stopping there. Only a word's start counts, as claude 2.1.284's one other short option,
`-d`, takes the rest of the word as its value. Every short option that takes none is refused. A
value spelled like one goes after `=`, and a prompt starts with another word.

### 41.4 Not refused

claude's commands, such as `mcp`, which claude runs in place of a conversation; the maintainer chose
not to tell them from a prompt. Every other option passes, `--bare` and `--safe-mode` among them,
which leave out the title's hooks and the record's. So the session's entry keeps no ID but the one
`join` wrote, and the time cld wrote it. Not looked into: `--cloud` and `--environment`. A claude
that fails at startup stays on screen ([decision 5](0005-failures-stay-on-screen.md)). cld passes no
new option of its own, so [decision 6](0006-versions.md) stands.

### 41.5 The length of tmux's command

tmux takes a command of 16364 bytes at most. It fails on a longer one after cld has printed the
title, leaving its socket ([tmux session findings](../findings/tmux-sessions.md)). cld counts its
command as tmux does, with the record's hooks, the marks
([decision 48.1](0048-restore-after-reboot.md)) and the keys
([decision 51.1](0051-moving-between-sessions.md)). It refuses a longer one with status 2, naming
its size. The count comes before the terminal check
([decision 31.2](0031-a-terminal-to-attach-from.md)) and the entry, so a refusal leaves the record
unchanged. Only claude's words, or a long SESSION, make it so: cld's own take 7 to 8 KB by their
paths ([tmux session findings](../findings/tmux-sessions.md)). The keys take 420 bytes and six paths
of it. The message and the guide say to give claude long text in a file.

### 41.6 The help

`join`'s usage line ends in `[-- ARGS...]` after `[flags]`
([decision 12.4](0012-help-from-cobra.md)). Its help says what goes to claude and what is refused.

### 41.7 Tests

Tests check claude's words in tmux's command and each refused option in each form. Against the real
tmux, words past the limit leave no socket and no entry ([testing](../testing.md)).

## Consequences

Out of scope: completing claude's options after `--`, and keeping the words for a later resume,
which `restore` does not replay either ([decision 48](0048-restore-after-reboot.md)).
