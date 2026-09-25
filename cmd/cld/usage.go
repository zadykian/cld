package main

// usage is what help, -h and --help print, for every command.
const usage = `usage: cld COMMAND [OPTIONS]

Run Claude Code in named sessions on a private tmux server that ignores ~/.tmux.conf.
Session NAME is the tmux session "cld-NAME", running "claude --name cld-NAME" with Remote
Control on. cld sees only the sessions it started; tmux -L cld ls lists every session on its
server.

commands:
  new [-n NAME] [-w]  create session NAME in the current directory and attach to it
  join [-n NAME]      attach to session NAME, detaching any other terminal from it
  kill [-n NAME]      end session NAME and the claude running in it
  list                list the sessions cld started: name, whether a terminal is
                      attached (or claude exited), and the directory claude is in
  help                show this help
  version             show the version

options:
  -n, --name NAME  the session: letters, digits, "_" and "-", starting with a letter
                   or digit; "main" when not given
  -w, --worktree   new: run claude in git worktree NAME, which claude creates from
                   HEAD or reopens (claude --worktree NAME)

Detach with C-q d; C-q C-q sends C-q to claude. With tmux 3.5 or newer, a session whose
claude fails stays, showing why, until cld kill ends it.
`
