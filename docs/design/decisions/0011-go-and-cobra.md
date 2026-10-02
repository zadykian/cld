# 11. Go and cobra

Status: Accepted (#20). Amended by [12](0012-help-from-cobra.md),
[13](0013-a-server-per-session.md) and [33](0033-terminal-variables.md).

## Context

Up to 0.3.0 cld was a bash script. bash stepped in on its own: it changed the environment it
handed tmux, followed the locale in its name pattern, and wrote its own errors
([environment findings](../findings/environment.md)).

## Decision

cld is a Go program on [cobra](https://github.com/spf13/cobra) and pflag, a port of the 0.3.0
script. Commands, messages, exit statuses, output, tmux commands and tmux's environment stay,
but for the differences below.

### 11.1 pflag's spellings

cld takes what pflag takes, such as `-nNAME`, combined short options and `--help=false`, which
the script refused.

### 11.2 Arguments as pflag reports them

Where no test pins the message, an unexpected argument is quoted as pflag reports it, at times
less than was typed.

### 11.3 Arguments for go test

An argument starting with `-test.` is left to `go test`.

### 11.4 Empty arguments

An empty argument to `list`, `help` or `version` is refused. The script took it for the end of
the arguments.

### 11.5 Tools from absolute PATH entries

cld finds `tmux`, `claude` and `git` in the absolute `PATH` entries alone, and runs them and
`tty` from there. One found only through `.` or an empty entry counts as not installed. Go's
lookup stops at a match in a relative entry, so cld skips those entries itself. Within the
absolute entries cld searches as bash does, and with `PATH` unset it finds none.

### 11.6 The environment tmux gets

tmux gets the environment cld got, but for an empty `TMUX`
([decision 2](0002-inside-another-tmux.md)) and the variables that name the terminal
([decision 33](0033-terminal-variables.md)). cld hands on every other variable as it got it, without
bash's changes. It adds no `SHLVL`, sets `PWD` only where it enters another directory
([decision 40.4](0040-session-record.md)), and keeps `_`, `PS1`, `PS2`, `OLDPWD` and the variables
bash rewrote or dropped. So claude sees the environment as the cld that started the server got it.

### 11.7 Padding by characters

`list` pads a name by its characters, where bash padded by bytes.

### 11.8 ASCII names

Names are ASCII whatever the locale ([decision 1](0001-naming.md)). Under `en_US.UTF-8` the
script also took letters such as `é`.

### 11.9 Where bash stepped in

cld keeps bash's exit statuses, not its words. A failed write to stdout ends cld with status 1,
before any handover to tmux. A reader gone from its pipe ends cld by SIGPIPE, even where SIGPIPE
was ignored at startup: Go's runtime cannot tell the two apart. A tmux the system cannot run ends
cld with 127 for no such file, and 126 otherwise.

### 11.10 A directory that is gone

cld refuses to make a session in a directory that has been removed, or that it cannot enter
(#21). tmux would start claude in the home directory
([tmux findings](../findings/tmux-sessions.md)).

## Consequences

- Go gives one language for cld and its tests, no bash 3.2 to write for, cobra's completion, and
  key input with timeouts under a second. The cost is a binary per platform, and Go to build from
  a clone.
- Refusing pflag's other spellings before pflag sees them was the alternative to 11.1.
- Releases publish plain binaries and one `cld.sha256`, not archives, so installing stays one
  `curl` and a `chmod`.
- [Decision 13.4](0013-a-server-per-session.md) dropped the non-ASCII names of 11.7 and 11.8
  from `list`.
