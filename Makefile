BINARY  := termaid
PKG     := ./cmd/termaid
PREFIX  ?= $(HOME)/.local
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all build install test race vet fmt fmt-check tidy run clean help

all: build ## Build the binary (default)

build: ## Compile the termaid binary
	go build -o $(BINARY) $(PKG)

install: ## Install termaid into $(PREFIX)/bin
	install -d $(PREFIX)/bin
	go build -o $(PREFIX)/bin/$(BINARY) $(PKG)
	@echo "installed $(PREFIX)/bin/$(BINARY)"

test: ## Run the test suite
	go test ./...

race: ## Run tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go sources
	gofmt -w .

fmt-check: ## Fail if any file is not gofmt-clean
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

tidy: ## Sync go.mod/go.sum
	go mod tidy

run: build ## Build and launch the interactive TUI
	./$(BINARY)

clean: ## Remove build artifacts and run output
	rm -f $(BINARY)
	rm -rf workdir
	rm -f run-*.log

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
