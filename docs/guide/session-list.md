# The session list and moving between sessions

## cld list

- After the first `Ctrl+X`, `Esc`, two seconds or any other key keep the session; that key then
  does what it does. A held `Ctrl+X` stops at one session.
- The list reads the sessions as it opens and after a kill. `Enter` on a session that ended since
  brings it back. Where it has gone, or ended before the second `Ctrl+X`, the list says so.
- `Ctrl+X` twice on an `ended` row forgets the session, and its conversation stays in claude's
  history. After a kill by mistake, `Enter` on its row brings the session back.
- claude's status shows `waiting` in bold. There is none before claude's first prompt, once claude
  has exited, or without hooks; some changes escape them (see [the tab's title](terminal.md)).
- `attached` counts terminals only: someone on the session through Remote Control does not show,
  and a kill ends the session for them too.
- `cld list` prints the table where its input or output is no terminal, `TERM` is unset or `dumb`,
  or it runs in the background. A script that leaves it the terminal gets the list: pipe it.
- In a JetBrains IDE, `Esc` and `Ctrl+X` reach the list as its keymap says; `Ctrl+C` also leaves.

## Moving between sessions

- The terminal leaves its session and runs `cld join` in its own environment, as after `C-q d`. A
  session it creates gets the terminal's variables, none of claude's.
- `! cld join` runs in claude's directory, so the session's name comes from there.
- What `cld join` refuses before it starts claude, `! cld join` refuses in claude, and the terminal
  stays. What it refuses later, such as a claude too old, shows in the terminal, which has left:
  run `cld join` again there.
- Of several terminals on the session, the one used last moves, as for `! cld detach`.
- `C-q s` ends no idle session: it runs with the server's environment, whose `CLD_IDLE_DAYS` may be
  out of date.
- `C-q L` goes back where the last terminal to arrive on the session came from, whichever terminal
  presses it. Where there is nowhere to go, the message line says so for three seconds.
- tmux runs the terminal's `cld join` with your shell, which cld supports for sh, bash, zsh, fish,
  ksh and csh; ksh and csh are not checked yet. With another, such as nu or pwsh, cld says why.
- Not checked yet with real terminals and the real claude, only in the tests' terminals with a
  stand-in for claude.
- A session started before cld 0.11.0 keeps tmux's own keys until it ends: `cld kill` it and
  `cld join` it again.
