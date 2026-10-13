#!/bin/bash

# Pre-commit Setup Script for Unraid Management Agent
# This script automates the installation of pre-commit hooks and required tools

set -e

echo "🚀 Setting up pre-commit hooks for Unraid Management Agent..."
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running in a git repository
if [ ! -d ".git" ]; then
    echo -e "${RED}❌ Error: This script must be run from the root of the git repository${NC}"
    exit 1
fi

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check Python
echo "📦 Checking dependencies..."
if ! command_exists python3; then
    echo -e "${RED}❌ Python 3 is not installed${NC}"
    echo "Install with: sudo apt-get install python3 python3-pip (Debian/Ubuntu)"
    exit 1
fi
echo -e "${GREEN}✓ Python 3 found${NC}"

# Check pip
if ! command_exists pip3 && ! command_exists pip; then
    echo -e "${YELLOW}⚠️  pip not found, installing...${NC}"
    sudo apt-get update && sudo apt-get install -y python3-pip || {
        echo -e "${RED}❌ Failed to install pip${NC}"
        exit 1
    }
fi
echo -e "${GREEN}✓ pip found${NC}"

# Install pre-commit
echo ""
echo "📦 Installing pre-commit..."
if ! command_exists pre-commit; then
    pip3 install pre-commit --user || pip install pre-commit --user || {
        echo -e "${RED}❌ Failed to install pre-commit${NC}"
        exit 1
    }

    # Add to PATH if needed
    if ! command_exists pre-commit; then
        export PATH="$HOME/.local/bin:$PATH"
        echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
    fi
fi
echo -e "${GREEN}✓ pre-commit installed${NC}"

# Check Go
echo ""
echo "📦 Checking Go installation..."
if ! command_exists go; then
    echo -e "${RED}❌ Go is not installed${NC}"
    echo "Install from: https://go.dev/dl/"
    exit 1
fi
GO_VERSION=$(go version | awk '{print $3}')
echo -e "${GREEN}✓ Go ${GO_VERSION} found${NC}"

# Install Go tools
echo ""
echo "📦 Installing Go development tools..."

tools=(
    "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
    "golang.org/x/tools/cmd/goimports@latest"
    "golang.org/x/vuln/cmd/govulncheck@latest"
)

for tool in "${tools[@]}"; do
    tool_name=$(basename "${tool%%@*}")
    echo "  → Installing ${tool_name}..."
    go install "$tool" || echo -e "${YELLOW}⚠️  Failed to install ${tool_name}, continuing...${NC}"
done

echo -e "${GREEN}✓ Go tools installed${NC}"

# Install pre-commit hooks
echo ""
echo "🔗 Installing pre-commit hooks..."
pre-commit install || {
    echo -e "${RED}❌ Failed to install pre-commit hooks${NC}"
    exit 1
}
pre-commit install --hook-type commit-msg || {
    echo -e "${YELLOW}⚠️  Failed to install commit-msg hook, continuing...${NC}"
}
echo -e "${GREEN}✓ Pre-commit hooks installed${NC}"

# Install hook dependencies
echo ""
echo "📦 Installing pre-commit hook dependencies..."
pre-commit install-hooks || {
    echo -e "${YELLOW}⚠️  Some hooks may not have installed correctly${NC}"
}

# Create secrets baseline if it doesn't exist
if [ ! -f ".secrets.baseline" ]; then
    echo ""
    echo "🔐 Creating secrets baseline..."
    if command_exists detect-secrets; then
        detect-secrets scan --baseline .secrets.baseline
    else
        echo '{"version": "1.5.0", "plugins_used": [], "filters_used": [], "results": {}}' > .secrets.baseline
    fi
    echo -e "${GREEN}✓ Secrets baseline created${NC}"
fi

# Run a quick test
echo ""
echo "🧪 Running pre-commit checks on all files (this may take a minute)..."
if pre-commit run --all-files; then
    echo -e "${GREEN}✓ All pre-commit checks passed!${NC}"
else
    echo -e "${YELLOW}⚠️  Some checks failed. This is normal for first-time setup.${NC}"
    echo -e "${YELLOW}   Run 'make pre-commit-run' to see details and fix issues.${NC}"
fi

# Summary
echo ""
echo "═══════════════════════════════════════════════════════════"
echo -e "${GREEN}✅ Pre-commit setup complete!${NC}"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo "📚 What's next:"
echo "  • Pre-commit will now run automatically on git commit"
echo "  • Run 'make pre-commit-run' to check all files manually"
echo "  • Run 'make lint' for just linting checks"
echo "  • Run 'make security-check' for security scans"
echo "  • Read docs/PRE_COMMIT_HOOKS.md for detailed documentation"
echo ""
echo "🚫 Zero Tolerance Policy:"
echo "  • No linting warnings or errors allowed"
echo "  • No security vulnerabilities (medium+ severity)"
echo "  • All tests must pass"
echo "  • Code must be properly formatted"
echo ""
echo "💡 Tip: To skip hooks temporarily (not recommended):"
echo "   git commit --no-verify -m 'message'"
echo ""
echo -e "${GREEN}Happy coding! 🎉${NC}"
