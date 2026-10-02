# claude's options and worktrees

## The words after --

- cld refuses `--settings`, which would replace cld's: agent view off, the worktree's base, and the
  hooks of the title and the record. Put yours in a settings file.
- A short option counts at the start of a word, as claude reads it: `-pc` is `-p` then `-c`.
- claude looks for `--tmux`, `--bg` and `--background` after a second `--` too, and so does cld.
  Give a value that looks like an option after `=`; a prompt cannot start with one.
- claude's commands pass on, and end the session as they exit: `cld join -- mcp list`.
- `--bare` and `--safe-mode` drop cld's hooks. The title then shows no status, and cld resumes by
  name and forgets the session 30 days after it started.
- cld's words take 7 to 8 KB of tmux's 16364-byte command, and cld refuses words that would go over.
  Pass long text in a file, as with `--append-system-prompt-file`.

## Worktrees

- As with `claude --worktree`, `.worktreeinclude` files are copied in, and claude asks on exit
  whether to keep the worktree. Unlike it, the worktree branches from `HEAD`, whatever
  `worktree.baseRef` says.
- Where the worktree has gone, `cld join -s SUFFIX` resumes where `cld join -w` ran. With
  `-w --new`, it reopens the worktree with a new conversation.
- A worktree claude makes during a resumed session, for a subagent say, branches as your settings
  say.
- `cld join -w` counts sessions, not worktrees: 30 days after `api-0` last ran, it can make `api-0`
  again and reopen its worktree. Give `-s` for a new one.
- claude makes a worktree only where you have accepted workspace trust: run `claude` there once
  first.
