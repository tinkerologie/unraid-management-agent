# AGENTS.md — AI Agent Instructions

> Single source of truth for all AI coding assistants working on this project.
> Individual tool files (CLAUDE.md, GEMINI.md, copilot-instructions.md, .cursorrules) point here.

## Project Identity

| Key | Value |
|-----|-------|
| **Name** | Unraid Management Agent |
| **Language** | Go 1.26 |
| **Target** | Linux/amd64 (Unraid OS) |
| **Type** | Third-party community plugin (not official Unraid) |
| **Purpose** | REST API + WebSocket + MCP interface for system monitoring and control |
| **Repo** | `github.com/ruaan-deysel/unraid-management-agent` |

## Project Structure

```text
/
├── daemon/                     # Application source code
│   ├── cmd/                    # CLI entry points (boot command)
│   ├── constants/              # System paths, binary locations, collection intervals
│   ├── domain/                 # Core domain types (Context, Config)
│   ├── dto/                    # Data Transfer Objects (shared structs)
│   ├── lib/                    # Utilities: shell exec, parsing, validation
│   ├── logger/                 # Logging wrapper (lumberjack)
│   └── services/
│       ├── api/                # HTTP server, REST handlers, WebSocket hub
│       ├── alerting/           # Alert engine, evaluation, dispatch
│       ├── collectors/         # Data collection goroutines
│       ├── controllers/        # Control operations (Docker, VM, Array, etc.)
│       ├── mcp/                # Model Context Protocol server for AI agents
│       ├── mqtt/               # MQTT client for Home Assistant integration
│       └── watchdog/           # Health probes, remediation, runner
├── docs/                       # Documentation (API, MCP, WebSocket, guides)
├── meta/                       # Plugin metadata (XML, page files, scripts)
├── scripts/                    # Utility scripts (deployment, testing)
├── tests/                      # Integration tests
├── .github/
│   ├── instructions/           # Path-specific AI instructions (applyTo globs)
│   └── prompts/                # Reusable task prompts for common workflows
├── Makefile                    # Build automation
├── go.mod / go.sum             # Go dependencies
├── CHANGELOG.md                # Release notes (MUST be updated with every change)
├── VERSION                     # Current version (YYYY.MM.DD format)
└── CONTRIBUTING.md             # Contribution guidelines
```

## Architecture

### Event-Driven PubSub Design

```
Collectors → Event Bus (github.com/cskr/pubsub) → API Server Cache → REST Endpoints
                                                 ↓                   ↓
                                          WebSocket Hub        MCP Server → AI Agents
                                                 ↓
                                          Connected Clients
```

### Critical Initialization Order

In `daemon/services/orchestrator.go`:

1. API server creates subscriptions via `Hub.Sub()` **FIRST**
2. 100ms delay ensures subscriptions are ready
3. Then collectors start publishing via `Hub.Pub(data, "topic_name")`

**Never change this order** — collectors publishing before subscriptions causes lost events.

### Native API Integration

Prefer native Go libraries over shell commands:

| Component | Library | Purpose |
|-----------|---------|---------|
| Docker | `github.com/moby/moby/client` | Docker Engine SDK |
| VMs | `github.com/digitalocean/go-libvirt` | Native libvirt bindings |
| System | Direct `/proc`, `/sys` access | Kernel interfaces |

### Data Flow Example

1. **System Collector** reads CPU/RAM from `/proc/meminfo`, `/proc/stat`
2. Publishes `dto.SystemInfo` to event bus topic `system_update`
3. **API Server** receives event, updates `systemCache`
4. **WebSocket Hub** receives event, broadcasts to all clients
5. **REST endpoint** `/api/v1/system` returns cached `systemCache` data

### Core Components

#### Domain Layer (`daemon/domain/`)

- `Context`: Application runtime context holding the PubSub hub and configuration
- `Config`: Configuration settings (version, port)

#### Data Transfer Objects (`daemon/dto/`)

All data structures shared between collectors, API, and WebSocket clients:

- `SystemInfo`, `ArrayStatus`, `DiskInfo`, `NetworkInfo`
- `ContainerInfo`, `VMInfo`, `UPSStatus`, `GPUMetrics`
- `ShareInfo`, `WebSocketMessage`, `HardwareInfo`, `Registration`
- `NotificationList`, `UnassignedDeviceList`, `ZFSPool`, `ZFSDataset`

#### Collectors (`daemon/services/collectors/`)

Independent goroutines that collect data at fixed intervals (defined in `daemon/constants/const.go`):

| Collector | Interval | Event Topic | Notes |
|-----------|----------|-------------|-------|
| System | 15s | `system_update` | CPU/RAM/temps — sensors is CPU intensive |
| Array | 30s | `array_status_update` | Array state rarely changes |
| Disk | 30s | `disk_list_update` | Per-disk SMART data |
| Network | 30s | `network_list_update` | Interface status |
| Docker | 30s | `container_list_update` | Very CPU intensive with many containers |
| VM | 30s | `vm_list_update` | virsh commands spawn multiple processes |
| UPS | 60s | `ups_status_update` | UPS status rarely changes |
| GPU | 60s | `gpu_metrics_update` | intel_gpu_top is extremely CPU intensive |
| Share | 60s | `share_list_update` | User share information |
| Notification | 30s | `notifications_update` | System notifications |
| Unassigned | 60s | `unassigned_devices_update` | Unassigned devices |
| ZFS | 30s | `zfs_*_update` | Pools, datasets, snapshots |
| Hardware | 300s | `hardware_update` | Rarely changes |
| Registration | 300s | `registration_update` | License info |

**Intervals optimized for power efficiency** — lower intervals increase CPU usage.

Each collector:

- Runs in its own goroutine with context cancellation support
- **Must wrap work in defer/recover** for panic recovery
- Publishes events via `ctx.Hub.Pub(data, topic)`

#### API Server (`daemon/services/api/`)

- **server.go**: In-memory cache, event subscriptions, WebSocket broadcasts. Uses `sync.RWMutex`.
- **handlers.go**: REST endpoint handlers. **Always use RLock/RUnlock** for cache reads.
- **websocket.go**: WebSocket hub with client registration and ping/pong health checks.
- **middleware.go**: CORS, logging, and recovery middleware.

#### MCP Server (`daemon/services/mcp/`)

Model Context Protocol endpoint at `POST /mcp` (Streamable HTTP, spec 2025-06-18):

- **server.go**: MCP server with 54+ tools for monitoring and control
- **transport.go**: HTTP transport for JSON-RPC requests
- Tools expose system info, Docker/VM control, notifications, etc.

#### Controllers (`daemon/services/controllers/`)

Execute control operations via `lib.ExecuteShellCommand()`:

- `docker.go`: Container start/stop/restart/pause/unpause
- `vm.go`: VM start/stop/restart/pause/resume/hibernate
- `array.go`: Array start/stop, parity check
- `notification.go`: Notification create/archive/delete
- `plugin.go`: Plugin management
- `process.go`: Process management
- `service.go`: Service management

#### Library Utilities (`daemon/lib/`)

- `shell.go`: Execute shell commands with error handling
- `parser.go`: Parse Unraid-specific file formats (.ini files)
- `validation.go`: Input validation (CWE-22 path traversal protection)
- `dmidecode.go`, `ethtool.go`, `emhttp.go`: Hardware/system info parsing

#### Orchestrator (`daemon/services/orchestrator.go`)

Coordinates the entire application lifecycle:

1. Initializes all collectors
2. Starts API server subscriptions **before** collectors (critical!)
3. Starts collectors in separate goroutines
4. Starts HTTP server
5. Manages graceful shutdown (signal handling, context cancellation)

## Build Commands

```bash
make deps           # Install dependencies
make local          # Build for current architecture
make release        # Build for Linux/amd64 (Unraid)
make package        # Create plugin .tgz package
make test           # Run all tests with race detection
make test-coverage  # Generate coverage.html
make lint           # Run golangci-lint
make security-check # Run gosec and govulncheck
make swagger        # Generate Swagger API documentation
make clean          # Remove build artifacts
make pre-commit-install  # Install pre-commit hooks
make pre-commit-run      # Run all pre-commit checks
```

### Running the Agent

```bash
./unraid-management-agent boot              # Standard mode
./unraid-management-agent boot --debug      # Debug mode (stdout logging)
./unraid-management-agent boot --port 8043  # Custom port
```

### Deployment to Unraid

**Use Ansible (preferred)** for building, deploying, and verifying the plugin on Unraid hardware. It provides idempotent deployments, selective execution via tags, and built-in endpoint verification.

```bash
# Prerequisites (one-time)
brew install ansible sshpass           # macOS
cp ansible/inventory.yml.example ansible/inventory.yml  # Add server IP + password

# Full deploy: build → deploy → verify
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml

# Build only
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build

# Deploy only (skip build)
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags deploy

# Verify endpoints only
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags verify

# Full lifecycle: uninstall → build → deploy → verify
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags redeploy
```

See `ansible/README.md` for full documentation including backup options and tag reference.

**Alternative:** The legacy shell script is also available:

```bash
cp scripts/config.sh.example scripts/config.sh  # Add SSH credentials
./scripts/deploy-plugin.sh                       # Build and deploy
```

## Code Style and Conventions

- **Standard Go**: `gofmt` and `goimports` enforced. Zero tolerance for linting errors.
- **Error handling**: Wrap errors with context: `fmt.Errorf("context: %w", err)`
- **Context propagation**: Pass `context.Context` through all goroutine chains
- **Naming**: Follow Go conventions — exported names are PascalCase, unexported are camelCase
- **Commit messages**: Follow **Conventional Commits**: `feat(scope):`, `fix(scope):`, `docs(scope):`
- **Pre-commit**: Run `make pre-commit-run` before every commit

## Security Requirements

### Input Validation

All user-provided input must be validated using functions in `daemon/lib/validation.go`:

| Function | Purpose |
|----------|---------|
| `ValidateContainerID()` | Docker container IDs (12 or 64 hex chars) |
| `ValidateVMName()` | VM names (alphanumeric, spaces, hyphens, underscores, dots) |
| `ValidateShareName()` | Share names (path traversal protection) |
| `ValidateConfigPath()` | File paths (CWE-22 protection) |
| `ValidateNotificationFilename()` | Notification filenames |
| `ValidateLogFilename()` | Log filenames (CWE-22 protection) |

- No directory traversal (`..`), absolute paths (`/`), or null bytes allowed
- Protection against CWE-22 path traversal vulnerabilities

### Command Execution

- **Always** use `lib.ExecCommand()` or `lib.ExecCommandOutput()` from `daemon/lib/shell.go`
- **Never** use `exec.Command` directly
- **Never** directly interpolate user input into shell commands
- Validate all inputs before passing to shell commands

## Core Patterns

### Panic Recovery in Collectors

**All collector loops MUST wrap work in defer/recover:**

```go
func (c *Collector) Start(ctx context.Context, interval time.Duration) {
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

### Cache Access (Thread-Safe)

```go
s.cacheMutex.RLock()   // Read lock for GET handlers
data := s.someCache
s.cacheMutex.RUnlock()
respondJSON(w, http.StatusOK, data)
```

### Controller Pattern (Validate-Execute-Return)

```go
func (c *DockerController) StartContainer(id string) error {
    if err := lib.ValidateContainerID(id); err != nil {
        return err
    }
    _, err := lib.ExecCommand(constants.DockerBin, "start", id)
    return err
}
```

### REST Handler Pattern

```go
func (s *Server) handleNew(w http.ResponseWriter, _ *http.Request) {
    s.cacheMutex.RLock()
    data := s.newCache
    s.cacheMutex.RUnlock()
    respondJSON(w, http.StatusOK, data)
}
```

Use `respondJSON()` helper for all responses. Control endpoints return `dto.Response`.

### WebSocket Events

Events broadcast via `/ws` endpoint use `dto.WSEvent`:

```go
type WSEvent struct {
    Event     string      `json:"event"`
    Timestamp time.Time   `json:"timestamp"`
    Data      interface{} `json:"data"`
}
```

## Adding New Components

### Adding a New Collector

1. Create collector in `daemon/services/collectors/` following `system.go` pattern
2. Define DTO in `daemon/dto/`
3. Register in `collector_manager.go` `RegisterAllCollectors()`
4. Add subscription topic in `api/server.go` `subscribeToEvents()` (both `Hub.Sub()` and switch case)
5. Add cache field in `Server` struct and handler in `handlers.go`
6. Register route in `setupRoutes()`

### Adding a REST Endpoint

1. Add handler method in `daemon/services/api/handlers.go`
2. Register route in `server.go` `setupRoutes()`
3. Update Swagger annotations: run `make swagger`

### Adding a Controller

1. Create file in `daemon/services/controllers/`
2. Follow validate-execute-return pattern
3. Use `lib.ExecCommand()` — never `exec.Command`
4. Add corresponding API handler and route

### Adding an MCP Tool

1. Register tool in `daemon/services/mcp/server.go`
2. Follow existing tool patterns for input validation and response format
3. Update MCP documentation in `docs/integrations/mcp.md`

### Adding a WebSocket Event

1. Define event topic constant in `daemon/constants/const.go`
2. Add collector publishing to the topic
3. Add subscription in `api/server.go` `subscribeToEvents()`
4. Add cache field and broadcast logic

## Testing Conventions

- **Table-driven tests** with security cases (path traversal, command injection)
- Tests located alongside source files (`*_test.go`)
- Include security test cases: SQL injection, command injection, path traversal
- Mock file system access and external commands
- Use `daemon/lib/testutil/` for shared test utilities

```bash
make test                                       # Run all tests
make test-coverage                              # Generate coverage.html
go test -v ./daemon/services/api/handlers_test.go  # Specific test
```

## Logging

**Location:** `/var/log/unraid-management-agent.log`
**Logger:** `daemon/logger/logger.go` (wrapper around lumberjack for rotation)

| Level | Function | Usage |
|-------|----------|-------|
| Debug | `logger.Debug()` | Detailed diagnostics (requires `--debug`) |
| Info | `logger.Info()` | General operations |
| Success | `logger.Success()` | Successful operations (green) |
| Warning | `logger.Warning()` | Warning conditions (yellow) |
| Error | `logger.Error()` | Error conditions (red) |

**Rotation:** 5 MB max, no backups, no age-based retention.

## Release Process

Uses date-based versioning: `YYYY.MM.DD` (e.g., `2025.12.01`).

1. **Update `CHANGELOG.md`** — required for every change
   - Add entry at the top under new version section
   - Follow format: `## [YYYY.MM.DD]` with date
   - Group changes: Added, Fixed, Changed, Security, Performance
2. **Update `VERSION` file** with new version number
3. **Update `.plg` files** (both root and `meta/template/`):
   - Set `<!ENTITY version "YYYY.MM.DD">`
   - Set `<!ENTITY md5 "...">` with checksum **from GitHub release** (not local build)
   - MD5 must match the GitHub release artifact
4. **Tag and push:** `git tag vYYYY.MM.DD && git push origin vYYYY.MM.DD`
5. GitHub Actions builds and releases automatically
6. Verify MD5 matches published release artifact

## Hardware Compatibility

This plugin supports diverse Unraid hardware. When fixing hardware-specific bugs:

1. Identify the failing component (disk collector, GPU collector, etc.)
2. Update command parsing in `daemon/lib/` (parser.go, dmidecode.go, ethtool.go)
3. Add fallback logic for different hardware variations
4. Document the fix in the PR with hardware details

**Common variation areas:**

- GPU metrics parsing (`nvidia-smi` output formats)
- Disk controller command outputs
- UPS monitoring tool differences (apcupsd vs NUT)
- Network interface variations
- DMI/SMBIOS data structure differences

## Unraid Integration

The agent reads from Unraid-specific locations (see `daemon/constants/const.go`):

**Configuration files:** `/var/local/emhttp/{var,disks,shares,network}.ini`

**System files:** `/proc/cpuinfo`, `/proc/meminfo`, `/proc/uptime`, `/sys/class/hwmon/`, `/proc/spl/kstat/zfs/arcstats`

**Binaries:** `/usr/local/sbin/mdcmd`, `/usr/bin/docker`, `/usr/bin/virsh`, `/usr/sbin/smartctl`, `/sbin/apcaccess`, `/usr/bin/upsc`, `/usr/bin/nvidia-smi`, `/usr/sbin/zpool`, `/usr/sbin/zfs`, `/usr/sbin/dmidecode`

## API Structure

Base URL: `http://localhost:8043/api/v1`

- `GET /system`, `/array`, `/disks`, `/docker`, `/vm`, etc. — monitoring data
- `POST /docker/{id}/{action}`, `/vm/{id}/{action}` — control operations
- `GET /ws` — WebSocket real-time events
- `POST /mcp` — MCP JSON-RPC endpoint for AI agents
- Swagger docs served at `/swagger/`

See `docs/api/` for complete endpoint documentation.

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/alecthomas/kong` | CLI framework |
| `github.com/cskr/pubsub` | Event bus (PubSub pattern) |
| `github.com/gorilla/mux` | HTTP router |
| `github.com/gorilla/websocket` | WebSocket implementation |
| `github.com/moby/moby/client` | Docker Engine SDK |
| `github.com/digitalocean/go-libvirt` | Native libvirt bindings for VMs |
| `github.com/metoro-io/mcp-golang` | Model Context Protocol server |
| `gopkg.in/ini.v1` | INI file parsing |
| `gopkg.in/natefinsh/lumberjack.v2` | Log rotation |

## Anti-Patterns (DO NOT)

- **DO NOT** use `exec.Command` directly — use `lib.ExecCommand()` / `lib.ExecCommandOutput()`
- **DO NOT** change the initialization order in `orchestrator.go` (subscriptions before collectors)
- **DO NOT** skip input validation for user-provided values
- **DO NOT** read cache without `RLock`/`RUnlock` or write without `Lock`/`Unlock`
- **DO NOT** create collectors without panic recovery wrappers
- **DO NOT** skip CHANGELOG.md updates — every change must be documented
- **DO NOT** commit secrets, credentials, or `.env` files
- **DO NOT** lower collection intervals without considering power/CPU impact
- **DO NOT** use `exec.Command` directly for shell commands

## Important Notes

- **Initialization order is critical** — API subscriptions before collectors in orchestrator.go
- **Always use mutex locks** — RLock/RUnlock for cache reads, Lock/Unlock for writes
- **Always validate user input** — use `lib.Validate*()` functions
- **Test on actual Unraid** — local development differs from production
- **Context cancellation** — respect `ctx.Done()` in goroutines for graceful shutdown
- **Keep Swagger docs updated** — run `make swagger` after modifying API endpoints

## Documentation References

- **API Reference:** `docs/api/`
- **WebSocket Events:** `docs/api/websocket-events.md`
- **MCP Integration:** `docs/integrations/mcp.md`
- **System Requirements:** `docs/guides/system-requirements.md`
- **Configuration:** `docs/guides/configuration.md`
