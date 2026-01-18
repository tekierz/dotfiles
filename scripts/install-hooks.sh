#!/bin/bash
# Install git pre-commit hooks for the dotfiles project
# Run this once after cloning: ./scripts/install-hooks.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOKS_DIR="$REPO_ROOT/.git/hooks"

echo "Installing git hooks..."

# Create pre-commit hook
cat > "$HOOKS_DIR/pre-commit" << 'HOOK'
#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Running pre-commit checks...${NC}"

# Check if we're in a Go project
if [ ! -f "go.mod" ]; then
    echo -e "${RED}Not in a Go project root${NC}"
    exit 1
fi

# Format check
echo -n "Checking gofmt... "
UNFORMATTED=$(gofmt -l . 2>&1 | grep -v vendor || true)
if [ -n "$UNFORMATTED" ]; then
    echo -e "${RED}FAILED${NC}"
    echo "The following files need formatting:"
    echo "$UNFORMATTED"
    echo ""
    echo "Run: gofmt -w ."
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# Vet
echo -n "Running go vet... "
if ! go vet ./... 2>&1; then
    echo -e "${RED}FAILED${NC}"
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# Build
echo -n "Building... "
if ! go build ./... 2>&1; then
    echo -e "${RED}FAILED${NC}"
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# Tests
echo -n "Running tests... "
if ! go test ./... 2>&1 > /dev/null; then
    echo -e "${RED}FAILED${NC}"
    echo "Run 'go test ./...' to see failures"
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# Optional: Run golangci-lint if available
if command -v golangci-lint &> /dev/null; then
    echo -n "Running golangci-lint... "
    if ! golangci-lint run --timeout=5m 2>&1 > /dev/null; then
        echo -e "${YELLOW}WARNINGS (non-blocking)${NC}"
        echo "Run 'golangci-lint run' to see issues"
    else
        echo -e "${GREEN}OK${NC}"
    fi
fi

# Optional: Run govulncheck if available
if command -v govulncheck &> /dev/null; then
    echo -n "Running govulncheck... "
    if ! govulncheck ./... 2>&1 > /dev/null; then
        echo -e "${YELLOW}WARNINGS (non-blocking)${NC}"
        echo "Run 'govulncheck ./...' to see vulnerabilities"
    else
        echo -e "${GREEN}OK${NC}"
    fi
fi

echo ""
echo -e "${GREEN}All pre-commit checks passed!${NC}"
HOOK

chmod +x "$HOOKS_DIR/pre-commit"

echo "Pre-commit hook installed successfully!"
echo ""
echo "Optional: Install additional tools for enhanced checks:"
echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
echo "  go install golang.org/x/vuln/cmd/govulncheck@latest"
echo "  go install honnef.co/go/tools/cmd/staticcheck@latest"
