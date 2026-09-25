PREFIX ?= $(HOME)/.local
# Terminals the terminal contract runs against: tmux, jediterm (see tests/main_test.go).
TERMINALS ?= tmux
# Docker checks: tmux release TMUX_VERSION, built from source on BASE; the release the checks run
# on is pinned here and in tests/Dockerfile, and bumped with cld's minimum (docs/design.md,
# decision 6). Another release builds by hand for a probe: make docker-image TMUX_VERSION=X.
BASE ?= debian:trixie
TMUX_VERSION ?= 3.7c
DOCKER_TERMINALS ?= tmux,jediterm
IMAGE = cld-test:$(subst /,-,$(subst :,-,$(BASE)))-tmux-$(TMUX_VERSION)
# The version make dist and make install stamp into cld.
VERSION ?= dev
# The platforms make dist builds cld for, as dist/cld-OS-ARCH.
PLATFORMS = linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
BUILD = CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=$(VERSION)'

.PHONY: check lint test docker-image docker-test docker-check dist install uninstall

check: lint test

lint:
	shellcheck tests/jediterm/fetch-deps
	shfmt -d -i 4 tests/jediterm/fetch-deps
	@test -z "$$(gofmt -l .)" || { gofmt -d .; exit 1; }
	go vet ./...

test:
	cd tests && CLD_TERMINALS=$(TERMINALS) go test -count=1 ./...

docker-image:
	docker build --build-arg BASE=$(BASE) --build-arg TMUX_VERSION=$(TMUX_VERSION) \
		-t $(IMAGE) -f tests/Dockerfile .

# Runs the checks in an image that is already built: by docker-image, or by CI from its layer cache.
docker-test:
	docker run --rm --init --user "$$(id -u):$$(id -g)" -e HOME=/tmp \
		-v "$(CURDIR):/src:ro" -w /src $(IMAGE) \
		make check TERMINALS=$(DOCKER_TERMINALS)

docker-check: docker-image
	@$(MAKE) --no-print-directory docker-test

# With cgo off every platform cross-compiles without a C toolchain, into a static binary on Linux.
dist:
	rm -rf dist && mkdir dist
	set -e; for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} $(BUILD) -o dist/cld-$${platform%/*}-$${platform#*/} ./cmd/cld; \
	done
	cd dist && { sha256sum cld-* 2>/dev/null || shasum -a 256 cld-*; } > cld.sha256

install:
	install -d "$(PREFIX)/bin"
	$(BUILD) -o "$(PREFIX)/bin/cld" ./cmd/cld

uninstall:
	rm -f "$(PREFIX)/bin/cld"
