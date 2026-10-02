# 30. Links

Status: Accepted (#60).

## Context

claude marks file paths and URLs as OSC 8 links under tmux 3.4 or newer. tmux writes a link only to
a terminal with the `hyperlinks` feature. It gives that by XTVERSION to iTerm2, foot and tmux alone
(claude 2.1.284, tmux 3.7c; [tmux terminal findings](../findings/tmux-terminal.md)). So links were
text in kitty, Ghostty, WezTerm, Alacritty, VTE's terminals, Konsole, Windows Terminal, VS Code and
JediTerm, though each takes OSC 8 ([terminal findings](../findings/terminals.md)).

## Decision

### 30.1 The feature by TERM

cld adds `terminal-features` entries at fixed indexes past tmux's defaults, so that two starts keep
one each. `xterm*:extkeys:hyperlinks` goes at 100, the entry that had `extkeys` alone
([overview](../overview.md#the-servers-options)). `wezterm:hyperlinks` goes at 101, for WezTerm
where its `term` sets that `TERM`, and `alacritty:hyperlinks` at 102, where that terminfo entry is
installed. `xterm*` covers kitty, Ghostty and the terminals that set `xterm-256color`. An entry's
features are separated by `:`, as with `,` tmux takes neither.

### 30.2 Terminals without links

A terminal that takes no links ignores the code and shows the text, as xterm does. Those that garble
links are old releases, or set no `xterm*` `TERM`.

### 30.3 The terminal's click

The terminal opens a link with its own click, so links need no Ctrl+click reaching claude
([decision 35](0035-modifier-clicks.md)). With `mouse on` the terminal reports clicks to tmux, so
where it opens links only with the modifier that keeps a click from the program, that modifier opens
them. Which click does so in each terminal was not checked ([testing](../testing.md)).

### 30.4 Tests

C2 expects `hyperlinks` on both terminals, and C5 a link the probe writes in the terminal's output.
The baseline outer tmux has the feature by XTVERSION whatever cld sets, so only JediTerm tests cld's
entry. It failed both without it ([testing](../testing.md)).

## Consequences

A terminal whose `TERM` matches no entry, and that tmux does not know by XTVERSION, still gets text.
A session an older cld started keeps its links text until it ends.
