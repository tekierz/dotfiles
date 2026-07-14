.PHONY: build install test clean run dev release release-check slice-check slice-check-test slice-check-contract-candidate slice-check-test-candidate slice-check-guardrail-candidate slice-check-ledger-candidate slice-check-candidate-digest slice-check-candidate

# Binary names
DOTFILES_BIN = bin/dotfiles

# Go build flags
# Tagged and local builds use the same version spelling as GoReleaser. Untagged
# builds report the nearest tag-derived value, commit hash, or "dev".
VERSION := $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
LDFLAGS = -s -w -X main.version=$(VERSION)

# Build the main dotfiles CLI (new)
build:
	@echo "Building dotfiles CLI..."
	go build -ldflags "$(LDFLAGS)" -o $(DOTFILES_BIN) ./cmd/dotfiles

# Build for development (with debug info)
dev:
	@echo "Building for development..."
	go build -o $(DOTFILES_BIN) ./cmd/dotfiles

# Run the dotfiles CLI
run: dev
	./$(DOTFILES_BIN)

# Run with skip-intro flag
run-quick: dev
	./$(DOTFILES_BIN) --skip-intro install

# Run specific subcommand
run-install: dev
	./$(DOTFILES_BIN) install

run-status: dev
	./$(DOTFILES_BIN) status

run-theme: dev
	./$(DOTFILES_BIN) theme list

# Install to system
install: build
	@echo "Installing..."
	install -m 755 $(DOTFILES_BIN) /usr/local/bin/dotfiles

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Fail closed when the active remediation slice drifts beyond its frozen contract.
slice-check:
	bash scripts/check-slice-scope.sh

slice-check-test:
	bash tests/check-slice-scope_test.sh

slice-check-contract-candidate:
	bash scripts/check-slice-scope.sh --contract-candidate

slice-check-test-candidate:
	bash scripts/check-slice-scope.sh --test-candidate

slice-check-guardrail-candidate:
	bash scripts/check-slice-scope.sh --guardrail-candidate

slice-check-ledger-candidate:
	bash scripts/check-slice-scope.sh --ledger-candidate

slice-check-candidate-digest:
	@bash scripts/check-slice-scope.sh --candidate-digest

slice-check-candidate:
	bash scripts/check-slice-scope.sh --candidate

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(DOTFILES_BIN)
	rm -f coverage.out coverage.html

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting..."
	golangci-lint run

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Check if durdraw is installed
check-durdraw:
	@which durdraw > /dev/null 2>&1 && echo "durdraw: installed" || echo "durdraw: not installed (pip install durdraw)"

# Install development dependencies
dev-deps:
	@echo "Installing development dependencies..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
	go install honnef.co/go/tools/cmd/staticcheck@v0.7.0

# Validate the pinned GoReleaser v2 configuration and its SBOM dependency.
release-check:
	@command -v goreleaser >/dev/null || { echo "goreleaser is required (release workflow pins v2.17.0)"; exit 1; }
	@command -v syft >/dev/null || { echo "syft is required (release workflow pins v1.44.0)"; exit 1; }
	goreleaser check

# Build an unpublished local snapshot for all supported release targets.
release: release-check
	@echo "Building releases..."
	goreleaser release --snapshot --clean

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the dotfiles CLI binary"
	@echo "  dev          - Build with debug info"
	@echo "  run          - Build and run the dotfiles CLI"
	@echo "  run-quick    - Run without intro animation"
	@echo "  install      - Install to /usr/local/bin"
	@echo "  test         - Run tests"
	@echo "  slice-check  - Verify the active remediation slice scope"
	@echo "  slice-check-test - Run the slice guardrail tests"
	@echo "  slice-check-contract-candidate - Verify an exact staged contract transition"
	@echo "  slice-check-test-candidate - Verify an exact staged red-test candidate"
	@echo "  slice-check-guardrail-candidate - Verify an exact staged control candidate"
	@echo "  slice-check-ledger-candidate - Verify an exact staged ledger closure"
	@echo "  slice-check-candidate-digest - Print the verified worktree candidate digest"
	@echo "  slice-check-candidate - Verify the exact staged slice"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  clean        - Remove build artifacts"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  tidy         - Tidy go modules"
	@echo "  check-durdraw- Check if durdraw is installed"
	@echo "  dev-deps     - Install development dependencies"
	@echo "  release-check- Validate the release configuration and tooling"
	@echo "  release      - Build an unpublished checksummed snapshot with SBOMs"
