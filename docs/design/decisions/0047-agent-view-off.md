# 47. Agent view is off in cld's sessions

Status: Accepted (#111). Amended by [50](0050-one-command-join.md).

## Context

Claude Code's agent view hands a conversation to claude's daemon: `/bg`, the move in `/exit`'s
dialog, a `/fork` into the background, and `←` on an empty prompt. Inside a session of cld's that
went badly. The daemon ran the conversation on as a copy while the session ended, or `←` left claude
in agent view. The copy kept cld's hooks, which name the session ([decision 16.10](0016-resume.md)).
The [claude findings](../findings/claude.md) hold claude's switch for it (2.1.232 and 2.1.285,
read). This amends [decision 42.2](0042-remote-control-is-claudes.md) and narrows 16.10.

## Decision

cld gives claude `"disableAgentView": true` in `--settings`, in every session. A claude started
without cld keeps agent view, so how claude starts chooses where a conversation runs.

### 47.1 Why in `--settings`

In a session of cld's, agent view is a way out of cld that leaves the hooks behind, not a way of
working cld offers. In `--settings` the switch needs no project setup and reaches the session's
claude alone. Rejected:

- `CLAUDE_CODE_DISABLE_AGENT_VIEW=1` in tmux's environment, which reaches every process of the
  server, a claude started by hand among them;
- `leftArrowOpensAgents` off, which leaves `/bg` and the dialog's move;
- a hint once a move has happened, too late for the session.

### 47.2 Not the user's to choose

No option turns it back on, and `--settings` after `--` stays refused
([41.2](0041-claude-options.md)). Flag settings outrank the user's and the project's. 42.2 now
reads: `--settings` carries what cld needs, agent view off among it, and nothing that is the user's
to choose.

### 47.3 Sessions from before

16.10 still holds for a session of cld 0.10.0 or earlier, and for a conversation moved before. The
[guide](../../guide/resuming.md) keeps the way back: `claude stop ID`, then `kill` where the session
stays, then `join`.

### 47.4 claude's floor

[Decision 6](0006-versions.md) stands: 2.1.232 has the setting, and checks it in the merged
settings, `--settings` among them.

### 47.5 The command's length

The key adds 24 bytes to tmux's command, counted against its limit ([41.5](0041-claude-options.md)).

### 47.6 The docs

The README and the guide say that agent view is off in a session and on in a claude started without
cld. Their comparison of the two becomes a choice, dated to claude 2.1.285.

### 47.7 Tests

The settings word for word ([25.7](0025-title-follows-status.md)) start with the key, also where cld
cannot write its record or finds no git ([testing](../testing.md)).

## Consequences

- A `claude agents` in another pane reads the user's settings, not cld's, and a daemon another
  claude started runs on.
- Unrun, as an interactive claude may connect to claude.ai ([testing](../testing.md)): `/bg`, `←`,
  `/exit`'s dialog and `/fork` under the setting, `ListAgents` and `SendMessage` between cld's
  sessions, and whatever else of claude needs the daemon.
- Out of scope: bridging cld's sessions and background sessions.
