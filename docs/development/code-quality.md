# Code Quality & Pre-Commit Hooks

Complete guide to maintaining code quality in the Unraid Management Agent project.

## Overview

This project uses automated code quality checks via **pre-commit hooks** to ensure consistency, security, and best practices before code is committed.

## Quick Setup

### Automated Setup (Recommended)

```bash
# Clone the repository
git clone https://github.com/ruaan-deysel/unraid-management-agent.git
cd unraid-management-agent

# Run the setup script
./scripts/setup-pre-commit.sh
```

This script:

1. Installs Python and pip (if needed)
2. Installs pre-commit via pip
3. Configures pre-commit hooks
4. Verifies the installation

### Manual Setup

```bash
# Install pre-commit
pip install pre-commit

# Install the git hooks
make pre-commit-install

# Or directly:
pre-commit install
```

## Pre-Commit Hooks

The project uses the following pre-commit hooks (configured in `.pre-commit-config.yaml`):

### 1. Go Formatting (`gofmt`)

Automatically formats Go code to match Go standards.

**What it checks**:

- Proper indentation (tabs)
- Consistent spacing
- Import organization

**How to run manually**:

```bash
gofmt -w .
```

### 2. Go Imports (`goimports`)

Manages Go import statements automatically.

**What it checks**:

- Unused imports
- Import grouping (stdlib, external, internal)
- Alphabetical ordering

**How to run manually**:

```bash
goimports -w .
```

### 3. Golangci-lint

Comprehensive linter running 50+ checks.

**What it checks**:

- Code complexity
- Dead code
- Inefficient assignments
- Error handling
- Security issues
- Style violations
- And much more...

**Configuration**: `.golangci.yml`

**How to run manually**:

```bash
make lint
# Or directly:
golangci-lint run
```

### 4. Gosec

Security-focused static analysis tool.

**What it checks**:

- SQL injection vulnerabilities
- Command injection risks
- Path traversal attacks
- Hardcoded credentials
- Weak cryptography
- File permission issues

**How to run manually**:

```bash
make security-check
# Or directly:
gosec ./...
```

### 5. Go Vet

Official Go static analysis tool.

**What it checks**:

- Suspicious constructs
- Printf format strings
- Unreachable code
- Nil pointer dereferences
- Invalid interface implementations

**How to run manually**:

```bash
go vet ./...
```

### 6. Trailing Whitespace

Removes trailing whitespace from all text files.

### 7. End of File Fixer

Ensures files end with a newline.

### 8. YAML/JSON Syntax

Validates YAML and JSON file syntax.

## Using Pre-Commit

### Automatic Checks

Once installed, pre-commit runs automatically when you commit:

```bash
git add .
git commit -m "Your commit message"
# Pre-commit hooks run here
```

If any check fails, the commit is aborted with error messages.

### Manual Checks

Run checks without committing:

```bash
# Run all hooks
make pre-commit-run

# Run on all files
pre-commit run --all-files

# Run specific hook
pre-commit run golangci-lint --all-files
```

### Skipping Hooks (Not Recommended)

```bash
# Skip all hooks for a commit (use sparingly)
git commit -m "Message" --no-verify
```

**Warning**: Only skip hooks if absolutely necessary (e.g., work-in-progress commits for backup).

## Makefile Commands

### Code Quality Commands

```bash
make lint              # Run golangci-lint
make security-check    # Run gosec security scan
make pre-commit-run    # Run all pre-commit checks
make pre-commit-install # Install pre-commit hooks
```

### Testing Commands

```bash
make test              # Run all tests with race detection
make test-coverage     # Generate coverage report (coverage.html)
```

### Build Commands

```bash
make deps              # Install/update dependencies
make local             # Build for current architecture
make release           # Build for Linux/amd64 (Unraid)
make clean             # Clean build artifacts
```

## Code Style Guidelines

### Go Code Style

Follow official Go conventions:

```go
// Good: Clear naming, proper error handling
func (c *SystemCollector) Collect() error {
    info, err := c.collectSystemInfo()
    if err != nil {
        return fmt.Errorf("failed to collect system info: %w", err)
    }

    c.ctx.Hub.Pub(info, "system_update")
    return nil
}

// Bad: Unclear naming, poor error handling
func (c *SystemCollector) C() error {
    i, e := c.csi()
    if e != nil {
        return e
    }
    c.ctx.Hub.Pub(i, "system_update")
    return nil
}
```

### Error Handling

Always wrap errors with context:

```go
// Good: Provides context
if err := someFunction(); err != nil {
    return fmt.Errorf("failed to execute someFunction: %w", err)
}

// Bad: Loses context
if err := someFunction(); err != nil {
    return err
}
```

### Security Best Practices

From `.github/copilot-instructions.md`:

1. **Always validate user input** using `lib/validation.go`:

   ```go
   if err := lib.ValidateContainerID(containerID); err != nil {
       return err
   }
   ```

2. **Never use `exec.Command` directly** — use `lib.ExecCommand()` or `lib.ExecCommandOutput()`

3. **Path traversal protection** — validate all file paths:

   ```go
   if err := lib.ValidateShareName(shareName); err != nil {
       return err
   }
   ```

## Golangci-lint Configuration

The project uses a strict linting configuration in `.golangci.yml`:

### Enabled Linters

- **errcheck**: Check for unchecked errors
- **gosimple**: Suggest code simplifications
- **govet**: Official Go static analysis
- **ineffassign**: Detect ineffectual assignments
- **staticcheck**: Advanced static analysis
- **typecheck**: Type checking
- **unused**: Detect unused code
- **gosec**: Security vulnerabilities
- **misspell**: Common spelling mistakes
- **gofmt**: Code formatting
- **goimports**: Import management
- **revive**: Extended linting rules
- **goconst**: Find repeated strings that could be constants
- **dupl**: Code duplication detection

### Configuration Example

```yaml
linters-settings:
  errcheck:
    check-blank: true
    check-type-assertions: true

  govet:
    enable-all: true

  gosec:
    severity: medium
    confidence: medium
```

## Continuous Integration

Pre-commit checks also run in GitHub Actions CI:

```yaml
name: Pre-commit
on: [push, pull_request]

jobs:
  pre-commit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - uses: pre-commit/action@v3.0.0
```

## Troubleshooting

### Pre-commit Installation Failed

```bash
# Update pip
pip install --upgrade pip

# Reinstall pre-commit
pip install --force-reinstall pre-commit

# Verify installation
pre-commit --version
```

### Hook Failed with Error

```bash
# View detailed error
pre-commit run --all-files --verbose

# Update hooks
pre-commit autoupdate

# Clean and reinstall
pre-commit uninstall
pre-commit install
```

### Golangci-lint Too Slow

```bash
# Run only fast linters
golangci-lint run --fast

# Run on changed files only
golangci-lint run --new-from-rev=main
```

### Security Check False Positives

Edit `.golangci.yml` to exclude specific issues:

```yaml
linters-settings:
  gosec:
    excludes:
      - G304 # File path provided as taint input (if validated elsewhere)
```

**Important**: Only exclude after verifying it's a false positive.

## Testing Standards

### Test Coverage

Maintain test coverage above 70%:

```bash
# Generate coverage report
make test-coverage

# View coverage.html in browser
open coverage.html
```

### Test Patterns

Use table-driven tests:

```go
func TestValidateContainerID(t *testing.T) {
    tests := []struct {
        name    string
        id      string
        wantErr bool
    }{
        {"valid short ID", "bbb57ffa3c50", false},
        {"empty ID", "", true},
        {"SQL injection", "'; DROP TABLE--", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateContainerID(tt.id)
            if (err != nil) != tt.wantErr {
                t.Errorf("got error=%v, wantErr=%v", err, tt.wantErr)
            }
        })
    }
}
```

## Best Practices Summary

1. **Run pre-commit before pushing**: `make pre-commit-run`
2. **Write tests for new code**: Aim for 70%+ coverage
3. **Handle errors properly**: Always wrap with context
4. **Validate all user input**: Use validation functions
5. **Document complex code**: Clear comments explain "why"
6. **Keep functions small**: Max 50 lines recommended
7. **Follow Go conventions**: Use `gofmt`, `goimports`

## Next Steps

- [Contributing Guide](contributing.md) - Contribution workflow
- [Testing Guide](testing.md) - Comprehensive testing guide
- [Architecture](architecture.md) - System architecture overview

---

**Last Updated**: January 2026  
**Pre-commit Version**: 3.5.0+  
**Golangci-lint Version**: 1.55.0+
