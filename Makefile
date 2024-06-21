# Image URL to use all building/pushing image targets
IMG ?= plugin:latest

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: ## Run tests.
	@go test ./...

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.59.2
golangci-lint:
	@[ -f $(GOLANGCI_LINT) ] || { \
	set -e ;\
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell dirname $(GOLANGCI_LINT)) $(GOLANGCI_LINT_VERSION) ;\
	}

.PHONY: tidy
tidy: ## Runs go mod tidy on the plugin
	@go mod tidy -v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: ## Build WASM binary.
	@mkdir -p dist
	@tinygo build -o dist/main.wasm -scheduler=none -target=wasi ./main.go

.PHONY: run
run: envoy ## Run an envoy using your plugin from your host
	$(ENVOY) -c ./docs/samples/envoy-config.yaml --concurrency 2 --log-format '%v'

# Build docker images of *compat* variant of Wasm Image Specification
# If you wish to build the plugin image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: build ## Build docker image with the plugin
	$(CONTAINER_TOOL) build -t ${IMG} . -f Dockerfile --build-arg WASM_BINARY_PATH=dist/main.wasm

.PHONY: docker-push
docker-push: ## Push docker image with the plugin.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/myplugin0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the plugin for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

# Build OCI images of *compat* variant of Wasm Image Specification with built example binaries,
# and push to ghcr.io/tetratelabs/proxy-wasm-go-sdk-examples.
# See https://github.com/solo-io/wasm/blob/master/spec/spec-compat.md for details.
# Only-used in github workflow on the main branch, and not for developers.
# Requires "buildah" CLI.
.PHONY: docker-build-oci
docker-build-oci: build
	@buildah bud -f Dockerfile --build-arg WASM_BINARY_PATH=dist/main.wasm -t ${IMG} .

.PHONY: docker-push-oci
docker-push-oci: build
	@buildah push ${IMG}

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
ENVOY ?= $(LOCALBIN)/envoy

## Tool Versions
ENVOY_VERSION ?= 1.30.2

.PHONY: envoy
envoy: $(ENVOY) ## Download envoy locally if necessary.
$(ENVOY): $(LOCALBIN)
	@test -s $(LOCALBIN)/envoy || { \
		wget --timestamping --quiet https://github.com/envoyproxy/envoy/releases/download/v$(ENVOY_VERSION)/envoy-contrib-$(ENVOY_VERSION)-linux-x86_64 -P $(LOCALBIN); \
		mv $(LOCALBIN)/envoy-contrib-$(ENVOY_VERSION)-linux-x86_64 $(LOCALBIN)/envoy; \
		chmod +x $(LOCALBIN)/envoy; \
	}
