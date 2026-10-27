# Testing

The tests build cld and run it against real tmux, which is local and cheap, so never faked. A
probe stands in for claude, and for the programs that would reach outside the sandbox. The
[overview](overview.md) maps the rest of the design record.

The rest has a page per subject, in `testing/`:

- [The terminal contract](testing/contract.md): the checks C1-C10, the drivers that run them in
  each terminal, and what they found.
- [The harness](testing/harness.md): the probe, the fakes and the sandbox, and what shapes them.
- [Status](testing/status.md): where the tests run, what was checked by hand or read rather than
  run, and what is not checked yet.

## Layers

1. **Static.** `make vet` runs gofmt, go vet, ShellCheck and shfmt. `make lint`, CI's `lint` job,
   adds golangci-lint, the size caps, Vale, rumdl, govulncheck, the workflow linters and lychee.
   Each checks every file, and any finding fails it, warnings included.
2. **Behaviour against real tmux.** The probe replaces claude, `tmux -V` for the version check,
   and `systemctl` and `loginctl` for `setup restore`
   ([decision 48](decisions/0048-restore-after-reboot.md)). As claude it sets the terminal modes
   claude sets, logs its arguments, directory, environment and raw input, and writes what claude
   writes on command.
3. **Terminal contract.** The same checks run against several outer terminals, through drivers
   ([the terminal contract](testing/contract.md)).

Isolation needs no seams in cld. `TMUX_TMPDIR` moves the sockets into a sandbox, `HOME` is a
temporary directory, `TMUX` is unset, and the probe comes first on the `PATH`. `cld join`
attaches, so it needs a pty: a driver's, or the sandbox's own where a test hands over to the fake
tmux ([decision 31.6](decisions/0031-a-terminal-to-attach-from.md)).

Vale checks Go comments in Markdown mirrors that `tools/valemirror` writes, line for line. Vale
3.23.0 reads Go itself through tree-sitter, but masks the name a doc comment starts with, and drops
the alerts anchored there. A first sentence of 40 words passed `Microsoft.SentenceLength` that way,
while a second sentence of 40 words failed. The mirrors mask the name alone, and its sentence
still counts it. Vale 3.23.0 also skips the text between two `$` of a Markdown paragraph, across
lines too: a sentence of 40 words between `$A` and `($)` passed. The mirrors write `¤` for `$`,
which Vale counts alike.

## CI

- Every push and pull request runs the `lint` job, and the contract on tmux, JediTerm and Ghostty's
  core in the Docker image, tmux built from source on `debian:trixie`. `blesh` runs the completion
  tests in bash with ble.sh, in the image built on Ubuntu without the Ghostty stage
  ([decision 27](decisions/0027-completion-with-blesh.md)). `macos` runs `make check` with
  Homebrew's tmux.
- The image runs 3.7c (`linux`) and 3.5a, the oldest cld runs on (`linux-oldest`, advisory). A
  developer runs the same image locally.
- A tag `vX.Y.Z` runs the checks and publishes a release.
- iTerm2, nightly and on tags, waits on its driver, and stays advisory until it proves stable.

Pull requests land on `main` by GitHub's merge commits: a pull request's own signed commits land as
they are, under a merge commit that GitHub signs. The ruleset on `main` allows the merge method
alone and requires branches to be up to date, so a merge's tree is the tree CI checked.
`/flow:land` merges once the required checks pass.
