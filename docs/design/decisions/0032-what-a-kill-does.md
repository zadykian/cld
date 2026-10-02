# 32. What a kill does to claude

Status: Accepted (#66).

## Context

`kill` and the list's Ctrl+X ([decision 15](0015-killing-from-the-list.md)) end claude as a closing
terminal does. On SIGHUP claude kills its running shell commands, runs its `SessionEnd` hooks with
the reason `other` for 1.5 s to 60 s, and exits 129 (claude 2.1.284;
[claude findings](../findings/claude.md)). The kill does not wait: a stand-in for claude ran its
hook 1.5 s after `cld kill` returned (tmux 3.7c).

## Decision

### 32.1 No wait for claude

A wait for the pane's processes would be bounded only by claude's failsafe, some 70 s where a hook
asks for 60 s. The kill, and the list, would hang as long, which no closing terminal does. The help
of `kill` and the [user guide](../../guide.md) say that claude may still be shutting down.

### 32.2 No /exit first

Typed for the reason `prompt_input_exit`, the keys would land wherever claude's input is, a dialog
or a prompt half typed. In a `-w` session `/exit` opens a dialog that asks whether to keep the
worktree.

### 32.3 Tests

The probe has no shutdown of claude's. The help test compares `kill`'s help
([testing](../testing.md)).

## Consequences

A `join` right after a `kill` may start a claude beside the old one's hooks, as the user guide says.
