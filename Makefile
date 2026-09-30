# pulumi-tplink control surface. Run `make help` for a target overview.
#
# Every tool runs in a container, so the host needs only docker or podman and make.
# One pinned image supplies Pulumi and the Go/Node toolchains; linting and release
# builds use the vendors' own images.

CONTAINER_RUNTIME := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

PULUMI_IMAGE        := docker.io/pulumi/pulumi:3.262.0@sha256:cb023f08fc0f8776dc243aa371c11f923c4da52c4477e5679a4610de10e50423
GOLANGCI_LINT_IMAGE := docker.io/golangci/golangci-lint:v2.14.0@sha256:ad862ba6b3798cbe0fd9fd7408d498fd74fbd2623a92406b2fd3898faf0bf98f
GORELEASER_IMAGE    := docker.io/goreleaser/goreleaser:v2.18.2

PACK        := tplink
PROVIDER    := pulumi-resource-$(PACK)
CMD_DIR     := provider/cmd/$(PROVIDER)
SCHEMA_FILE := $(CMD_DIR)/schema.json

# Rootless podman already maps the container's root onto the invoking user, so files
# written into the work tree come out owned correctly. Docker's daemon runs containers
# as real root, which would leave root-owned build output behind. Pin the host uid/gid
# there. HOME moves with it because the image's /root is not writable by another uid.
ifeq ($(notdir $(CONTAINER_RUNTIME)),docker)
CONTAINER_AS_USER := --user $(shell id -u):$(shell id -g)
endif

# Caches live in the work tree (gitignored) so repeat runs stay fast and nothing
# escapes the checkout.
CONTAINER_ENV := \
	-e HOME=/tmp \
	-e GOPATH=/workspace/.cache/go \
	-e GOCACHE=/workspace/.cache/go-build \
	-e GOFLAGS=-buildvcs=false \
	-e PULUMI_HOME=/workspace/.cache/pulumi \
	-e PULUMI_SKIP_UPDATE_CHECK=true \
	-e PULUMI_AUTOMATION_API_SKIP_VERSION_CHECK=true

define RUN_IN
$(CONTAINER_RUNTIME) run --rm $(CONTAINER_AS_USER) $(CONTAINER_ENV) \
	--entrypoint /usr/bin/env \
	-v $(CURDIR):/workspace -w /workspace $(1)
endef

TOOLBOX := $(call RUN_IN,$(PULUMI_IMAGE))

# Version stamped into the binary. CI overrides it from the release tag.
VERSION ?= 0.1.0-dev
VERSION_PKG := github.com/lutyjj/pulumi-tplink/provider.Version

.DEFAULT_GOAL := help

.PHONY: help
help: ## List targets.
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_-]+:.*## /{printf "  %-16s %s\n",$$1,$$2}' $(MAKEFILE_LIST)

.PHONY: tidy
tidy: ## go mod tidy.
	@$(TOOLBOX) go mod tidy

.PHONY: fmt
fmt: ## Format Go sources with gofmt.
	@$(TOOLBOX) gofmt -s -w provider internal

.PHONY: build
build: ## Build the provider binary into bin/.
	@mkdir -p bin
	@$(TOOLBOX) go build -o bin/$(PROVIDER) -ldflags "-X $(VERSION_PKG)=$(VERSION)" ./$(CMD_DIR)

.PHONY: test
test: ## Run the unit and lifecycle tests.
	@$(TOOLBOX) go test -race -count=1 ./provider/... ./internal/...

.PHONY: vet
vet: ## go vet.
	@$(TOOLBOX) go vet ./provider/... ./internal/...

.PHONY: lint
lint: ## golangci-lint.
	@$(call RUN_IN,$(GOLANGCI_LINT_IMAGE)) golangci-lint run ./provider/... ./internal/...

.PHONY: schema
schema: build ## Regenerate the committed schema from the built provider.
	@$(TOOLBOX) sh -c 'pulumi package get-schema ./bin/$(PROVIDER) | jq "del(.version)" > $(SCHEMA_FILE)'

.PHONY: sdks
sdks: schema ## Generate the language SDKs under sdk/ (gitignored; not committed).
	@for lang in python go dotnet java; do \
		rm -rf sdk/$$lang; \
		$(TOOLBOX) pulumi package gen-sdk --language $$lang --out sdk --version $(VERSION) $(SCHEMA_FILE) || exit $$?; \
	done
	@$(MAKE) --no-print-directory sdk-nodejs

.PHONY: sdk-nodejs
sdk-nodejs: build ## Generate and compile the local Node SDK.
	@$(TOOLBOX) pulumi package gen-sdk --local --language nodejs --out sdk ./bin/$(PROVIDER)
	@$(TOOLBOX) sh -c 'cd sdk/nodejs && npm pkg set version=$(VERSION) && npm install --no-audit --no-fund'

.PHONY: check
check: vet test lint tools-check ## Everything CI runs.

.PHONY: release-snapshot
release-snapshot: ## Cross-compile the release archives into dist/ without publishing.
	@$(call RUN_IN,$(GORELEASER_IMAGE)) goreleaser release --snapshot --clean --skip=publish

.PHONY: api-spec api-spec-sampled tools-check
api-spec: ## Recover the firmware API spec. TPLINK_HOST=<router> [TPLINK_INSECURE=true].
	@$(TOOLBOX) npm ci --ignore-scripts
	@$(CONTAINER_RUNTIME) run --rm $(CONTAINER_AS_USER) $(CONTAINER_ENV) -e TPLINK_HOST -e TPLINK_INSECURE --entrypoint /usr/bin/env -v $(CURDIR):/workspace -w /workspace $(PULUMI_IMAGE) node tools/codegen/main.mts all

api-spec-sampled: ## Recover the spec with read-only sampling; also requires TPLINK_PASSWORD.
	@$(TOOLBOX) npm ci --ignore-scripts
	@$(CONTAINER_RUNTIME) run --rm $(CONTAINER_AS_USER) $(CONTAINER_ENV) \
		-e TPLINK_HOST -e TPLINK_INSECURE -e TPLINK_PASSWORD \
		--entrypoint /usr/bin/env -v $(CURDIR):/workspace -w /workspace $(PULUMI_IMAGE) \
		node tools/codegen/main.mts all

tools-check: ## Typecheck the API recovery tooling (no router access).
	@$(TOOLBOX) npm ci --ignore-scripts
	@$(TOOLBOX) npx tsc --noEmit -p tools

.PHONY: release
release: ## Publish release archives for the current tag. Requires explicit operator approval.
	@$(CONTAINER_RUNTIME) run --rm $(CONTAINER_AS_USER) $(CONTAINER_ENV) -e GITHUB_TOKEN \
		--entrypoint /usr/bin/env -v $(CURDIR):/workspace -w /workspace $(GORELEASER_IMAGE) \
		goreleaser release --clean
