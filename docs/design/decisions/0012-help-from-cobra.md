# 12. Help from cobra

Status: Accepted (#28). Amended by [17](0017-shell-completion.md), [18](0018-telemetry.md),
[22](0022-setting-completion-up.md), [24](0024-names-from-the-repository.md),
[42](0042-remote-control-is-claudes.md), [50](0050-one-command-join.md) and
[52](0052-telemetry-outside-cld.md).

## Context

The Go port kept the script's one usage text ([decision 11](0011-go-and-cobra.md)). Each feature
edited it by hand, beside option descriptions nobody saw, and the two could drift. cobra sorts
commands unless told not to, wraps nothing, and drops a write that fails
([environment findings](../findings/environment.md)).

## Decision

`help`, `-h` and `--help` print the help cobra generates from each command's `Use`, `Short` and
`Long` and its options, so a command's text lives with the command.

### 12.1 cobra's default templates

cld uses cobra's default templates with command sorting off, so the commands keep their order.
An option's usage names its value in backquotes. The texts break their lines by hand within 80
columns, which a test checks, but for cobra's own last line
([decision 22.6](0022-setting-completion-up.md)).

### 12.2 help COMMAND

`help [COMMAND]` shows the help of a command the root's help lists, or of one of that command's
own, as in `help setup telemetry` ([decision 18.8](0018-telemetry.md)). Anything else is refused
with status 2, where cobra would show the root's usage or pass over it. Completion offers the
command names ([decision 17.3](0017-shell-completion.md)).

### 12.3 Error messages

Error messages keep `(see cld help)`, so no message changes.

### 12.4 Options after an argument

Given first, `-h` and `--help` are `help` spelled otherwise. After an argument they are one more
argument, and refused. The test of the help's text refuses an option after an argument in any
usage line.

### 12.5 A help that cannot be written

The help goes to a buffer that cld prints itself, so a failed write ends cld with status 1
([decision 11.9](0011-go-and-cobra.md)).

### 12.6 The root's help

The root's help says what a session is, the private server, the detach keys and failed sessions.
A command's line there is short, and its own help says the rest.

## Consequences

- cobra's templates change with cobra, and the test of the help's text shows the change.
- A template of cld's own would have kept closer to what cld printed, but would be one more thing
  to keep.
- A `help` without an argument would have left help per command to `-h` alone.
