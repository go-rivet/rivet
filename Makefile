# Variables
BINARY_NAME=rivet
SRC_DIR=./cmd/rivet
SRCS := $(shell find . -name '*.go')
BUILD_DIR=bin
DIST_DIR     = dist
MODULE_PATH = github.com/go-rivet/rivet/internal/version
GOLANGCI_LINT_VERSION := v2.12.2
GOTESTSUM_VERSION := v1.12.0

# Dynamic build information extracted from local environment
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0-dev")
COMMIT_SHA  = $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME  = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Consolidate Linker Injection Flags (-X sets the string value at link time)
LD_FLAGS = -s -w -extldflags \"-static\" \
           -X '$(MODULE_PATH).Version=$(VERSION)' \
           -X '$(MODULE_PATH).CommitSHA=$(COMMIT_SHA)' \
           -X '$(MODULE_PATH).BuildTime=$(BUILD_TIME)'

# Target OS and Architectures (The GoReleaser matrix equivalent)
PLATFORMS    = darwin/amd64 \
               darwin/arm64 \
               linux/386 \
               linux/amd64 \
               linux/arm64 \
               linux/arm/7 \
               windows/386 \
               windows/amd64 \
               windows/arm64 \
               windows/arm/7

# Dynamically locate the user's Go path or Go bin directory
GOPATH=$(shell go env GOPATH)
GOBIN=$(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN=$(GOPATH)/bin
endif

.PHONY: all build clean run release test test-all generate mod lint install help $(PLATFORMS)

# Default target runs help to guide the user
all: help

## build: Build the binary for the current OS/Architecture
build: $(BUILD_DIR)/$(BINARY_NAME)
$(BUILD_DIR)/$(BINARY_NAME): $(SRCS)
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build \
		-a \
		-tags "netgo osusergo" \
		-ldflags "$(LD_FLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME) $(SRC_DIR)
	@chmod +x $(BUILD_DIR)/$(BINARY_NAME)
	@echo "Binary built at $(BUILD_DIR)/$(BINARY_NAME)"

## install: Install the binary to the standard user Go bin directory
install: build
	@echo "Installing $(BINARY_NAME) to $(GOBIN)..."
	@mkdir -p $(GOBIN)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOBIN)/$(BINARY_NAME)
	@echo "Installation successful."

## run: Build and run the local binary immediately
run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --version

## test: Run all project unit tests natively with race detection
test:
	@echo "Running tests..."
#	gotestsum --format short-verbose -- ./... 
	gotestsum --format short-verbose -- -count=1 ./... 

## test-all: Run unit tests and other integration/e2e tests
test-all:
	@echo "Running tests..."
	go test -v -race ./... -tags 'signals watch'

## lint: Lint (and fix) the code base
lint:
	@echo "Running golangci-lint..."
	golangci-lint run --fix ./...

## generate: Generate golden fixture files
generate:
	@echo "==> Cleaning old golden fixtures..."
	find ./testdata -name '*.golden' -delete 2>/dev/null || true
	@echo "==> Generating new golden fixtures via go test..."
	GOLDIE_UPDATE='true' GOLDIE_TEMPLATE='true' go test ./...

## mod: Download dependencies and tidy module files
mod:
	@echo "==> Downloading Go modules..."
	go mod download
	@echo "==> Tidying go.mod and go.sum..."
	go mod tidy
	@echo "==> Verifying dependency integrity..."
	go mod verify

## deps: Install all required CLI tools and download Go module dependencies
deps: 
	@echo "==> Verifying and installing system tool dependencies..."
	@mkdir -p $(GOBIN)
	# Install golangci-lint if missing
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION); \
	else \
		echo "✓ golangci-lint is already installed."; \
	fi
	# Install gotestsum if missing (required for test summaries and test-watch)
	@if ! command -v gotestsum >/dev/null 2>&1; then \
		echo "Installing gotestsum $(GOTESTSUM_VERSION)..."; \
		CGO_ENABLED=0 go install github.com; \
	else \
		echo "✓ gotestsum is already installed."; \
	fi
	@echo "==> All dependencies successfully installed and verified."

## clean: Remove all build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)

## help: Show this help screen
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/## //' | awk -F: '{printf "  %-15s %s\n", $$1, $$2}'




# Entry point for building everything, packaging, and generating checksums
release: clean $(PLATFORMS) checksums

# Dynamic matrix execution block
$(PLATFORMS):
	$(eval OS := $(word 1,$(subst /, ,$@)))
	$(eval ARCH := $(word 2,$(subst /, ,$@)))
	$(eval SUBARCH := $(word 3,$(subst /, ,$@)))
	$(eval EXT := $(if $(filter windows,$(OS)),.exe,))
	
	# Determine naming and sub-architecture constraints (GOARM / GO386)
	$(eval FILENAME_ARCH := $(if $(filter arm,$(ARCH)),$(if $(SUBARCH),$(ARCH)v$(SUBARCH),$(ARCH)),$(ARCH)))
	$(eval GOARM_FLAG := $(if $(filter arm,$(ARCH)),$(if $(SUBARCH),$(SUBARCH),7),))
	$(eval GO386_FLAG := $(if $(filter 386,$(ARCH)),sse2,))
	$(eval OUT_NAME := $(BINARY_NAME)-$(OS)-$(FILENAME_ARCH))
	
	@echo "Building for $(OS) ($(FILENAME_ARCH))..."
	@mkdir -p $(DIST_DIR)
	
	# Compile using your exact static linking safety flags and mapped environment variables
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) GOARM=$(GOARM_FLAG) GO386=$(GO386_FLAG) go build \
		-a \
		-trimpath \
		-tags "netgo osusergo" \
		-ldflags "$(LD_FLAGS)" \
		-o $(DIST_DIR)/$(OUT_NAME)$(EXT) $(SRC_DIR)
	
	@if [ "$(OS)" != "windows" ]; then chmod +x $(DIST_DIR)/$(OUT_NAME)$(EXT); fi
	
	# GoReleaser style packaging
	@cd $(DIST_DIR) && \
	if [ "$(OS)" = "windows" ]; then \
		zip -q $(OUT_NAME).zip $(OUT_NAME)$(EXT); \
		rm $(OUT_NAME)$(EXT); \
	else \
		tar -czf $(OUT_NAME).tar.gz $(OUT_NAME)$(EXT); \
		rm $(OUT_NAME)$(EXT); \
	fi

# Generate GoReleaser-style sha256 checksum tracking verification file
checksums:
	@echo "=> Generating sha256 checksums..."
	@cd $(DIST_DIR) && shasum -a 256 * > $(BINARY_NAME)_checksums.txt
	@echo "=> Release artifacts generated successfully in ./$(DIST_DIR)/"