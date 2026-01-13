# Copilot Instructions

## Project Overview

Go-based Unraid plugin exposing system monitoring/control via REST API and WebSockets. **Language:** Go 1.24, **Target:** Linux/amd64 (Unraid OS). This is a **third-party community plugin** providing REST/WebSocket interface as alternative to official Unraid GraphQL API.

## Important Notes

- **Context7**: Always use Context7 MCP tools to resolve library IDs and get docs for code generation, setup, or API documentation—without explicit prompting.
- **PubSub Library**: Uses `github.com/cskr/pubsub` v1.0.2 for event bus (see `daemon/domain/context.go`).

## Architecture: Event-Driven PubSub

```
Collectors (goroutines) → Event Bus (github.com/cskr/pubsub) → API Server Cache → REST/WebSocket
                                      ↓
                              WebSocket Hub → Connected Clients
```

**Critical initialization order in `orchestrator.go`:**

1. API server creates subscriptions via `Hub.Sub()` FIRST (see `server.go` line ~234)
2. 100ms delay ensures subscriptions are ready (prevents lost events)
3. Then collectors start publishing via `Hub.Pub(data, "topic_name")`
4. **Never change this order**—collectors publishing before subscriptions causes race conditions.

## Key Patterns

### Collector Intervals

Defined in `daemon/constants/const.go`:

- System: 5s, Array: 10s, Disk: 30s, Docker: 10s, VM: 10s
- UPS: 10s, GPU: 10s, Network: 15s, Shares: 60s
- Adjust based on data volatility and performance impact

### Adding a New Collector

1. Create collector in `daemon/services/collectors/` following `system.go` pattern:

   ```go
   type NewCollector struct { ctx *domain.Context }
   func (c *NewCollector) Start(ctx context.Context, interval time.Duration) {
       // Run once immediately with panic recovery (wrap in defer/recover)
       // Use ticker loop, select on ctx.Done() and ticker.C
       // Call c.ctx.Hub.Pub(data, "new_topic_update")
   }
   ```

2. Define DTO in `daemon/dto/` (e.g., `NewInfo struct`)
3. Add subscription in `api/server.go` `subscribeToEvents()`:
   - Add `"new_topic_update"` to `Hub.Sub()` call (~line 234)
   - Add `case` in switch statement to update cache (~line 265+)
4. Add cache field in `api/server.go` `Server` struct (e.g., `newCache *dto.NewInfo`)
5. Add handler in `api/handlers.go`:

   ```go
   func (s *Server) handleNew(w http.ResponseWriter, _ *http.Request) {
       s.cacheMutex.RLock()
       data := s.newCache
       s.cacheMutex.RUnlock()
       respondJSON(w, http.StatusOK, data)
   }
   ```

6. Register route in `server.go` `setupRoutes()` (~line 69+)
7. Initialize and start in `orchestrator.go` (~line 62+): add to WaitGroup, create collector, launch goroutine

### Adding REST Endpoints

**Always use RLock/RUnlock** for cache reads, **Lock/Unlock** for writes:

```go
// Read-only endpoint
func (s *Server) handleNewEndpoint(w http.ResponseWriter, _ *http.Request) {
    s.cacheMutex.RLock()
    data := s.newCache
    s.cacheMutex.RUnlock()
    respondJSON(w, http.StatusOK, data)
}
```

### Control Operations

Use `lib.ExecCommand()` for all shell commands (never use `exec.Command` directly):

```go
// From daemon/services/controllers/docker.go
output, err := lib.ExecCommand(constants.DockerBin, "start", containerID)
if err != nil {
    return fmt.Errorf("failed to start container: %w", err)
}
```

**Always validate user input before passing to commands** (see Security Requirements).

### Error Handling & Responses

Use `respondJSON()` helper for all HTTP responses (located in `handlers.go`):

```go
// Success response with data
respondJSON(w, http.StatusOK, data)

// Error response with message
respondJSON(w, http.StatusBadRequest, dto.Response{
    Success:   false,
    Message:   "error description",
    Timestamp: time.Now(),
})
```

**Control endpoint pattern** (see `handlers.go` Docker/VM handlers):

1. Validate input (container ID, VM name, etc.)
2. Log the operation with `logger.Info()`
3. Call controller method
4. Return `dto.Response` with success/failure

### Panic Recovery

**All collector loops MUST wrap work in defer/recover** to prevent crashes:

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

## Deployment to Unraid Server

**IMPORTANT**: Always use the provided deployment scripts for building and testing:

1. **Prepare credentials**: Create `scripts/config.sh` with Unraid server SSH credentials

   ```bash
   cp scripts/config.sh.example scripts/config.sh
   # Edit config.sh with actual Unraid server IP, username, and password
   ```

2. **Deploy and test**: Use `scripts/deploy-plugin.sh`

   ```bash
   ./scripts/deploy-plugin.sh
   ```

   This script:

   - Builds the plugin using `make release`
   - Uses `sshpass` to SSH into Unraid server without interactive password prompt
   - Uploads the plugin package
   - Tests deployment on the actual Unraid system

3. **Verify deployment**: After deployment, test the plugin on the Unraid server:
   - Check plugin UI loads at `https://<unraid-ip>:8043`
   - Verify WebSocket connections work
   - Test REST API endpoints
   - Monitor logs at `/var/log/unraid-management-agent.log`

**Do NOT** use manual SSH commands or skip automated deployment scripts. Always verify changes work on actual Unraid hardware before merging PRs.

## Project Structure

| Directory                      | Purpose                                    |
| ------------------------------ | ------------------------------------------ |
| `daemon/constants/`            | System paths, binary locations, intervals  |
| `daemon/dto/`                  | Data transfer objects (shared structs)     |
| `daemon/lib/`                  | Utilities: shell exec, parsing, validation |
| `daemon/services/collectors/`  | Data collection goroutines                 |
| `daemon/services/controllers/` | Control operations (Docker, VM, Array)     |
| `daemon/services/api/`         | HTTP server, handlers, WebSocket hub       |

## Unraid-Specific Paths

Collectors read from Unraid files defined in `daemon/constants/const.go`:

- `/var/local/emhttp/*.ini` - Configuration files
- `/proc/*` and `/sys/class/hwmon/` - System metrics
- Binaries: `/usr/bin/docker`, `/usr/bin/virsh`, `/usr/sbin/smartctl`, etc.

## Testing

Use **table-driven tests** for comprehensive coverage (see `daemon/lib/validation_test.go`):

```go
func TestValidateContainerID(t *testing.T) {
    tests := []struct {
        name    string
        id      string
        wantErr bool
        errMsg  string
    }{
        {name: "valid short ID", id: "bbb57ffa3c50", wantErr: false},
        {name: "empty ID", id: "", wantErr: true, errMsg: "cannot be empty"},
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateContainerID(tt.id)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
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

This runs on varied hardware. When fixing hardware-specific issues:

1. Identify the failing collector
2. Update parsing in `daemon/lib/` (parser.go, dmidecode.go, ethtool.go)
3. Add fallback logic for variations
4. Document hardware details in PR
