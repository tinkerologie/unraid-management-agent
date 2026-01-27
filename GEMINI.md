# Unraid Management Agent

## Project Overview

The **Unraid Management Agent** is a community-developed, third-party plugin for Unraid OS. It provides a comprehensive **REST API** and **WebSocket** interface for system monitoring and control, acting as a bridge between Unraid's underlying system (Docker, Libvirt, storage arrays) and external tools or UIs.

**Key Features:**

* **Real-time Monitoring:** CPU, RAM, Disk, Network, Docker, VM, UPS, GPU.
* **Control Operations:** Docker/VM power management, Array/Parity control, User Scripts.
* **Architecture:** Event-driven architecture using a Pub/Sub model.
* **Integration:** Supports Model Context Protocol (MCP) for AI agent integration.
* **Dependencies:** Written in Go (1.24+), utilizing native libraries (Docker SDK, go-libvirt) where possible, with no external plugin dependencies.

## Architecture

The system follows a clean, layered architecture:

* **Collectors (`daemon/services/collectors`):** Gather data from system sources (native APIs or shell commands) at configurable intervals.
* **Event Bus:** Decouples collectors from consumers.
* **API Server (`daemon/services/api`):** Maintains an in-memory cache of the latest state and serves REST endpoints.
* **WebSocket Hub:** Broadcasts real-time events to connected clients.
* **Orchestrator:** Manages the lifecycle of all components.

## Environment Setup

### Prerequisites

* **Go:** Version 1.24 or later.
* **Make:** For build automation.
* **Docker:** Required for dev container or building specific components.
* **VS Code (Recommended):** The project includes a `.devcontainer` configuration with all necessary tools (Go, Node.js, GitHub CLI, etc.).

### Initial Setup

```bash
# Clone the repository
git clone https://github.com/ruaan-deysel/unraid-management-agent.git
cd unraid-management-agent

# Install Go dependencies
make deps

# Install Pre-commit hooks (Highly Recommended)
./scripts/setup-pre-commit.sh
# OR
make pre-commit-install
```

## Building and Running

### Local Development (macOS/Linux)

Build the binary for your local architecture:

```bash
make local
```

Run the agent locally:

```bash
# Standard boot
./unraid-management-agent boot

# Debug mode (verbose stdout logging)
./unraid-management-agent boot --debug

# Custom port
./unraid-management-agent boot --port 8043
```

### Building for Unraid (Linux/amd64)

Compile the binary specifically for the Unraid environment:

```bash
make release
```

### Creating the Plugin Package

Create the `.tgz` package for installation on Unraid:

```bash
make package
# Output: build/unraid-management-agent-<VERSION>.tgz
```

## Testing

Run the full test suite:

```bash
make test
```

Generate and view a coverage report:

```bash
make test-coverage
# Opens coverage.html
```

## Key Commands (Makefile)

| Command | Description |
| :--- | :--- |
| `make deps` | Download and tidy Go modules. |
| `make local` | Build binary for the current OS/Arch. |
| `make release` | Build binary for Linux/amd64 (Unraid). |
| `make package` | Create the Unraid plugin package (`.tgz`). |
| `make test` | Run all unit tests. |
| `make test-coverage` | Run tests with coverage reporting. |
| `make lint` | Run `golangci-lint` check. |
| `make security-check` | Run `gosec` and `govulncheck`. |
| `make swagger` | Generate Swagger API documentation. |
| `make clean` | Remove build artifacts. |

## Project Structure

```text
/
├── daemon/                 # Application Source Code
│   ├── cmd/                # CLI entry points (boot command)
│   ├── common/             # Shared constants and paths
│   ├── domain/             # Core domain types (Context, Config)
│   ├── dto/                # Data Transfer Objects
│   ├── lib/                # Utilities (Shell, Parsing, Validation)
│   ├── logger/             # Logging wrapper
│   └── services/           # Business Logic
│       ├── api/            # REST API & WebSocket handlers
│       ├── collectors/     # Data collection modules (CPU, Docker, Disk, etc.)
│       └── controllers/    # Action controllers (Start/Stop containers, etc.)
├── docs/                   # Documentation (API, MCP, WebSocket)
├── meta/                   # Plugin metadata (XML, page files)
├── scripts/                # Utility scripts (Deployment, Testing)
├── tests/                  # Integration tests
├── Makefile                # Build automation
├── go.mod                  # Go dependencies
└── CONTRIBUTING.md         # Contribution guidelines
```

## Development Conventions

* **Code Style:** Standard Go conventions. `gofmt` and `goimports` are enforced.
* **Linting:** Zero tolerance for linting errors. Use `make lint` and `make pre-commit-run` before committing.
* **Security:** No secrets in code. Input validation is mandatory for control endpoints.
* **Commit Messages:** Follow **Conventional Commits**:
  * `feat(scope): description`
  * `fix(scope): description`
  * `docs(scope): description`
* **Hardware Compatibility:** When modifying collectors, consider different hardware configurations (Intel/AMD CPUs, different GPUs, HBA cards). Add fallback logic if parsing command output.

## Documentation References

* **API Reference:** `docs/api/API_REFERENCE.md`
* **WebSocket Events:** `docs/websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md`
* **MCP Integration:** `docs/MCP_INTEGRATION.md`
* **System Requirements:** `docs/SYSTEM_REQUIREMENTS_AND_DEPENDENCIES.md`
