PREFIX ?= $(HOME)/.local
# Terminals the terminal contract runs against: tmux, jediterm, ghostty (see tests/main_test.go).
TERMINALS ?= tmux
# ghostty's driver links libghostty-vt, so it builds only with the tag ghostty, added where
# TERMINALS lists it (docs/design/decisions/0054-ghostty.md).
comma := ,
TEST_TAGS = $(if $(filter ghostty,$(subst $(comma), ,$(TERMINALS))),-tags ghostty)
# Docker checks: tmux release TMUX_VERSION, built from source on BASE. The newest the checks run on
# is pinned here and in tests/Dockerfile, bumped by hand (docs/design/decisions/0006-versions.md).
# The oldest, cld's floor, runs as make docker-check TMUX_VERSION=3.5a, as CI's linux-oldest does.
# Another release builds by hand for a probe: make docker-image TMUX_VERSION=X.
BASE ?= debian:trixie
TMUX_VERSION ?= 3.7c
DOCKER_TERMINALS ?= tmux,jediterm,ghostty
IMAGE = cld-test:$(subst /,-,$(subst :,-,$(BASE)))-tmux-$(TMUX_VERSION)
# The completion checks in bash with ble.sh: the same image, on BLESH_BASE, whose package ble.sh
# the tests load - Ubuntu 26.04's is 0.4.0~git20250806.8060b7a, as CI builds it too.
BLESH_BASE ?= ubuntu:26.04
BLESH_IMAGE = cld-test:$(subst /,-,$(subst :,-,$(BLESH_BASE)))-blesh-tmux-$(TMUX_VERSION)
# The version make dist and make install stamp into cld.
VERSION ?= dev
# The platforms make dist builds cld for, as dist/cld-OS-ARCH.
PLATFORMS = linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
BUILD = CGO_ENABLED=0 go build -trimpath -ldflags '-X main.version=$(VERSION)'
# The scripts vet checks, POSIX sh but for fetch-deps and build-lib, which are bash.
SCRIPTS = install.sh tests/jediterm/fetch-deps tests/ghostty/build-lib tools/run tools/valecheck \
	.claude/hooks/lint-file.sh .claude/hooks/lint-branch.sh
# vet's ShellCheck and shfmt: those on the PATH, as the Docker image and Homebrew have them, but
# the pinned ones under lint.
SHELLCHECK = shellcheck
SHFMT = shfmt
# The files sizecheck and vale check, every one when empty: make vale FILES='README.md'.
FILES =

.PHONY: check lint vet golangci-lint sizecheck vale govulncheck actionlint zizmor lychee test \
	docker-image docker-test docker-check docker-blesh-image docker-blesh-test docker-blesh-check \
	dist install uninstall

check: vet test

# Every gate, as CI's lint job runs them, each failing on any finding. tools/run downloads the
# tools pinned there into .cache/tools; check keeps to vet, whose tools the Docker image has.
lint: SHELLCHECK = tools/run shellcheck
lint: SHFMT = tools/run shfmt
lint: vet golangci-lint sizecheck vale govulncheck actionlint zizmor lychee

vet:
	$(SHELLCHECK) $(SCRIPTS)
	$(SHFMT) -d -i 4 $(SCRIPTS)
	@test -z "$$(gofmt -l .)" || { gofmt -d .; exit 1; }
	go vet ./...

# Every line of every package, the ghostty driver's tagged files too, as .golangci.yml sets it.
golangci-lint:
	tools/run golangci-lint run ./...

# The size caps. tools/sizecheck/baseline.txt is empty: no file may exceed them.
sizecheck:
	go test -count=1 ./tools/...
	go run ./tools/sizecheck $(FILES)

# Vale. tools/valecheck.txt is empty: no file may fail it.
vale:
	tools/valecheck $(FILES)

# Any finding fails, as do those govulncheck lists without failing: in packages cld imports but
# does not call, and in modules it only requires. The ghostty driver's tag brings its bindings in,
# type-checked against the headers tools/run fetches (docs/design/decisions/0054-ghostty.md).
GOVULNCHECK = go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -tags ghostty
govulncheck:
	pc=$$(tools/run -path libghostty-vt) && \
		export PKG_CONFIG_PATH="$$pc$${PKG_CONFIG_PATH:+:$$PKG_CONFIG_PATH}" && \
		$(GOVULNCHECK) -show verbose ./... && \
		mkdir -p .cache && $(GOVULNCHECK) -format json ./... >.cache/govulncheck.json
	@! grep -q '"finding"' .cache/govulncheck.json || \
		{ echo 'govulncheck: the findings above fail the check, called or not'; exit 1; }

# The workflows' shell scripts go through the pinned ShellCheck.
actionlint:
	shellcheck="$$(tools/run -path shellcheck)" && \
		tools/run actionlint -shellcheck="$$shellcheck" -pyflakes=

# The workflows and .github/dependabot.yml, with the online audits where GH_TOKEN is set, as in CI.
zizmor:
	tools/run zizmor .

# Links to files and their headings in the Markdown files git lists, new ones included.
lychee:
	tools/run lychee --offline --include-fragments --no-progress \
		$$(git ls-files --cached --others --exclude-standard '*.md')

test:
	cd tests && CLD_TERMINALS=$(TERMINALS) go test -count=1 $(TEST_TAGS) ./...

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

docker-blesh-image:
	docker build --build-arg BASE=$(BLESH_BASE) --build-arg PACKAGES=ble.sh \
		--build-arg TMUX_VERSION=$(TMUX_VERSION) -t $(BLESH_IMAGE) -f tests/Dockerfile .

# The completion tests in an image that is already built, by docker-blesh-image or by CI from its
# layer cache; with CLD_BLESH, the ble.sh test fails where it would skip without ble.sh.
docker-blesh-test:
	docker run --rm --init --user "$$(id -u):$$(id -g)" -e HOME=/tmp \
		-e CLD_BLESH=/usr/share/blesh/ble.sh -v "$(CURDIR):/src:ro" -w /src $(BLESH_IMAGE) \
		sh -c 'cd tests && go test -count=1 -run Completion .'

docker-blesh-check: docker-blesh-image
	@$(MAKE) --no-print-directory docker-blesh-test

# With cgo off every platform cross-compiles without a C toolchain, into a static binary on Linux.
# The installer goes with the binaries it downloads, outside cld.sha256.
dist:
	rm -rf dist && mkdir dist
	set -e; for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} $(BUILD) -o dist/cld-$${platform%/*}-$${platform#*/} ./cmd/cld; \
	done
	cd dist && { sha256sum cld-* 2>/dev/null || shasum -a 256 cld-*; } > cld.sha256
	cp install.sh dist/

install:
	install -d "$(PREFIX)/bin"
	$(BUILD) -o "$(PREFIX)/bin/cld" ./cmd/cld

uninstall:
	rm -f "$(PREFIX)/bin/cld"
