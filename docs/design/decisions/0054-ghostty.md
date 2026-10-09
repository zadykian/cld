# 54. Ghostty

Status: Accepted (#162). Amended by [55](0055-passthrough-from-claudes-pane.md), under which
54.5's beacon goes once.

## Context

The terminal contract ran against tmux and JediTerm alone ([testing](../testing.md)). Ghostty is
one of the terminals claude turns extended keys on for, and notifies through
([claude findings](../findings/claude.md)). tmux does not recognise it, so its `extkeys` comes from
cld's entry for `xterm*` ([tmux terminal findings](../findings/tmux-terminal.md)).

Ghostty publishes its terminal core as libghostty-vt, in Zig with a C API, with no tagged release.
`go.mitchellh.com/libghostty` binds it through cgo, its API not yet stable, and pins the Ghostty
commit it binds. A spike (#162) built that commit with Zig 0.16.0 in the test image, and a driver
on it passed C1 to C10.

The app adds to the core: its answers to tmux's queries, its environment and two default modes. It
reports focus at each request, and a notch of the wheel as three
([terminal findings](../findings/terminals.md)).

## Decision

The terminal contract runs in Ghostty's core too, as the terminal `ghostty`.

### 54.1 The core, not the app

The driver runs libghostty-vt in the test process, on a pty of the test's, and copies what the app
adds, read in its source at the pinned commit. Keybindings and link clicks stay unchecked, as the
IDE around JediTerm does.

### 54.2 The pin

`tests/ghostty/deps.txt` pins Zig by SHA-256 and Ghostty by commit, fetched with git and checked:
GitHub may change the bytes of the archives it generates. `go.mod` pins the bindings that name the
same commit. They move together by hand, as JediTerm does ([decision 7](0007-jediterm-pin.md)). A
test compares the two commits, and Dependabot leaves the bindings alone.

The driver answers XTVERSION with the version in `build.zig.zon` at that commit, which
`tests/ghostty/build-lib` records. The app built there would add build metadata, and libghostty-vt
has a version of its own ([terminal findings](../findings/terminals.md#ghostty)).

### 54.3 Behind a build tag

Only a build with the tag `ghostty` has the driver, which needs cgo and the library. `make test`
adds the tag where `TERMINALS` lists `ghostty`; otherwise the tests stop before running, saying
so. A stage of the Docker image builds the static library and Ghostty's terminfo entry,
`xterm-ghostty`. cld itself still builds with `CGO_ENABLED=0`.

### 54.4 Linted all the same

golangci-lint and govulncheck build the tag. `tools/run` fetches libghostty-vt's headers alone
from the pinned commit, and writes a pkg-config file naming them. Type-checking needs neither Zig
nor the library, only a C compiler and pkg-config, which macOS lacks by default. `make vet` stays
untagged, so `make check` needs nothing new.

### 54.5 A faithful driver, and tests that wait

The driver reports focus at each request, as the app does. tmux asks again at each answer to its
queries, primary and secondary device attributes and then XTVERSION. A report between the prefix
and its key returns tmux to its root table, and the key goes to claude
([tmux terminal findings](../findings/tmux-terminal.md)).

So a test that types a prefix just after a client attaches waits first:

- until `#{client_termtype}` has the terminal's answer to XTVERSION, the last query;
- then until a beacon that claude writes, in tmux passthrough, reaches the terminal's output.

tmux writes to a client in order, so the beacon follows every request, and the driver answers each
before it reads on. tmux drops passthrough while a redraw is due, so the beacon goes again until it
arrives. JediTerm answers no XTVERSION, and reports no focus either. With the wait left out, the
prefix tests failed 8 runs in 30 under load.

Holding the reports back in the driver while it types was not taken: it would hide what the app
does.

### 54.6 Not done

- The app itself: on Linux it needs a display and OpenGL, and offers no way to read the screen.
- A driver module of its own: `make lint`, govulncheck and Dependabot would not reach it.
- libghostty-vt as WebAssembly in a Go runtime: no bindings exist for it.
- arm64: the image's stage has Zig's sums for it, but no run has checked it.

## Consequences

`linux` and `linux-oldest` run the contract in Ghostty beside tmux and JediTerm. Uncached, the
Ghostty stage took 118 s to 153 s, and adds about a minute to the 3.7c image, as it builds beside
the main stage. `go.mod` gains a cgo module that only the tests import, which
govulncheck now scans. A bump moves the commit, the bindings and the findings together. The first
`make lint` fetches the headers, 1.4 MB over git.

Left for a change to `.github/workflows`, which the maintainer pushes by hand: the `blesh` image
builds the Ghostty stage it never uses, about 70 s uncached, and `ci.yml`'s comment on build times.
