# 20. An install script

Status: Accepted.

## Context

The README's two install lines curled a binary picked from `uname` straight over
`~/.local/bin/cld`, unchecked, for `x86_64` and `aarch64` only. GitHub redirects
`releases/latest/download/NAME` to the latest release's tag (curl 8.18.0;
[environment findings](../findings/environment.md)).

## Decision

### 20.1 A release asset

`install.sh` ships in each release, so it matches that release's binaries and is tested before
publishing; one read from `main` could run ahead of them. `CLD_VERSION` picks another release,
0.4.0 or later, the first with binaries.

### 20.2 POSIX sh, safe to cut short

It runs under any POSIX sh, needing curl and `sha256sum` or `shasum`. All its code sits in functions
that the last line calls, so a download cut short runs nothing.

### 20.3 The binary from `uname`

It takes Linux and macOS on amd64 and arm64. Under Rosetta 2 it picks arm64, which runs natively
(Apple's documentation; not probed).

### 20.4 Checked, then renamed

The binary downloads beside its destination, and becomes `cld` by a rename once it matches its
checksum and runs. A running cld keeps its old file, a `noexec` `/tmp` does not matter, and a
failure changes nothing. The rename replaces what is at `DIR/cld`, a symbolic link too, never the
file it points to; `cld update` replaces the file that runs ([decision 21.3](0021-self-update.md)).

### 20.5 Saying where cld went

It warns where the directory is missing from the `PATH` or another `cld` comes first, and edits no
shell profile.

### 20.6 Tests

The tests serve releases of their own (`CLD_RELEASES_URL`) under a fake `uname`, and cut the script
short at every line ([testing](../testing.md)).

## Consequences

`install.sh` and `cld update` ([decision 21](0021-self-update.md)) share the file names `make dist`
publishes. Out of scope:

- wget;
- checking tmux and claude, which cld does as it starts ([decision 6](0006-versions.md));
- shell profiles;
- signatures, as a checksum from the same release catches only a broken download.
