# wn Makefile — formatter, linter, tests, coverage, build

BUILD_DIR := build
GOLANGCI_LINT_VERSION := v2.11.3
GOLANGCI_LINT := $(BUILD_DIR)/golangci-lint

.PHONY: fmt lint test cover build completions clean all

# Default target runs all quality checks then builds
all: fmt lint cover build

# Check that all Go files are formatted (fails if any need formatting)
fmt:
	@test -z "$$(gofmt -l .)" || (echo "These files need formatting (run: gofmt -w .):"; gofmt -l .; exit 1)

# Download golangci-lint binary if not present at the pinned version
$(GOLANGCI_LINT):
	@mkdir -p $(BUILD_DIR)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(BUILD_DIR) $(GOLANGCI_LINT_VERSION)

# Run golangci-lint
lint: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run

# Run unit tests (WN_PICKER=numbered forces numbered list so tests don't block on fzf)
# WN_SETTINGS_USER/USER_LOCAL are cleared so user's env doesn't leak into test isolation via WN_CONFIG_DIR.
# GIT_AUTHOR_*/GIT_COMMITTER_* are set so tests that call "git commit" work without global git config (e.g. CI).
TEST_ENV := WN_PICKER=numbered WN_SETTINGS_USER= WN_SETTINGS_USER_LOCAL= \
	GIT_AUTHOR_NAME="Test" GIT_AUTHOR_EMAIL="test@example.com" \
	GIT_COMMITTER_NAME="Test" GIT_COMMITTER_EMAIL="test@example.com"

test:
	@$(TEST_ENV) go test ./...

# Run tests with coverage and print report
cover:
	@mkdir -p $(BUILD_DIR)
	@$(TEST_ENV) go test ./... -coverprofile=$(BUILD_DIR)/coverage.out
	@go tool cover -func=$(BUILD_DIR)/coverage.out

# Build the binary (inject version from nearest git tag, fallback to "dev")
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
build:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/wn ./cmd/wn

# Generate shell completion scripts (zsh, bash, fish) into build/completions/
completions: build
	@mkdir -p $(BUILD_DIR)/completions
	@$(BUILD_DIR)/wn completion zsh > $(BUILD_DIR)/completions/_wn
	@$(BUILD_DIR)/wn completion bash > $(BUILD_DIR)/completions/wn.bash
	@$(BUILD_DIR)/wn completion fish > $(BUILD_DIR)/completions/wn.fish

# Remove all build outputs (keep .wn/ for work items)
clean:
	@rm -rf $(BUILD_DIR)
