.DEFAULT_GOAL := help

SOURCES := $(shell find . -prune -o -name "*.go" -not -name '*_test.go' -print)

GOIMPORTS ?= goimports
GOCILINT ?= golangci-lint

.PHONY: help
help: ## Show help text
	@echo "Commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: fmt
fmt: $(SOURCES) ## Formatting source codes.
	$(GOIMPORTS) -w $^

.PHONY: lint
lint: ## Run golangci-lint.
	$(GOCILINT) run --no-config --disable-all --enable=goimports --enable=misspell ./...

.PHONY: test
test:  ## Run tests with race condition checking.
	go test -race ./...

.PHONY: vet
vet:  ## Run go vet on packages.
	go vet ./...

.PHONY: bench
bench:  ## Run benchmarks.
	go test -bench=. -run=- -benchmem ./...

.PHONY: coverage
cover:  ## Run the tests.
	go test -coverprofile=coverage.o
	go tool cover -func=coverage.o

.PHONY: generate
generate: ## Run go generate
	go generate ./...

.PHONY: build
build: ## Build example command lines.
	./_example/build.sh
