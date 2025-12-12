GOCACHE ?= $(CURDIR)/.gocache
BIN_DIR ?= $(CURDIR)/bin
APP      = formlander
TAILWIND = $(BIN_DIR)/tailwindcss

WATCHEXEC ?= $(shell command -v watchexec 2>/dev/null)
GOTESTSUM ?= $(shell command -v gotestsum 2>/dev/null)

.PHONY: help build run dev test test-unit test-e2e test-e2e-setup tidy fmt clean deps release vendor css css-watch

	help:
	@echo "Available targets:"
	@echo "  deps         - ensure local build cache directory exists"
	@echo "  vendor       - download JS/CSS dependencies (htmx, highlight.js, tailwind)"
	@echo "  css          - build Tailwind CSS for production"
	@echo "  css-watch    - watch and rebuild Tailwind CSS on changes"
	@echo "  build        - compile the CLI binary to $(BIN_DIR)"
	@echo "  run          - run the application from source"
	@echo "  dev          - hot-reload the server using watchexec (requires .env)"
	@echo "  test         - run unit & e2e tests"
	@echo "  test-unit    - run package tests under internal/"
	@echo "  test-e2e     - run Playwright end-to-end tests in e2e/"
	@echo "  test-e2e-setup - install Playwright dependencies"
	@echo "  tidy         - add/remove go.mod entries"
	@echo "  fmt          - gofmt Go source files"
	@echo "  clean        - remove build artifacts"
	@echo "  release      - build & push multi-arch Docker images"

deps:
	@mkdir -p $(GOCACHE) $(BIN_DIR)

vendor:
	@./scripts/vendor.sh

css: vendor
	@echo ">> building Tailwind CSS"
	$(TAILWIND) -i web/static/app.css -o web/static/app.built.css --minify

css-watch: vendor
	@echo ">> watching Tailwind CSS"
	$(TAILWIND) -i web/static/app.css -o web/static/app.built.css --watch

COMMIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")

build: deps css
	@echo ">> building $(APP)"
	GOCACHE=$(GOCACHE) go build -ldflags="-X formlander/internal/pkg/cartridge.buildCommit=$(COMMIT_SHA)" -o $(BIN_DIR)/$(APP) ./cmd/$(APP)

run: deps
	GOCACHE=$(GOCACHE) go run ./cmd/$(APP)

dev: deps
ifeq ($(strip $(WATCHEXEC)),)
	@echo "watchexec not found. Install via 'brew install watchexec' or see https://github.com/watchexec/watchexec"
	@exit 1
else
	GOCACHE=$(GOCACHE) $(WATCHEXEC) --clear --restart \
		--watch cmd --watch internal --watch web \
		--exts go,html,tmpl \
		-- go run ./cmd/$(APP)
endif

test: test-unit test-e2e

test-unit: deps
ifeq ($(strip $(GOTESTSUM)),)
	@echo ">> go test ./internal/..."
	GOCACHE=$(GOCACHE) go test ./internal/...
else
	@echo ">> gotestsum ./internal/..."
	GOCACHE=$(GOCACHE) $(GOTESTSUM) --format testname -- -count=1 ./internal/...
endif

test-e2e-setup:
	@echo ">> installing Playwright dependencies"
	cd e2e && npm install && npx playwright install --with-deps chromium

test-e2e: deps
	@echo ">> running Playwright E2E tests"
	cd e2e && npm test

tidy: deps
	GOCACHE=$(GOCACHE) go mod tidy

fmt:
	@echo ">> formatting Go files"
	go fmt ./...

clean:
	@echo ">> removing build artifacts"
	rm -rf $(BIN_DIR)/$(APP)

release:
	@scripts/release.sh
