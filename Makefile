BINARY := relayctl
DIST_DIR := dist
GO ?= go
GOCACHE ?= .cache/go-build
GOMODCACHE ?= .cache/go-mod
GOENV := GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X localrelay/cmd.version=$(VERSION) -X localrelay/cmd.commit=$(COMMIT) -X localrelay/cmd.buildDate=$(BUILD_DATE)

TARGETS := \
	linux-amd64 \
	linux-arm64 \
	darwin-amd64 \
	darwin-arm64

.PHONY: build test clean release docker-build license-audit

build:
	$(GOENV) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	$(GOENV) $(GO) test ./...

clean:
	rm -rf $(DIST_DIR) $(BINARY)

release: clean test
	mkdir -p $(DIST_DIR)
	@for target in $(TARGETS); do \
		os=$${target%-*}; arch=$${target#*-}; \
		out=$(DIST_DIR)/$(BINARY)-$${os}-$${arch}; \
		$(GOENV) GOOS=$${os} GOARCH=$${arch} CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$out .; \
		tar -czf $$out.tar.gz -C $(DIST_DIR) $$(basename $$out); \
		shasum -a 256 $$out.tar.gz > $$out.tar.gz.sha256; \
	done


docker-build:
	docker compose build app

license-audit:
	./scripts/license_audit.sh
