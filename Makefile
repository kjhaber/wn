# wn Makefile — formatter, linter, tests, coverage, build

BUILD_DIR := build

.PHONY: fmt lint test cover build clean all

# Default target runs all quality checks then builds
all: fmt lint cover build

# Check that all Go files are formatted (fails if any need formatting)
fmt:
	@test -z "$$(gofmt -l .)" || (echo "These files need formatting (run: gofmt -w .):"; gofmt -l .; exit 1)

# Run golangci-lint
lint:
	@golangci-lint run

# Run unit tests (WN_PICKER=numbered forces numbered list so tests don't block on fzf)
test:
	@WN_PICKER=numbered go test ./...

# Run tests with coverage and print report
cover:
	@mkdir -p $(BUILD_DIR)
	@WN_PICKER=numbered go test ./... -coverprofile=$(BUILD_DIR)/coverage.out
	@go tool cover -func=$(BUILD_DIR)/coverage.out

# Build the binary (inject version from nearest git tag, fallback to "dev")
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
build:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/wn ./cmd/wn

# Remove all build outputs
clean:
	@rm -rf $(BUILD_DIR)
