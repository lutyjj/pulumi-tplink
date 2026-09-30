# One control surface for containerised Go, Pulumi and API recovery tooling.
.DEFAULT_GOAL := help
.DELETE_ON_ERROR:
export HOST_UID := $(shell id -u)
export HOST_GID := $(shell id -g)
COMPOSE := docker compose
GO := $(COMPOSE) run --rm go-tools
NODE := $(COMPOSE) run --rm node-tools
PACK := tplink
CMD_DIR := provider/cmd/pulumi-resource-$(PACK)
SCHEMA_FILE := $(CMD_DIR)/schema.json
# Release builds get their version from the Git tag; local consumers use this version.
VERSION ?= 0.1.0-dev
VERSION_PKG := github.com/lutyjj/pulumi-tplink/provider.Version

.PHONY: help tidy format build test lint schema sdks sdk-nodejs check tools-check release release-snapshot api-spec api-spec-sampled
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
	$(NODE) sh -c 'pulumi package get-schema ./bin/pulumi-resource-$(PACK) > .cache/schema.json && node tools/schema.mjs .cache/schema.json $(SCHEMA_FILE)'

sdk-nodejs: build ## Generate and compile the local Node SDK.
	$(NODE) pulumi package gen-sdk --local --language nodejs --out sdk ./bin/pulumi-resource-$(PACK)
	$(NODE) sh -c 'cd sdk/nodejs && npm pkg set version=$(VERSION) && npm install --no-audit --no-fund'

sdks: schema sdk-nodejs ## Generate SDKs for all Pulumi languages.
	@for lang in python go dotnet java; do \
		$(NODE) pulumi package gen-sdk --language $$lang --out sdk $(SCHEMA_FILE) || exit $$?; \
	done

node_modules/.package-lock.json: package.json package-lock.json
	$(NODE) npm ci --ignore-scripts --no-audit --no-fund

tools-check: node_modules/.package-lock.json ## Typecheck API recovery tooling without router access.
	$(NODE) npx tsc --noEmit -p tools

check: lint test tools-check ## Run the checks used by CI.

api-spec: node_modules/.package-lock.json ## Recover the API spec. Requires TPLINK_HOST; optional TPLINK_INSECURE=true.
	$(COMPOSE) run --rm -e TPLINK_HOST -e TPLINK_INSECURE node-tools node tools/codegen/main.mts all

api-spec-sampled: node_modules/.package-lock.json ## Recover API spec with read-only sampling; also requires TPLINK_PASSWORD.
	$(COMPOSE) run --rm -e TPLINK_HOST -e TPLINK_INSECURE -e TPLINK_PASSWORD node-tools node tools/codegen/main.mts all

release-snapshot: ## Build all six plugin archives without publishing.
	$(COMPOSE) run --rm release-tools goreleaser release --snapshot --clean --skip=publish

release: ## Publish private archives from a version tag. Requires operator approval.
	$(COMPOSE) run --rm -e GITHUB_TOKEN release-tools goreleaser release --clean
