# 27. Completion in bash with ble.sh

Status: Accepted.

## Context

ble.sh takes readline's place in bash, and the maintainer uses it. Through cld's script
([decision 22](0022-setting-completion-up.md)) it offered file names wherever cld offers none,
against [decision 17.3](0017-shell-completion.md). This decision stops that, amending
[decision 22.1](0022-setting-completion-up.md).

Probed with ble.sh 0.4.0 (Ubuntu 26.04's and a 2026 nightly), bash 5.3.9 and cobra 1.10.2
([environment findings](../findings/environment.md)):

- cobra's script turns file names off only where `compopt` is a builtin, which ble.sh replaces
  while it completes;
- ble.sh adds completions of its own where a function offers none, unless told not to;
- turning both off at the end of cobra's function offers nothing, and changes nothing else;
- ble.sh 0.3.4 offers file names there whatever a function does.

## Decision

### 27.1 Five lines in bash's script

`completion bash`, and so `setup completion bash`, adds five lines to cobra's function. Under
ble.sh, where cld offers no file names, they do what cobra's lines do in bash, and turn off ble.sh's
own completions. Elsewhere they do nothing; the other shells' scripts stay cobra's.

### 27.2 Where the lines go

cld puts them before the `}` that ends `__start_cld`, right after
`__cld_process_completion_results`, and panics without that point. So a bump of cobra that moves it
fails the tests. The script without descriptions gets them too. It still starts as cobra's does, so
`update` writes it anew ([decision 22.5](0022-setting-completion-up.md)).

### 27.3 Other ways, rejected

A fix in ble.sh's adapter for cobra would leave users waiting on a release, and still offer bash's
file names. A bash script of cld's own is one more copy to keep in step
([decision 17.1](0017-shell-completion.md)). cld edits no `~/.bashrc`
([decision 22.4](0022-setting-completion-up.md)).

### 27.4 Not done

`--mcp=SERVER` and `-n=NAME` completed file names under ble.sh while descriptions are on, and with
the lines complete nothing. That is the adapter's to fix; the guide says to write `-n NAME`. ble.sh
0.3 keeps offering file names.

### 27.5 Tests

A test types into bash with ble.sh in a tmux pane. TAB runs a widget that completes and writes the
line to a file, since ble.sh cancels a completion on any key. The test waits for ble.sh's prompt, as
keys before it are lost. It makes the session itself, since a pane gets the client's `PATH`
([tmux session findings](../findings/tmux-sessions.md)). The sandbox's home has a `~/.cache`,
without which ble.sh does not load as CI's user. `make docker-blesh-check` and CI's `blesh` job run
it on Ubuntu, where a skip fails ([testing](../testing.md)).

## Consequences

bash's script is no longer cobra's alone, so each bump of cobra must keep that fixed point. The test
needs an image and a CI job of its own.
