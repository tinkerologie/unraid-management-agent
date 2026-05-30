# Continuous Container Update Detection — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface Docker container update-available status continuously on every container (REST, MCP, WebSocket), with alerting and an opt-in notification, via a dedicated rate-limit-safe collector.

**Architecture:** A new `DockerUpdate` collector runs on an independent long interval (default 6h, staggered start), calls the existing `DockerController.CheckAllContainerUpdates()` (digest comparison via `DistributionInspect`), and publishes `*dto.ContainerUpdatesResult` on a new typed topic. The API caches it and `CacheStore.GetDockerCache()` merges the cached update status into every `ContainerInfo` at read time — so REST handlers, MCP tools, and the alert engine all see update status with one change point. The raw container cache stays pure. No database; state is in-memory and re-checked after restart.

**Tech Stack:** Go 1.26, `github.com/moby/moby/client` (Docker SDK), typed event bus (`daemon/domain`), `expr-lang` (alerting), `github.com/alecthomas/kong` (CLI).

**Design spec:** `docs/superpowers/specs/2026-05-30-continuous-container-updates-design.md`

---

## Task 1: Data model — update fields, status helper, and event topic

**Goal:** Add update status fields to `ContainerInfo`, a status-derivation helper, status constants, and the new typed event topic — the shared vocabulary every later task depends on.

**Files:**

- Modify: `daemon/dto/docker.go`
- Modify: `daemon/constants/topics.go`
- Test: `daemon/dto/docker_test.go` (create if absent)

**Acceptance Criteria:**

- [ ] `ContainerInfo` has `UpdateStatus string`, `UpdateAvailable *bool`, `UpdateChecked *time.Time` (last two `omitempty`).
- [ ] Status constants `UpdateStatusUpToDate`, `UpdateStatusAvailable`, `UpdateStatusUnknown` exist.
- [ ] `ContainerUpdateInfo.Status()` returns the correct tri-state string.
- [ ] `constants.TopicDockerUpdatesUpdate` is a `Topic[*dto.ContainerUpdatesResult]` named `"docker_updates_update"`.
- [ ] A container with `UpdateAvailable == nil` marshals without an `update_available` key and with `update_status` present.

**Verify:** `go test ./daemon/dto/... -run TestContainerUpdate -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** in `daemon/dto/docker_test.go`

```go
package dto

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestContainerUpdateInfo_Status(t *testing.T) {
 avail := true
 notAvail := false
 tests := []struct {
  name string
  info ContainerUpdateInfo
  want string
 }{
  {"available", ContainerUpdateInfo{LatestDigest: "sha256:b", CurrentDigest: "sha256:a", checked: &avail}, UpdateStatusAvailable},
  {"up to date", ContainerUpdateInfo{LatestDigest: "sha256:a", CurrentDigest: "sha256:a", checked: &notAvail}, UpdateStatusUpToDate},
  {"unknown when no latest digest", ContainerUpdateInfo{CurrentDigest: "sha256:a"}, UpdateStatusUnknown},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   if got := tt.info.Status(); got != tt.want {
    t.Errorf("Status() = %q, want %q", got, tt.want)
   }
  })
 }
}

func TestContainerInfo_UpdateMarshalOmitsNilBool(t *testing.T) {
 c := ContainerInfo{Name: "plex", UpdateStatus: UpdateStatusUnknown}
 b, err := json.Marshal(c)
 if err != nil {
  t.Fatalf("marshal: %v", err)
 }
 s := string(b)
 if strings.Contains(s, `"update_available"`) {
  t.Errorf("expected update_available omitted when nil, got %s", s)
 }
 if !strings.Contains(s, `"update_status":"unknown"`) {
  t.Errorf("expected update_status=unknown, got %s", s)
 }
}
```

> Note: the test references an unexported `checked` field only to keep the example self-contained. Replace with the real derivation in Step 3 — `Status()` derives purely from digests, so delete the `checked:` literals and the field. Final test (use this version):

```go
func TestContainerUpdateInfo_Status(t *testing.T) {
 tests := []struct {
  name string
  info ContainerUpdateInfo
  want string
 }{
  {"available", ContainerUpdateInfo{CurrentDigest: "sha256:a", LatestDigest: "sha256:b", UpdateAvailable: true}, UpdateStatusAvailable},
  {"up to date", ContainerUpdateInfo{CurrentDigest: "sha256:a", LatestDigest: "sha256:a"}, UpdateStatusUpToDate},
  {"unknown when no latest digest", ContainerUpdateInfo{CurrentDigest: "sha256:a"}, UpdateStatusUnknown},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   if got := tt.info.Status(); got != tt.want {
    t.Errorf("Status() = %q, want %q", got, tt.want)
   }
  })
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/dto/... -run TestContainerUpdate -v`
Expected: FAIL — `undefined: UpdateStatusAvailable`, `info.Status undefined`.

- [ ] **Step 3: Add fields, constants, and helper** in `daemon/dto/docker.go`

Add to the `ContainerInfo` struct (after `Uptime`, before `Timestamp`):

```go
 // Update status — populated by merging the DockerUpdate collector's cache at read time.
 UpdateStatus    string     `json:"update_status" example:"up_to_date"` // up_to_date | update_available | unknown
 UpdateAvailable *bool      `json:"update_available,omitempty"`          // nil = not yet checked / registry unreachable
 UpdateChecked   *time.Time `json:"update_checked,omitempty"`
```

Add near the top of the file (after the imports):

```go
// Container update status values for ContainerInfo.UpdateStatus.
const (
 UpdateStatusUpToDate  = "up_to_date"
 UpdateStatusAvailable = "update_available"
 UpdateStatusUnknown   = "unknown"
)

// Status derives the tri-state update status from the digests.
// Returns "unknown" when the latest (remote) digest could not be determined,
// so callers never report "up to date" when the check actually failed.
func (u ContainerUpdateInfo) Status() string {
 if u.LatestDigest == "" {
  return UpdateStatusUnknown
 }
 if u.UpdateAvailable {
  return UpdateStatusAvailable
 }
 return UpdateStatusUpToDate
}
```

Add to `daemon/constants/topics.go` inside the `var (...)` block:

```go
 // TopicDockerUpdatesUpdate is published by the docker_update collector with *dto.ContainerUpdatesResult.
 TopicDockerUpdatesUpdate = domain.NewTopic[*dto.ContainerUpdatesResult]("docker_updates_update")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/dto/... -run TestContainerUpdate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/dto/docker.go daemon/dto/docker_test.go daemon/constants/topics.go
git commit -m "feat(docker): add container update status fields and event topic"
```

---

## Task 2: DockerUpdate collector

**Goal:** A new collector that periodically checks all containers for updates, publishes the result on `TopicDockerUpdatesUpdate`, and only re-publishes on a scheduled tick when the result changed (dedupe).

**Files:**

- Create: `daemon/services/collectors/docker_update.go`
- Test: `daemon/services/collectors/docker_update_test.go`

**Acceptance Criteria:**

- [ ] `DockerUpdateCollector` implements `Start(ctx, interval)` with panic recovery and `ctx.Done()` handling.
- [ ] The registry check is injectable via a `checkFn` field so tests run without Docker.
- [ ] First `Collect()` always publishes; subsequent `Collect()` calls publish only when the update signature changed.
- [ ] A startup stagger delay is applied before the first scheduled check (skippable when interval is tiny, for tests).

**Verify:** `go test ./daemon/services/collectors/... -run TestDockerUpdateCollector -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** in `daemon/services/collectors/docker_update_test.go`

```go
package collectors

import (
 "testing"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestDockerUpdateCollector_PublishesAndDedupes(t *testing.T) {
 hub := domain.NewEventBus(16)
 sub := hub.Sub(constants.TopicDockerUpdatesUpdate.Name)
 defer hub.Unsub(sub)

 avail := true
 result := &dto.ContainerUpdatesResult{
  Containers: []dto.ContainerUpdateInfo{
   {ContainerID: "abc123", ContainerName: "plex", LatestDigest: "sha256:b", CurrentDigest: "sha256:a", UpdateAvailable: avail},
  },
  TotalCount: 1, UpdatesAvailable: 1,
 }

 c := NewDockerUpdateCollector(&domain.Context{Hub: hub})
 c.checkFn = func() (*dto.ContainerUpdatesResult, error) { return result, nil }

 // First collect publishes.
 c.Collect()
 select {
 case msg := <-sub:
  got, ok := msg.(*dto.ContainerUpdatesResult)
  if !ok || got.UpdatesAvailable != 1 {
   t.Fatalf("unexpected first publish: %#v", msg)
  }
 case <-time.After(time.Second):
  t.Fatal("expected first publish, got none")
 }

 // Second collect with identical signature must NOT publish.
 c.Collect()
 select {
 case msg := <-sub:
  t.Fatalf("expected no re-publish on unchanged result, got %#v", msg)
 case <-time.After(200 * time.Millisecond):
  // success
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/collectors/... -run TestDockerUpdateCollector -v`
Expected: FAIL — `undefined: NewDockerUpdateCollector`.

- [ ] **Step 3: Implement the collector** in `daemon/services/collectors/docker_update.go`

```go
package collectors

import (
 "context"
 "fmt"
 "sort"
 "strings"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

// dockerUpdateStartupStagger delays the first scheduled check so update
// detection does not pile onto boot alongside every other collector.
const dockerUpdateStartupStagger = 45 * time.Second

// DockerUpdateCollector periodically checks all containers for available image
// updates (registry digest comparison) and publishes the result. It runs on a
// long interval because DistributionInspect hits the registry and Docker Hub
// rate-limits anonymous manifest requests.
type DockerUpdateCollector struct {
 appCtx  *domain.Context
 checkFn func() (*dto.ContainerUpdatesResult, error)
 lastSig string
}

// NewDockerUpdateCollector creates a new DockerUpdate collector.
func NewDockerUpdateCollector(ctx *domain.Context) *DockerUpdateCollector {
 return &DockerUpdateCollector{
  appCtx: ctx,
  checkFn: func() (*dto.ContainerUpdatesResult, error) {
   return controllers.NewDockerController().CheckAllContainerUpdates()
  },
 }
}

// Start begins the periodic update check after a startup stagger.
func (c *DockerUpdateCollector) Start(ctx context.Context, interval time.Duration) {
 logger.Info("Starting docker_update collector (interval: %v)", interval)

 // Startup stagger — skip the wait if cancelled.
 select {
 case <-ctx.Done():
  return
 case <-time.After(dockerUpdateStartupStagger):
 }

 func() {
  defer func() {
   if r := recover(); r != nil {
    logger.LogPanicWithStack("DockerUpdate collector", r)
   }
  }()
  c.Collect()
 }()

 ticker := time.NewTicker(interval)
 defer ticker.Stop()

 for {
  select {
  case <-ctx.Done():
   logger.Info("DockerUpdate collector stopping due to context cancellation")
   return
  case <-ticker.C:
   func() {
    defer func() {
     if r := recover(); r != nil {
      logger.LogPanicWithStack("DockerUpdate collector", r)
     }
    }()
    c.Collect()
   }()
  }
 }
}

// Collect runs an update check and publishes the result only if it changed
// since the last publish (dedupe to avoid no-op WebSocket broadcasts).
func (c *DockerUpdateCollector) Collect() {
 result, err := c.checkFn()
 if err != nil {
  logger.Warning("DockerUpdate: check failed: %v", err)
  return
 }
 if result == nil {
  return
 }

 sig := updateSignature(result)
 if sig == c.lastSig {
  logger.Debug("DockerUpdate: no change (%d updates available), skipping publish", result.UpdatesAvailable)
  return
 }
 c.lastSig = sig

 domain.Publish(c.appCtx.Hub, constants.TopicDockerUpdatesUpdate, result)
 logger.Info("DockerUpdate: published (%d/%d containers have updates)", result.UpdatesAvailable, result.TotalCount)
}

// updateSignature builds an order-independent fingerprint of update status
// (container ID + available flag), ignoring the timestamp.
func updateSignature(r *dto.ContainerUpdatesResult) string {
 parts := make([]string, 0, len(r.Containers))
 for _, c := range r.Containers {
  parts = append(parts, fmt.Sprintf("%s=%t", c.ContainerID, c.UpdateAvailable))
 }
 sort.Strings(parts)
 return strings.Join(parts, ",")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/services/collectors/... -run TestDockerUpdateCollector -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/services/collectors/docker_update.go daemon/services/collectors/docker_update_test.go
git commit -m "feat(docker): add DockerUpdate collector with change-detection"
```

---

## Task 3: Interval plumbing + collector registration

**Goal:** Wire the new collector into the CLI/config/interval system and register it with the collector manager (default 6h), and relax the runtime interval cap to the documented 86400s so 6h is settable at runtime.

**Files:**

- Modify: `daemon/constants/const.go`
- Modify: `daemon/domain/context.go`
- Modify: `daemon/domain/fileconfig.go`
- Modify: `main.go`
- Modify: `daemon/services/collector_manager.go`
- Test: `daemon/services/collector_manager_coverage_test.go` (add a case) or a new focused test

**Acceptance Criteria:**

- [ ] `constants.IntervalDockerUpdate == 21600`.
- [ ] `domain.Intervals` has a `DockerUpdate int` field; `FileConfigIntervals` has `DockerUpdate *int`.
- [ ] `main.go` exposes `--interval-docker-update` (env `INTERVAL_DOCKER_UPDATE`, default 21600), maps it through `getInterval("docker_update", ...)`, applies file config, and adds `docker_update` to `validCollectorNames`.
- [ ] The manager registers `docker_update` and `getDefaultInterval("docker_update") == 21600`.
- [ ] `UpdateInterval` accepts values up to 86400 (was 3600).

**Verify:** `go build ./... && go test ./daemon/services/ -run TestCollectorManager -v` → PASS

**Steps:**

- [ ] **Step 1: Add the constant** in `daemon/constants/const.go` (after `IntervalTuning = 120`)

```go
 // IntervalDockerUpdate is the interval for checking container image updates in seconds.
 // Long by design: DistributionInspect hits the registry and Docker Hub rate-limits
 // anonymous manifest requests. Default 6 hours.
 IntervalDockerUpdate = 21600
```

- [ ] **Step 2: Add the Intervals fields**

In `daemon/domain/context.go`, add to `Intervals` (after `Tuning int`):

```go
 DockerUpdate int
```

In `daemon/domain/fileconfig.go`, add to `FileConfigIntervals` (after `Tuning *int ...`):

```go
 DockerUpdate *int `yaml:"docker_update,omitempty"`
```

- [ ] **Step 3: Wire the CLI flag** in `main.go`

Add to the CLI struct near `IntervalTuning` (line ~95):

```go
 IntervalDockerUpdate int `default:"21600" env:"INTERVAL_DOCKER_UPDATE" help:"container update check interval (seconds, 0=disabled, max 86400)"`
```

Add to `validCollectorNames` map (line ~26):

```go
 "docker_update": true,
```

Add to the `domain.Intervals{...}` literal (after `Tuning:`):

```go
  DockerUpdate: getInterval("docker_update", cli.IntervalDockerUpdate),
```

Add to the file-config application block (near `setInt(&cli.IntervalTuning, iv.Tuning)`, line ~342):

```go
  setInt(&cli.IntervalDockerUpdate, iv.DockerUpdate)
```

- [ ] **Step 4: Register with the manager** in `daemon/services/collector_manager.go`

Add at the end of `RegisterAllCollectors()` (after the tuning registration):

```go
 // DockerUpdate collector — checks container images for available updates.
 cm.Register("docker_update", func(ctx *domain.Context) Collector {
  return collectors.NewDockerUpdateCollector(ctx)
 }, intervals.DockerUpdate, false)
```

Add `"docker_update"` to the `collectorOrder` slice in `GetAllStatus()` (after `"tuning"`):

```go
  "docker_update",
```

Add to the `getDefaultInterval` map (after `"tuning": 120,`):

```go
  "docker_update": 21600,
```

Relax the runtime cap in `UpdateInterval` — change:

```go
 if intervalSeconds < 5 || intervalSeconds > 3600 {
  return fmt.Errorf("invalid interval: must be between 5 and 3600 seconds")
 }
```

to:

```go
 if intervalSeconds < 5 || intervalSeconds > 86400 {
  return fmt.Errorf("invalid interval: must be between 5 and 86400 seconds")
 }
```

- [ ] **Step 5: Write/extend the test** in `daemon/services/collector_manager_coverage_test.go`

```go
func TestRegisterAllCollectors_IncludesDockerUpdate(t *testing.T) {
 cm := NewCollectorManager(&domain.Context{
  Hub:       domain.NewEventBus(16),
  Intervals: domain.Intervals{System: 15, DockerUpdate: 21600},
 }, &sync.WaitGroup{})
 cm.RegisterAllCollectors()

 st, err := cm.GetStatus("docker_update")
 if err != nil {
  t.Fatalf("docker_update not registered: %v", err)
 }
 if st.Interval != 21600 {
  t.Errorf("default interval = %d, want 21600", st.Interval)
 }
 // Runtime cap now allows 6h.
 if err := cm.UpdateInterval("docker_update", 43200); err != nil {
  t.Errorf("UpdateInterval(43200) rejected: %v", err)
 }
}
```

(Ensure imports `sync` and `domain` are present in the test file.)

- [ ] **Step 6: Run build + test**

Run: `go build ./... && go test ./daemon/services/ -run TestRegisterAllCollectors_IncludesDockerUpdate -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add daemon/constants/const.go daemon/domain/context.go daemon/domain/fileconfig.go main.go daemon/services/collector_manager.go daemon/services/collector_manager_coverage_test.go
git commit -m "feat(docker): register docker_update collector and interval config"
```

---

## Task 4: Cache binding + read-time merge in GetDockerCache

**Goal:** Cache the published update result and merge per-container update status into every `ContainerInfo` returned by `CacheStore.GetDockerCache()` — covering REST, MCP, and alerting in one place while keeping the raw stored cache pure.

**Files:**

- Modify: `daemon/services/api/cache_store.go`
- Modify: `daemon/services/api/event_bindings.go`
- Test: `daemon/services/api/cache_store_test.go` (create if absent)

**Acceptance Criteria:**

- [ ] `CacheStore` has `dockerUpdatesCache atomic.Pointer[dto.ContainerUpdatesResult]`.
- [ ] `event_bindings.go` binds `TopicDockerUpdatesUpdate` to store it.
- [ ] `GetDockerCache()` overlays `UpdateStatus`/`UpdateAvailable`/`UpdateChecked` from the updates cache, matching by container ID; containers absent from the cache report `"unknown"`.
- [ ] When no updates cache is present, every container reports `"unknown"` and the raw stored slice is not mutated.
- [ ] New getter `GetContainerUpdatesCache() *dto.ContainerUpdatesResult` returns the cached result (or nil).

**Verify:** `go test ./daemon/services/api/ -run TestGetDockerCacheMerge -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** in `daemon/services/api/cache_store_test.go`

```go
package api

import (
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestGetDockerCacheMerge(t *testing.T) {
 var cs CacheStore
 containers := []dto.ContainerInfo{
  {ID: "abc123", Name: "plex"},
  {ID: "def456", Name: "sonarr"},
 }
 cs.dockerCache.Store(&containers)

 // No updates cache yet → all unknown, raw slice untouched.
 got := cs.GetDockerCache()
 for _, c := range got {
  if c.UpdateStatus != dto.UpdateStatusUnknown {
   t.Errorf("%s: status = %q, want unknown", c.Name, c.UpdateStatus)
  }
 }
 if containers[0].UpdateStatus != "" {
  t.Error("raw stored slice was mutated")
 }

 // Publish update result for plex only.
 avail := true
 cs.dockerUpdatesCache.Store(&dto.ContainerUpdatesResult{
  Containers: []dto.ContainerUpdateInfo{
   {ContainerID: "abc123", ContainerName: "plex", CurrentDigest: "sha256:a", LatestDigest: "sha256:b", UpdateAvailable: avail},
  },
  TotalCount: 1, UpdatesAvailable: 1,
 })

 got = cs.GetDockerCache()
 byName := map[string]dto.ContainerInfo{}
 for _, c := range got {
  byName[c.Name] = c
 }
 if byName["plex"].UpdateStatus != dto.UpdateStatusAvailable {
  t.Errorf("plex status = %q, want update_available", byName["plex"].UpdateStatus)
 }
 if byName["plex"].UpdateAvailable == nil || !*byName["plex"].UpdateAvailable {
  t.Error("plex UpdateAvailable should be non-nil true")
 }
 if byName["sonarr"].UpdateStatus != dto.UpdateStatusUnknown {
  t.Errorf("sonarr status = %q, want unknown (not in updates cache)", byName["sonarr"].UpdateStatus)
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/api/ -run TestGetDockerCacheMerge -v`
Expected: FAIL — `cs.dockerUpdatesCache undefined` and merge not applied.

- [ ] **Step 3: Add the cache field** in `daemon/services/api/cache_store.go`

Add to the `CacheStore` struct (after `tuningCache`):

```go
 dockerUpdatesCache   atomic.Pointer[dto.ContainerUpdatesResult]
```

Replace the existing `GetDockerCache` method with the merging version:

```go
// GetDockerCache returns cached Docker container information with update status
// merged in from the docker_update collector's cache. The raw stored slice is
// never mutated — a shallow copy is returned with update fields overlaid.
func (c *CacheStore) GetDockerCache() []dto.ContainerInfo {
 v := c.dockerCache.Load()
 if v == nil {
  return nil
 }

 // Build lookup from the updates cache (keyed by container ID).
 updates := map[string]dto.ContainerUpdateInfo{}
 var checkedAt *time.Time
 if u := c.dockerUpdatesCache.Load(); u != nil {
  for i := range u.Containers {
   updates[u.Containers[i].ContainerID] = u.Containers[i]
  }
  if !u.Timestamp.IsZero() {
   t := u.Timestamp
   checkedAt = &t
  }
 }

 out := make([]dto.ContainerInfo, len(*v))
 for i, c := range *v {
  if info, ok := updates[c.ID]; ok {
   status := info.Status()
   c.UpdateStatus = status
   if status != dto.UpdateStatusUnknown {
    avail := info.UpdateAvailable
    c.UpdateAvailable = &avail
   }
   c.UpdateChecked = checkedAt
  } else {
   c.UpdateStatus = dto.UpdateStatusUnknown
  }
  out[i] = c
 }
 return out
}

// GetContainerUpdatesCache returns the cached container update result, or nil.
func (c *CacheStore) GetContainerUpdatesCache() *dto.ContainerUpdatesResult {
 return c.dockerUpdatesCache.Load()
}
```

(`time` is already imported in `cache_store.go`.)

- [ ] **Step 4: Add the event binding** in `daemon/services/api/event_bindings.go`

Add inside the `cacheBindings()` returned slice (after the tuning binding):

```go
  bind(constants.TopicDockerUpdatesUpdate, func(c *CacheStore, v *dto.ContainerUpdatesResult) {
   c.dockerUpdatesCache.Store(v)
  }),
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./daemon/services/api/ -run TestGetDockerCacheMerge -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add daemon/services/api/cache_store.go daemon/services/api/event_bindings.go daemon/services/api/cache_store_test.go
git commit -m "feat(docker): cache update results and merge into GetDockerCache"
```

---

## Task 5: REST endpoints — cached /docker/updates + /docker/updates/refresh

**Goal:** Serve `/docker/updates` from cache (instant, no registry traffic) and add `POST /docker/updates/refresh` to force an immediate out-of-band check that publishes to the topic (updating cache, WebSocket, and alerts).

**Files:**

- Modify: `daemon/services/api/handlers.go`
- Modify: `daemon/services/api/server.go`
- Test: `daemon/services/api/handlers_test.go` (add cases)

**Acceptance Criteria:**

- [ ] `GET /docker/updates` returns the cached `ContainerUpdatesResult` without calling the controller; returns an empty result (not 500) when nothing is cached yet.
- [ ] `POST /docker/updates/refresh` runs `CheckAllContainerUpdates()`, publishes the result on `TopicDockerUpdatesUpdate`, and returns the result.
- [ ] Route `POST /api/v1/docker/updates/refresh` is registered.

**Verify:** `go test ./daemon/services/api/ -run TestDockerUpdates -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** in `daemon/services/api/handlers_test.go`

```go
func TestDockerUpdatesServedFromCache(t *testing.T) {
 s := &Server{}
 s.dockerUpdatesCache.Store(&dto.ContainerUpdatesResult{TotalCount: 2, UpdatesAvailable: 1})

 req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/updates", nil)
 rr := httptest.NewRecorder()
 s.handleDockerCheckUpdates(rr, req)

 if rr.Code != http.StatusOK {
  t.Fatalf("status = %d, want 200", rr.Code)
 }
 var got dto.ContainerUpdatesResult
 if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
  t.Fatalf("decode: %v", err)
 }
 if got.UpdatesAvailable != 1 || got.TotalCount != 2 {
  t.Errorf("got %+v, want cached result", got)
 }
}

func TestDockerUpdatesEmptyWhenUncached(t *testing.T) {
 s := &Server{}
 req := httptest.NewRequest(http.MethodGet, "/api/v1/docker/updates", nil)
 rr := httptest.NewRecorder()
 s.handleDockerCheckUpdates(rr, req)
 if rr.Code != http.StatusOK {
  t.Fatalf("status = %d, want 200", rr.Code)
 }
}
```

(Ensure imports: `encoding/json`, `net/http`, `net/http/httptest`, `testing`, and the `dto` package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/api/ -run TestDockerUpdates -v`
Expected: FAIL — current `handleDockerCheckUpdates` calls the controller and does not read the cache.

- [ ] **Step 3: Repoint the cached handler** — replace the body of `handleDockerCheckUpdates` in `daemon/services/api/handlers.go` (line ~2639)

```go
func (s *Server) handleDockerCheckUpdates(w http.ResponseWriter, _ *http.Request) {
 if cached := s.GetContainerUpdatesCache(); cached != nil {
  respondJSON(w, http.StatusOK, cached)
  return
 }
 // Nothing checked yet — return an empty (not error) result.
 respondJSON(w, http.StatusOK, dto.ContainerUpdatesResult{Timestamp: time.Now()})
}
```

- [ ] **Step 4: Add the refresh handler** in `daemon/services/api/handlers.go` (below `handleDockerCheckUpdates`)

```go
// handleDockerUpdatesRefresh godoc
//
// @Summary  Force a container update re-check
// @Description Runs an immediate registry digest comparison for all containers and publishes the result.
// @Tags   Docker
// @Produce  json
// @Success  200 {object} dto.ContainerUpdatesResult "Refreshed update status"
// @Failure  500 {object} dto.Response    "Check failed"
// @Router   /docker/updates/refresh [post]
func (s *Server) handleDockerUpdatesRefresh(w http.ResponseWriter, _ *http.Request) {
 result, err := controllers.NewDockerController().CheckAllContainerUpdates()
 if err != nil {
  respondJSON(w, http.StatusInternalServerError, dto.Response{
   Success: false, Message: fmt.Sprintf("update check failed: %v", err), Timestamp: time.Now(),
  })
  return
 }
 domain.Publish(s.ctx.Hub, constants.TopicDockerUpdatesUpdate, result)
 respondJSON(w, http.StatusOK, result)
}
```

(Confirm `controllers`, `domain`, and `constants` are imported in `handlers.go`; add any that are missing.)

- [ ] **Step 5: Register the route** in `daemon/services/api/server.go` (near line 123, with the other `/docker/updates` route)

```go
 api.HandleFunc("/docker/updates/refresh", s.handleDockerUpdatesRefresh).Methods("POST")
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./daemon/services/api/ -run TestDockerUpdates -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add daemon/services/api/handlers.go daemon/services/api/server.go daemon/services/api/handlers_test.go
git commit -m "feat(docker): serve cached updates and add refresh endpoint"
```

---

## Task 6: Alerting metric — ContainerUpdatesAvailable

**Goal:** Expose a `ContainerUpdatesAvailable` count to the alert engine so users can write a rule like `ContainerUpdatesAvailable > 0`.

**Files:**

- Modify: `daemon/dto/alert.go`
- Modify: `daemon/services/alerting/engine.go`
- Test: `daemon/services/alerting/engine_test.go` (add a case)

**Acceptance Criteria:**

- [ ] `AlertEnv` has `ContainerUpdatesAvailable int` with `expr:"ContainerUpdatesAvailable"`.
- [ ] The engine populates it by counting containers whose `UpdateAvailable != nil && *UpdateAvailable` from the (merged) docker cache.
- [ ] A rule expression `ContainerUpdatesAvailable > 0` evaluates true when one container has an update.

**Verify:** `go test ./daemon/services/alerting/ -run TestContainerUpdatesAvailableMetric -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** in `daemon/services/alerting/engine_test.go`

```go
func TestContainerUpdatesAvailableMetric(t *testing.T) {
 avail := true
 notAvail := false
 provider := &fakeProvider{ // see existing test helpers in this package
  docker: []dto.ContainerInfo{
   {Name: "plex", State: "running", UpdateAvailable: &avail, UpdateStatus: dto.UpdateStatusAvailable},
   {Name: "sonarr", State: "running", UpdateAvailable: &notAvail, UpdateStatus: dto.UpdateStatusUpToDate},
   {Name: "radarr", State: "running", UpdateStatus: dto.UpdateStatusUnknown},
  },
 }
 e := NewEngine(NewStore(t.TempDir()+"/rules.json"), provider)
 env := e.buildEnv()
 if env.ContainerUpdatesAvailable != 1 {
  t.Errorf("ContainerUpdatesAvailable = %d, want 1", env.ContainerUpdatesAvailable)
 }
}
```

> If the package's existing tests use a different provider mock or `buildEnv` is named differently, adapt to the existing helper. Inspect `engine_test.go` for the established `DataProvider` fake (search for a struct implementing `GetDockerCache`) and reuse it; only the `docker` field assignment and the assertion are new. If `buildEnv` is unexported under another name, call the actual env-builder used by existing tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/alerting/ -run TestContainerUpdatesAvailableMetric -v`
Expected: FAIL — `env.ContainerUpdatesAvailable undefined`.

- [ ] **Step 3: Add the AlertEnv field** in `daemon/dto/alert.go` (in the Aggregated block, after `StoppedContainers`)

```go
 ContainerUpdatesAvailable int `expr:"ContainerUpdatesAvailable"`
```

- [ ] **Step 4: Populate it** in `daemon/services/alerting/engine.go` — extend the Docker block (line ~261)

```go
 // Docker
 if containers := e.provider.GetDockerCache(); containers != nil {
  env.ContainerCount = len(containers)
  for _, c := range containers {
   if c.State == "running" {
    env.RunningContainers++
   } else {
    env.StoppedContainers++
   }
   if c.UpdateAvailable != nil && *c.UpdateAvailable {
    env.ContainerUpdatesAvailable++
   }
  }
 }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./daemon/services/alerting/ -run TestContainerUpdatesAvailableMetric -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add daemon/dto/alert.go daemon/services/alerting/engine.go daemon/services/alerting/engine_test.go
git commit -m "feat(alerting): add ContainerUpdatesAvailable metric"
```

---

## Task 7: MCP — refresh tool + docs (cached fields already merged)

**Goal:** Add a `refresh_container_updates` MCP tool (forces a re-check) and document that `list_containers`/`get_container_info` now carry update status (automatic via the `GetDockerCache` merge from Task 4).

**Files:**

- Modify: `daemon/services/mcp/server.go`
- Modify: `docs/integrations/mcp.md`
- Test: `daemon/services/mcp/server_test.go` or the existing tool-registration test

**Acceptance Criteria:**

- [ ] A `refresh_container_updates` tool is registered that calls `CheckAllContainerUpdates()`, publishes on `TopicDockerUpdatesUpdate`, and returns the result.
- [ ] `docs/integrations/mcp.md` documents the new tool and the `update_status` field on container listings.
- [ ] Tool count assertion / registration test (if present) updated to include the new tool.

**Verify:** `go test ./daemon/services/mcp/ -run TestRefreshContainerUpdates -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** in `daemon/services/mcp/server_test.go`

```go
func TestRefreshContainerUpdatesToolRegistered(t *testing.T) {
 s := NewServer(&domain.Context{Hub: domain.NewEventBus(16)}, &fakeCacheProvider{})
 names := s.toolNames() // use existing introspection helper if available
 found := false
 for _, n := range names {
  if n == "refresh_container_updates" {
   found = true
  }
 }
 if !found {
  t.Error("refresh_container_updates tool not registered")
 }
}
```

> If the MCP package has no `toolNames()` helper or `fakeCacheProvider`, mirror the registration assertion style already used in `server_test.go` (search for how existing tools like `check_container_updates` are tested). The only new expectation is the presence of `refresh_container_updates`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/mcp/ -run TestRefreshContainerUpdates -v`
Expected: FAIL — tool not found.

- [ ] **Step 3: Register the tool** in `daemon/services/mcp/server.go` — alongside the existing `check_container_updates` registration (line ~894). Follow the exact registration pattern used by neighboring tools in this file (the snippet below shows intent; match the local `RegisterTool`/handler signature):

```go
 s.RegisterTool(Tool{
  Name:        "refresh_container_updates",
  Description: "Force an immediate registry digest re-check for all containers and publish the result (updates cache, WebSocket, and alerts).",
 }, func(args struct{}) (any, error) {
  dockerCtrl := controllers.NewDockerController()
  result, err := dockerCtrl.CheckAllContainerUpdates()
  if err != nil {
   return nil, err
  }
  domain.Publish(s.ctx.Hub, constants.TopicDockerUpdatesUpdate, result)
  return result, nil
 })
```

(Confirm `domain` and `constants` are imported in `server.go`; the `controllers` import already exists.)

- [ ] **Step 4: Document** in `docs/integrations/mcp.md`

Add `refresh_container_updates` to the Docker tools section, and note that `list_containers` and `get_container_info` now include `update_status` (`up_to_date`/`update_available`/`unknown`), `update_available`, and `update_checked`. Mention `check_container_update(s)` remain for synchronous on-demand checks.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./daemon/services/mcp/ -run TestRefreshContainerUpdates -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add daemon/services/mcp/server.go daemon/services/mcp/server_test.go docs/integrations/mcp.md
git commit -m "feat(mcp): add refresh_container_updates tool and document update status"
```

---

## Task 8: Opt-in notification on new updates

**Goal:** When the scheduled check detects a container transition into "update available", optionally raise an Unraid notification — gated behind an opt-in flag so it never spams by default.

**Files:**

- Modify: `daemon/domain/context.go`
- Modify: `main.go`
- Modify: `daemon/services/collectors/docker_update.go`
- Test: `daemon/services/collectors/docker_update_test.go` (add a case)

**Acceptance Criteria:**

- [ ] `domain.Context` has a `DockerUpdateNotify bool` field set from CLI `--docker-update-notify` (env `DOCKER_UPDATE_NOTIFY`, default false).
- [ ] The collector tracks the set of containers with updates between runs and, when notify is enabled, fires a notification listing newly-available updates (the transition set), not the full set every run.
- [ ] The notifier is injectable so the test asserts the message without writing files.
- [ ] No notification fires on the first run or when notify is disabled.

**Verify:** `go test ./daemon/services/collectors/... -run TestDockerUpdateNotify -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** in `daemon/services/collectors/docker_update_test.go`

```go
func TestDockerUpdateNotify_FiresOnNewTransitionOnly(t *testing.T) {
 hub := domain.NewEventBus(16)
 var notified []string
 c := NewDockerUpdateCollector(&domain.Context{Hub: hub, DockerUpdateNotify: true})
 c.notifyFn = func(names []string) { notified = append(notified, names...) }

 avail := true
 step1 := &dto.ContainerUpdatesResult{
  Containers: []dto.ContainerUpdateInfo{{ContainerID: "a", ContainerName: "plex", LatestDigest: "x", UpdateAvailable: avail}},
  TotalCount: 1, UpdatesAvailable: 1,
 }
 // First run: establishes baseline, no notification.
 c.checkFn = func() (*dto.ContainerUpdatesResult, error) { return step1, nil }
 c.Collect()
 if len(notified) != 0 {
  t.Fatalf("first run should not notify, got %v", notified)
 }

 // Second run: sonarr newly has an update → notify only about sonarr.
 step2 := &dto.ContainerUpdatesResult{
  Containers: []dto.ContainerUpdateInfo{
   {ContainerID: "a", ContainerName: "plex", LatestDigest: "x", UpdateAvailable: avail},
   {ContainerID: "b", ContainerName: "sonarr", LatestDigest: "y", UpdateAvailable: avail},
  },
  TotalCount: 2, UpdatesAvailable: 2,
 }
 c.checkFn = func() (*dto.ContainerUpdatesResult, error) { return step2, nil }
 c.Collect()
 if len(notified) != 1 || notified[0] != "sonarr" {
  t.Fatalf("expected notify [sonarr], got %v", notified)
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/collectors/... -run TestDockerUpdateNotify -v`
Expected: FAIL — `notifyFn` / `DockerUpdateNotify` undefined.

- [ ] **Step 3: Add the context field** in `daemon/domain/context.go` (in `Context`, after `LogsDir`)

```go
 DockerUpdateNotify bool
```

- [ ] **Step 4: Wire the CLI flag** in `main.go`

Add to the CLI struct (near the docker-update interval):

```go
 DockerUpdateNotify bool `default:"false" env:"DOCKER_UPDATE_NOTIFY" help:"raise an Unraid notification when new container updates become available"`
```

Add to the `domain.Context{...}` literal (after `LogsDir: cli.LogsDir,`):

```go
  DockerUpdateNotify: cli.DockerUpdateNotify,
```

- [ ] **Step 5: Extend the collector** in `daemon/services/collectors/docker_update.go`

Add a `notifyFn` field and a `prevAvailable` set to the struct, initialize in the constructor, and add transition logic to `Collect()`:

```go
// add to imports:
//   "github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"  (already imported)

// add fields to DockerUpdateCollector:
 notifyFn      func(names []string)
 prevAvailable map[string]bool
 baselineSet   bool
```

In `NewDockerUpdateCollector`, set:

```go
 dc := &DockerUpdateCollector{
  appCtx:        ctx,
  prevAvailable: map[string]bool{},
 }
 dc.checkFn = func() (*dto.ContainerUpdatesResult, error) {
  return controllers.NewDockerController().CheckAllContainerUpdates()
 }
 dc.notifyFn = func(names []string) {
  _ = controllers.CreateNotification(
   "Container updates available",
   "Docker",
   fmt.Sprintf("Updates available for: %s", strings.Join(names, ", ")),
   "info",
   "",
  )
 }
 return dc
```

In `Collect()`, after computing `result` and before/after the publish dedupe, add the transition detection:

```go
 // Notify on newly-available updates (opt-in). Skip the first run so we
 // establish a baseline instead of announcing every existing update.
 if c.appCtx.DockerUpdateNotify && c.notifyFn != nil {
  current := map[string]bool{}
  var newlyAvailable []string
  for _, ct := range result.Containers {
   if ct.UpdateAvailable {
    current[ct.ContainerID] = true
    if c.baselineSet && !c.prevAvailable[ct.ContainerID] {
     newlyAvailable = append(newlyAvailable, ct.ContainerName)
    }
   }
  }
  if c.baselineSet && len(newlyAvailable) > 0 {
   c.notifyFn(newlyAvailable)
  }
  c.prevAvailable = current
  c.baselineSet = true
 }
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./daemon/services/collectors/... -run TestDockerUpdate -v`
Expected: PASS (both the dedupe and notify tests)

- [ ] **Step 7: Commit**

```bash
git add daemon/domain/context.go main.go daemon/services/collectors/docker_update.go daemon/services/collectors/docker_update_test.go
git commit -m "feat(docker): opt-in notification on newly available container updates"
```

---

## Task 9: Docs, swagger, CHANGELOG, and full verification

**Goal:** Regenerate API docs, document the feature, update CHANGELOG, and verify the whole suite and linters pass.

**Files:**

- Modify: `CHANGELOG.md`
- Modify: `docs/api/` (the docker endpoints doc, if a hand-written one exists)
- Regenerate: swagger (`make swagger`)

**Acceptance Criteria:**

- [ ] `CHANGELOG.md` has a new dated entry describing continuous container update detection (Added: collector, `update_status` fields, `/docker/updates/refresh`, `refresh_container_updates` MCP tool, `ContainerUpdatesAvailable` alert metric, opt-in notification).
- [ ] `make swagger` regenerates without error and includes the refresh endpoint.
- [ ] `make test` and `make pre-commit-run` pass.

**Verify:** `make test && make pre-commit-run` → PASS

**Steps:**

- [ ] **Step 1: Update CHANGELOG.md** — add a new version section at the top:

```markdown
## [2026.05.30]

### Added

- Continuous Docker container update detection via a dedicated `docker_update` collector (default 6h interval, staggered start, registry-rate-limit-safe).
- `update_status` (`up_to_date`/`update_available`/`unknown`), `update_available`, and `update_checked` fields on every container in REST `/docker`, `/docker/{id}`, and MCP `list_containers`/`get_container_info`.
- `POST /api/v1/docker/updates/refresh` and MCP `refresh_container_updates` to force an immediate re-check.
- `ContainerUpdatesAvailable` alerting metric (e.g. rule `ContainerUpdatesAvailable > 0`).
- Opt-in Unraid notification when new container updates appear (`--docker-update-notify` / `DOCKER_UPDATE_NOTIFY`).

### Changed

- `GET /api/v1/docker/updates` now serves cached results instantly instead of triggering a live registry check.
- Runtime collector interval cap raised from 3600s to 86400s to match documented limits.
```

- [ ] **Step 2: Regenerate swagger**

Run: `make swagger`
Expected: completes without error; `daemon/docs/swagger.json` includes `/docker/updates/refresh`.

- [ ] **Step 3: Full test + lint**

Run: `make test`
Expected: all packages PASS (race detector clean).

Run: `make pre-commit-run`
Expected: lint + security checks PASS.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md daemon/docs/ docs/
git commit -m "docs(docker): document continuous container update detection"
```

---

## Self-Review Notes

- **Spec coverage:** dedicated collector (T2/T3), tri-state model (T1/T4), read-merge (T4), cached `/docker/updates` + refresh (T5), WebSocket on-change (T2 publishes only on change; broadcast is downstream of publish — covered), alert metric (T6), MCP cached fields + refresh (T7), opt-in notification (T8), no DB (in-memory caches throughout), docs/CHANGELOG (T9). All spec sections map to a task.
- **WebSocket "broadcast on change":** delivered by the collector publishing only when the signature changes (Task 2). The generic broadcast layer (`broadcastEvents`) forwards every publish, so dedupe at publish time is the correct single point. A forced refresh (Task 5/7) intentionally always publishes.
- **Type consistency:** `ContainerUpdateInfo.Status()`, `UpdateStatus*` constants, `TopicDockerUpdatesUpdate`, `dockerUpdatesCache`, `GetContainerUpdatesCache()`, `ContainerUpdatesAvailable`, `DockerUpdateNotify` used consistently across tasks.
- **Interval cap caveat:** default 21600s exceeds the old 3600s runtime cap; Task 3 raises it to 86400s (matching the CLI help text) so the collector's interval is also runtime-adjustable.
