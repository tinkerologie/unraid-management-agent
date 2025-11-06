---
type: "always_apply"
description: "Example description"
---

# Unraid Management Agent - Project Rules

## Code Quality Standards

### Go Best Practices (Go 1.25+)

1. **Error Handling**
   - Always check and handle errors explicitly
   - Use `errors.Is()` and `errors.As()` for error comparison
   - Wrap errors with context using `fmt.Errorf()` with `%w` verb
   - Never ignore errors with blank identifier `_` without justification
   - Return errors as the last return value

2. **Idiomatic Go Code**
   - Follow standard Go naming conventions (MixedCaps for exported, mixedCaps for unexported)
   - Use `gofmt` formatting (enforced by linters)
   - Prefer composition over inheritance
   - Keep functions small and focused (single responsibility)
   - Use meaningful variable names (avoid single-letter names except for short-lived loop variables)
   - Document all exported functions, types, and constants with proper godoc comments

3. **Concurrency**
   - Use goroutines and channels appropriately
   - Always handle goroutine lifecycle (ensure they can be stopped gracefully)
   - Avoid goroutine leaks by ensuring all goroutines terminate
   - Use `sync.WaitGroup` for coordinating multiple goroutines
   - Protect shared state with `sync.Mutex` or `sync.RWMutex`
   - Prefer channels for communication, mutexes for state protection

4. **Context Usage**
   - Pass `context.Context` as the first parameter to functions that need it
   - Never store contexts in structs (pass them explicitly)
   - Use `context.WithTimeout()` or `context.WithCancel()` for cancellation
   - Respect context cancellation in long-running operations
   - Propagate context through the call chain

5. **Standard Library**
   - Prefer standard library over third-party dependencies when possible
   - Use `net/http` for HTTP servers and clients
   - Use `encoding/json` for JSON marshaling/unmarshaling
   - Use `time.Duration` for time intervals
   - Use `log/slog` or structured logging libraries for logging

### Unraid Plugin Development Standards

1. **Plugin Lifecycle**
   - Implement proper initialization in the `boot` command
   - Support graceful shutdown with signal handling (SIGTERM, SIGINT)
   - Clean up resources (close connections, stop goroutines) on shutdown
   - Handle plugin start, stop, and restart operations correctly

2. **File System Integration**
   - Use standard Unraid paths:
     - Plugin directory: `/usr/local/emhttp/plugins/unraid-management-agent/`
     - Log files: `/var/log/unraid-management-agent.log`
     - Configuration: Follow Unraid conventions for config storage
   - Respect Unraid file system permissions
   - Never write to read-only file systems
   - Use appropriate file permissions (644 for files, 755 for executables)

3. **Plugin Structure**
   - Follow the existing structure:
     - `meta/plugin/` - Plugin metadata and Unraid-specific files
     - `meta/scripts/` - Installation and lifecycle scripts
     - `meta/template/` - Plugin template files
   - Include proper plugin metadata (name, version, author, description)
   - Provide installation and removal scripts

4. **System Integration**
   - Use Unraid's existing commands and utilities where available
   - Parse Unraid configuration files correctly (`/var/local/emhttp/` paths)
   - Respect Unraid's array state and operations
   - Don't interfere with Unraid's core functionality

## Linting and Validation

### Required Linters

All code MUST pass `golangci-lint` with the following linters enabled:

**Mandatory Linters:**
- `errcheck` - Check for unchecked errors
- `gosimple` - Suggest code simplifications
- `govet` - Report suspicious constructs
- `ineffassign` - Detect ineffectual assignments
- `staticcheck` - Advanced static analysis
- `unused` - Check for unused constants, variables, functions, and types
- `gofmt` - Verify code is formatted with gofmt
- `goimports` - Verify imports are formatted correctly
- `misspell` - Find commonly misspelled English words
- `revive` - Fast, configurable, extensible, flexible, and beautiful linter for Go

**Recommended Additional Linters:**
- `gocritic` - Comprehensive Go source code linter
- `gocyclo` - Detect cyclomatic complexity (threshold: 15)
- `dupl` - Detect code duplication
- `gosec` - Inspect source code for security problems
- `unconvert` - Remove unnecessary type conversions

### Linting Requirements

1. **Zero Tolerance Policy**
   - All code MUST pass linting with **zero errors**
   - All code MUST pass linting with **zero warnings**
   - No linter warnings should be suppressed without documented justification
   - Run `golangci-lint run` before every commit

2. **Pre-Commit Validation**
   - Lint all changed files before committing
   - Run tests before committing (`make test`)
   - Ensure build succeeds (`make local` or `make release`)

3. **CI/CD Integration**
   - Linting should be part of the CI/CD pipeline
   - Failed linting should block merges
   - Coverage reports should be generated for all PRs

## File Creation Restrictions

### Prohibited File Types

**NEVER create the following types of files:**

1. **Documentation Files** (beyond existing structure)
   - ❌ Validation documents (e.g., `VALIDATION.md`, `VALIDATION_REPORT.md`)
   - ❌ Summary documents (e.g., `SUMMARY.md`, `ANALYSIS.md`)
   - ❌ Analysis files (e.g., `CODE_ANALYSIS.md`, `REVIEW.md`)
   - ❌ Redundant README files in subdirectories
   - ❌ Architecture decision records (ADRs) unless explicitly requested
   - ❌ Design documents unless explicitly requested
   - ✅ Only update existing: `README.md`, `CHANGELOG.md`, files in `docs/`

2. **Redundant Scripts**
   - ❌ Helper scripts that duplicate existing functionality
   - ❌ Wrapper scripts around existing tools
   - ❌ One-off utility scripts that aren't reusable
   - ✅ Only add scripts that are essential for build, test, or deployment

3. **Speculative Code**
   - ❌ "Future-proofing" code that isn't needed now
   - ❌ Abstract interfaces with single implementations
   - ❌ Unused utility functions "just in case"
   - ❌ Over-engineered solutions to simple problems

4. **Test Artifacts**
   - ❌ Temporary test files that should be cleaned up
   - ❌ Mock data files that aren't used by tests
   - ❌ Debug output files

### Allowed File Creation

**ONLY create files that are:**

1. **Essential for Core Functionality**
   - Source code files (`.go`) that implement required features
   - Test files (`_test.go`) for new functionality
   - Configuration files required by the application

2. **Required for Build/Deploy**
   - Build scripts that are part of the release process
   - Deployment scripts for Unraid plugin installation
   - Makefile targets for new build steps

3. **Explicitly Requested**
   - Files specifically requested by the user
   - Documentation updates to existing files when requested

## General Principles

### Project Structure

1. **Maintain Existing Structure**
   - Follow the established directory layout:
     ```
     daemon/
     ├── cmd/              # CLI commands
     ├── common/           # Constants and shared utilities
     ├── domain/           # Core types and interfaces
     ├── dto/              # Data transfer objects
     ├── lib/              # Reusable libraries
     ├── logger/           # Logging infrastructure
     └── services/
         ├── api/          # HTTP server, handlers, WebSocket
         ├── collectors/   # Data collection subsystems
         └── controllers/  # Control operations
     ```
   - Don't create new top-level directories without justification
   - Keep related code together in appropriate packages

2. **Minimize Dependencies**
   - Prefer standard library over third-party packages
   - Evaluate necessity before adding new dependencies
   - Keep `go.mod` clean and up-to-date
   - Document why each third-party dependency is needed

3. **Keep It Simple**
   - Avoid over-engineering solutions
   - Write code that is easy to understand and maintain
   - Don't add features that aren't needed now
   - Refactor when complexity grows, don't anticipate it

### Code Organization

1. **Package Design**
   - Each package should have a clear, single purpose
   - Avoid circular dependencies between packages
   - Keep packages small and focused
   - Use internal packages for implementation details

2. **Testing**
   - Write tests for all new functionality
   - Maintain or improve test coverage
   - Use table-driven tests for multiple test cases
   - Mock external dependencies appropriately
   - Tests should be fast and deterministic

3. **Documentation**
   - Update existing documentation when behavior changes
   - Keep README.md current with new features
   - Update CHANGELOG.md for all user-facing changes
   - Document complex algorithms or business logic in code comments
   - Don't create new documentation files unless explicitly needed

### Development Workflow

1. **Before Making Changes**
   - Understand the existing code structure
   - Check for similar existing implementations
   - Plan changes to minimize impact
   - Consider backward compatibility

2. **During Development**
   - Write clean, readable code
   - Add tests as you go
   - Run linters frequently
   - Commit logical, atomic changes

3. **Before Committing**
   - Run `make test` - all tests must pass
   - Run `golangci-lint run` - zero errors, zero warnings
   - Run `make local` - build must succeed
   - Review your changes for unnecessary files
   - Update CHANGELOG.md if user-facing changes

4. **Code Review**
   - Keep PRs focused and small
   - Provide context in PR descriptions
   - Respond to review feedback promptly
   - Don't merge until all checks pass

## Enforcement

These rules are **mandatory** and will be enforced through:

1. **Automated Checks**
   - Linting in CI/CD pipeline
   - Test coverage requirements
   - Build verification

2. **Code Review**
   - Manual review of all changes
   - Verification of adherence to project structure
   - Check for prohibited file types

3. **Pre-Commit Hooks** (recommended)
   - Run linters before commit
   - Run tests before commit
   - Prevent commits with linting errors

## Summary

**Golden Rules:**
1. ✅ Write idiomatic Go 1.21+ code
2. ✅ Follow Unraid plugin conventions
3. ✅ Zero linting errors, zero warnings
4. ❌ Never create unnecessary documentation or analysis files
5. ❌ Never add speculative or "just in case" code
6. ✅ Keep the project structure clean and minimal
7. ✅ Only add files essential to core functionality
8. ✅ Test everything, lint everything, document changes

