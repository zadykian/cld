# 21. Self-update

Status: Accepted. Amended by [22](0022-setting-completion-up.md).

## Context

Upgrading meant running the install command again, which does not know where cld lives. GitHub's
`releases/latest` answers 302 to the latest release's tag. A cld 0.4.0 updated itself to 0.5.0
through a symbolic link (curl 8.18.0; [environment findings](../findings/environment.md)).

## Decision

### 21.1 The latest release from the redirect

`cld update` reads the tag from that redirect without following it. GitHub's API allows 60 calls an
hour without a token, too few for a shared address. Versions compare by number; a current cld
downloads nothing, and a version that is no X.Y.Z, such as `dev`, is refused.

### 21.2 Go, not `install.sh`

cld knows its platform and its own file, and needs no curl or shell. It downloads from the release's
tag, so a release published meanwhile cannot mix in. An amd64 cld under Rosetta 2 stays amd64.

### 21.3 Replacing the file that runs

It replaces the file cld runs from, symbolic links resolved, by a rename once the new binary matches
its checksum and prints the release's version. A running cld keeps its old file.

### 21.4 Failures and signals

A failure leaves cld untouched. SIGINT, SIGTERM or SIGHUP before the rename cancels the update and
exits with 128 plus the signal's number, as a shell reports it.

### 21.5 Neither tmux nor claude

It runs no tmux or claude, so it makes none of their checks ([decision 6](0006-versions.md)). It
takes a proxy from the environment, waits 30 s at most for an answer to begin, and keeps redirects
on HTTPS.

### 21.6 Tests

The tests update a cld built as 0.4.0 from releases of their own (`CLD_RELEASES_URL`)
([testing](../testing.md)).

### 21.7 Completion scripts written anew

Once cld is replaced, the new cld writes anew the completion scripts that differ
([decision 22.5](0022-setting-completion-up.md)).

## Consequences

cld 0.5.0 and earlier have no `update`; the guide says to run the installer again. Out of scope:
checking without updating, picking a release (the installer's `CLD_VERSION` does), looking for
releases during other commands, and updating tmux or claude.
