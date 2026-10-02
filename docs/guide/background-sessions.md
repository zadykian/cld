# cld and Claude Code's background sessions

Claude Code keeps conversations without a terminal itself: `claude --bg PROMPT`, `/bg` or `←` on an
empty prompt hands one to a supervisor. `claude attach ID` opens it in any terminal, and
[agent view](https://code.claude.com/docs/en/agent-view), `claude agents`, lists them.

How you start claude picks one or the other: cld turns agent view off in its sessions, and a claude
started without cld keeps it. As of Claude Code 2.1.285; agent view is a research preview, and its
docs say what changed since.

| | cld | Background sessions |
|---|---|---|
| Needs | cld and tmux, of the versions the [README](../../README.md) names | claude alone; `disableAgentView`, or `CLAUDE_CODE_DISABLE_AGENT_VIEW`, turns them off |
| Address | a name, `-s SUFFIX` in its repository, which TAB completes | an ID of 8 hex digits, or its start: `claude attach 7c5d` |
| Coming back | claude as you left it, in the renderer `/tui` chose | always fullscreen; scrollback and copy mode see only the screen |
| Idle | claude runs until you end it, or 30 days without a terminal (see [idle sessions](idle-sessions.md)) | the supervisor stops claude after about an hour done and unattached, unless pinned (`Ctrl+T`) |
| claude crashes | the session stays, with claude's last screen, `exited` in `cld list` | the supervisor starts claude again; `claude logs ID` shows its output |
| Reboot | `cld restore` resumes each conversation, and continues a turn the reboot cut off | the session shows failed (stopped after 48 hours); attaching resumes it |
| Listing | `cld list`: name, state, claude's status, last activity, directory | `claude agents`: state, activity, age; attach, peek, reply, dispatch, stop |

- Agent view does not show a cld session, nor does `cld list` show background sessions.
- With agent view on in a session, `claude agents --json` of 2.1.284 listed it as
  `"kind": "interactive"`, without the `id` that `claude attach` takes. Not checked since.
- A session of cld 0.10.0 or earlier keeps agent view until it ends: see
  [resuming a conversation](resuming.md) to bring a conversation moved out back into cld.
