# hsdebug Makefile

BINARY      := hsdebug
PKG         := ./cmd/hsdebug
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
PORT        ?= 7654
HOST        ?= 127.0.0.1

# Prefer a locally installed Go toolchain if present.
GO ?= $(shell command -v go 2>/dev/null || echo $(HOME)/.local/go/bin/go)

.DEFAULT_GOAL := build

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the binary into ./$(BINARY)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

.PHONY: install
install: ## Install the binary into GOBIN / $GOPATH/bin
	$(GO) install -ldflags "$(LDFLAGS)" $(PKG)

.PHONY: run
run: ## Run without building a binary (pass ARGS="...")
	$(GO) run $(PKG) $(ARGS)

.PHONY: server
server: build ## Build then start the server (HOST/PORT overridable)
	./$(BINARY) server --host $(HOST) --port $(PORT)

.PHONY: serve-lan
serve-lan: build ## Start the server bound to 0.0.0.0 (LAN-accessible)
	./$(BINARY) server --host 0.0.0.0 --port $(PORT)

.PHONY: doctor
doctor: build ## Run self-diagnostics
	./$(BINARY) doctor

.PHONY: test
test: ## Run all tests
	$(GO) test ./...

.PHONY: test-race
test-race: ## Run tests with the race detector and coverage
	$(GO) test -race -cover ./...

.PHONY: cover
cover: ## Generate and open an HTML coverage report
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format all Go source
	$(GO) fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

.PHONY: check
check: fmt vet test ## Format, vet, and test

.PHONY: docker-build
docker-build: ## Build the Docker image (tag with VERSION)
	docker build -t $(BINARY):$(VERSION) -t $(BINARY):latest .

.PHONY: docker-run
docker-run: ## Run the Docker image (LAN-accessible on PORT)
	docker run --rm -p $(PORT):7654 -v $(BINARY)-data:/data $(BINARY):latest

.PHONY: clean
clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out
