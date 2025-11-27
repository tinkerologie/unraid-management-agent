# Copilot Instructions

## Project Overview

Go-based Unraid plugin exposing system monitoring/control via REST API and WebSockets. **Language:** Go 1.24, **Target:** Linux/amd64 (Unraid OS).

## Important Notes

- **Context7**: Always use Context7 MCP tools to resolve library IDs and get docs for code generation, setup, or API documentation—without explicit prompting.

## Architecture: Event-Driven PubSub

```
Collectors (goroutines) → Event Bus (PubSub) → API Server Cache → REST/WebSocket
```

**Critical initialization order in `orchestrator.go`:**
1. API server subscriptions start FIRST
2. 100ms delay ensures subscriptions are ready
3. Then collectors start publishing

This prevents race conditions—never change this order.

## Key Patterns

### Adding a New Collector
1. Create collector in `daemon/services/collectors/` following `system.go` pattern
2. Define DTO in `daemon/dto/`
3. Implement `Start(ctx context.Context, interval time.Duration)` with panic recovery
4. Publish: `c.ctx.Hub.Pub(data, "topic_name")`
5. Add subscription in `api/server.go` `subscribeToEvents()`
6. Add cache field + handler in `api/handlers.go`
7. Register in `orchestrator.go`

### Adding REST Endpoints
```go
// In handlers.go - always use mutex for cache access
func (s *Server) handleNewEndpoint(w http.ResponseWriter, _ *http.Request) {
    s.cacheMutex.RLock()
    data := s.newCache
    s.cacheMutex.RUnlock()
    respondJSON(w, http.StatusOK, data)
}
```

### Control Operations
Use `lib.ExecCommand()` for shell commands (see `daemon/services/controllers/docker.go`):
```go
_, err := lib.ExecCommand(constants.DockerBin, "start", containerID)
```

## Security Requirements

**Always validate user input** using functions from `daemon/lib/validation.go`:
- `ValidateContainerID()` - Docker container IDs
- `ValidateVMName()` - VM names
- `ValidateConfigPath()` - File paths (CWE-22 protection)
- `ValidateNotificationFilename()` - Notification files

**Never** interpolate user input directly into shell commands.

## Commands

```bash
make deps          # Install dependencies
make local         # Build for current architecture
make release       # Build for Linux/amd64 (Unraid)
make test          # Run all tests
make test-coverage # Coverage report → coverage.html
make package       # Create plugin .tgz

# Run agent
./unraid-management-agent boot --debug --port 8043
```

## Project Structure

| Directory | Purpose |
|-----------|---------|
| `daemon/constants/` | System paths, binary locations, intervals |
| `daemon/dto/` | Data transfer objects (shared structs) |
| `daemon/lib/` | Utilities: shell exec, parsing, validation |
| `daemon/services/collectors/` | Data collection goroutines |
| `daemon/services/controllers/` | Control operations (Docker, VM, Array) |
| `daemon/services/api/` | HTTP server, handlers, WebSocket hub |

## Unraid-Specific Paths

Collectors read from Unraid files defined in `daemon/constants/const.go`:
- `/var/local/emhttp/*.ini` - Configuration files
- `/proc/*` and `/sys/class/hwmon/` - System metrics
- Binaries: `/usr/bin/docker`, `/usr/bin/virsh`, `/usr/sbin/smartctl`, etc.

## Testing

- Use table-driven tests (see `daemon/lib/validation_test.go`)
- Mock external dependencies
- Tests located alongside source files (`*_test.go`)

## WebSocket Events

Events broadcast via `/ws` endpoint use `dto.WSEvent` structure:
```go
type WSEvent struct {
    Event     string      `json:"event"`     // e.g., "update"
    Timestamp time.Time   `json:"timestamp"`
    Data      interface{} `json:"data"`      // Collector-specific DTO
}
```

**Event topics** (from collectors → WebSocket clients):
- `system_update`, `array_status_update`, `disk_list_update`
- `container_list_update`, `vm_list_update`, `network_list_update`
- `ups_status_update`, `gpu_metrics_update`, `share_list_update`
- `hardware_update`, `registration_update`, `notifications_update`
- `unassigned_devices_update`, `zfs_pools_update`, `zfs_datasets_update`

## Logging

Use `daemon/logger/logger.go` with these levels:
- `logger.Debug()` - Detailed diagnostics (requires `--debug` flag)
- `logger.Info()` - General operations
- `logger.Success()` - Successful operations (green output)
- `logger.Warning()` - Warning conditions (yellow output)
- `logger.Error()` - Error conditions (red output)

Log file: `/var/log/unraid-management-agent.log` (5MB max, auto-rotated)

## Versioning & Releases

Uses date-based semantic versioning: `YYYY.MM.DD` (e.g., `2025.11.25`)

**Release process:**
1. Update `VERSION` file with new version
2. Update `CHANGELOG.md` with release notes
3. Create and push tag: `git tag v2025.11.25 && git push origin v2025.11.25`
4. GitHub Actions builds and releases automatically

## Hardware Compatibility

This runs on varied hardware. When fixing hardware-specific issues:
1. Identify the failing collector
2. Update parsing in `daemon/lib/` (parser.go, dmidecode.go, ethtool.go)
3. Add fallback logic for variations
4. Document hardware details in PR
