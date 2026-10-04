GO_CMD?=go

GOOS?=$(shell $(GO_CMD) env GOOS)
GOARCH?=$(shell $(GO_CMD) env GOARCH)
CGO_ENABLED?=0
BIN_EXT=$(if $(filter windows,$(GOOS)),.exe)
BIN_SUFFIX=_$(GOOS)_$(GOARCH)$(BIN_EXT)

GOLANGCI_LINT_CMD?=$(GO_CMD) tool golangci-lint

GOVULNCHECK_CMD?=$(GO_CMD) tool govulncheck

.DEFAULT_GOAL := .default

.default: format build lint test

.PHONY: help
help: ## Show help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: go-build ## Build every command

.PHONY: go-build
go-build: ## Build every command into bin/<name>_<os>_<arch>
	@set -e; for c in $$(ls cmd); do \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO_CMD) build -trimpath -ldflags="-s -w" -o bin/$${c}$(BIN_SUFFIX) ./cmd/$$c; \
	done

.PHONY: format
format: ## Format the code and tidy go.mod
	$(GO_CMD) fix ./...
	$(GOLANGCI_LINT_CMD) fmt ./...
	$(GO_CMD) mod tidy

.PHONY: lint
lint: ## Run the linters
	$(GOLANGCI_LINT_CMD) run ./...
	$(GOVULNCHECK_CMD) ./...

.PHONY: test
test: ## Run the tests
	$(GO_CMD) test ./...
