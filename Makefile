# Variables
BINARY_NAME=rivet
SRC_DIR=./cmd/rivet
SRCS := $(shell find . -type f \( -name '*.go' -o -name '*.c' -o -name '*.h' \))
BUILD_DIR=bin
DIST_DIR     = dist
MODULE_PATH = github.com/go-rivet/rivet/internal/version
GOLANGCI_LINT_VERSION := v2.12.2
GOTESTSUM_VERSION := v1.12.0
ZIG ?= zig
ZIG_VERSION ?= 0.14.1
TOOLCHAIN_IMAGE ?= rivet-build-toolchain:local
TOOLCHAIN_CACHE_DIR ?= $(or $(XDG_CACHE_HOME),$(HOME)/.cache)/rivet/toolchain
TOOLCHAIN_CACHE_FLAGS = -v "$(TOOLCHAIN_CACHE_DIR):/cache" -e GOPATH=/cache/go -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/go-build -e ZIG_GLOBAL_CACHE_DIR=/cache/zig
GO_VERSION ?= $(shell awk '$$1 == "go" { print $$2; exit }' go.mod)
MACOS_SDK_PATH ?= $(shell xcrun --sdk macosx --show-sdk-path 2>/dev/null)
CONTAINER_SDK_MOUNT = $(if $(MACOS_SDK_PATH),-v "$(MACOS_SDK_PATH):/opt/MacOSX.sdk:ro" -e MACOS_SDK_PATH=/opt/MacOSX.sdk,)
TARGET_OS ?= $(if $(GOOS),$(GOOS),$(shell uname -s | tr '[:upper:]' '[:lower:]' | sed 's/^mingw.*/windows/;s/^msys.*/windows/'))
TARGET_ARCH ?= $(if $(GOARCH),$(GOARCH),$(shell uname -m | sed 's/^x86_64$$/amd64/;s/^aarch64$$/arm64/;s/^arm64$$/arm64/;s/^armv7l$$/arm/;s/^i[3-6]86$$/386/'))

# Dynamic build information extracted from local environment
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0-dev")
COMMIT_SHA  = $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME  = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Consolidate Linker Injection Flags (-X sets the string value at link time)
LD_FLAGS = -s -w \
           -X '$(MODULE_PATH).Version=$(VERSION)' \
           -X '$(MODULE_PATH).CommitSHA=$(COMMIT_SHA)' \
           -X '$(MODULE_PATH).BuildTime=$(BUILD_TIME)'
LINUX_LD_FLAGS = -extldflags '-static -s'
NATIVE_LD_FLAGS = $(LD_FLAGS) $(if $(filter linux,$(shell go env GOOS)),$(LINUX_LD_FLAGS),)
NATIVE_PLATFORM = $(shell go env GOOS)/$(shell go env GOARCH)
NATIVE_ZIG_TARGET = $(call zig_target,$(if $(filter linux/arm,$(NATIVE_PLATFORM)),linux/arm/$(shell go env GOARM),$(NATIVE_PLATFORM)))
NATIVE_SDK_FLAGS = $(if $(filter darwin,$(shell go env GOOS)),-isysroot $(MACOS_SDK_PATH) -L$(MACOS_SDK_PATH)/usr/lib -F$(MACOS_SDK_PATH)/System/Library/Frameworks,)

# Target OS and Architectures (The GoReleaser matrix equivalent)
PLATFORMS    = linux/386 \
               linux/amd64 \
               linux/arm64 \
               linux/arm/7 \
               windows/386 \
               windows/amd64 \
               windows/arm64

# Dynamically locate the user's Go path or Go bin directory
GOPATH=$(shell go env GOPATH)
GOBIN=$(or $(shell go env GOBIN),$(GOPATH)/bin)

.PHONY: all build clean run release test test-42 test-all bench bench-all bench-mak generate generate-fixtures mod lint install help
.PHONY: .toolchain .build .release .checksums $(addprefix .release/,$(PLATFORMS))
.NOTPARALLEL: .release

# Default target runs help to guide the user
all: help

# quiet_test: run a verbose go test command; print the PASS/SKIP lines and package
# results, and for failing tests only, their own output
define quiet_test
out=$$(mktemp); $(1) >$$out 2>&1; rc=$$?; \
awk '\
/^=== (RUN|CONT|PAUSE|NAME) / { cur=$$3; next } \
/^ *--- FAIL/ { n=$$3; failed[n]=1; print; next } \
/^ *--- (PASS|SKIP)/ { print; next } \
/^(PASS|ok|FAIL)([ \t]|$$)/ { print; cur=""; next } \
{ buf[cur]=buf[cur] $$0 "\n" } \
END { for (n in failed) if (length(buf[n])) printf "\n=== output of %s\n%s", n, buf[n]; if (length(buf[""])) printf "\n%s", buf[""] }' $$out; \
rm -f $$out; exit $$rc
endef

## build: Build the binary for the current OS/Architecture using Docker
build: $(BUILD_DIR)/$(BINARY_NAME)
$(BUILD_DIR)/$(BINARY_NAME): $(SRCS) Makefile tools/toolchain/Dockerfile go.mod go.sum
	$(MAKE) .toolchain
	@mkdir -p "$(TOOLCHAIN_CACHE_DIR)"
	docker run --rm --user "$$(id -u):$$(id -g)" -e HOME=/tmp $(TOOLCHAIN_CACHE_FLAGS) \
		$(CONTAINER_SDK_MOUNT) -e GOOS="$(TARGET_OS)" -e GOARCH="$(TARGET_ARCH)" $(if $(filter arm,$(TARGET_ARCH)),-e GOARM=$(if $(GOARM),$(GOARM),7),) \
		-v "$(CURDIR):/src" -w /src $(TOOLCHAIN_IMAGE) \
		make .build BUILD_DIR="$(BUILD_DIR)" BINARY_NAME="$(BINARY_NAME)" SRC_DIR="$(SRC_DIR)" \
		VERSION="$(VERSION)" COMMIT_SHA="$(COMMIT_SHA)" BUILD_TIME="$(BUILD_TIME)"

.build:
	@test -n "$(NATIVE_ZIG_TARGET)" || { echo "Unsupported build platform: $(NATIVE_PLATFORM)" >&2; exit 1; }
	$(if $(filter darwin,$(shell go env GOOS)),$(if $(strip $(MACOS_SDK_PATH)),,@echo "A macOS SDK is required for Darwin cgo builds; set MACOS_SDK_PATH." >&2; exit 1),)
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CC="$(ZIG) cc -target $(NATIVE_ZIG_TARGET) $(NATIVE_SDK_FLAGS)" CGO_ENABLED=1 go build \
		-tags "netgo osusergo" \
		-ldflags "$(NATIVE_LD_FLAGS)" \
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
#	gotestsum --format short-verbose -- -failfast -count=1 -run="$(TEST)" ./...
	@$(call quiet_test,CC="$(ZIG) cc" CGO_ENABLED=1 go test -v -count=1 -run="$(TEST)" ./...)

## test-42: Run the v42 script tests (TEST=name filters, e.g. TEST=v42_hello)
test-42:
	@echo "Running v42 script tests..."
	@$(call quiet_test,CC="$(ZIG) cc" CGO_ENABLED=1 go test -v -count=1 ./cmd/rivet -run '^TestScript/$(TEST)')

## test-all: Run unit tests and other integration tests
test-all:
	@echo "Running tests..."
	@$(call quiet_test,CC="$(ZIG) cc" CGO_ENABLED=1 go test -v -race ./... -tags 'signals watch')

## bench: Run the selected benchmark (BENCH=BenchmarkRivet, BENCHTIME=3x by default); PROFILE=1 collects profiles
BENCH_DIR = bench_profiles
BENCH      ?= BenchmarkRivet
BENCHTIME  ?= 3x
PROFILE    ?= 0
BENCH_PROFILE_ENV =
BENCH_PROFILE_SETUP =
BENCH_PROFILE_REPORT =
ifeq ($(PROFILE),1)
BENCH_PROFILE_ENV = RIVET_BENCH_PROFILE_DIR=$(abspath $(BENCH_DIR))
BENCH_PROFILE_SETUP = @mkdir -p $(BENCH_DIR) && rm -f $(BENCH_DIR)/*.out
BENCH_PROFILE_REPORT = @echo "Profiles written to $(BENCH_DIR)/:"; ls -1 $(BENCH_DIR)/*_cpu.out $(BENCH_DIR)/*_mem.out 2>/dev/null | sed 's/^/  /' || echo "  (none found)"; echo "View with: go tool pprof -http=:8080 $(BENCH_DIR)/<profile>.out"
endif
bench:
	@echo "Running benchmarks..."
	$(BENCH_PROFILE_SETUP)
	$(BENCH_PROFILE_ENV) go test ./cmd/rivet/... \
		-run='^$$' \
		-bench='$(BENCH)' \
		-benchmem \
		-benchtime='$(BENCHTIME)'
	$(BENCH_PROFILE_REPORT)

## bench-all: Run default and optional Rivet benchmarks, excluding Make comparisons
bench-all:
	@echo "Running default Rivet benchmarks..."
	go test ./cmd/rivet/... -run='^$$' -bench='^BenchmarkRivet$$' -benchmem -benchtime='$(BENCHTIME)'
	@echo "Running optional Rivet mtime benchmarks..."
	go test ./cmd/rivet/... -run='^$$' -bench='^BenchmarkMtimeRivetFull$$/^mtime_' -benchmem -benchtime='$(BENCHTIME)'
	@echo "Running optional Rivet many-task benchmarks..."
	go test ./cmd/rivet/... -run='^$$' -bench='^BenchmarkManyTasksCompare$$/^rivet_' -benchmem -benchtime='$(BENCHTIME)'

## bench-mak: Run all optional Make comparison benchmarks
bench-mak:
	@echo "Running optional Make mtime benchmarks..."
	go test ./cmd/rivet/... -run='^$$' -bench='^BenchmarkMtimeRivetFull$$/^make_mtime_' -benchmem -benchtime='$(BENCHTIME)'
	@echo "Running optional Make many-task benchmarks..."
	go test ./cmd/rivet/... -run='^$$' -bench='^BenchmarkManyTasksCompare$$/^make_' -benchmem -benchtime='$(BENCHTIME)'

## test-e2e: Run E2E tests (from ../rivet-e2e/catalog)

ifdef TEST 
TESTSCRIPT_E2E_FILES := ../rivet-e2e/catalog/$(TEST)
else 
TESTSCRIPT_E2E_FILES := ../rivet-e2e/catalog
endif
test-e2e:
	re2e -d $(TESTSCRIPT_E2E_FILES)

## lint: Lint (and fix) the code base
lint:
	@echo "Running golangci-lint..."
	golangci-lint run --fix ./...

## generate: Generate documentation (AST based)
generate:
	go generate ./...

## generate-fixtures: Generate golden fixture files
generate-fixtures:
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
	@echo ""
	@echo "Examples:"
	@echo "  make test TEST=TestFoo"
	@echo "  make test-e2e TEST=task/transform/task.transform.patsubst.txtar"

## release: Build Linux and Windows release artifacts using Docker
release: .toolchain
	$(if $(filter darwin/%,$(PLATFORMS)),@test -n "$(MACOS_SDK_PATH)" && test -d "$(MACOS_SDK_PATH)" || { echo "A macOS SDK is required for Darwin cgo builds; set MACOS_SDK_PATH." >&2; exit 1; },)
	@mkdir -p "$(TOOLCHAIN_CACHE_DIR)"
	docker run --rm --user "$$(id -u):$$(id -g)" -e HOME=/tmp $(TOOLCHAIN_CACHE_FLAGS) $(CONTAINER_SDK_MOUNT) \
		-v "$(CURDIR):/src" -w /src $(TOOLCHAIN_IMAGE) \
		make .release BUILD_DIR="$(BUILD_DIR)" DIST_DIR="$(DIST_DIR)" BINARY_NAME="$(BINARY_NAME)" SRC_DIR="$(SRC_DIR)" PLATFORMS="$(PLATFORMS)" \
		VERSION="$(VERSION)" COMMIT_SHA="$(COMMIT_SHA)" BUILD_TIME="$(BUILD_TIME)"

.toolchain:
	docker build \
		--build-arg GO_VERSION=$(GO_VERSION) \
		--build-arg ZIG_VERSION=$(ZIG_VERSION) \
		-f tools/toolchain/Dockerfile \
		-t $(TOOLCHAIN_IMAGE) .

.release: clean $(addprefix .release/,$(PLATFORMS)) .checksums

# Dynamic matrix execution block
$(addprefix .release/,$(PLATFORMS)):
	$(eval PLATFORM := $(patsubst .release/%,%,$@))
	$(eval OS := $(word 1,$(subst /, ,$(PLATFORM))))
	$(eval ARCH := $(word 2,$(subst /, ,$(PLATFORM))))
	$(eval SUBARCH := $(word 3,$(subst /, ,$(PLATFORM))))
	$(eval EXT := $(if $(filter windows,$(OS)),.exe,))
	
	# Determine naming and sub-architecture constraints (GOARM / GO386)
	$(eval FILENAME_ARCH := $(if $(filter arm,$(ARCH)),$(if $(SUBARCH),$(ARCH)v$(SUBARCH),$(ARCH)),$(ARCH)))
	$(eval GOARM_FLAG := $(if $(filter arm,$(ARCH)),$(if $(SUBARCH),$(SUBARCH),7),))
	$(eval GO386_FLAG := $(if $(filter 386,$(ARCH)),sse2,))
	$(eval OUT_NAME := $(BINARY_NAME)-$(OS)-$(FILENAME_ARCH))
	$(eval ZIG_TARGET := $(call zig_target,$(PLATFORM)))
	
	@echo "Building for $(OS) ($(FILENAME_ARCH))..."
	@mkdir -p $(DIST_DIR)
	$(if $(filter darwin,$(OS)),$(if $(strip $(MACOS_SDK_PATH)),,@echo "A macOS SDK is required for Darwin cgo builds; set MACOS_SDK_PATH or build on macOS." >&2; exit 1),)
	
	# Linux uses musl and static linking; other targets use Zig's platform toolchain.
	CC="$(ZIG) cc -target $(ZIG_TARGET) $(if $(filter darwin,$(OS)),-isysroot $(MACOS_SDK_PATH) -L$(MACOS_SDK_PATH)/usr/lib -F$(MACOS_SDK_PATH)/System/Library/Frameworks,)" CGO_ENABLED=1 GOOS=$(OS) GOARCH=$(ARCH) GOARM=$(GOARM_FLAG) GO386=$(GO386_FLAG) go build \
		-buildvcs=false \
		-trimpath \
		-tags "netgo osusergo" \
		-ldflags "$(LD_FLAGS) $(if $(filter linux,$(OS)),$(LINUX_LD_FLAGS),)" \
		-o $(DIST_DIR)/$(OUT_NAME)$(EXT) $(SRC_DIR)
	
	# GoReleaser style packaging
	@cd $(DIST_DIR) && \
	if [ "$(OS)" = "windows" ]; then \
		zip -q $(OUT_NAME).zip $(OUT_NAME)$(EXT); \
		rm $(OUT_NAME)$(EXT); \
	else \
		tar --mode=a+x -czf $(OUT_NAME).tar.gz $(OUT_NAME)$(EXT); \
		rm $(OUT_NAME)$(EXT); \
	fi

# Generate GoReleaser-style sha256 checksum tracking verification file
.checksums:
	@echo "=> Generating sha256 checksums..."
	@cd $(DIST_DIR) && shasum -a 256 * > $(BINARY_NAME)_checksums.txt
	@echo "=> Release artifacts generated successfully in ./$(DIST_DIR)/"

zig_target = $(if $(filter darwin/amd64,$1),x86_64-macos,$(if $(filter darwin/arm64,$1),aarch64-macos,$(if $(filter linux/386,$1),x86-linux-musl,$(if $(filter linux/amd64,$1),x86_64-linux-musl,$(if $(filter linux/arm64,$1),aarch64-linux-musl,$(if $(filter linux/arm/7,$1),arm-linux-musleabihf,$(if $(filter windows/386,$1),x86-windows-gnu,$(if $(filter windows/amd64,$1),x86_64-windows-gnu,$(if $(filter windows/arm64,$1),aarch64-windows-gnu,)))))))))