# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

A Go-based Unraid plugin exposing system monitoring and control via REST API and WebSockets. The project is built on an event-driven architecture using a pubsub pattern for real-time data collection and distribution.

## Essential Commands

### Building
```bash
# Install Go dependencies
make deps

# Build for local development (current platform)
make local

# Build for Unraid deployment (Linux/amd64)
make release

# Create Unraid plugin package (.tgz)
make package
```

### Testing
```bash
# Run all tests
make test

# Generate coverage report (creates coverage.html)
make test-coverage

# Run specific test file
go test -v ./daemon/services/api/handlers_test.go
```

### Development
```bash
# Run with mock mode (for non-Unraid development)
./unraid-management-agent --mock

# Or with environment variable
MOCK_MODE=true ./unraid-management-agent

# Change port (default 8080)
./unraid-management-agent --port 8043
```

### Cleaning
```bash
# Remove build artifacts and coverage files
make clean
```

## Architecture Overview

### Event-Driven Architecture

The application uses a **pubsub event bus** (github.com/cskr/pubsub) as the central communication mechanism:

1. **Collectors** gather data at fixed intervals and publish events to specific topics
2. **API Server** subscribes to all topics, caches the latest data, and serves via REST/WebSocket
3. **WebSocket Hub** broadcasts events to connected clients in real-time

**Event Flow:**
```
Collector → Event Bus (pubsub) → API Server Cache → REST Endpoints
                                 ↓
                          WebSocket Hub → Connected Clients
```

**Key Event Topics:**
- `system_update` - CPU, RAM, temps, uptime
- `array_status_update` - Array state, parity
- `disk_list_update` - Disk metrics
- `container_list_update` - Docker containers
- `vm_list_update` - Virtual machines
- `ups_status_update` - UPS status
- `gpu_metrics_update` - GPU data
- `share_list_update` - User shares

### Orchestrator Pattern

The `services/orchestrator.go` coordinates the entire application lifecycle:
- Instantiates all collectors with their intervals
- Starts collectors as goroutines
- Launches the API server
- Handles graceful shutdown on SIGTERM/SIGINT

### Collector Pattern

Each collector (`services/collectors/*.go`) follows a consistent pattern:
1. Implements a `Start(interval time.Duration)` method that runs in a goroutine
2. Collects data from Unraid system files, commands, or APIs
3. Publishes domain-specific DTO to the event bus
4. Intervals are defined in `daemon/common/const.go`

Collection intervals (seconds):
- System: 5s
- Array: 10s
- Disk: 30s
- Docker/VM/UPS/GPU: 10s
- Shares: 60s

### API Server Architecture

**Caching Layer:**
The API server maintains an in-memory cache (`Server.systemCache`, `Server.dockerCache`, etc.) that is updated via event subscriptions. This allows instant REST responses without re-collecting data.

**Dual Event Subscriptions:**
- One subscription updates the internal cache for REST endpoints
- Another subscription broadcasts to WebSocket clients

**REST API:** Gorilla Mux router with middleware (CORS, logging, recovery)

**WebSocket:** Gorilla WebSocket with a hub pattern managing client connections, broadcasting, and ping/pong keepalives.

## Key Technical Details

### Entry Point
- Uses **Kong CLI framework** for command-line argument parsing
- Single command: `Boot` which invokes the orchestrator
- Structured logging with **Lumberjack** (10MB files, 10 backups, 28-day retention)

### Domain Structure
```
daemon/
├── cmd/           # CLI commands (Boot)
├── common/        # Constants (intervals, paths, binaries)
├── domain/        # Core types (Context, Config)
├── dto/           # Data transfer objects
├── lib/           # Utilities (shell execution with timeouts)
├── logger/        # Logging wrapper
└── services/
    ├── api/           # HTTP server, handlers, WebSocket
    ├── collectors/    # Data collection for each subsystem
    └── controllers/   # Control operations (start/stop Docker/VMs)
```

### Unraid-Specific Paths
Defined in `daemon/common/const.go`:
- Config files: `/var/local/emhttp/*.ini` (var.ini, disks.ini, shares.ini)
- System files: `/proc/*` (cpuinfo, meminfo, stat)
- Binaries: `/usr/bin/docker`, `/usr/bin/virsh`, `/usr/local/sbin/mdcmd`, `/usr/sbin/smartctl`

### Mock Mode
When `--mock` flag or `MOCK_MODE=true` is set:
- Collectors skip real data collection
- Allows development on non-Unraid systems
- Check implemented via `ctx.MockMode` in collector methods

### Context Object
The `domain.Context` struct carries application state throughout the codebase:
- `Config` - Version, port, mock mode flag
- `Hub` - PubSub instance for event bus (initialized with 1024 buffer)

Passed to all collectors, controllers, and the API server.

### Controller Pattern
Controllers in `services/controllers/` execute control operations:
- Docker operations via Docker CLI commands
- VM operations via `virsh` commands
- Commands executed through `lib.ExecCommand()` with 60s timeout
- Return errors for command failures

### Testing Strategy
Tests exist for:
- DTOs (`daemon/dto/system_test.go`)
- Utilities (`daemon/lib/shell_test.go`)
- API handlers (`daemon/services/api/handlers_test.go`)

Empty directories `tests/unit/` and `tests/integration/` suggest planned test expansion.

## Plugin Packaging

The Unraid plugin structure created by `make package`:
```
build/
└── usr/local/emhttp/plugins/unraid-management-agent/
    ├── unraid-management-agent  # Binary
    ├── VERSION
    └── meta/plugin/*             # Unraid plugin metadata
```

Packaged as `unraid-management-agent-<version>.tgz` in the `build/` directory.
