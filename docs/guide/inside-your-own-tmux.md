# Inside your own tmux

Your tmux gets the terminal's keys before cld's session does. Unless set up for it, it keeps from
claude:

- its prefix, `C-b` by default, claude's key to background a task: `C-b C-b` sends one on. A
  second prefix (`prefix2`) is kept too;
- Shift+Enter, which arrives as Enter: `Ctrl+J`, or `\` then Enter, starts a new line. Ctrl+Enter
  and the like lose their modifier too;
- clipboard copies, focus events, and links to a terminal it does not know takes them;
- the notifications OSC 9, 99 and 777, whatever its settings. The bell gets through.

Where your tmux keeps a key, the session's last line names the keys once attached, until a key:

```text
your tmux keeps C-b and Shift+Enter: see "Inside your own tmux" in cld's guide
```

- claude can draw over the line as it starts. tmux draws it again one and three seconds later, and
  `C-q ~` lists the messages tmux has shown.
- The line comes back at each attach, but not after a move to another session, whose `cld join` no
  longer knows your tmux. The keys stay kept.
- cld shows no line where its own tmux is 3.5, nor where `TMUX` does not name your tmux, as over
  ssh from one of its panes.

## Setting your tmux up

These lines in `~/.tmux.conf` bring back all but the prefix and those notifications:

```
set -s extended-keys on
set -as terminal-features 'xterm*:extkeys:hyperlinks'
set -s set-clipboard on
set -s focus-events on
```

- Where cld's own tmux is 3.5 or 3.6, write `set -s extended-keys always`. Only from 3.7 does cld's
  tmux ask yours for modified keys, which `on` sends only to a program that asks. With `always`,
  every program in your tmux gets them.
- The second line tells your tmux that a terminal whose `TERM` starts with `xterm` sends modified
  keys and takes links, which tmux knows of few terminals. Where your `TERM` differs, put it in
  place of `xterm*`.
- With `set-clipboard on`, any program in your tmux can set the clipboard, not claude alone.
- A terminal attached before keeps what it had. After `tmux source ~/.tmux.conf`, attach to your
  tmux again, then join the session again.
