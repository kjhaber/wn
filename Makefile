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

# Run unit tests
test:
	@go test ./...

# Run tests with coverage and print report
cover:
	@mkdir -p $(BUILD_DIR)
	@go test ./... -coverprofile=$(BUILD_DIR)/coverage.out
	@go tool cover -func=$(BUILD_DIR)/coverage.out

# Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/wn ./cmd/wn

# Remove all build outputs
clean:
	@rm -rf $(BUILD_DIR)
