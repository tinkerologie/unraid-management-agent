# Copilot Instructions

Go-based Unraid plugin exposing system monitoring/control via REST API, WebSockets, and MCP. **Language:** Go 1.25, **Target:** Linux/amd64 (Unraid OS). This is a **third-party community plugin** providing a REST/WebSocket interface as an alternative to the official Unraid GraphQL API.

**Follow Go best practices**: idiomatic Go style, proper error handling with wrapped errors (`fmt.Errorf("context: %w", err)`), context propagation, and effective use of interfaces. Code must pass `golangci-lint` and `go vet`.

**Run pre-commit before committing**: `make pre-commit-run` to verify linting, security checks, and formatting pass.

**Keep Swagger docs updated**: Run `make swagger` after modifying API endpoints. Docs are in [daemon/docs/](../daemon/docs/) and served at `/swagger/`.

## Architecture: Event-Driven PubSub

```
Collectors → Event Bus (github.com/cskr/pubsub) → API Server Cache → REST/WebSocket/MCP
                                                        ↓
                                                 WebSocket Hub → Clients
```

**Critical initialization order** (in [orchestrator.go](../daemon/services/orchestrator.go)):

1. API server creates subscriptions via `Hub.Sub()` **FIRST**
2. 100ms delay ensures subscriptions are ready
3. Then collectors start publishing via `Hub.Pub(data, "topic_name")`

⚠️ **Never change this order** — collectors publishing before subscriptions causes lost events.

## Commands

```bash
make deps           # Install dependencies
make local          # Build for current architecture
make release        # Build for Linux/amd64 (Unraid)
make test           # Run all tests with race detection
make test-coverage  # Generate coverage.html
make package        # Create plugin .tgz

./unraid-management-agent boot --debug --port 8043  # Run agent
```

## Adding a New Collector

1. Create collector in `daemon/services/collectors/` following [system.go](../daemon/services/collectors/system.go) pattern
2. Define DTO in `daemon/dto/`
3. Register in [collector_manager.go](../daemon/services/collector_manager.go) `RegisterAllCollectors()`
4. Add subscription topic in [server.go](../daemon/services/api/server.go) `subscribeToEvents()` (both `Hub.Sub()` and switch case)
5. Add cache field and handler in [handlers.go](../daemon/services/api/handlers.go)
6. Register route in `setupRoutes()`

**Collector pattern** (panic recovery is required):

```go
func (c *Collector) Start(ctx context.Context, interval time.Duration) {
    // Run once immediately with recovery
    func() {
        defer func() { if r := recover(); r != nil { logger.Error("PANIC: %v", r) } }()
        c.Collect()
    }()
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            func() {
                defer func() { if r := recover(); r != nil { logger.Error("PANIC: %v", r) } }()
                c.Collect()
            }()
        }
    }
}
```

## Security Requirements

**Always validate user input** using [validation.go](../daemon/lib/validation.go):

- `ValidateContainerID()` — Docker container IDs (12 or 64 hex chars)
- `ValidateVMName()` — VM names (alphanumeric, spaces, hyphens, underscores, dots)
- `ValidateShareName()` — Share names (path traversal protection)
- `ValidateLogFilename()` — Log filenames (CWE-22 protection)

**Never** use `exec.Command` directly — use `lib.ExecCommand()` or `lib.ExecCommandOutput()` from [shell.go](../daemon/lib/shell.go).

## Key Patterns

### Cache Access (thread-safe)

```go
s.cacheMutex.RLock()  // Read lock for GET handlers
data := s.someCache
s.cacheMutex.RUnlock()
respondJSON(w, http.StatusOK, data)
```

### HTTP Responses

Use `respondJSON()` helper for all responses. Control endpoints return `dto.Response`.

### Controller Pattern (Docker/VM/Array operations)

Controllers in `daemon/services/controllers/` execute system operations:

```go
// Validate → Execute → Return response
func (c *DockerController) StartContainer(id string) error {
    if err := lib.ValidateContainerID(id); err != nil {
        return err
    }
    _, err := lib.ExecCommand(constants.DockerBin, "start", id)
    return err
}
```

### MCP Integration

The agent exposes 54+ tools via Model Context Protocol at `POST /mcp` and `/mcp/sse` for AI agents. See [MCP_INTEGRATION.md](../docs/MCP_INTEGRATION.md) and [mcp/server.go](../daemon/services/mcp/server.go).

### Native APIs (preferred over shell commands)

- Docker: `github.com/moby/moby/client` — Docker Engine SDK
- VMs: `github.com/digitalocean/go-libvirt` — Native libvirt bindings
- System: Direct `/proc`, `/sys` access

## Project Structure

| Directory                      | Purpose                                              |
| ------------------------------ | ---------------------------------------------------- |
| `daemon/constants/`            | System paths, binary locations, collection intervals |
| `daemon/dto/`                  | Data transfer objects (shared structs)               |
| `daemon/lib/`                  | Utilities: shell exec, parsing, validation           |
| `daemon/services/collectors/`  | Data collection goroutines                           |
| `daemon/services/controllers/` | Control operations (Docker, VM, Array)               |
| `daemon/services/api/`         | HTTP server, handlers, WebSocket hub                 |
| `daemon/services/mcp/`         | Model Context Protocol for AI agents                 |

## Testing

Use **table-driven tests** with security cases. See [validation_test.go](../daemon/lib/validation_test.go) for pattern.
Tests are located alongside source files (`*_test.go`).

## Release Process

Uses date-based versioning: `YYYY.MM.DD`

1. **Update `CHANGELOG.md`** — required for every change
2. Update `VERSION` file
3. Update `.plg` files (root + `meta/template/`) — version and MD5 from GitHub release
4. Tag and push: `git tag vYYYY.MM.DD && git push origin vYYYY.MM.DD`

## Deployment

Use `scripts/deploy-plugin.sh` for testing on actual Unraid hardware:

```bash
cp scripts/config.sh.example scripts/config.sh  # Add SSH credentials
./scripts/deploy-plugin.sh                       # Build and deploy
```

**Test guidelines:**

- Include security test cases (SQL injection, command injection, path traversal)
- Mock file system access and external commands
- Tests located alongside source files (`*_test.go`)
- Use `daemon/lib/testutil/` for shared test utilities

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

1. **Update `CHANGELOG.md`** - ALWAYS keep this file up to date with every change:
   - Add entry at the top under new version section
   - Follow existing format: `## [YYYY.MM.DD]` with date
   - Include all bug fixes, features, and improvements
   - Link to relevant issues/PRs: `(#123)`
   - Group changes: Features, Bug Fixes, Security, Performance, etc.
   - **Example entry**:

     ```markdown
     ## [2025.12.22]

     ### Added

     - New ZFS pool monitoring feature (#85)
     - WebSocket reconnection logic

     ### Fixed

     - Memory leak in Docker collector (#92)
     - CPU usage calculation on ARM systems (#88)

     ### Changed

     - Increased default collection intervals for performance
     ```

   - **DO NOT** skip or delay CHANGELOG updates—these must be kept current at all times

2. Update `VERSION` file with new version

3. **Update `.plg` files** (both root and `meta/template/` directory):
   - Update `<!ENTITY version "2025.11.26">` with new version
   - Update `<!ENTITY md5 "...">` with checksum **from GitHub release** (not local build)
   - **CRITICAL**: MD5 must match the GitHub release artifact or users cannot download updates
   - Get MD5 from GitHub release page or via: `curl -sL <release-url> | md5sum`

4. Create and push tag: `git tag v2025.11.25 && git push origin v2025.11.25`

5. GitHub Actions builds and releases automatically

6. Verify MD5 checksum matches the published release artifact

## Hardware Compatibility

When fixing hardware-specific issues:

1. Identify the failing collector
2. Update parsing in `daemon/lib/` (parser.go, dmidecode.go, ethtool.go)
3. Add fallback logic for hardware variations
4. Document hardware details in PR
