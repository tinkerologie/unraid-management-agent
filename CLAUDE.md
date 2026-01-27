# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The Unraid Management Agent is a Go-based plugin for Unraid that exposes comprehensive system monitoring and control via REST API, WebSockets, and MCP (Model Context Protocol). This is a **third-party community plugin**, not an official Unraid product. It provides a REST API + WebSocket interface as an alternative/complement to the official Unraid GraphQL API.

**Language:** Go 1.25
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

### Deployment to Unraid

Use the provided deployment scripts for building and testing on actual hardware:

```bash
# 1. Create config with Unraid SSH credentials
cp scripts/config.sh.example scripts/config.sh
# Edit config.sh with actual Unraid server IP, username, and password

# 2. Deploy and test
./scripts/deploy-plugin.sh
```

### Pre-commit Hooks

The project enforces code quality via pre-commit hooks:

```bash
# Automated setup
./scripts/setup-pre-commit.sh

# Or manual setup
pip install pre-commit
make pre-commit-install

# Run checks manually
make pre-commit-run
make lint
make security-check
```

### Release Workflow

The project uses date-based versioning: `YYYY.MM.DD` (e.g., `2025.12.01`).

1. Update `CHANGELOG.md` with changes (required for every change)
2. Update `VERSION` file with new version number
3. Update `.plg` files (both root and `meta/template/`):
   - Set `<!ENTITY version "YYYY.MM.DD">`
   - Set `<!ENTITY md5 "...">` with checksum **from GitHub release** (not local build)
4. Create and push tag: `git tag vYYYY.MM.DD && git push origin vYYYY.MM.DD`
5. GitHub Actions builds and releases automatically
6. Verify MD5 matches published release artifact

## Architecture

### Event-Driven Design with PubSub

The agent uses an event bus pattern (`github.com/cskr/pubsub`) for decoupled, real-time data flow:

```
Collectors → Event Bus (PubSub) → API Server Cache → REST Endpoints
                                ↓                   ↓
                         WebSocket Hub        MCP Server → AI Agents
                                ↓
                         Connected Clients
```

**Critical Initialization Order (in `orchestrator.go`):**

1. API server creates subscriptions via `Hub.Sub()` FIRST
2. 100ms delay ensures subscriptions are ready
3. Then collectors start publishing via `Hub.Pub(data, "topic_name")`

**Never change this order** — collectors publishing before subscriptions causes lost events.

### Native API Integration

For optimal performance, collectors use native Go libraries instead of shell commands:

| Component | Library                              | Purpose                 |
| --------- | ------------------------------------ | ----------------------- |
| Docker    | `github.com/moby/moby/client`        | Docker Engine SDK       |
| VMs       | `github.com/digitalocean/go-libvirt` | Native libvirt bindings |
| System    | Direct `/proc`, `/sys` access        | Kernel interfaces       |

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

Independent goroutines that collect data at fixed intervals (defined in `daemon/constants/const.go`):

| Collector    | Interval | Event Topic                 | Notes                                            |
| ------------ | -------- | --------------------------- | ------------------------------------------------ |
| System       | 15s      | `system_update`             | CPU/RAM/temps - sensors command is CPU intensive |
| Array        | 30s      | `array_status_update`       | Array state rarely changes                       |
| Disk         | 30s      | `disk_list_update`          | Per-disk SMART data                              |
| Network      | 30s      | `network_list_update`       | Interface status                                 |
| Docker       | 30s      | `container_list_update`     | Very CPU intensive with many containers          |
| VM           | 30s      | `vm_list_update`            | virsh commands spawn multiple processes          |
| UPS          | 60s      | `ups_status_update`         | UPS status rarely changes                        |
| GPU          | 60s      | `gpu_metrics_update`        | intel_gpu_top is extremely CPU intensive         |
| Share        | 60s      | `share_list_update`         | User share information                           |
| Notification | 30s      | `notifications_update`      | System notifications                             |
| Unassigned   | 60s      | `unassigned_devices_update` | Unassigned devices                               |
| ZFS          | 30s      | `zfs_*_update`              | Pools, datasets, snapshots                       |
| Hardware     | 300s     | `hardware_update`           | Rarely changes                                   |
| Registration | 300s     | `registration_update`       | License info                                     |

**Intervals optimized for power efficiency** — lower intervals increase CPU usage and power consumption.

Each collector:

- Runs in its own goroutine with context cancellation support
- **Must wrap work in defer/recover** for panic recovery
- Publishes events via `ctx.Hub.Pub(data, topic)`

#### 4. API Server (`daemon/services/api/`)

- **server.go**: Maintains in-memory cache, subscribes to event topics, broadcasts to WebSocket clients. Uses `sync.RWMutex` for thread-safe cache access.
- **handlers.go**: REST endpoint handlers. **Always use RLock/RUnlock** for cache reads.
- **websocket.go**: WebSocket hub with client registration and ping/pong health checks.
- **middleware.go**: CORS, logging, and recovery middleware.

#### 5. MCP Server (`daemon/services/mcp/`)

Model Context Protocol endpoint at `POST /mcp` for AI agent integration:

- **server.go**: MCP server with tools for monitoring and control
- **transport.go**: HTTP transport for JSON-RPC requests
- Tools expose system info, Docker/VM control, notifications, etc.
- See `docs/MCP_INTEGRATION.md` for full documentation (54 tools available)

#### 6. Controllers (`daemon/services/controllers/`)

Execute control operations via `lib.ExecuteShellCommand()`:

- `docker.go`: Container start/stop/restart/pause/unpause
- `vm.go`: VM start/stop/restart/pause/resume/hibernate
- `array.go`: Array start/stop, parity check
- `notification.go`: Notification create/archive/delete
- `userscripts.go`: User script execution

#### 7. Library Utilities (`daemon/lib/`)

- `shell.go`: Execute shell commands with error handling
- `parser.go`: Parse Unraid-specific file formats (.ini files)
- `validation.go`: Input validation (CWE-22 path traversal protection)
- `dmidecode.go`, `ethtool.go`: Hardware info parsing

#### 8. Orchestrator (`daemon/services/orchestrator.go`)

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

See `docs/api/API_REFERENCE.md` for complete endpoint documentation (46 endpoints).

**Key endpoint patterns:**

- `GET /system`, `/array`, `/disks`, `/docker`, `/vm`, etc. — monitoring data
- `POST /docker/{id}/{action}`, `/vm/{id}/{action}` — control operations
- `GET /ws` — WebSocket real-time events
- `POST /mcp` — MCP JSON-RPC endpoint for AI agents

## Testing

```bash
make test           # Run all tests
make test-coverage  # Generate coverage.html report

# Run specific test
go test -v ./daemon/services/api/handlers_test.go
```

Use **table-driven tests** with security cases (path traversal, command injection). Tests located alongside source (`*_test.go`).

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
- `github.com/moby/moby/client` - Docker Engine SDK (native API)
- `github.com/digitalocean/go-libvirt` - Native libvirt bindings for VMs
- `github.com/metoro-io/mcp-golang` - Model Context Protocol server
- `gopkg.in/ini.v1` - INI file parsing
- `gopkg.in/natefinch/lumberjack.v2` - Log rotation

## Common Patterns

### Panic Recovery in Collectors

**All collector loops MUST wrap work in defer/recover:**

```go
func (c *Collector) Start(ctx context.Context, interval time.Duration) {
    // Run once immediately with recovery
    func() {
        defer func() {
            if r := recover(); r != nil {
                logger.Error("Collector PANIC on startup: %v", r)
            }
        }()
        c.Collect()
    }()

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            func() {
                defer func() {
                    if r := recover(); r != nil {
                        logger.Error("Collector PANIC in loop: %v", r)
                    }
                }()
                c.Collect()
            }()
        }
    }
}
```

### Adding a New Collector

1. Create collector in `daemon/services/collectors/` following pattern above
2. Define DTO in `daemon/dto/`
3. Add subscription in `api/server.go` `subscribeToEvents()` — add topic to `Hub.Sub()` call and case in switch
4. Add cache field in `Server` struct and handler in `handlers.go`
5. Register in `orchestrator.go` — add to WaitGroup, create collector, launch goroutine

### Adding a REST Endpoint

```go
func (s *Server) handleNew(w http.ResponseWriter, _ *http.Request) {
    s.cacheMutex.RLock()
    data := s.newCache
    s.cacheMutex.RUnlock()
    respondJSON(w, http.StatusOK, data)
}
```

Register route in `server.go` `setupRoutes()`.

### Control Operations

Use `lib.ExecuteShellCommand()` for all shell commands — never use `exec.Command` directly. Always validate input with `lib.ValidateContainerID()`, `lib.ValidateVMName()`, etc.

## Important Notes

- **Initialization order is critical** — API subscriptions must start before collectors in orchestrator.go
- **Always use mutex locks** — RLock/RUnlock for cache reads, Lock/Unlock for writes
- **Always validate user input** — use `lib.Validate*()` functions to prevent injection and path traversal
- **Test on actual Unraid** — local development differs from production; use deployment scripts
- **Context cancellation** — respect `ctx.Done()` in goroutines for graceful shutdown
- **Keep CHANGELOG.md updated** — every change must be documented before release

### Claude-Specific Instructions

- **Use Context7** — automatically use Context7 MCP tools to get library documentation without explicit prompting
- **Sequential Thinking** — reason step-by-step internally before answering, keeping reasoning hidden unless requested
