# trim

cld's notes for the `trim` skill.

- [writing.md](../rules/writing.md) adds the Go comments' caps and form to the `writing` skill's
  policy. [docs.md](../rules/docs.md) says which document holds what, and the headings that a
  file besides Markdown links to, such as the guide's `Upgrading`.
- The baselines, `tools/valecheck.txt` and `tools/sizecheck/baseline.txt`, stay empty: no file
  goes in either.

## Checks

```sh
make vale FILES='PATHS'        # Markdown, and Go comments through tools/valemirror
make sizecheck FILES='PATHS'   # the files' caps, and the Go comments'
make rumdl                     # every Markdown file: rumdl takes no FILES here
make lychee                    # every Markdown file's links and anchors
```

For Go files, also `gofmt -l .` and `make golangci-lint`: `lll` holds a changed comment line to
100 columns.
