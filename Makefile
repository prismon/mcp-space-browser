# Makefile for mcp-space-browser

# Binary name
BINARY_NAME=mcp-space-browser

# Module path
MODULE=github.com/prismon/mcp-space-browser

# Build directory
BUILD_DIR=build

# Main package path
MAIN_PACKAGE=./cmd/mcp-space-browser

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOCLEAN=$(GOCMD) clean
GOINSTALL=$(GOCMD) install
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet
GOMOD=$(GOCMD) mod

# Test environment
TEST_ENV=GO_ENV=test

# Build flags
LDFLAGS=-ldflags "-s -w"
BUILD_FLAGS=-trimpath

# Default target
.PHONY: all
all: clean build

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build without optimizations (for debugging)
.PHONY: build-debug
build-debug:
	@echo "Building $(BINARY_NAME) (debug)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Debug build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Install the binary to $GOPATH/bin
.PHONY: install
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOINSTALL) $(MAIN_PACKAGE)
	@echo "Install complete"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(TEST_ENV) $(GOTEST) ./...

# Run tests with verbose output
.PHONY: test-verbose
test-verbose:
	@echo "Running tests (verbose)..."
	$(TEST_ENV) $(GOTEST) -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(TEST_ENV) $(GOTEST) -v -cover -coverprofile=coverage.out ./...
	@echo "Coverage report generated: coverage.out"

# View coverage report in browser
.PHONY: coverage-html
coverage-html: test-coverage
	$(GOCMD) tool cover -html=coverage.out

# Run specific package tests
.PHONY: test-pkg
test-pkg:
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make test-pkg PKG=./pkg/crawler"; \
		exit 1; \
	fi
	@echo "Running tests for $(PKG)..."
	$(TEST_ENV) $(GOTEST) -v $(PKG)

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out
	@rm -rf cache
	@echo "Clean complete"

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Run go mod tidy
.PHONY: tidy
tidy:
	@echo "Running go mod tidy..."
	$(GOMOD) tidy

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

# Verify dependencies
.PHONY: verify
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Run the MCP server
.PHONY: run
run: build
	@echo "Starting MCP server..."
	$(BUILD_DIR)/$(BINARY_NAME) server --port=3000

# Run the MCP server in development mode (without building)
.PHONY: run-dev
run-dev:
	@echo "Starting MCP server (dev mode)..."
	$(GOCMD) run $(MAIN_PACKAGE) server --port=3000

# Run disk-index command (requires PATH argument)
.PHONY: index
index:
	@if [ -z "$(PATH_ARG)" ]; then \
		echo "Usage: make index PATH_ARG=/path/to/scan"; \
		exit 1; \
	fi
	$(GOCMD) run $(MAIN_PACKAGE) disk-index $(PATH_ARG)

# Run disk-du command (requires PATH argument)
.PHONY: du
du:
	@if [ -z "$(PATH_ARG)" ]; then \
		echo "Usage: make du PATH_ARG=/path/to/analyze"; \
		exit 1; \
	fi
	$(GOCMD) run $(MAIN_PACKAGE) disk-du $(PATH_ARG)

# Run disk-tree command (requires PATH argument)
.PHONY: tree
tree:
	@if [ -z "$(PATH_ARG)" ]; then \
		echo "Usage: make tree PATH_ARG=/path/to/show"; \
		exit 1; \
	fi
	$(GOCMD) run $(MAIN_PACKAGE) disk-tree $(PATH_ARG)

# Check for common issues (fmt, vet, test)
.PHONY: check
check: fmt vet test

# Pre-commit hook (format, vet, test)
.PHONY: pre-commit
pre-commit: fmt vet test
	@echo "Pre-commit checks passed!"

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make build          - Build the binary"
	@echo "  make build-debug    - Build with debug symbols"
	@echo "  make install        - Install binary to GOPATH/bin"
	@echo "  make test           - Run all tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make coverage-html  - View coverage report in browser"
	@echo "  make test-pkg       - Run tests for specific package (use PKG=...)"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make fmt            - Format code"
	@echo "  make vet            - Run go vet"
	@echo "  make tidy           - Run go mod tidy"
	@echo "  make deps           - Download dependencies"
	@echo "  make verify         - Verify dependencies"
	@echo "  make run            - Build and run MCP server"
	@echo "  make run-dev        - Run MCP server without building"
	@echo "  make index          - Run disk-index (use PATH_ARG=...)"
	@echo "  make du             - Run disk-du (use PATH_ARG=...)"
	@echo "  make tree           - Run disk-tree (use PATH_ARG=...)"
	@echo "  make check          - Run fmt, vet, and test"
	@echo "  make pre-commit     - Run pre-commit checks"
	@echo "  make help           - Show this help message"
