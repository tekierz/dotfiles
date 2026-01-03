.PHONY: build build-all install test clean run dev

# Binary names
DOTFILES_BIN = bin/dotfiles
INSTALLER_BIN = bin/dotfiles-installer
SETUP_SCRIPT = bin/dotfiles-setup

# Go build flags
LDFLAGS = -s -w
VERSION = 2.0.0-dev

# Build the main dotfiles CLI (new)
build:
	@echo "Building dotfiles CLI..."
	go build -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" -o $(DOTFILES_BIN) ./cmd/dotfiles

# Build both the new CLI and legacy installer
build-all: build
	@echo "Building legacy installer..."
	go build -ldflags "$(LDFLAGS)" -o $(INSTALLER_BIN) ./cmd/installer

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
	install -m 755 $(SETUP_SCRIPT) /usr/local/bin/dotfiles-setup

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(INSTALLER_BIN)
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
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Build for all platforms (requires goreleaser)
release:
	@echo "Building releases..."
	goreleaser release --snapshot --clean

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the installer binary"
	@echo "  dev          - Build with debug info"
	@echo "  run          - Build and run the installer"
	@echo "  run-quick    - Run without intro animation"
	@echo "  install      - Install to /usr/local/bin"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  clean        - Remove build artifacts"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  tidy         - Tidy go modules"
	@echo "  check-durdraw- Check if durdraw is installed"
	@echo "  dev-deps     - Install development dependencies"
	@echo "  release      - Build release binaries"
