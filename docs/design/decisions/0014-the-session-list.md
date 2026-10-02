# 14. The session list

Status: Accepted (#23). Amended by [15](0015-killing-from-the-list.md),
[23](0023-joining-beside-other-terminals.md), [40](0040-session-record.md),
[49](0049-status-in-the-list.md), [50](0050-one-command-join.md) and
[51](0051-moving-between-sessions.md).

## Context

Joining a session took reading `cld list` and typing its name. Claude Code's agent view
(`claude agents`) lists its background sessions to pick one. Its hints are lower case, joined by
` · `, drawn dim, and end in `esc to quit` (claude 2.1.282;
[claude's findings](../findings/claude.md)).

- tmux reflows a program's lines as a pane narrows (tmux 3.3a, 3.7c). A program that redraws
  relative to its cursor then draws over the wrong lines
  ([tmux's findings](../findings/tmux-sessions.md)).
- A tmux client starting on a terminal throws away output the terminal has not read yet. It then
  asks for primary device attributes (DA1) itself and takes the first answer
  ([tmux's terminal findings](../findings/tmux-terminal.md)).
- Go drops SIGTSTP once `os/signal` has had it (Go 1.27.1;
  [the environment's findings](../findings/environment.md)).

## Decision

On a terminal, `cld list` shows the sessions to pick one: `↑` and `↓` move, Enter joins, and Esc and
Ctrl+C leave with status 0. The dim footer follows agent view:
`↑/↓ to navigate · enter to join · ctrl+x to kill · esc to quit`. A message, such as why Enter could
not join, takes its place until the next key. Other keys do nothing, as agent view binds none. Keys
with Alt, which terminals send as Esc and the key, do nothing either.

### 14.1 When the list opens

The list opens when stdin and stdout are terminals, `TERM` is not `dumb`, and cld is in the
terminal's foreground. Otherwise `cld list` prints the table: to a pipe, for completion, and in the
background, where taking the terminal would stop the job. A script that leaves `cld list` the
terminal pipes the output to get the table. Rejected: a flag (`list -i`), which costs the main use
an option, and a bare `cld`, which would reverse [decision 3](0003-commands.md).

### 14.2 Where it draws

On the alternate screen, redrawn whole after each key and on SIGWINCH. Each line is cut at the
terminal's width in cells, and a control character shows as `?`. Esc and Ctrl+C print the table on
the way out, so the scrollback keeps what `cld list` printed before. Rejected: drawing in place
below the prompt. tmux's reflow breaks such a redraw, and recovering means clearing what the shell
showed.

### 14.3 In a pane of cld's servers

The list opens there as anywhere else, and Enter moves the terminal on the pane's session
([decision 51.6](0051-moving-between-sessions.md)). Before 51 it printed the table there, since
`join` refused to attach ([decision 2](0002-inside-another-tmux.md)).

### 14.4 Nothing to show

With no session, running or ended ([decision 40.3](0040-session-record.md)), `cld list` prints
nothing and exits 0: the list opens only with something to pick. Once its last row has gone, it
shows `no sessions`.

### 14.5 Enter joins

Enter does what `cld join` does: it joins beside a terminal attached
([decision 23](0023-joining-beside-other-terminals.md)), and shows an `exited` row's claude's last
words. Asking first, or refusing an exited row, would protect nothing, since joining ends nothing.

### 14.6 When it reads the sessions

On opening and after its own actions, a failed Enter or a kill; never on a timer or a key, so no row
changes under a key. A stale row costs at most a message, a kill that detaches a terminal the list
did not show, or joining a session made again under its name. After a read the selection stays on
its session, or else goes to the row that took its place.

### 14.7 Libraries

The list is Go ([decision 11](0011-go-and-cobra.md)) on `golang.org/x/term`, which the module
required already. It waits for input in `select`, since macOS's `poll` does not support terminals.
So it reads no key after its last: they stay for the shell, or for tmux. A 100 ms timer tells a lone
Esc from the start of an arrow's sequence. Cells are counted with `golang.org/x/text/width`.
Rejected: `go-runewidth` 0.0.23, which counts ambiguous characters as two under a CJK locale, and
Bubble Tea 2.0.10, which requires 17 modules.

### Enter's handover to tmux

Enter runs join's checks while the list still owns the terminal, then gives it back and runs `join`
itself under the record's lock ([decision 50.8](0050-one-command-join.md)). Before giving it back,
the list asks for DA1 and waits up to five seconds for the answer. A terminal answers once it has
read the list's last output, which a starting tmux would throw away. Under load that lost the main
screen and the title. The lookup runs beside the list, so a server that hangs does not hold it.

### Signals and stops

Every way out brings back the main screen, the cursor and the terminal's mode. A signal ends cld
with 128 plus its number. SIGTSTP puts the terminal back and stops cld, as less and vim do, and
SIGCONT redraws the list. Since Go drops SIGTSTP, cld stops itself with SIGSTOP. In an orphaned
process group, where nothing would continue it, cld goes on at once.

## Consequences

- A look at the sessions costs an Esc; `cld list | cat` still prints and returns.
- Every terminal the list runs on answers DA1, so only one that never answers waits the five
  seconds.
