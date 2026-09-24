PREFIX ?= $(HOME)/.local
# Terminals the terminal contract runs against: tmux, jediterm (see tests/main_test.go).
TERMINALS ?= tmux
# Docker checks: the base image picks the tmux version (see tests/Dockerfile).
BASE ?= debian:bookworm
DOCKER_TERMINALS ?= tmux,jediterm
IMAGE = cld-test:$(subst /,-,$(subst :,-,$(BASE)))

.PHONY: check lint test docker-image docker-check install uninstall

check: lint test

lint:
	shellcheck bin/cld tests/jediterm/fetch-deps
	shfmt -d -i 4 bin/cld tests/jediterm/fetch-deps
	@test -z "$$(gofmt -l tests)" || { gofmt -d tests; exit 1; }
	go vet ./...

test:
	cd tests && CLD_TERMINALS=$(TERMINALS) go test -count=1 ./...

docker-image:
	docker build --build-arg BASE=$(BASE) -t $(IMAGE) -f tests/Dockerfile .

docker-check: docker-image
	docker run --rm --init --user "$$(id -u):$$(id -g)" -e HOME=/tmp \
		-v "$(CURDIR):/src:ro" -w /src $(IMAGE) \
		make check TERMINALS=$(DOCKER_TERMINALS)

install:
	install -d "$(PREFIX)/bin"
	install -m 755 bin/cld "$(PREFIX)/bin/cld"

uninstall:
	rm -f "$(PREFIX)/bin/cld"
