# The environment cld's checks run in, locally and in CI: `make docker-check`.
# BASE picks the tmux under test: debian:bookworm ships 3.3a (the oldest cld supports),
# ubuntu:24.04 3.4, debian:trixie 3.5a. TMUX_VERSION builds that tmux release from source instead.
ARG BASE=debian:bookworm
ARG GO_IMAGE=golang:1.27-bookworm

FROM ${GO_IMAGE} AS go

FROM ${BASE}
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        bash ca-certificates curl default-jdk-headless git make shellcheck shfmt tmux \
    && rm -rf /var/lib/apt/lists/*

ARG TMUX_VERSION=
RUN if [ -n "$TMUX_VERSION" ]; then \
        apt-get update \
        && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
            bison build-essential libevent-dev libncurses-dev pkg-config \
        && rm -rf /var/lib/apt/lists/* \
        && curl -fsSL "https://github.com/tmux/tmux/releases/download/$TMUX_VERSION/tmux-$TMUX_VERSION.tar.gz" \
            | tar -xz -C /tmp \
        && cd "/tmp/tmux-$TMUX_VERSION" \
        && ./configure --prefix=/usr/local && make -j"$(nproc)" install \
        && cd / && rm -rf "/tmp/tmux-$TMUX_VERSION" \
        && test "$(tmux -V)" = "tmux $TMUX_VERSION"; \
    fi

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
