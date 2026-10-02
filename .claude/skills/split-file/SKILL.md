---
name: split-file
description: >-
  Split a cld Go file over 300 lines into files by responsibility, in its package and with its API
  unchanged. Use when sizecheck reports a file over its line cap, or for #125.
argument-hint: FILE
---

# Split a file

Split `$ARGUMENTS` into files of 300 lines or fewer. The package, its exported names, their
signatures and their behaviour stay as they are: callers and tests change nothing.

## Plan

1. Read the file and its entry in `tools/sizecheck/baseline.txt`.
2. Group its declarations by responsibility: what a reader looks for together, such as the
   record's lock, a command's flags or the switch keys. Each group becomes a file named for it,
   such as `record_lock.go`; never `util.go` or `part2.go`.
3. A type stays with its methods. The package comment stays in one file, 15 lines or fewer;
   move a longer one's details to the files they explain, or to a decision record.
4. For a test file, group by the behaviour the tests cover, and keep helpers beside their tests.
   Shared helpers go in one file of their own.

## Move

- Move the code unchanged, then improve it in place. Every moved line is new to the golangci-lint
  ratchet, so it must pass everything: `lll` at 100 columns, `errcheck`, `nolintlint`.
- Break up a function over `gocognit` 20, or over revive's `function-length` of 40 statements or
  60 lines, into helpers named for what they do. The behaviour stays the same.
- Trim the comments as they move, as the `trim` skill does: purpose or reason only, blocks of 4
  lines or fewer, sentences of 30 words or fewer.
- A new file must pass every gate outright: no baseline entry for it.

## Baselines

Lower the split file's entries; they may only shrink.

```sh
go run ./tools/sizecheck -update   # lowers ceilings, drops stale entries
tools/valecheck -update            # drops files that now pass
git diff tools/                    # keep only the lines of the files you split
```

## Check

1. `gofmt -l .`, `go build ./...`, `go vet ./...`.
2. `make golangci-lint`, `make sizecheck FILES='OLD NEW...'`, `make vale FILES='OLD NEW...'`.
3. The tests of the package, narrowly: `cd tests && go test -count=1 -run 'A|B' .`. CI runs the
   rest; run no full suite and no `make check` locally.
4. Commit as `refactor(scope): split FILE by responsibility`, listing the new files in the body.
