# The environment cld's checks run in, locally and in CI: `make docker-check`.
# The tmux under test is the release TMUX_VERSION, built from source: no released Debian or Ubuntu
# version ships the newest, and a package would move with the base image. The newest is pinned
# here, and bumped by hand (docs/design/decisions/0006-versions.md); CI's linux-oldest builds the
# oldest tmux cld runs on, as `make docker-check TMUX_VERSION=3.5a` does, and another release builds
# with `make docker-image TMUX_VERSION=X`, to try it by hand.
ARG BASE=debian:trixie
ARG GO_IMAGE=golang:1.27-bookworm

FROM ${GO_IMAGE} AS go

# libghostty-vt, Ghostty's terminal core, and Ghostty's terminfo entry, for the ghostty driver
# (docs/design/decisions/0054-ghostty.md): tests/ghostty/deps.txt pins Zig and Ghostty's commit,
# the one go.mitchellh.com/libghostty pins in go.mod. The final image takes the artefacts.
FROM ${BASE} AS ghostty
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        bash ca-certificates curl git xz-utils \
    && rm -rf /var/lib/apt/lists/*
COPY tests/ghostty/ /tmp/ghostty/
RUN /tmp/ghostty/build-lib /opt/ghostty

# Everything but Ghostty. The ble.sh image stops here (`make docker-blesh-image`): the completion
# tests never use Ghostty, and BuildKit builds only the stages a target needs.
FROM ${BASE} AS no-ghostty
# bash-completion, fish and zsh load what cld setup completion writes, in the tests that check it.
# PACKAGES are more, for a base that has them: ble.sh on Ubuntu, which Debian's trixie lacks
# (`make docker-blesh-check`).
ARG PACKAGES=
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        bash ca-certificates curl default-jdk-headless git make shellcheck shfmt \
        bash-completion fish zsh $PACKAGES \
        bison build-essential libevent-dev libncurses-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*

ARG TMUX_VERSION=3.7c
RUN curl -fsSL "https://github.com/tmux/tmux/releases/download/$TMUX_VERSION/tmux-$TMUX_VERSION.tar.gz" \
        | tar -xz -C /tmp \
    && cd "/tmp/tmux-$TMUX_VERSION" \
    && ./configure --prefix=/usr/local && make -j"$(nproc)" install \
    && cd / && rm -rf "/tmp/tmux-$TMUX_VERSION" \
    && test "$(tmux -V)" = "tmux $TMUX_VERSION"

COPY --from=go /usr/local/go /usr/local/go
# The source is mounted read-only and the tests run as the calling user: modules are baked into
# the image, the build cache lives in /tmp.
ENV PATH=/usr/local/go/bin:$PATH \
    GOPATH=/go \
    GOCACHE=/tmp/go-cache \
    GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local \
    CLD_JEDITERM_LIB=/opt/jediterm/lib

COPY tests/jediterm/deps.txt tests/jediterm/fetch-deps /opt/jediterm/
RUN /opt/jediterm/fetch-deps /opt/jediterm/lib

COPY go.mod go.sum /tmp/module/
RUN cd /tmp/module && go mod download && chmod -R a+rX /go && rm -rf /tmp/module

# The checks' image: no-ghostty, with Ghostty. The ghostty driver links libghostty-vt statically
# through pkg-config. Its terminfo entry lies where the app's TERMINFO names it, as the app ships
# it, not in the system's database.
FROM no-ghostty
COPY --from=ghostty /opt/ghostty /opt/ghostty
ENV PKG_CONFIG_PATH=/opt/ghostty/share/pkgconfig \
    CLD_GHOSTTY_DIR=/opt/ghostty
