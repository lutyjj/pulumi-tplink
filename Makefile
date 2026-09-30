# Containerised Go provider builds and standard Pulumi schema/SDK generation.
.DEFAULT_GOAL := help
.DELETE_ON_ERROR:
export HOST_UID := $(shell id -u)
export HOST_GID := $(shell id -g)
COMPOSE := docker compose
GO := $(COMPOSE) run --rm go-tools
PULUMI := $(COMPOSE) run --rm pulumi-tools
PACK := tplink
CMD_DIR := provider/cmd/pulumi-resource-$(PACK)
SCHEMA_FILE := $(CMD_DIR)/schema.json
# Release builds get their version from the Git tag; local consumers use this version.
VERSION ?= 0.1.0-dev
VERSION_PKG := github.com/lutyjj/pulumi-tplink/provider.Version

.PHONY: help tidy format build test lint schema sdks sdk-nodejs check release release-snapshot
help: ## List targets.
	@awk -F ':.*?## ' '/^[a-z0-9-]+:.*?## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tidy: ## Resolve Go module metadata.
	$(GO) go mod tidy

format: ## Format handwritten Go sources.
	$(GO) gofmt -s -w provider internal

build: ## Build the provider into bin/.
	$(GO) go build -o bin/pulumi-resource-$(PACK) -ldflags "-X $(VERSION_PKG)=$(VERSION)" ./$(CMD_DIR)

test: ## Run focused client and provider lifecycle tests.
	$(GO) go test -race -count=1 ./provider/... ./internal/...

lint: ## Check Go format, vet and lint.
	$(GO) sh -c 'test -z "$$(gofmt -l provider internal)"'
	$(GO) go vet ./provider/... ./internal/...
	$(COMPOSE) run --rm lint-tools golangci-lint run ./provider/... ./internal/...

schema: build ## Derive the committed Pulumi schema from Go types.
	@mkdir -p .cache
	$(PULUMI) sh -c 'pulumi package get-schema ./bin/pulumi-resource-$(PACK) > .cache/schema.json'
	$(COMPOSE) run --rm schema-tools 'del(.version)' .cache/schema.json > $(SCHEMA_FILE)

sdk-nodejs: build ## Generate and compile the local Node SDK.
	$(PULUMI) pulumi package gen-sdk --local --language nodejs --out sdk ./bin/pulumi-resource-$(PACK)
	$(PULUMI) sh -c 'cd sdk/nodejs && npm pkg set version=$(VERSION) && npm install --no-audit --no-fund'

sdks: schema sdk-nodejs ## Generate SDKs for all Pulumi languages.
	@for lang in python go dotnet java; do \
		$(PULUMI) pulumi package gen-sdk --language $$lang --out sdk $(SCHEMA_FILE) || exit $$?; \
	done

check: lint test ## Run the checks used by CI.

release-snapshot: ## Build all six plugin archives without publishing.
	$(COMPOSE) run --rm release-tools goreleaser release --snapshot --clean --skip=publish

release: ## Publish private archives from a version tag. Requires operator approval.
	$(COMPOSE) run --rm -e GITHUB_TOKEN release-tools goreleaser release --clean
