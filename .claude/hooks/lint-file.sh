#!/bin/sh
# Claude Code's PostToolUse hook for Edit and Write (.claude/settings.json): runs make lint's gates
# on the Go or Markdown file just written. A finding goes to stderr with status 2, which Claude
# sees; a file outside this repository, or one git ignores, such as a plan, is passed over.

set -u

# file_path prints tool_input.file_path of the hook's JSON on stdin. sed reads it, as jq may be
# missing; a quote in a JSON string is escaped, so only a key matches.
file_path() {
    tr '\n' ' ' | sed -E -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)".*/\1/p'
}

# common prints the git common directory of the directory $1, which every work tree of a
# repository shares.
common() {
    git -C "$1" rev-parse --path-format=absolute --git-common-dir 2>/dev/null
}

# gate runs a command from the repository's root, and adds its output to $report where it fails.
gate() {
    if ! "$@" >"$tmp/out" 2>&1; then
        printf '%s fails:\n' "$*" >>"$report"
        cat "$tmp/out" >>"$report"
    fi
}

main() {
    file=$(file_path)
    # A path with an escape in its JSON, a backslash or a quote, is none of the repository's.
    case $file in
    *\\*) return 0 ;;
    *.go | *.md) ;;
    *) return 0 ;;
    esac
    [ -f "$file" ] || return 0
    dir=$(dirname -- "$file")
    repo=$(common "$(dirname -- "$0")")
    if [ -z "$repo" ] || [ "$(common "$dir")" != "$repo" ]; then
        return 0
    fi
    # The file's own work tree, which may be a linked one, and the file's path in it. A branch
    # from before the gates has no .vale.ini.
    cd -- "$dir" || return 0
    path=$(git rev-parse --show-prefix)$(basename -- "$file")
    cd -- "$(git rev-parse --show-toplevel)" || return 0
    if git check-ignore -q -- "$path" || [ ! -f .vale.ini ]; then
        return 0
    fi

    tmp=$(mktemp -d) || return 0
    trap 'rm -rf "$tmp"' EXIT
    report=$tmp/report
    : >"$report"
    case $path in
    *.go)
        if [ -n "$(gofmt -l "$path" 2>&1)" ]; then
            printf 'gofmt -l %s lists it:\n' "$path" >>"$report"
            gofmt -d "$path" >>"$report" 2>&1
        fi
        gate tools/run golangci-lint run --allow-serial-runners "./$(dirname -- "$path")/"
        ;;
    *.md)
        gate tools/run lychee --offline --include-fragments --no-progress "$path"
        ;;
    esac
    gate go run ./tools/sizecheck "$path"
    gate tools/valecheck "$path"
    if [ -s "$report" ]; then
        cat "$report" >&2
        return 2
    fi
}

main "$@"
