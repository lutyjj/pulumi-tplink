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

.PHONY: help tidy format build test lint schema sdks sdk-nodejs check release release-snapshot sdk-check release-check sdk-go-check sdk-python-check sdk-dotnet-check sdk-java-check sdk-python sdk-go sdk-dotnet sdk-java secret-check workflow-check
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
	$(COMPOSE) run --rm schema-tools 'del(.version)' .cache/schema.json > .cache/schema-unversioned.json
	cp .cache/schema-unversioned.json $(SCHEMA_FILE)

sdk-nodejs: build ## Generate and compile the local Node SDK.
	$(PULUMI) pulumi package gen-sdk --local --language nodejs --version $(VERSION) --out sdk ./bin/pulumi-resource-$(PACK)
	$(PULUMI) sh -c 'cd sdk/nodejs && npm install --no-audit --no-fund'

sdk-python sdk-go sdk-dotnet sdk-java: sdk-%: build
	$(COMPOSE) run --rm sdk-$*-tools pulumi package gen-sdk --language $* --version $(VERSION) --out sdk ./bin/pulumi-resource-$(PACK)

sdks: schema sdk-nodejs sdk-python sdk-go sdk-dotnet sdk-java ## Generate SDKs for all Pulumi languages.

sdk-check: sdks ## Generate and compile every language SDK; imports Python's installed package.
	$(MAKE) -j4 sdk-go-check sdk-python-check sdk-dotnet-check sdk-java-check

sdk-go-check:
	$(GO) sh -c 'cd sdk/go && go mod tidy && go test ./...'

sdk-python-check:
	$(COMPOSE) run --rm sdk-python-tools sh -c 'python3 -m venv --clear .cache/python-sdk && .cache/python-sdk/bin/pip install --disable-pip-version-check ./sdk/python && .cache/python-sdk/bin/python -c "from lutyjj_tplink import Provider, DhcpReservation, DhcpServer, Upnp, Dmz, RemoteAdmin, InboundRules"'

sdk-dotnet-check:
	$(COMPOSE) run --rm dotnet-tools dotnet build sdk/dotnet --nologo

sdk-java-check:
	$(COMPOSE) run --rm -e PACKAGE_VERSION=$(VERSION) java-tools gradle --no-daemon -p sdk/java build

secret-check: ## Scan committed history and the working diff with Gitleaks.
	$(COMPOSE) run --rm secret-tools gitleaks git --redact --log-opts=--all .
	git diff HEAD --no-ext-diff | $(COMPOSE) run --rm -T secret-tools gitleaks stdin --redact

workflow-check: ## Validate GitHub Actions workflows.
	$(GO) go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12

check: lint test workflow-check ## Check provider source, lifecycle tests and workflows.

release-snapshot: ## Build all six plugin archives without publishing.
	$(COMPOSE) run --rm release-tools goreleaser release --snapshot --clean --skip=publish

release-check: release-snapshot ## Verify archives and load an installed snapshot in an isolated plugin cache.
	@for os in linux darwin windows; do for arch in amd64 arm64; do \
		binary=pulumi-resource-$(PACK); [ "$$os" != windows ] || binary=$$binary.exe; \
		tar -tzf dist/pulumi-resource-$(PACK)-v*-$$os-$$arch.tar.gz | grep -Fx "$$binary" >/dev/null || exit 1; \
	done; done
	@set -eu; version=$$($(COMPOSE) run --rm schema-tools -r .version dist/metadata.json); \
	$(PULUMI) sh -ec 'export PULUMI_HOME=$$(mktemp -d); trap "rm -rf $$PULUMI_HOME" EXIT; \
		case $$(uname -m) in x86_64) arch=amd64;; aarch64) arch=arm64;; *) exit 1;; esac; \
		pulumi plugin install resource $(PACK) "$$1" --file "dist/pulumi-resource-$(PACK)-v$$1-linux-$$arch.tar.gz"; \
		pulumi package get-schema "$$PULUMI_HOME/plugins/resource-$(PACK)-v$$1/pulumi-resource-$(PACK)" > .cache/release-schema.json' sh "$$version"; \
	$(COMPOSE) run --rm schema-tools -e --arg version "$$version" '.name == "$(PACK)" and .version == $$version and (.resources | length) == 6' .cache/release-schema.json

release: ## Attach archives to an existing release draft. Publication belongs to CI.
	$(COMPOSE) run --rm -e GITHUB_TOKEN release-tools goreleaser release --clean
