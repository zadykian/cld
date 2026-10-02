# 33. The terminal's variables

Status: Accepted (#67).

## Context

A tmux server keeps the environment of the client that started it, but for `TERM` and `TERM_PROGRAM`
(tmux 3.7c; [tmux session findings](../findings/tmux-sessions.md)). claude reads several variables
before `TERM_PROGRAM` to tell its terminal (claude 2.1.284;
[claude findings](../findings/claude.md)). So claude in a session created in Cursor's terminal, or a
JetBrains IDE's on macOS, took itself to be there and left extended keys off: Shift+Enter submitted.

## Decision

`join` runs tmux, and so the server, without the variables below.

### 33.1 Each whatever its value

`CURSOR_TRACE_ID`, `__CFBundleIdentifier`, `VisualStudioVersion` and `TERMINAL_EMULATOR` name the
terminal the session was created in, which any other can join. `__CFBundleIdentifier` names every
macOS app a shell runs in, iTerm2 too, which claude also reads for its iTerm2 clipboard offer.

### 33.2 VS Code's helpers as units

`VSCODE_GIT_ASKPASS_MAIN` names Cursor, Windsurf or Antigravity to claude. It belongs to VS Code's
askpass, which asks through `VSCODE_GIT_IPC_HANDLE`, the creating window's socket, and fails with a
part missing. So every `VSCODE_GIT_ASKPASS_*` goes, with `VSCODE_GIT_IPC_HANDLE` and a `GIT_ASKPASS`
beside the `MAIN`. VS Code's git editor asks through the same socket, so every `VSCODE_GIT_EDITOR_*`
goes too, with a `GIT_EDITOR` naming a script beside its `MAIN`
([terminal findings](../findings/terminals.md)). A `GIT_ASKPASS` or `GIT_EDITOR` elsewhere is the
user's own and stays.

The editor names no terminal to claude, whose Bash tool runs git with `GIT_EDITOR=true`. It goes for
the other programs on the server, such as a shell in a window of its own, as it fails at once
without the creating window's socket.

### 33.3 The rest stays

The rest stays as the creating shell had it, for claude's life
([decision 13](0013-a-server-per-session.md)). A later `join` updates `SSH_AUTH_SOCK`, `DISPLAY` and
tmux's other `update-environment` variables for what starts later, not for claude. The
[user guide](../../guide.md) says so, with the ways around it.

### 33.4 Not done

Dropping all that claude's own background sessions drop, such as `LC_TERMINAL` or `SSH_CONNECTION`,
which claude reads after `TERM_PROGRAM` or for other things. The maintainer chose the variables that
change which terminal claude takes itself to be in.

### 33.5 Tests

Tests check tmux's command, VS Code's helpers beside the user's own, and that neither claude nor the
server sees any of the variables (C6; [testing](../testing.md)).

## Consequences

git on the server asks as it would outside VS Code, not in a window that may have closed.
