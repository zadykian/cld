# 1. Naming

Status: Accepted. Amended by [13](0013-a-server-per-session.md) and
[24](0024-names-from-the-repository.md).

## Context

tmux takes `.` and `:` in a target for separators, so it renames a session `cld-foo.bar` to
`cld-foo_bar`, while claude's `--name` and the tab's title keep the dot
([tmux findings](../findings/tmux-sessions.md)).

## Decision

- A name matches `^[A-Za-z0-9][A-Za-z0-9_-]*$`, in ASCII whatever the locale
  ([decision 11.8](0011-go-and-cobra.md)). cld checks a name and refuses a bad one; it never
  sanitises one.
- A name has at most 64 characters, so that its server's socket path fits in `sun_path`
  ([decision 13.2](0013-a-server-per-session.md)).
- A session's name is `NAME-SUFFIX`. The one name cld changes is `NAME`'s default, made from
  the repository's or the directory's name, since nobody typed it
  ([decision 24.4](0024-names-from-the-repository.md)).

## Consequences

A silent rename would make `cld foo.bar` and the session it attaches to disagree. A refusal says
what is wrong instead.
