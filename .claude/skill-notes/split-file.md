# split-file

cld's notes for the `split-file` skill, which came in for #125: every file passes every gate
outright, and the baselines stay empty.

## The caps

300 lines a Go or Markdown file, as `make sizecheck` checks. The Go comments' caps are in
[writing.md](../rules/writing.md).

## Go

- A new file's name is its group's, in lower case with underscores, such as `record_lock.go`.
- Each file keeps the package clause of the file it came from, and a test file its own: `pkg` or
  `pkg_test`.
- Build constraints, `//go:` directives and cgo preambles go with the declarations they govern.
  The ghostty driver's files, in `tests/internal/terminal`, build only with `//go:build ghostty`.
- Run `gofmt -w` on each file, and drop the imports it no longer uses: they fail the build.
  `goimports`, where installed, does both.
- A test file splits as [tests.md](../rules/tests.md)'s "Where a test goes" says, and each new
  file takes its line there.

## The linters

golangci-lint checks every line, the ghostty driver's too, as `.golangci.yml` sets it:

- `lll`: 100 columns;
- `gocognit`: a complexity of 20 at most, and `nestif`;
- revive's `function-length`: 40 statements and 60 lines at most;
- `errcheck`, with blank assignments checked too, and `nolintlint`: a `//nolint` names its linter
  and gives a reason.

## Checks

1. `gofmt -l .`, `go build ./...` and `go vet ./...`.
2. `make golangci-lint`, `make sizecheck FILES='OLD NEW...'` and `make vale FILES='OLD NEW...'`.
3. The tests of the package, narrowly: `cd tests && go test -count=1 -run 'A|B' .`, or
   `go test -count=1 ./tools/NAME` for a tool. CI runs the rest: run no full suite and no
   `make check` locally.
