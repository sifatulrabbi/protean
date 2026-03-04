.PHONY: build run dev test test-integration build-sandbox-image smoke

SANDBOX_IMAGE_TAG ?= protean-sandbox:alpha
DOCKERFILE ?= sandbox/Dockerfile
DOCKER_CONTEXT ?= sandbox

build:
	go build -o bin/sandbox-service ./cmd/sandbox-service

run: build
	./bin/sandbox-service

dev:
	go run ./cmd/sandbox-service

test:
	go test ./...

test-integration:
	SANDBOX_TEST_IMAGE=$(SANDBOX_IMAGE_TAG) go test -tags=integration ./...

build-sandbox-image:
	docker build -t $(SANDBOX_IMAGE_TAG) -f $(DOCKERFILE) $(DOCKER_CONTEXT)

smoke: build-sandbox-image
	docker run --rm $(SANDBOX_IMAGE_TAG) bash -lc 'set -euo pipefail; test "$$(id -u)" = "1000"; test "$$PWD" = "/workspace"; for bin in bash curl git rg node bun python3; do command -v "$$bin" >/dev/null; done'
	$(MAKE) test
	$(MAKE) test-integration
