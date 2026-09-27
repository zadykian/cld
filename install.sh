#!/bin/sh
# Installs cld from its GitHub releases, each of which publishes this script:
#
#   curl -fsSL https://github.com/zadykian/cld/releases/latest/download/install.sh | sh
#
# It downloads the binary for this system, cld-OS-ARCH, from the latest release, or from the one
# CLD_VERSION names (0.4.0 or later, the first to publish binaries), checks it against the
# release's cld.sha256, runs it for its version, and only then moves it to cld in
# CLD_INSTALL_DIR, ~/.local/bin by default, replacing any cld there. CLD_RELEASES_URL stands in
# for https://github.com/zadykian/cld/releases, for the tests. It runs in any POSIX sh, and needs
# curl, and sha256sum or shasum.
#
# Everything is in functions that the last line calls, so that a download cut short runs nothing.

set -eu

# say prints a message of the installer's.
say() {
    printf 'install.sh: %s\n' "$*" >&2
}

# die prints a message of the installer's and exits with status 1.
die() {
    say "$*"
    exit 1
}

# platform sets binary to the name the releases give cld for this system.
platform() {
    os=$(uname -s)
    arch=$(uname -m)
    case $os in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "cld is released for Linux and macOS, not $os" ;;
    esac
    case $arch in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "cld is released for x86_64 and arm64, not $arch" ;;
    esac
    # A shell that Rosetta 2 translates on Apple silicon sees x86_64: the native binary runs too.
    if [ "$os/$arch" = darwin/amd64 ] &&
        [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = 1 ]; then
        arch=arm64
    fi
    binary=cld-$os-$arch
}

# release sets url to where the files of the release to install are: the latest, or CLD_VERSION's.
release() {
    releases=${CLD_RELEASES_URL:-https://github.com/zadykian/cld/releases}
    version=${CLD_VERSION:-latest}
    case ${version#v} in
    latest) url=$releases/latest/download ;;
    *[!0-9.]*) die "CLD_VERSION is not a version, X.Y.Z: $version" ;;
    0.[0-3].*)
        die "cld $version was a script, not a binary: see" \
            https://github.com/zadykian/cld/blob/main/docs/guide.md#upgrading
        ;;
    [0-9]*.[0-9]*.[0-9]*) url=$releases/download/v${version#v} ;;
    *) die "CLD_VERSION is not a version, X.Y.Z: $version" ;;
    esac
}

# download writes the file at the URL $1 to stdout. Redirects stay on HTTPS.
download() {
    curl -fsSL --proto-redir =https "$1"
}

# sha256 prints the SHA-256 checksum of the file $1.
sha256() {
    if command -v sha256sum >/dev/null; then
        sha256sum <"$1"
    else
        shasum -a 256 <"$1"
    fi | awk '{ print $1 }'
}

main() {
    platform
    release
    command -v curl >/dev/null || die "curl is not installed"
    command -v sha256sum >/dev/null || command -v shasum >/dev/null ||
        die "sha256sum or shasum is needed to check cld's checksum"

    sums=$(download "$url/cld.sha256") || die "cannot download $url/cld.sha256"
    expected=$(printf '%s\n' "$sums" |
        awk -v name="$binary" '$2 == name || $2 == "*" name { print $1 }')
    [ -n "$expected" ] || die "$url/cld.sha256 has no checksum for $binary"

    dir=${CLD_INSTALL_DIR:-$HOME/.local/bin}
    mkdir -p "$dir" || die "cannot create $dir"
    dir=$(CDPATH='' cd -- "$dir" && pwd)
    # The download goes next to cld, so that moving it there replaces cld at once, even while it
    # runs, and so that it runs from where cld will.
    new=
    trap '[ -z "$new" ] || rm -f "$new"' EXIT
    trap 'exit 1' HUP INT TERM
    new=$(mktemp "$dir/.cld.XXXXXX") || die "cannot write to $dir"
    download "$url/$binary" >"$new" || die "cannot download $url/$binary"
    [ "$(sha256 "$new")" = "$expected" ] ||
        die "$binary does not match its checksum in $url/cld.sha256"
    chmod 755 "$new"
    installed=$("$new" --version </dev/null) || die "the $binary downloaded does not run"
    mv -f "$new" "$dir/cld" || die "cannot move cld to $dir"
    new=
    echo "installed $installed as $dir/cld"

    found=$(command -v cld || true)
    if [ "$found" != "$dir/cld" ]; then
        case :$PATH: in
        *:"$dir":*) say "$found comes first on your PATH, before $dir/cld" ;;
        *) say "$dir is not on your PATH: add it there, in your shell's profile" ;;
        esac
    fi
}

main
