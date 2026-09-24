# The environment cld's checks run in, locally and in CI: `make docker-check`.
# BASE picks the tmux under test: debian:bookworm ships 3.3a (the oldest cld supports),
# ubuntu:24.04 3.4, debian:trixie 3.5a.
ARG BASE=debian:bookworm
ARG GO_IMAGE=golang:1.27-bookworm

FROM ${GO_IMAGE} AS go

FROM ${BASE}
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        bash ca-certificates git make shellcheck shfmt tmux \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go /usr/local/go /usr/local/go
# The source is mounted read-only and the tests run as the calling user: modules are baked into
# the image, the build cache lives in /tmp.
ENV PATH=/usr/local/go/bin:$PATH \
    GOPATH=/go \
    GOCACHE=/tmp/go-cache \
    GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local

COPY go.mod go.sum /tmp/module/
RUN cd /tmp/module && go mod download && chmod -R a+rX /go && rm -rf /tmp/module
