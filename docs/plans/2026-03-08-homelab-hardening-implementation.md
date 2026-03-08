# Homelab Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the WebSocket hub defect, eliminate the current lint and security findings, and keep REST, WebSocket, and Home Assistant behavior compatible for LAN-only homelab users.

**Architecture:** Use a targeted hardening pass. Fix the WebSocket client lifecycle so client-map mutation only happens under exclusive ownership, replace avoidable shell-outs with native Go implementations, and tighten boundary validation where it removes real risk. Keep existing API and MQTT contracts intact, and only use narrow `#nosec` suppressions where a generic abstraction is intentionally safe but cannot be proven to `gosec`. Final integration confidence must come from the repository's Ansible workflow on real Unraid hardware, not local commands alone.

**Tech Stack:** Go 1.26, gorilla/websocket, pahomqtt, standard library filesystem APIs, golangci-lint, gosec, govulncheck.

---

## Task 1: Fix the WebSocket client-eviction path

**Files:**

- Modify: `daemon/services/api/websocket_coverage_test.go`
- Modify: `daemon/services/api/websocket.go`
- Verify: `daemon/services/api/websocket_test.go`

**Step 1: Write the failing test**

Add a regression test in `daemon/services/api/websocket_coverage_test.go` that exercises a blocked client during broadcast and asserts:

- the blocked client is evicted
- a healthy client still receives the event
- the hub remains usable for subsequent broadcasts

Model the blocked client by registering a `WSClient` whose `send` channel is already full.

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./daemon/services/api -run TestWSHubBroadcastEvictsBlockedClientAndKeepsHealthyClients -count=1
```

Expected: fail because the current implementation does not provide a clean, explicit stale-client eviction path that the new test can assert against.

**Step 3: Write minimal implementation**

Refactor `WSHub.Run` in `daemon/services/api/websocket.go` so the broadcast case:

- snapshots target clients while holding `RLock`
- attempts delivery without mutating `h.clients` under `RLock`
- collects stale clients that could not accept the event
- reacquires `Lock` to remove and close only those stale clients

Keep the event format unchanged.

Sketch:

```go
case msg := <-h.broadcast:
    event := dto.WSEvent{...}

    h.mu.RLock()
    targets := make([]*WSClient, 0, len(h.clients))
    for client := range h.clients {
        if client.wantsTopic(msg.Topic) {
            targets = append(targets, client)
        }
    }
    h.mu.RUnlock()

    stale := make([]*WSClient, 0)
    for _, client := range targets {
        select {
        case client.send <- event:
        default:
            stale = append(stale, client)
        }
    }

    if len(stale) > 0 {
        h.mu.Lock()
        for _, client := range stale {
            if _, ok := h.clients[client]; ok {
                delete(h.clients, client)
                close(client.send)
            }
        }
        h.mu.Unlock()
    }
```

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./daemon/services/api -run TestWSHubBroadcastEvictsBlockedClientAndKeepsHealthyClients -count=1
go test -race ./daemon/services/api
```

Expected: both commands pass.

**Step 5: Commit**

```bash
git add daemon/services/api/websocket.go daemon/services/api/websocket_coverage_test.go
git commit -m "fix(api): harden websocket client eviction"
```

## Task 2: Tighten log-file resolution to the known allowlist

**Files:**

- Modify: `daemon/services/api/logs_test.go`
- Modify: `daemon/services/api/logs.go`
- Verify: `daemon/services/api/handlers.go`

**Step 1: Write the failing test**

Add two tests in `daemon/services/api/logs_test.go`:

- one that proves `getLogContent` rejects a real file that is not in the known log inventory
- one that proves an allowlisted path still opens successfully

Example:

```go
func TestGetLogContent_RejectsPathOutsideAllowlist(t *testing.T) {
    server, _ := setupTestServer()
    tmp := filepath.Join(t.TempDir(), "rogue.log")
    require.NoError(t, os.WriteFile(tmp, []byte("line 1\n"), 0o644))

    _, err := server.getLogContent(tmp, "", "")
    require.Error(t, err)
    require.Equal(t, "log file not allowed", err.Error())
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./daemon/services/api -run 'TestGetLogContent_(RejectsPathOutsideAllowlist|AllowsKnownPath)$' -count=1
```

Expected: fail because `getLogContent` currently trusts any existing path that does not contain `..`.

**Step 3: Write minimal implementation**

In `daemon/services/api/logs.go`:

- extract a resolver that checks whether a path matches one of the paths returned by `listLogFiles()`
- call that resolver at the top of `getLogContent`
- return a stable error such as `log file not allowed` when the path is outside the inventory

Sketch:

```go
func (s *Server) resolveAllowedLogPath(path string) (string, error) {
    for _, log := range s.listLogFiles() {
        if log.Path == path {
            return log.Path, nil
        }
    }
    return "", fmt.Errorf("log file not allowed")
}
```

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./daemon/services/api -run 'TestGetLogContent_(RejectsPathOutsideAllowlist|AllowsKnownPath)$' -count=1
```

Expected: pass.

**Step 5: Commit**

```bash
git add daemon/services/api/logs.go daemon/services/api/logs_test.go
git commit -m "fix(api): restrict log access to known files"
```

## Task 3: Replace `df` subprocess usage in the unassigned collector

**Files:**

- Modify: `daemon/services/collectors/unassigned_test.go`
- Modify: `daemon/services/collectors/unassigned.go`

**Step 1: Write the failing test**

Add a helper-focused test in `daemon/services/collectors/unassigned_test.go` that asserts the new filesystem-stat helper returns:

- non-zero total size for a writable temporary directory
- used + free values consistent with total size
- a usage percentage in the `0..100` range

Example:

```go
func TestGetFilesystemUsage(t *testing.T) {
    path := t.TempDir()

    size, used, free, pct, err := getFilesystemUsage(path)

    require.NoError(t, err)
    require.NotZero(t, size)
    require.GreaterOrEqual(t, used, uint64(0))
    require.GreaterOrEqual(t, free, uint64(0))
    require.GreaterOrEqual(t, pct, 0.0)
    require.LessOrEqual(t, pct, 100.0)
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./daemon/services/collectors -run TestGetFilesystemUsage -count=1
```

Expected: fail because `getFilesystemUsage` does not exist yet.

**Step 3: Write minimal implementation**

In `daemon/services/collectors/unassigned.go`:

- add `getFilesystemUsage(path string) (size, used, free uint64, usagePct float64, err error)`
- implement it with `unix.Statfs` or `syscall.Statfs`
- update `getPartitionSizeInfo` and `getRemoteShareSizeInfo` to use that helper
- remove the direct `exec.Command("df", ...)` calls

Sketch:

```go
var stat unix.Statfs_t
if err := unix.Statfs(path, &stat); err != nil {
    return 0, 0, 0, 0, err
}
size := stat.Blocks * uint64(stat.Bsize)
free := stat.Bavail * uint64(stat.Bsize)
used := size - (stat.Bfree * uint64(stat.Bsize))
```

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./daemon/services/collectors -run TestGetFilesystemUsage -count=1
```

Expected: pass.

**Step 5: Commit**

```bash
git add daemon/services/collectors/unassigned.go daemon/services/collectors/unassigned_test.go
git commit -m "fix(collectors): replace df shell usage with statfs"
```

## Task 4: Normalize MQTT QoS before conversion

**Files:**

- Modify: `daemon/services/mqtt/client_test.go`
- Modify: `daemon/services/mqtt/client.go`
- Modify: `daemon/services/mqtt/commands.go`

**Step 1: Write the failing test**

Add table-driven tests in `daemon/services/mqtt/client_test.go` for a new QoS normalization helper:

- `-1` becomes `0`
- `0`, `1`, and `2` remain unchanged
- `3` and larger values become `0`

Example:

```go
func TestNormalizeQoS(t *testing.T) {
    tests := []struct {
        in   int
        want byte
    }{
        {-1, 0},
        {0, 0},
        {1, 1},
        {2, 2},
        {3, 0},
    }
    for _, tt := range tests {
        if got := normalizeQoS(tt.in); got != tt.want {
            t.Fatalf("normalizeQoS(%d) = %d, want %d", tt.in, got, tt.want)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./daemon/services/mqtt -run TestNormalizeQoS -count=1
```

Expected: fail because `normalizeQoS` does not exist yet.

**Step 3: Write minimal implementation**

In `daemon/services/mqtt/client.go`:

- add `normalizeQoS(qos int) byte`
- use it in `SetWill` and `Publish`

In `daemon/services/mqtt/commands.go`:

- use the same helper in `Subscribe`

Sketch:

```go
func normalizeQoS(qos int) byte {
    switch qos {
    case 0, 1, 2:
        return byte(qos)
    default:
        return 0
    }
}
```

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./daemon/services/mqtt -run TestNormalizeQoS -count=1
```

Expected: pass.

**Step 5: Commit**

```bash
git add daemon/services/mqtt/client.go daemon/services/mqtt/commands.go daemon/services/mqtt/client_test.go
git commit -m "fix(mqtt): normalize qos values before use"
```

## Task 5: Clear the remaining collector-manager, alerting, and shell-wrapper findings

**Files:**

- Modify: `daemon/services/collector_manager_test.go`
- Modify: `daemon/services/collector_manager.go`
- Modify: `daemon/services/alerting/dispatcher.go`
- Modify: `daemon/lib/shell.go`
- Modify: `CHANGELOG.md`

**Step 1: Write the failing test**

Add a collector-manager lifecycle test in `daemon/services/collector_manager_test.go` that proves disabling a running collector clears its stored cancel function.

Example:

```go
func TestCollectorManagerDisableClearsCancel(t *testing.T) {
    ctx := createTestContext()
    var wg sync.WaitGroup
    cm := NewCollectorManager(ctx, &wg)

    cm.Register("cancel-test", func(dctx *domain.Context) Collector {
        return &mockCollector{}
    }, 1, false)

    require.NoError(t, cm.EnableCollector("cancel-test"))
    require.NoError(t, cm.DisableCollector("cancel-test"))
    require.Nil(t, cm.collectors["cancel-test"].cancel)
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./daemon/services -run TestCollectorManagerDisableClearsCancel -count=1
```

Expected: fail because the cancel lifecycle is not fully explicit yet.

**Step 3: Write minimal implementation**

- In `daemon/services/collector_manager.go`, clear stored cancel state after it is invoked on disable/stop paths.
- In `daemon/services/alerting/dispatcher.go`, replace `WriteString(fmt.Sprintf(...))` with `fmt.Fprintf`.
- In `daemon/lib/shell.go`, add narrow `#nosec G204` annotations and a short safety-contract comment explaining that callers must validate command paths and arguments before invoking the shared wrapper.
- Update `CHANGELOG.md` with a concise entry under the current work-in-progress version covering the WebSocket, collector, MQTT, and tooling hardening.

**Step 4: Run tests and linters**

Run:

```bash
go test ./daemon/services -run TestCollectorManagerDisableClearsCancel -count=1
golangci-lint run --config .golangci.yml --max-issues-per-linter 0 --max-same-issues 0 ./...
gosec ./...
govulncheck ./...
```

Expected: all commands pass.

**Step 5: Commit**

```bash
git add daemon/services/collector_manager.go daemon/services/collector_manager_test.go daemon/services/alerting/dispatcher.go daemon/lib/shell.go CHANGELOG.md
git commit -m "fix: clear remaining lint and security findings"
```

## Task 6: Final verification

**Files:**

- Verify only

**Step 1: Confirm Ansible prerequisites**

Run:

```bash
test -f ansible/inventory.yml
ansible-playbook --version
```

Expected: the inventory file exists and Ansible is installed.

**Step 2: Run the full local test suite**

Run:

```bash
go test ./...
```

Expected: pass.

**Step 3: Run focused race coverage**

Run:

```bash
go test -race ./daemon/services/api
go test -race ./daemon/services
```

Expected: both pass.

**Step 4: Run the final local quality gates**

Run:

```bash
golangci-lint run --config .golangci.yml --max-issues-per-linter 0 --max-same-issues 0 ./...
gosec ./...
govulncheck ./...
```

Expected: all pass with zero findings.

**Step 5: Build via Ansible**

Run:

```bash
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build
```

Expected: Ansible runs the `build` role, executes `make package`, and verifies the plugin bundle exists.

**Step 6: Deploy via Ansible**

Run:

```bash
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags deploy
```

Expected: Ansible uploads the package, installs it on Unraid, starts the service, and waits for the health endpoint to return `200`.

**Step 7: Validate endpoints via Ansible**

Run:

```bash
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags verify
```

Expected: Ansible executes the verify role and passes the endpoint, schema, MCP, Prometheus, Swagger, WebSocket, MQTT, alerting, and health check validations.

**Step 8: Run the full Ansible lifecycle once**

Run:

```bash
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml
```

Expected: the combined build, deploy, and verify workflow completes successfully end to end.

**Step 9: Inspect the tree**

Run:

```bash
git status --short
git log --oneline -n 5
```

Expected: only the intended files are modified or committed.

**Step 10: Commit any remaining docs-only cleanup**

```bash
git add CHANGELOG.md
git commit -m "docs: finalize homelab hardening notes"
```
