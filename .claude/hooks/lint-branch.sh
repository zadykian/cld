#!/bin/sh
# Claude Code's Stop hook (.claude/settings.json): runs make lint as Claude ends its turn, where
# the work tree differs from origin/main. A failure prints the end of the output to stderr with
# status 2, and Claude goes on; then stop_hook_active is true, and the hook lets Claude stop.

set -u

# common prints the git common directory of the directory $1, which every work tree of a
# repository shares.
common() {
    git -C "$1" rev-parse --path-format=absolute --git-common-dir 2>/dev/null
}

main() {
    # All of stdin is read first. A quote in a JSON string is escaped, so only the key matches.
    input=$(cat)
    if printf '%s\n' "$input" | tr '\n' ' ' |
        grep -q '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
        return 0
    fi
    # The work tree Claude is in, where it is one of this repository's, else the hook's own.
    repo=$(common "$(dirname -- "$0")")
    [ -n "$repo" ] || return 0
    if [ "$(common .)" != "$repo" ]; then
        cd -- "$(dirname -- "$0")" || return 0
    fi
    cd -- "$(git rev-parse --show-toplevel)" || return 0
    if git diff --quiet origin/main -- 2>/dev/null &&
        [ -z "$(git ls-files --others --exclude-standard)" ]; then
        return 0
    fi

    log=$(mktemp) || return 0
    trap 'rm -f "$log"' EXIT
    if ! make lint >"$log" 2>&1; then
        printf 'make lint fails, ending with:\n' >&2
        tail -n 50 "$log" >&2
        return 2
    fi
}

main "$@"
