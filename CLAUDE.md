# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The Unraid Management Agent is a Go-based plugin for Unraid that exposes comprehensive system monitoring and control via REST API and WebSockets. This is a **third-party community plugin**, not an official Unraid product. It provides a REST API + WebSocket interface as an alternative/complement to the official Unraid GraphQL API.

**Language:** Go 1.24
**Target Platform:** Linux/amd64 (Unraid OS)

## Essential Commands

### Building and Testing

```bash
# Install dependencies
make deps

# Build for local development (current architecture)
make local

# Build for Unraid (Linux/amd64)
make release

# Create full plugin package (.tgz)
make package

# Run all tests
make test

# Run tests with coverage report
make test-coverage

# Run specific test file
go test -v ./daemon/services/api/handlers_test.go

# Clean build artifacts
make clean
```

### Running the Agent

```bash
# Standard mode
./unraid-management-agent boot

# Debug mode (stdout logging)
./unraid-management-agent boot --debug

# Custom port
./unraid-management-agent boot --port 8043
```

### Development Workflow

The project uses semantic versioning with date-based releases (e.g., `2025.11.25`). When creating a release:

1. Update the `VERSION` file with the new version number
2. Update `CHANGELOG.md` with release notes
3. Create and push a git tag: `git tag v2025.11.25 && git push origin v2025.11.25`
4. GitHub Actions will automatically build and release the package

## Architecture

### Event-Driven Design with PubSub

The agent uses an event bus pattern for decoupled, real-time data flow:

```
Collectors → Event Bus (PubSub) → API Server Cache → REST Endpoints
                                ↓
                         WebSocket Hub → Connected Clients
```

**Critical Initialization Order:**

1. API server subscriptions are started FIRST (before collectors)
2. Small delay (100ms) to ensure subscriptions are ready
3. Then collectors start publishing events

This order is crucial to avoid race conditions where collectors publish events before the API server is ready to receive them.

### Core Components

#### 1. Domain Layer (`daemon/domain/`)

- `Context`: Application runtime context holding the PubSub hub and configuration
- `Config`: Configuration settings (version, port)

#### 2. Data Transfer Objects (`daemon/dto/`)

All data structures shared between collectors, API, and WebSocket clients:

- `SystemInfo`, `ArrayStatus`, `DiskInfo`, `NetworkInfo`
- `ContainerInfo`, `VMInfo`, `UPSStatus`, `GPUMetrics`
- `ShareInfo`, `WebSocketMessage`, `HardwareInfo`, `Registration`
- `NotificationList`, `UnassignedDeviceList`, `ZFSPool`, `ZFSDataset`

#### 3. Collectors (`daemon/services/collectors/`)

Independent goroutines that collect data at fixed intervals and publish to the event bus:

| Collector | Interval | Event Topic | Purpose |
|-----------|----------|-------------|---------|
| System | 5s | `system_update` | CPU, RAM, temps, uptime |
| Array | 10s | `array_status_update` | Array state, parity info |
| Disk | 30s | `disk_list_update` | Per-disk metrics, SMART data |
| Network | 15s | `network_list_update` | Interface status, bandwidth |
| Docker | 10s | `container_list_update` | Container information |
| VM | 10s | `vm_list_update` | Virtual machine data |
| UPS | 10s | `ups_status_update` | UPS status (if available) |
| GPU | 10s | `gpu_metrics_update` | GPU metrics (if available) |
| Share | 60s | `share_list_update` | User share information |
| Hardware | 300s | `hardware_update` | BIOS, baseboard, CPU, memory |
| Registration | 300s | `registration_update` | License/registration status |
| Notification | 15s | `notifications_update` | System notifications |
| Unassigned | 30s | `unassigned_devices_update` | Unassigned devices/shares |
| ZFS | 30s | `zfs_*_update` | ZFS pools, datasets, snapshots |

Each collector:

- Runs in its own goroutine with context cancellation support
- Has panic recovery to prevent crashes
- Publishes events via `ctx.Hub.Pub(data, topic)`
- Reads system data from Unraid-specific files or commands

#### 4. API Server (`daemon/services/api/`)

**server.go:**

- Maintains in-memory cache of latest collector data
- Subscribes to all event topics to update cache
- Broadcasts events to WebSocket clients
- Uses `sync.RWMutex` for thread-safe cache access

**handlers.go:**

- REST endpoint handlers that return cached data
- Control endpoints that execute Docker/VM/Array commands
- Configuration endpoints (read and write)

**websocket.go:**

- WebSocket hub managing connected clients
- Broadcasts events to all connected clients
- Client registration/unregistration
- Ping/pong for connection health

**middleware.go:**

- CORS middleware for API access
- Logging middleware for request/response
- Recovery middleware for panic handling

#### 5. Controllers (`daemon/services/controllers/`)

Execute control operations:

- `docker.go`: Start, stop, restart, pause, unpause containers
- `vm.go`: Start, stop, restart, pause, resume, hibernate VMs
- `array.go`: Start/stop array, parity check operations
- `notification.go`: Create, archive, delete notifications
- `userscripts.go`: Execute user scripts

#### 6. Library Utilities (`daemon/lib/`)

- `shell.go`: Execute shell commands with error handling
- `parser.go`: Parse Unraid-specific file formats (.ini files)
- `utils.go`: Common utility functions
- `validation.go`: Input validation for API requests (CWE-22 path traversal protection)
- `dmidecode.go`: DMI/SMBIOS data parsing for hardware info
- `ethtool.go`: Network interface tool parsing

#### 7. Orchestrator (`daemon/services/orchestrator.go`)

Coordinates the entire application lifecycle:

1. Initializes all collectors
2. Starts API server subscriptions **before** collectors (critical!)
3. Starts collectors in separate goroutines
4. Starts HTTP server
5. Manages graceful shutdown (signal handling, context cancellation)

### Data Flow Example

1. **System Collector** reads CPU/RAM from `/proc/meminfo`, `/proc/stat`
2. Publishes `dto.SystemInfo` to event bus topic `system_update`
3. **API Server** receives event, updates `systemCache`
4. **WebSocket Hub** receives event, broadcasts to all clients
5. **REST endpoint** `/api/v1/system` returns cached `systemCache` data

### Unraid Integration

The agent reads from Unraid-specific locations (see `daemon/constants/const.go`):

**Configuration Files:**

- `/var/local/emhttp/var.ini` - System variables
- `/var/local/emhttp/disks.ini` - Disk configuration
- `/var/local/emhttp/shares.ini` - Share configuration
- `/var/local/emhttp/network.ini` - Network configuration

**System Files:**

- `/proc/cpuinfo`, `/proc/meminfo`, `/proc/uptime` - System metrics
- `/sys/class/hwmon/` - Temperature sensors
- `/proc/spl/kstat/zfs/arcstats` - ZFS ARC statistics

**Binaries:**

- `/usr/local/sbin/mdcmd` - Unraid management command (array operations)
- `/usr/bin/docker` - Docker CLI
- `/usr/bin/virsh` - VM management
- `/usr/sbin/smartctl` - SMART data
- `/sbin/apcaccess`, `/usr/bin/upsc` - UPS monitoring
- `/usr/bin/nvidia-smi` - GPU metrics
- `/usr/sbin/zpool`, `/usr/sbin/zfs` - ZFS management
- `/usr/sbin/dmidecode` - Hardware information

## API Structure

Base URL: `http://localhost:8043/api/v1`

### Monitoring Endpoints (GET)

- `/health`, `/system`, `/array`, `/disks`, `/disks/{id}`
- `/network`, `/shares`, `/ups`, `/gpu`
- `/docker`, `/docker/{id}`, `/vm`, `/vm/{id}`
- `/hardware/*`, `/registration`, `/logs`
- `/notifications`, `/notifications/{id}`, `/unassigned`
- `/zfs/pools`, `/zfs/datasets`, `/zfs/snapshots`, `/zfs/arc`

### Control Endpoints (POST)

- `/docker/{id}/{action}` - start, stop, restart, pause, unpause
- `/vm/{id}/{action}` - start, stop, restart, pause, resume, hibernate, force-stop
- `/array/{action}` - start, stop
- `/array/parity-check/{action}` - start, stop, pause, resume
- `/notifications` - create, archive, delete notifications
- `/user-scripts/{name}/execute` - execute user scripts

### Configuration Endpoints

- GET: `/shares/{name}/config`, `/network/{interface}/config`, `/settings/{subsystem}`
- POST: `/shares/{name}/config`, `/settings/system`

### WebSocket

- `/ws` - Real-time event streaming

## Testing

**Test Locations:**

- `daemon/dto/system_test.go` - DTO tests
- `daemon/lib/shell_test.go`, `daemon/lib/validation_test.go` - Library tests
- `daemon/services/api/handlers_test.go` - API handler tests
- `daemon/services/collectors/config_security_test.go` - Config security tests
- `daemon/services/controllers/notification_security_test.go` - Notification security tests

**Test Conventions:**

- Use table-driven tests where appropriate
- Mock external dependencies (file system, command execution)
- Test both success and error paths
- Coverage target: Generate reports with `make test-coverage`

## Logging

**Location:** `/var/log/unraid-management-agent.log`
**Logger:** `daemon/logger/logger.go` (wrapper around lumberjack for rotation)

**Log Levels:**

- `logger.Debug()` - Detailed diagnostic info
- `logger.Info()` - General informational messages
- `logger.Success()` - Successful operations (green)
- `logger.Warning()` - Warning conditions (yellow)
- `logger.Error()` - Error conditions (red)

**Log Rotation:**

- Max size: 5 MB
- No backups (only current log)
- No age-based retention

## Security Considerations

**Input Validation:**

- All user-provided file paths must be validated using `lib.ValidateConfigPath()` or `lib.ValidateNotificationFilename()`
- Protection against CWE-22 path traversal vulnerabilities
- No directory traversal (`..`), absolute paths (`/`), or null bytes allowed
- See `daemon/lib/validation.go` for validation functions

**Command Injection:**

- Use `lib.ExecuteShellCommand()` for safe command execution
- Validate container names, VM names, and other user inputs before using in commands
- Never directly interpolate user input into shell commands

## Hardware Compatibility

This plugin was developed on a specific hardware configuration. Hardware variations (CPU, disk controllers, GPUs, UPS models) may cause compatibility issues. When fixing hardware-specific bugs:

1. Identify the failing component (disk collector, GPU collector, etc.)
2. Update command parsing in `daemon/lib/parser.go`, `daemon/lib/dmidecode.go`, or collector logic
3. Add fallback logic for different hardware variations
4. Document the fix in the PR with hardware details

Common hardware variation areas:

- GPU metrics parsing (`nvidia-smi` output formats)
- Disk controller command outputs
- UPS monitoring tool differences (apcupsd vs NUT)
- Network interface variations
- DMI/SMBIOS data structure differences

## Key Dependencies

- `github.com/alecthomas/kong` - CLI framework
- `github.com/cskr/pubsub` - Event bus (PubSub pattern)
- `github.com/gorilla/mux` - HTTP router
- `github.com/gorilla/websocket` - WebSocket implementation
- `gopkg.in/ini.v1` - INI file parsing
- `gopkg.in/natefinch/lumberjack.v2` - Log rotation
- `github.com/fsnotify/fsnotify` - File system notifications

## Common Patterns

### Adding a New Collector

1. Create collector in `daemon/services/collectors/`
2. Define DTO in `daemon/dto/`
3. Add event topic constant
4. Implement `Start(ctx context.Context, interval time.Duration)` method
5. Publish data: `ctx.Hub.Pub(data, "topic_name")`
6. Add subscription in `api/server.go` `subscribeToEvents()`
7. Add cache field and update logic
8. Create REST endpoint handler
9. Register collector in `orchestrator.go`

### Adding a New REST Endpoint

1. Define route in `api/server.go` `setupRoutes()`
2. Create handler function in `api/handlers.go`
3. Use `s.cacheMutex.RLock()` / `s.cacheMutex.RUnlock()` for cache access
4. Return JSON: `respondWithJSON(w, http.StatusOK, data)`
5. Handle errors: `respondWithError(w, http.StatusInternalServerError, message)`

### Adding a New Control Operation

1. Create controller function in `daemon/services/controllers/`
2. Use `lib.ExecuteShellCommand()` for command execution
3. Validate input with `lib.ValidateContainerName()` or similar
4. Add endpoint handler in `api/handlers.go`
5. Return appropriate HTTP status codes

## Important Notes

- **Never skip the initialization order** in orchestrator.go (API subscriptions before collectors)
- **Always use mutex locks** when accessing API server cache
- **Always validate user input** on control endpoints to prevent command injection and path traversal
- **Test on actual Unraid** if possible, as local development differs from production
- **Handle graceful shutdown** by respecting context cancellation in goroutines
- **Panic recovery** is built into collectors and middleware, but avoid panics when possible
- **Use Context7** Always use context7 when I need code generation, setup or configuration steps, or library/API documentation. This means you should automatically use the Context7 MCP tools to resolve library id and get library docs without me having to explicitly ask.
- **Always use Sequential Thinking** internally reason step-by-step before answering, ensuring each solution is planned, validated, and logically sequenced, while keeping the reasoning hidden unless explicitly requested.