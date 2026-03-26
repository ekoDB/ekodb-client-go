# Makefile for ekoDB Go Client Library

# Environment variables
GO := go
MODULE := github.com/ekoDB/ekodb-client-go

# Color codes for pretty output
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

# Color codes for Worthington jet
MAGENTA := \033[35m
PURPLE := \033[35m
BLUE := \033[34m
BOLD := \033[1m
DIM := \033[2m

# Worthington jet
JET := "                    $(MAGENTA)●$(RESET)\n                    $(PURPLE)█$(RESET)\n                $(BLUE)▄▀▄$(PURPLE)█▀█$(BLUE)▄▀▄$(RESET)"

# ASCII Banner for ekoDB (matches CLI banner)
BANNER := "$(BOLD) ██████═╗ ██╗  ██╗  ██████╗  ████████╗ ████████╗$(RESET)\n$(BOLD)██╔═══██╝ ██║ ██╔╝ ██╔═══██╗  ██╔═══██║ ██╔═══██╗$(RESET)\n$(BOLD)████████╗ █████╔╝  ██║   ██║  ██║   ██║████████╔╝$(RESET)\n$(BOLD)██╔═════╝ ██╔═██╗  ██║   ██║  ██║   ██║ ██╔═══██╗$(RESET)\n$(BOLD)████████╗ ██║  ██╗ ╚██████╔╝ ████████║ ████████╔╝$(RESET)\n$(BOLD)╚═══════╝ ╚═╝  ╚═╝  ╚═════╝  ╚═══════╝ ╚═══════╝$(RESET)"

.PHONY: all build test test-verbose test-coverage clean fmt fmt-go fmt-md fmt-check format lint vet mod-tidy mod-verify mod-download install help setup deps-check deps-update publish bump-version check-ready examples pre-commit ensure-hooks version info

# Language Sub-Banner
GO_BANNER := \
	"🔷 Go Client Library" "\n"

# Default target
all: build

help:
	@echo $(JET)
	@echo ""
	@echo $(BANNER)
	@echo ""
	@echo $(GO_BANNER)
	@echo "✨ $(CYAN)ekoDB Go Client Library ✨$(RESET)"
	@echo ""
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "📌 $(CYAN)BUILD & DEVELOPMENT$(RESET)"
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "  🛠️  $(GREEN)make build$(RESET)          - Build the Go client library"
	@echo "  🧪 $(GREEN)make test$(RESET)           - Run all tests"
	@echo "  🧪 $(GREEN)make test-verbose$(RESET)   - Run tests with verbose output"
	@echo "  📊 $(GREEN)make test-coverage$(RESET)  - Run tests with coverage report"
	@echo "  🖌️  $(GREEN)make fmt$(RESET)            - Format all code (Go + Markdown)"
	@echo "  🖌️  $(GREEN)make format$(RESET)         - Format all code (alias for fmt)"
	@echo "     $(GREEN)make fmt-go$(RESET)         - Format Go code only"
	@echo "     $(GREEN)make fmt-md$(RESET)         - Format Markdown files only"
	@echo "  🔍 $(GREEN)make fmt-check$(RESET)      - Check if code is formatted"
	@echo "  🔬 $(GREEN)make lint$(RESET)           - Run golangci-lint"
	@echo "  🔬 $(GREEN)make vet$(RESET)            - Run go vet"
	@echo "  🧹 $(GREEN)make clean$(RESET)          - Clean build artifacts"
	@echo ""
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "📦 $(CYAN)DEPENDENCIES$(RESET)"
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "  📥 $(GREEN)make mod-download$(RESET)   - Download dependencies"
	@echo "  🧹 $(GREEN)make mod-tidy$(RESET)       - Tidy go.mod and go.sum"
	@echo "  ✅ $(GREEN)make mod-verify$(RESET)     - Verify dependencies"
	@echo "  🔍 $(GREEN)make deps-check$(RESET)     - Check for outdated dependencies"
	@echo "  🔄 $(GREEN)make deps-update$(RESET)    - Update all dependencies"
	@echo ""
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "🚀 $(CYAN)PUBLISHING$(RESET)"
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "  🚀 $(GREEN)make publish$(RESET)        - Publish new version (runs publish.sh)"
	@echo "  🔢 $(GREEN)make bump-version$(RESET)   - Bump version and create git tag"
	@echo "  ✅ $(GREEN)make check-ready$(RESET)    - Check if ready to publish"
	@echo ""
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "🧪 $(CYAN)EXAMPLES$(RESET)"
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "  📚 $(GREEN)make test-examples$(RESET)  - Run Go examples from ../ekodb-client"
	@echo "  📚 $(GREEN)make examples$(RESET)       - Alias for test-examples"
	@echo ""
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "⚙️  $(CYAN)SETUP$(RESET)"
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "  🛠️  $(GREEN)make setup$(RESET)          - Initial setup (download deps, install tools)"
	@echo "  📦 $(GREEN)make install$(RESET)        - Install the package locally"
	@echo ""
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "💡 $(CYAN)QUICK START$(RESET)"
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "  1. $(GREEN)make setup$(RESET)     - Set up development environment"
	@echo "  2. $(GREEN)make test$(RESET)      - Run tests to verify everything works"
	@echo "  3. $(GREEN)make fmt$(RESET)       - Format code before committing"
	@echo "  4. $(GREEN)make publish$(RESET)   - Publish new version"

# Auto-install pre-commit hook if missing
ensure-hooks:
	@if [ ! -f .git/hooks/pre-commit ]; then \
		echo "🔗 $(CYAN)Installing pre-commit hook...$(RESET)"; \
		ln -s ../../scripts/pre-commit .git/hooks/pre-commit; \
		chmod +x .git/hooks/pre-commit; \
		echo "✅ $(GREEN)Pre-commit hook installed$(RESET)"; \
	fi

# Build the library
build: ensure-hooks
	@echo "🛠️  $(CYAN)Building Go client library...$(RESET)"
	@$(GO) build -v ./...
	@echo "✅ $(GREEN)Build complete!$(RESET)"

# Run tests
test: ensure-hooks
	@echo "🧪 $(CYAN)Running tests...$(RESET)"
	@TEST_OUTPUT=$$($(GO) test ./... -race -v 2>&1); \
	echo "$$TEST_OUTPUT"; \
	TEST_COUNT=$$(echo "$$TEST_OUTPUT" | grep -c "^--- PASS:" || echo "0"); \
	echo ""; \
	echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"; \
	echo "✅ $(GREEN)All tests complete!$(RESET)"; \
	echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"; \
	printf "  🔷 Go:         %3d tests\n" "$$TEST_COUNT"; \
	echo ""

# Run tests with verbose output
test-verbose:
	@echo "🧪 $(CYAN)Running tests (verbose)...$(RESET)"
	@$(GO) test -v ./... -race
	@echo "✅ $(GREEN)Tests complete!$(RESET)"

# Run tests with coverage
test-coverage:
	@echo "📊 $(CYAN)Running tests with coverage...$(RESET)"
	@$(GO) test ./... -race -coverprofile=coverage.out -covermode=atomic
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "✅ $(GREEN)Coverage report generated!$(RESET)"
	@echo "📄 $(YELLOW)HTML report: coverage.html$(RESET)"
	@echo "📊 $(YELLOW)Coverage summary:$(RESET)"
	@$(GO) tool cover -func=coverage.out | tail -1

# Format all code (Go + Markdown)
fmt: fmt-go fmt-md
	@echo "✅ $(GREEN)All formatting complete!$(RESET)"

# Format Go code only
fmt-go:
	@echo "🖌️  $(CYAN)Formatting Go code...$(RESET)"
	@$(GO) fmt ./...
	@echo "✅ $(GREEN)Go formatting complete!$(RESET)"

# Format Markdown files
fmt-md:
	@echo "📝 $(CYAN)Formatting Markdown files...$(RESET)"
	@if command -v prettier > /dev/null; then \
		prettier --write "**/*.md" --prose-wrap always --print-width 80; \
		echo "✅ $(GREEN)Markdown formatting complete with prettier!$(RESET)"; \
	elif command -v mdformat > /dev/null; then \
		find . -name "*.md" -not -path "./node_modules/*" -not -path "./vendor/*" -exec mdformat {} \; 2>/dev/null || true; \
		echo "✅ $(GREEN)Markdown formatting complete with mdformat!$(RESET)"; \
	else \
		echo "$(YELLOW)⚠️  No Markdown formatter found$(RESET)"; \
		echo "$(YELLOW)💡 Install prettier: npm install -g prettier$(RESET)"; \
		echo "$(YELLOW)💡 Or install mdformat: pip install mdformat$(RESET)"; \
		echo "$(YELLOW)Skipping Markdown formatting...$(RESET)"; \
	fi

# Check if code is formatted
fmt-check:
	@echo "🔍 $(CYAN)Checking code formatting...$(RESET)"
	@UNFORMATTED=$$(gofmt -l .); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "$(RED)❌ The following files are not formatted:$(RESET)"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi
	@echo "✅ $(GREEN)All files are properly formatted!$(RESET)"

# Run golangci-lint
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo "$(shell go env GOPATH)/bin/golangci-lint")

lint:
	@echo "🔬 $(CYAN)Running golangci-lint...$(RESET)"
	@if [ -x "$(GOLANGCI_LINT)" ]; then \
		$(GOLANGCI_LINT) run ./...; \
		echo "✅ $(GREEN)Linting complete!$(RESET)"; \
	else \
		echo "$(YELLOW)⚠️  golangci-lint not found$(RESET)"; \
		echo "$(YELLOW)Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(RESET)"; \
		echo "$(YELLOW)Or visit: https://golangci-lint.run/usage/install/$(RESET)"; \
	fi

# Run go vet
vet:
	@echo "🔬 $(CYAN)Running go vet...$(RESET)"
	@$(GO) vet ./...
	@echo "✅ $(GREEN)Vet complete!$(RESET)"

# Download dependencies
mod-download:
	@echo "📥 $(CYAN)Downloading dependencies...$(RESET)"
	@$(GO) mod download
	@echo "✅ $(GREEN)Dependencies downloaded!$(RESET)"

# Tidy go.mod and go.sum
mod-tidy:
	@echo "🧹 $(CYAN)Tidying go.mod and go.sum...$(RESET)"
	@$(GO) mod tidy
	@echo "✅ $(GREEN)Tidy complete!$(RESET)"

# Verify dependencies
mod-verify:
	@echo "✅ $(CYAN)Verifying dependencies...$(RESET)"
	@$(GO) mod verify
	@echo "✅ $(GREEN)Dependencies verified!$(RESET)"

# Check for outdated dependencies
deps-check:
	@echo "🔍 $(CYAN)Checking for outdated dependencies...$(RESET)"
	@$(GO) list -u -m all
	@echo "✅ $(GREEN)Dependency check complete!$(RESET)"

# Update all dependencies
deps-update:
	@echo "🔄 $(CYAN)Updating dependencies...$(RESET)"
	@$(GO) get -u ./...
	@$(GO) mod tidy
	@echo "✅ $(GREEN)Dependencies updated!$(RESET)"

# Clean build artifacts
clean:
	@echo "🧹 $(CYAN)Cleaning build artifacts...$(RESET)"
	@$(GO) clean
	@rm -f coverage.out coverage.html
	@echo "✅ $(GREEN)Clean complete!$(RESET)"

# Install the package locally
install:
	@echo "📦 $(CYAN)Installing package locally...$(RESET)"
	@$(GO) install
	@echo "✅ $(GREEN)Package installed!$(RESET)"

# Setup development environment
setup: mod-download
	@echo "🛠️  $(CYAN)Setting up development environment...$(RESET)"
	@echo "📦 $(CYAN)Installing development tools...$(RESET)"
	@if ! command -v golangci-lint > /dev/null; then \
		echo "  📥 Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	else \
		echo "  ✅ golangci-lint already installed"; \
	fi
	@echo "✅ $(GREEN)Setup complete!$(RESET)"
	@echo ""
	@echo "$(YELLOW)💡 Next steps:$(RESET)"
	@echo "  1. Run tests: make test"
	@echo "  2. Format code: make fmt"
	@echo "  3. Run linter: make lint"

# Check if ready to publish
check-ready: fmt-check vet test
	@echo "✅ $(CYAN)Checking if ready to publish...$(RESET)"
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "$(RED)❌ You have uncommitted changes$(RESET)"; \
		git status --short; \
		exit 1; \
	fi
	@echo "✅ $(GREEN)Ready to publish!$(RESET)"

# Publish new version (uses publish.sh script)
publish: check-ready
	@echo "🚀 $(CYAN)Publishing new version...$(RESET)"
	@chmod +x publish.sh
	@./publish.sh

# Bump version and create git tag
bump-version:
	@echo "🔢 $(CYAN)Bumping version...$(RESET)"
	@echo ""
	@LATEST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo "none"); \
	echo ""; \
	read -p "Enter new version (e.g. the latest tag: '$$LATEST_TAG'): " NEW_VERSION; \
	if [ -z "$$NEW_VERSION" ]; then \
		echo "$(RED)❌ No version provided$(RESET)"; \
		exit 1; \
	fi; \
	if [[ ! $$NEW_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$$ ]]; then \
		echo "$(RED)❌ Version must be in format vX.Y.Z (e.g., v0.2.0)$(RESET)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "$(YELLOW)📦 New version: $$NEW_VERSION$(RESET)"; \
	echo ""; \
	read -p "Continue? (y/N): " -n 1 -r; \
	echo; \
	if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "$(RED)❌ Cancelled$(RESET)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "$(CYAN)Running tests before tagging...$(RESET)"; \
	$(GO) test ./... -race || { echo "$(RED)❌ Tests failed$(RESET)"; exit 1; }; \
	echo ""; \
	echo "$(CYAN)Creating tag $$NEW_VERSION...$(RESET)"; \
	git tag -a "$$NEW_VERSION" -m "Release $$NEW_VERSION"; \
	echo "✅ $(GREEN)Tag created: $$NEW_VERSION$(RESET)"; \
	echo ""; \
	echo "$(YELLOW)💡 Next steps:$(RESET)"; \
	echo "  1. Push tag: git push origin $$NEW_VERSION"; \
	echo "  2. Push commits: git push origin main"; \
	echo "  3. Wait for pkg.go.dev to index (a few minutes)"; \
	echo "  4. Users can install: go get $(MODULE)@$$NEW_VERSION"

# Run example programs from ekodb-client repository
test-examples:
	@echo "📚 $(CYAN)Running Go examples from ekodb-client repository...$(RESET)"
	@if [ -d "../ekodb-client" ]; then \
		echo "📦 $(CYAN)Found ekodb-client, preparing Go examples...$(RESET)"; \
		if [ -d "../ekodb-client/examples/go" ]; then \
			echo "🧹 $(CYAN)Running go mod tidy in examples/go...$(RESET)"; \
			(cd ../ekodb-client/examples/go && go mod tidy); \
		fi; \
		echo "🧪 $(CYAN)Running make test-examples-go...$(RESET)"; \
		(cd ../ekodb-client && make test-examples-go); \
	else \
		echo "$(RED)❌ ekodb-client repository not found at ../ekodb-client$(RESET)"; \
		echo "$(YELLOW)💡 Clone ekodb-client next to this repository:$(RESET)"; \
		echo "$(YELLOW)   cd .. && git clone <ekodb-client-repo-url> ekodb-client$(RESET)"; \
		echo "$(YELLOW)   Then run: make test-examples$(RESET)"; \
		exit 1; \
	fi

# Alias for test-examples
examples: test-examples

# Run all checks before committing
pre-commit: fmt vet lint test
	@echo "✅ $(GREEN)All pre-commit checks passed!$(RESET)"

# Alias for format
format: fmt

# Show current version
version:
	@echo "📌 $(CYAN)Current version:$(RESET)"
	@git describe --tags --abbrev=0 2>/dev/null || echo "$(YELLOW)No tags found$(RESET)"
	@echo ""
	@echo "📦 $(CYAN)Module:$(RESET) $(MODULE)"
	@echo "🔷 $(CYAN)Go version:$(RESET) $$(go version | awk '{print $$3}')"

# Show module information
info:
	@echo "📦 $(CYAN)Module Information$(RESET)"
	@echo "$(CYAN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)"
	@echo "Module: $(MODULE)"
	@echo "Go Version: $$(go version | awk '{print $$3}')"
	@echo "Latest Tag: $$(git describe --tags --abbrev=0 2>/dev/null || echo 'none')"
	@echo ""
	@echo "📊 $(CYAN)Dependencies:$(RESET)"
	@$(GO) list -m all
	@echo ""
	@echo "📁 $(CYAN)Files:$(RESET)"
	@find . -name "*.go" -not -path "./vendor/*" | wc -l | xargs echo "  Go files:"
	@echo ""
	@echo "📏 $(CYAN)Lines of Code:$(RESET)"
	@find . -name "*.go" -not -path "./vendor/*" -exec wc -l {} + | tail -1 | awk '{print "  " $$1 " lines"}'
