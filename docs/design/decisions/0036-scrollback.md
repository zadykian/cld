# 36. Scrollback

Status: Accepted (#77).

## Context

While a terminal is attached, tmux draws in its alternate screen, and the terminal's scrollback gets
nothing (tmux 3.7c; [tmux terminal findings](../findings/tmux-terminal.md)). What claude's classic
renderer leaves in the scrollback is then in the pane's history alone, which the wheel and `C-q [`
open. The fullscreen renderer scrolls its own transcript (claude 2.1.284;
[claude findings](../findings/claude.md)). tmux keeps 2000 lines by default.

## Decision

### 36.1 Set before the session

A server keeps 50000 lines of a pane's history (`history-limit`), set before `new-session`. tmux 3.7
gives an existing pane a new limit, but 3.5 and 3.6 do not
([tmux session findings](../findings/tmux-sessions.md)). So the limit holds on the oldest tmux cld
runs on ([decision 6](0006-versions.md)).

### 36.2 50000 lines

As the issue proposed. The server holds about 35 MB for 50000 lines of 100 plain characters, 156 MB
where each has an RGB colour (tmux 3.7c), freed with the session.

### 36.3 The docs

The README says which renderer the wheel and the history serve. The [user guide](../../guide.md)
says how to read the history back, for screen readers too. Claude Code's screen-reader mode, always
the classic renderer, relies on the terminal's scrollback and on OSC 133 marks, which tmux keeps to
itself.

### 36.4 Not done

Binding copy mode's `previous-prompt` and `next-prompt` to jump between the turns claude marks.
Keeping tmux out of the terminal's alternate screen (`terminal-overrides` without `smcup`), not
tried.

### 36.5 Tests

Tests pin the option, the limit of claude's pane and its place in tmux's command
([testing](../testing.md)).

## Consequences

The sessions claude makes on its server get the limit too.
