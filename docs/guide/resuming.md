# Resuming a conversation

cld's record of a session, in `$XDG_STATE_HOME/cld`, holds claude's directory and the
conversation's ID. A hook writes the ID as claude starts the conversation, and after `/clear` or
`/resume`. The ID finds the conversation whatever its name has become.

## The record

- cld forgets a session 30 days after claude last started, answered or ended in it: claude's
  default `cleanupPeriodDays`. A value of your own is not read.
- A running server keeps the record. `cld kill` does not forget a session; `Ctrl+X` twice in
  `cld list` does.
- Where the directory has gone, `cld join` refuses, naming the `--resume ID` that resumes the
  conversation from here.
- Without a record, from an older cld or forgotten, `cld join` starts a new conversation.
  `--resume cld-S` resumes the old one by name.
- A record without an ID, where claude or tmux failed first, resumes by the name `cld-S`.
- claude finds a name in the directory or any checkout of its repository, and a session ID from
  anywhere. It looks in the history of the shell's `CLAUDE_CONFIG_DIR`.

## By name

- claude resumes by name where exactly one conversation has it, in any case. `/clear` keeps the name
  for the new conversation, and so does a later `cld join --new`. claude keeps a name unique only
  among the claudes running.
- With several, or none, claude opens its picker, which also lists names, branches and tags that
  contain the name: for `cld-rev`, `cld-review` too.
- The picker starts with the current checkout's conversations. Past its search box (`Enter` or
  `↓`), `Ctrl+W` shows every worktree's, `Ctrl+A` every project's, and `Ctrl+R` renames one.

## Copies and options

- Do not resume a conversation open in another claude without `--fork`: cld cannot tell, and the two
  would write one transcript, their messages interleaved
  ([Claude Code's sessions docs](https://code.claude.com/docs/en/sessions)).
- `--fork` starts the copy where `cld join` runs, outside its worktree, without its Remote Control
  session. The record gets the copy's ID.
- A copy can run beside the session that has the conversation open, as
  `cld join -s b --resume cld-api-0 --fork` beside `api-0`. claude also copies a conversation that
  runs in its background sessions. Neither is checked yet with claude running.
- `--fork` refuses the session's own name as `SESSION`: the copy would take that name too, and a
  resume by it would open the picker. Without `-s`, the index can give that name: give another `-s`.
- cld cannot tell a `SESSION` that names the same conversation by ID, nor check a `--fork-session`
  after `--`.
- Give the options a resume drops, which `cld help join` names, with absolute paths: claude starts
  in the session's directory. `Enter` in `cld list` resumes without them.
- Where another claude holds the conversation's Remote Control, the resumed one leaves it off until
  `/remote-control` moves it over.

## A conversation agent view moved out

A session of cld 0.10.0 or earlier keeps agent view until it ends. There `/bg`, `←` or `/exit`'s
"Move to background and exit" hands the conversation to claude's supervisor. The supervisor runs it
on as a copy under the same name:

- `/bg` and the dialog end the session. `←` leaves claude in agent view, and `cld kill` ends that
  claude but not the copy. `claude agents` lists the copies.
- While the copy runs, claude refuses `cld join` of the session, naming `claude attach ID` and
  `claude stop ID` (read in claude's code). The copy's title hooks still name the session, and
  fail after each tool while none runs (not checked yet).
- While the copy runs, a session of that name shows the copy's status in its title and `cld list`
  row. That is the session it left, or a later `cld join` of the index (not checked yet).
- To bring it back into cld, run `claude stop ID`, `cld kill` where the session stays, and
  `cld join`. Or resume a copy in another session: `cld join -s OTHER --resume cld-S --fork`.
  Neither is checked yet.
- `/config`'s `← opens agents` turns off that key alone.
