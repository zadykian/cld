# 61. Running in another directory

Status: Accepted (#186).

## Context

`join` works in the current directory: it makes a session there, and takes `NAME`'s default and the
session's home from it (decisions [24.3](0024-names-from-the-repository.md) and
[37.1](0037-sessions-of-another-repository.md)). `setup config project` sets up the project there
([decision 19.3](0019-project-settings.md)). In a shell, `cd DIR && cld join` does it in DIR, and
leaves the shell there.

`--host HOST`, the next part of #186 (decision 62), runs a command of cld's on another host through
ssh. There the command is cld's words alone, and starts in the remote home, so no `cd` can come
first.

This amends [decision 51.3](0051-moving-between-sessions.md).

## Decision

`join` and `setup config project` take `-C, --directory DIR`, and run as if in DIR.

### 61.1 The option

The option is `-C`, as git, make and tar name it, and `--directory`, as make and tar do. No other
command takes it (61.4).

cld enters DIR as POSIX's `cd` does, and dash's: a relative DIR from the current directory as
`PWD` names it, and `..` taken from the name. It then sets `PWD` to the result. So the session's
directory and home, and `NAME`'s default, are those `cd DIR && cld join` gives, from a symbolic
link too. Where that path does not exist, bash's `cd` tries DIR as typed, and cld does not.

`-s`'s completion takes `NAME`'s default and the home from DIR too, where the line has `-C`.

Only the current directory changes. An ended session still comes back in the directory claude was
last in ([decision 56](0056-following-claudes-cd.md)). Where that is gone, `join`'s advice resumes
the conversation with `-C` and the directory DIR named, since the user's shell is elsewhere.

Not taken: DIR as the system takes it, as `git -C` does. From a symbolic link, `..` would go up
from the link's target, where `cd ..` goes back to the link's parent (git 2.53.0;
[environment findings](../findings/environment.md)).

### 61.2 Order and statuses

cld enters DIR once the command line's own checks pass, and before any tool runs.

- An empty DIR, as in `-C ''` or `--directory=`, is refused with status 2:
  `option '-C' needs a value (see cld help)`. An empty `--resume` is refused so too
  ([decision 50.1](0050-one-command-join.md)).
- A DIR cld cannot enter is refused with status 1, as typed, with the system's reason:
  `cannot enter DIR: no such file or directory`, or `not a directory` for a file. `join --moved`
  refuses its directory alike (51.3).
- A relative DIR where the current directory is gone is refused with status 1, as `join` refuses
  that directory: `the current directory no longer exists`. An absolute DIR needs none.

### 61.3 Moves

In a pane of cld's servers, `join` moves the terminal
([decision 51.5](0051-moving-between-sessions.md)). It enters DIR first, so it looks the session up
from there, and refuses a DIR it cannot enter with the terminal staying.

The terminal's `join --moved` runs the words typed, `-C` among them, in the directory `join` ran in
(51.3). That is now the directory before `-C`, so a relative `-C` applies once. From DIR, `-C api`
would reach `api/api`.

Not taken: carrying DIR, and the words without `-C`. cld would have to find `-C` among them in each
of its forms, such as `-C DIR`, `-CDIR` and `--directory=DIR`.

### 61.4 Not taken

- Completing DIR: no argument of cld's completes a file name
  ([decision 17.3](0017-shell-completion.md)).
- `-C` for `detach` and `kill`, where `-n` names a session from anywhere. The other commands do not
  depend on the current directory.
- Expanding `~`, which is the shell's: it expands it on the machine the user types on. A `~` the
  shell leaves, quoted or after `=`, is a name, as for `git -C`
  ([environment findings](../findings/environment.md)).

## Consequences

- Users run `cld join -C DIR` and `cld setup config project -C DIR` from anywhere, and the shell
  keeps its own directory. `--host` can then name a directory on the host.
- A command that takes `-C` enters DIR after its own checks, and before anything reads the current
  directory. A move carries the directory before `-C`.
- The tests make a session with `join -C`, its `NAME` from DIR, by an absolute, a relative and a
  `..` DIR. They refuse a missing DIR and a file with status 1, and an empty value with 2.
- They move a terminal with a relative `-C`, which reaches DIR once, and check that
  `setup config project -C` writes DIR's files.
- The help of both commands names `-C`, and the guide's [names](../../guide/sessions.md#names) say
  how DIR is read.
