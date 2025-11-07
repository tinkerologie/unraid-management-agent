# Contributing to Unraid Management Agent

First off, thank you for considering contributing to the Unraid Management Agent! It's people like you that make this plugin better for everyone in the Unraid community.

This is a community-developed third-party plugin that provides REST API and WebSocket monitoring for Unraid systems. We welcome contributions from developers of all experience levels.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [What We're Looking For](#what-were-looking-for)
- [How to Contribute](#how-to-contribute)
  - [Reporting Bugs](#reporting-bugs)
  - [Suggesting Features](#suggesting-features)
  - [Hardware Compatibility Issues](#hardware-compatibility-issues)
- [Development Setup](#development-setup)
- [Development Workflow](#development-workflow)
- [Pull Request Process](#pull-request-process)
- [Hardware Compatibility Contributions](#hardware-compatibility-contributions)
- [Testing Guidelines](#testing-guidelines)
- [Coding Standards](#coding-standards)
- [Commit Message Guidelines](#commit-message-guidelines)
- [Community and Support](#community-and-support)

## Code of Conduct

### Our Pledge

We are committed to providing a welcoming and inspiring community for all. Please be respectful and constructive in your interactions with other contributors.

### Our Standards

- Be respectful and inclusive
- Provide constructive feedback
- Focus on what is best for the community
- Show empathy towards other community members
- Accept constructive criticism gracefully

## What We're Looking For

We especially appreciate contributions in these areas:

- 🔧 **Hardware-Specific Fixes**: Support for different disk controllers, GPU models, UPS brands, network cards
- 📊 **Data Collection Improvements**: Better parsing of system commands for different hardware configurations
- 🧪 **Testing**: Testing on different Unraid versions and hardware configurations
- 📝 **Documentation**: Improving docs, adding examples, documenting edge cases
- 🐛 **Bug Fixes**: Fixing issues you encounter on your system
- ✨ **New Features**: Adding support for additional hardware, metrics, or control operations
- 🌐 **API Enhancements**: New endpoints, improved error handling, validation

## How to Contribute

### Reporting Bugs

Before creating a bug report, please check existing issues to avoid duplicates.

When filing a bug report, include:

- **Unraid Version**: Your Unraid OS version (e.g., 7.2)
- **Plugin Version**: The version of this plugin you're running
- **Hardware Configuration**:
  - CPU model and architecture
  - Disk controllers (HBA, RAID controllers)
  - GPU model (if GPU metrics are affected)
  - UPS model (if UPS monitoring is affected)
  - Network cards (if network monitoring is affected)
- **Description**: Clear description of the problem
- **Steps to Reproduce**: Detailed steps to reproduce the issue
- **Expected Behavior**: What you expected to happen
- **Actual Behavior**: What actually happened
- **Logs**: Relevant logs from `/var/log/unraid-management-agent.log`
- **API Endpoint**: If API-related, which endpoint(s) are affected

**Example Bug Report:**

```
**Unraid Version:** 7.2
**Plugin Version:** 2025.11.1
**Hardware:** AMD Ryzen 9 5950X, LSI 9300-8i HBA, NVIDIA RTX 3080

**Description:** GPU temperature not detected, endpoint returns null

**Steps to Reproduce:**
1. Install plugin on system with NVIDIA RTX 3080
2. Call GET /api/v1/gpu
3. Observe temperature field is null

**Expected:** Temperature should show GPU temp in Celsius
**Actual:** Temperature field is null

**Logs:**
[Include relevant log entries]
```

### Suggesting Features

Feature requests are welcome! Please provide:

- **Use Case**: Why this feature would be useful
- **Proposed Solution**: How you envision this working
- **Alternatives Considered**: Other approaches you've thought about
- **Hardware Requirements**: Any specific hardware this relates to
- **API Design**: If proposing new endpoints, suggest the API structure

### Hardware Compatibility Issues

If the plugin doesn't work correctly on your hardware, see the [Hardware Compatibility Contributions](#hardware-compatibility-contributions) section below for detailed guidance on how you can help fix it.

## Development Setup

### Prerequisites

- Go 1.23 or later
- Git
- Access to an Unraid system for testing (recommended)
- Make

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/ruaan-deysel/unraid-management-agent.git
cd unraid-management-agent

# Install dependencies
make deps

# Build for local development (your architecture)
make local

# Run tests
make test

# Build for Unraid (Linux/amd64)
make release

# Create plugin package
make package
```

### Running Locally

```bash
# Standard mode
./unraid-management-agent boot

# Debug mode (stdout logging, more verbose)
./unraid-management-agent boot --debug

# Custom port
./unraid-management-agent boot --port 8043
```

### Project Structure

```
daemon/
├── cmd/              # CLI commands
├── common/           # Constants (intervals, paths)
├── domain/           # Core types (Context, Config)
├── dto/              # Data transfer objects
├── lib/              # Utilities (shell execution, parsing, validation)
├── logger/           # Logging wrapper
└── services/
    ├── api/          # HTTP server, handlers, WebSocket
    ├── collectors/   # Data collection subsystems
    └── controllers/  # Control operations (Docker, VM, Array)
```

See [CLAUDE.md](CLAUDE.md) for detailed architecture documentation.

## Development Workflow

1. **Fork the Repository**: Create your own fork on GitHub
2. **Create a Branch**: Use a descriptive branch name
   ```bash
   git checkout -b feature/add-disk-temperature-support
   git checkout -b fix/gpu-metrics-nvidia-rtx
   git checkout -b docs/improve-api-examples
   ```
3. **Make Your Changes**: Write clean, documented code
4. **Add Tests**: Include tests for new functionality
5. **Run Tests**: Ensure all tests pass
   ```bash
   make test
   ```
6. **Test on Unraid**: If possible, test on actual Unraid hardware
7. **Commit Your Changes**: Follow commit message guidelines (see below)
8. **Push to Your Fork**: Push your branch to GitHub
9. **Open a Pull Request**: Submit a PR with a clear description

## Pull Request Process

### Before Submitting

- ✅ All tests pass (`make test`)
- ✅ Code follows Go best practices and project conventions
- ✅ New features have corresponding tests
- ✅ Documentation is updated (README.md, CLAUDE.md, code comments)
- ✅ No sensitive information (API keys, passwords, personal data) in code
- ✅ Commit messages follow guidelines (see below)

### PR Description Template

```markdown
## Description
Brief description of what this PR does

## Type of Change
- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Hardware compatibility fix

## Hardware Configuration (if applicable)
- **CPU:**
- **Disk Controller:**
- **GPU:**
- **UPS:**
- **Other:**

## Testing Performed
- [ ] Unit tests pass
- [ ] Tested on actual Unraid system
- [ ] Verified affected API endpoints work correctly
- [ ] Tested WebSocket events (if applicable)

## Test Results
Describe what you tested and the results

## Related Issues
Fixes #(issue number)

## Screenshots/Logs (if applicable)
Include relevant screenshots or log excerpts
```

### Review Process

1. Maintainer will review your PR
2. Automated tests will run via GitHub Actions
3. Feedback may be provided for changes
4. Once approved, PR will be merged
5. Your contribution will be included in the next release

## Hardware Compatibility Contributions

### Why This Matters

As a single maintainer, it's challenging to:
- Test across all possible hardware configurations
- Debug issues on systems I don't have access to
- Support every variation of disk controllers, GPUs, UPS models, etc.

**Your contributions help make this plugin work for everyone!** Even small fixes for specific hardware configurations are valuable and appreciated.

### Common Hardware Variations

The plugin may behave differently across:

- **CPU Architectures**: Different CPU models, vendors (Intel vs AMD)
- **Disk Controllers**: RAID controllers, HBA cards (LSI, Dell PERC, etc.), SAS/SATA controllers
- **GPU Models**: NVIDIA (different generations), AMD, Intel
- **UPS Models**: Different UPS brands and monitoring software (apcupsd, NUT)
- **Network Interfaces**: Various network cards, bonding configurations, VLANs

### If The Plugin Doesn't Work on Your Hardware

**You can help fix it!** Here's how:

#### 1. Identify the Issue

- Which component is failing? (disk detection, GPU metrics, UPS monitoring, etc.)
- Which API endpoint returns incorrect data or errors?
- What error messages appear in the logs?

#### 2. Investigate the Root Cause

**Enable debug logging:**
```bash
./unraid-management-agent boot --debug
```

**Common problem areas:**
- Command output parsing differences
- Different command-line tool versions
- Missing binaries or tools
- Different file formats or locations

**Example Investigation:**

```bash
# If GPU metrics aren't working, manually run the command:
/usr/bin/nvidia-smi --query-gpu=temperature.gpu,utilization.gpu,utilization.memory --format=csv,noheader

# Compare the output to what the code expects in:
# daemon/services/collectors/gpu.go
```

#### 3. Make the Fix

**Update the appropriate collector:**
- Collectors are in `daemon/services/collectors/`
- Parsers are in `daemon/lib/parser.go`

**Add fallback logic for hardware variations:**

```go
// Example: Handle different nvidia-smi output formats
temp, err := parseNvidiaSMIOutput(output)
if err != nil {
    // Try alternative parsing method for older GPUs
    temp, err = parseNvidiaSMIOutputLegacy(output)
}
```

**Add error handling and logging:**

```go
if err != nil {
    logger.Warning("GPU: Failed to parse temperature (GPU model may not be supported): %v", err)
    return defaultGPUMetrics()
}
```

#### 4. Test Thoroughly

- ✅ Run the full test suite: `make test`
- ✅ Test all affected API endpoints
- ✅ Verify WebSocket events work correctly
- ✅ Test on your actual Unraid system
- ✅ Check logs for errors or warnings

#### 5. Document Your Changes

In your pull request, include:

**Hardware Configuration:**
```
- CPU: AMD Ryzen 9 5950X
- Disk Controller: LSI 9300-8i HBA
- GPU: NVIDIA RTX 3080
- UPS: APC Back-UPS Pro 1500
- Unraid Version: 7.2
```

**Issue Description:**
- What wasn't working and why
- Command output differences you discovered
- Error messages from logs

**Solution Implemented:**
- How your changes fix the issue
- What parsing/detection logic was added
- Any fallback mechanisms implemented

**Testing Performed:**
- Which endpoints you tested
- Command outputs before and after
- Any edge cases you tested

#### Example Hardware Compatibility PR

```markdown
## Description
Add support for AMD GPU temperature monitoring via `rocm-smi`

## Hardware Configuration
- **GPU:** AMD Radeon RX 6800 XT
- **Unraid Version:** 7.2

## Issue
GPU collector only supported NVIDIA GPUs via `nvidia-smi`. AMD GPUs
returned empty metrics.

## Solution
- Added AMD GPU detection via `rocm-smi` command
- Implemented parser for `rocm-smi` output format
- Added fallback: try NVIDIA first, then AMD
- Updated GPU collector to handle both vendors

## Testing
- ✅ AMD GPU temp/utilization correctly reported
- ✅ Existing NVIDIA support still works (tested on RTX 3080)
- ✅ All unit tests pass
- ✅ WebSocket events broadcast GPU metrics correctly

## Related Issues
Fixes #42
```

## Testing Guidelines

### Unit Tests

- Write tests for all new functions and features
- Use table-driven tests for multiple test cases
- Test both success and error paths
- Mock external dependencies (file system, shell commands)

**Example Test Structure:**

```go
func TestParseGPUTemperature(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected float64
        wantErr  bool
    }{
        {
            name:     "NVIDIA RTX format",
            input:    "65, 45, 30",
            expected: 65.0,
            wantErr:  false,
        },
        {
            name:     "AMD ROCm format",
            input:    "Temperature: 62.0°C",
            expected: 62.0,
            wantErr:  false,
        },
        {
            name:     "Invalid input",
            input:    "invalid",
            expected: 0,
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := parseGPUTemperature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("wantErr %v, got err %v", tt.wantErr, err)
            }
            if result != tt.expected {
                t.Errorf("expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific test file
go test -v ./daemon/services/api/handlers_test.go

# Run specific test function
go test -v ./daemon/lib -run TestParseGPUTemperature
```

### Integration Testing

If possible, test on an actual Unraid system:

1. Build the plugin: `make package`
2. Install on Unraid test system
3. Test all affected API endpoints
4. Monitor logs: `tail -f /var/log/unraid-management-agent.log`
5. Test WebSocket events with a WebSocket client

## Coding Standards

### Go Best Practices

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting (enforced by CI)
- Write clear, descriptive variable and function names
- Add comments for exported functions and complex logic
- Handle errors explicitly (don't ignore them)
- Use defer for cleanup operations
- Avoid global mutable state

### Project-Specific Conventions

**Collectors:**
- Always respect context cancellation
- Implement panic recovery
- Log at appropriate levels (Debug for frequent events, Info for important events)
- Publish events via `ctx.Hub.Pub(data, topic)`

**API Handlers:**
- Use mutex locks when accessing cache (`s.cacheMutex.RLock()`)
- Validate all user input
- Return appropriate HTTP status codes
- Use `respondWithJSON()` and `respondWithError()` helpers

**Controllers:**
- Validate input with `lib.Validate*()` functions
- Use `lib.ExecuteShellCommand()` for command execution
- Never trust user input in shell commands (prevent injection)
- Return detailed error messages

**Logging:**
- Debug: Detailed diagnostic info (frequent events)
- Info: General informational messages
- Success: Successful operations
- Warning: Warning conditions (degraded but working)
- Error: Error conditions (failures)

### Security Considerations

- **Input Validation**: Always validate user input on control endpoints
- **Command Injection Prevention**: Use whitelists, avoid shell interpolation
- **No Secrets in Code**: Never commit API keys, passwords, tokens
- **File Access**: Validate file paths to prevent directory traversal
- **Error Messages**: Don't leak sensitive information in error messages

## Commit Message Guidelines

### Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- **feat**: New feature
- **fix**: Bug fix
- **docs**: Documentation changes
- **test**: Adding or updating tests
- **refactor**: Code refactoring (no functional changes)
- **perf**: Performance improvements
- **chore**: Build process, dependencies, tooling

### Examples

```
feat(collectors): Add AMD GPU support via rocm-smi

Implements AMD GPU temperature and utilization monitoring using
the rocm-smi command-line tool. Maintains backward compatibility
with existing NVIDIA GPU support.

Tested on AMD Radeon RX 6800 XT with Unraid 7.2.
```

```
fix(api): Fix race condition in WebSocket hub

The WebSocket hub could panic when clients disconnected during
broadcast. Added mutex lock to synchronize client map access.

Fixes #123
```

```
docs(readme): Add troubleshooting section for UPS monitoring

Added common UPS monitoring issues and solutions based on
community feedback.
```

### Commit Best Practices

- Use present tense ("Add feature" not "Added feature")
- Use imperative mood ("Move cursor to..." not "Moves cursor to...")
- First line: 50 characters or less
- Body: Wrap at 72 characters
- Reference issues and PRs in the footer

## Community and Support

### Getting Help

- **Documentation**: Check [README.md](README.md) and [CLAUDE.md](CLAUDE.md)
- **Issues**: Search existing issues or create a new one
- **Discussions**: Use GitHub Discussions for questions and ideas

### Communication

- Be respectful and constructive
- Provide context and details when asking questions
- Share your hardware configuration when reporting issues
- Help others when you can

### Recognition

All contributors will be recognized in release notes and the project README. Thank you for helping make this plugin better for the entire Unraid community!

---

## Questions?

If you have questions about contributing, please open an issue with the "question" label, and we'll be happy to help!

Thank you for contributing! 🎉
