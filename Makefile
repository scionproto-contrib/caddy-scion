# Variables
BINARY_NAMES=scion-caddy scion-caddy-forward scion-caddy-reverse scion-caddy-native
RELEASE_BINARIES=scion-caddy-forward scion-caddy-reverse scion-caddy-native
SRC_DIR=./cmd
BUILD_DIR=./build

# Platform configurations
PLATFORMS_LINUX=linux-amd64 linux-arm64
PLATFORMS_DARWIN=darwin-amd64 darwin-arm64
PLATFORMS_WINDOWS=windows-amd64
PLATFORMS_ALL=$(PLATFORMS_LINUX) $(PLATFORMS_DARWIN) $(PLATFORMS_WINDOWS)

# Go commands
GO=go
GOFMT=gofmt
GOTEST=$(GO) test
GOBUILD=$(GO) build
GOCLEAN=$(GO) clean
GOVET=$(GO) vet

# Build flags for releases (static binaries)
RELEASE_FLAGS=CGO_ENABLED=0

# Build the project
all: build test

# Format the code
fmt:
	$(GOFMT) -w .

lint:
	@type golangci-lint > /dev/null || ( echo "golangci-lint not found. Install it manually"; exit 1 )
	golangci-lint run --timeout=2m

# Run all tests (E2E + integration via run-tests.sh)
test:
	@echo "Running all tests..."
	./run-tests.sh

# Run E2E tests only (requires SCION setup)
test-e2e:
	@echo "Running E2E tests..."
	./run-tests.sh e2e

# Run integration tests only (requires SCION setup, Docker, proxies)
test-integration:
	@echo "Running integration tests..."
	./run-tests.sh integration

# Build all binaries for local development (current platform)
build: $(BINARY_NAMES)

# Build each binary for local development
$(BINARY_NAMES):
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$@ $(SRC_DIR)/$@

# Helper function to build releases for given platforms
# Usage: $(MAKE) build-for-platforms PLATFORMS="linux-amd64 darwin-amd64"
build-for-platforms:
	@mkdir -p $(BUILD_DIR)
	@$(if $(PLATFORMS),,$(error PLATFORMS variable is required))
	@for binary in $(RELEASE_BINARIES); do \
		for platform in $(PLATFORMS); do \
			os=$${platform%%-*}; \
			arch=$${platform#*-}; \
			arch_name=$$arch; \
			if [ "$$arch" = "amd64" ]; then arch_name="x86_64"; fi; \
			ext=""; \
			if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
			output="$(BUILD_DIR)/$${binary}_"; \
			if [ "$$os" = "windows" ]; then \
				output="$${output}$${arch_name}$${ext}"; \
			else \
				output="$${output}$${os}_$${arch_name}"; \
			fi; \
			if [ "$$os" = "darwin" ] && ([ "$$binary" = "scion-caddy-reverse" ] || [ "$$binary" = "scion-caddy-native" ]); then \
				echo "Skipping $$binary for $$os/$$arch (not supported)"; \
			else \
				echo "Building $$binary for $$os/$$arch..."; \
				$(RELEASE_FLAGS) GOOS=$$os GOARCH=$$arch $(GOBUILD) -o $$output $(SRC_DIR)/$$binary || exit 1; \
			fi; \
		done; \
	done

# Build releases for all platforms
build-releases:
	@echo "Building releases for all platforms..."
	@$(MAKE) build-for-platforms PLATFORMS="$(PLATFORMS_ALL)"
	@echo "All release builds completed successfully!"

# Build for Linux platforms only
build-linux:
	@echo "Building for Linux platforms..."
	@$(MAKE) build-for-platforms PLATFORMS="$(PLATFORMS_LINUX)"
	@echo "Linux builds completed successfully!"

# Build for macOS/Darwin platforms only
build-macos:
	@echo "Building for macOS platforms..."
	@$(MAKE) build-for-platforms PLATFORMS="$(PLATFORMS_DARWIN)"
	@echo "macOS builds completed successfully!"

# Build for Windows platforms only
build-windows:
	@echo "Building for Windows platforms..."
	@$(MAKE) build-for-platforms PLATFORMS="$(PLATFORMS_WINDOWS)"
	@echo "Windows builds completed successfully!"

# Clean the build directory
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Run go vet
vet:
	$(GOVET) ./...

.PHONY: all fmt lint test test-e2e test-integration build clean vet $(BINARY_NAMES) build-for-platforms build-releases build-linux build-macos build-windows
