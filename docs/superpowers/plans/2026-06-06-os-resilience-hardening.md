# OS-Resilience & Compatibility Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the agent detect when an Unraid data source breaks (path moved, file format changed, binary gone), degrade gracefully while flagging it (never silently return wrong/empty data), surface it (REST self-test + MCP tool + inline flags + WebSocket + Prometheus + auto-alert), and catch breakage in tests — without a rewrite or any dependency on the official Unraid API.

**Architecture:** A new dependency-light `daemon/platform` package owns a thread-safe status `Registry` (with an injected transition-notifier), a `Detector` (capability/version probing), and a `Resolver` (path/binary existence). The shared `dto.SourceStatus` type lives in the leaf `dto` package. The six highest-risk collectors shape-validate what they read each run and report `healthy`/`degraded`/`unavailable`; the four highest-risk controllers capability-gate their actions. Status is exposed five ways, all reading the one registry.

**Tech Stack:** Go 1.26, gorilla/mux, prometheus/client_golang, expr-lang (alerting), the project's `lib`/`logger`/`domain` packages, and the existing Ansible deploy/verify role.

**Spec:** `docs/superpowers/specs/2026-06-06-os-resilience-design.md`

---

## MANDATORY verification gate (applies to EVERY task)

No task is "done" until, in order:

1. **Test** — `go test ./...` + `go vet ./...` + `gofmt -l` clean (or `make pre-commit-run`).
2. **Build** — `make local` succeeds.
3. **Verify on Unraid via Ansible** (tasks that change the running binary) — `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify`; the `verify` role must pass on `192.168.20.21`.
4. **CodeRabbit** — `coderabbit review --agent -t uncommitted`; fix valid findings; re-run until clean.
5. **CHANGELOG** — only the FINAL task updates `CHANGELOG.md`, after everything is verified on hardware.

> Unraid host has no Python — Ansible ad-hoc must use the `raw` module. CodeRabbit may rate-limit (~8 min); wait and retry.

---

## File structure

**New files**

- `daemon/dto/source_status.go` — `SourceState` + `SourceStatus` value types (leaf package, cycle-safe).
- `daemon/platform/registry.go` — thread-safe status registry + transition notifier + capabilities snapshot.
- `daemon/platform/detector.go` — version detection + capability probing (Unraid-agnostic; caller supplies probes).
- `daemon/platform/resolver.go` — path/binary existence helpers.
- `daemon/platform/validate.go` — generic shape-validation helpers (e.g. `MissingKeys`).
- `daemon/platform/*_test.go` — unit tests for the above.
- `daemon/services/probes.go` — the Unraid-specific probe list (imports `constants` + `platform`; breaks the would-be cycle).
- `daemon/services/api/diagnostics.go` — `/api/v1/diagnostics/self-test` handler + response DTO.
- `daemon/services/collectors/testdata/fixtures/unraid-<ver>/...` — sanitized golden fixtures.
- `daemon/services/collectors/validators.go` — per-subsystem shape validators + their tests.

**Modified files**

- `daemon/domain/context.go` — add `Platform *platform.Registry`.
- `main.go` — construct the registry; (probe seeding happens in orchestrator).
- `daemon/services/orchestrator.go` — seed detector, wire notifier → event bus.
- `daemon/constants/topics.go` — add `TopicSourceStatusChanged`.
- `daemon/dto/system.go`, `array.go`, `disk.go`, `share.go`, `container.go`, `vm.go` — add `SourceStatus *SourceStatus json:"source_status,omitempty"`.
- The 6 collectors (`system.go`, `array.go`, `disk.go`, `shares.go`, `docker.go`, `vm.go`) — validate + report + set inline flag.
- The 4 controllers (`array.go`, `disk*`, `docker.go`, `vm.go`) — capability-gate.
- `daemon/services/api/server.go` — register the self-test route; add the new topic to broadcast maps.
- `daemon/services/api/metrics.go` — new gauges + update from registry.
- `daemon/services/api/handlers.go` (health report) — degraded summary.
- `daemon/services/mcp/server.go` — `run_self_test` tool + health-report tool fields.
- `daemon/dto/alert.go` + `daemon/services/alerting/templates.go` + AlertEnv builder — degraded count + built-in rule.
- `ansible/roles/verify/tasks/*.yml` — assert self-test healthy.
- Docs: `docs/guides/configuration.md`, `docs/integrations/mcp.md` (counts), `AGENTS.md`, `skills/.../mcp-tools.md`.

---

### Task 0: `dto.SourceStatus` + `platform.Registry`

**Goal:** The shared status value type and the thread-safe registry with transition-notifier exist and are unit-tested.

**Files:**

- Create: `daemon/dto/source_status.go`
- Create: `daemon/platform/registry.go`
- Test: `daemon/platform/registry_test.go`

**Acceptance Criteria:**

- [ ] `dto.SourceStatus` + `dto.SourceState` constants compile.
- [ ] `Registry.Report` stores status, and invokes the notifier exactly once per state _transition_ (not on repeat of same state).
- [ ] `Snapshot()` returns subsystems sorted by name; `DegradedCount()` counts non-healthy; `StatusFor()` returns a pointer only when not healthy.

**Verify:** `go test ./daemon/platform/ -run TestRegistry -v` → PASS

**Steps:**

- [ ] **Step 1: Write `daemon/dto/source_status.go`**

```go
package dto

import "time"

// SourceState describes the health of a data source backing a subsystem.
type SourceState string

const (
 // SourceHealthy means the source was read and shape-validated successfully.
 SourceHealthy SourceState = "healthy"
 // SourceDegraded means the source was read but failed a sanity/shape check;
 // best-effort partial data is still served.
 SourceDegraded SourceState = "degraded"
 // SourceUnavailable means the source or its binary is absent.
 SourceUnavailable SourceState = "unavailable"
)

// Severity orders states for "worst-of" rollups: healthy < degraded < unavailable.
func (s SourceState) Severity() int {
 switch s {
 case SourceDegraded:
  return 1
 case SourceUnavailable:
  return 2
 default:
  return 0
 }
}

// SourceStatus is the health of one subsystem's data source.
type SourceStatus struct {
 Subsystem   string      `json:"subsystem"`
 State       SourceState `json:"state"`
 Reason      string      `json:"reason,omitempty"`
 LastChecked time.Time   `json:"last_checked"`
 LastError   string      `json:"last_error,omitempty"`
}
```

- [ ] **Step 2: Write the failing test `daemon/platform/registry_test.go`**

```go
package platform

import (
 "errors"
 "testing"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestRegistryTransitionNotifier(t *testing.T) {
 var calls []dto.SourceStatus
 r := NewRegistry()
 r.SetClock(func() time.Time { return time.Unix(0, 0) })
 r.SetNotifier(func(s dto.SourceStatus) { calls = append(calls, s) })

 r.Report("array", dto.SourceHealthy, "", nil)   // transition nil->healthy
 r.Report("array", dto.SourceHealthy, "", nil)   // no transition
 r.Report("array", dto.SourceDegraded, "stale", errors.New("boom")) // transition

 if len(calls) != 2 {
  t.Fatalf("notifier called %d times, want 2", len(calls))
 }
 if calls[1].State != dto.SourceDegraded || calls[1].Reason != "stale" || calls[1].LastError != "boom" {
  t.Fatalf("unexpected last notification: %+v", calls[1])
 }
}

func TestRegistrySnapshotAndCounts(t *testing.T) {
 r := NewRegistry()
 r.Report("b", dto.SourceDegraded, "x", nil)
 r.Report("a", dto.SourceHealthy, "", nil)
 r.Report("c", dto.SourceUnavailable, "missing", nil)

 snap := r.Snapshot()
 if len(snap) != 3 || snap[0].Subsystem != "a" || snap[1].Subsystem != "b" {
  t.Fatalf("snapshot not sorted by name: %+v", snap)
 }
 if r.DegradedCount() != 2 {
  t.Fatalf("DegradedCount = %d, want 2", r.DegradedCount())
 }
 if r.StatusFor("a") != nil {
  t.Fatalf("StatusFor healthy subsystem must be nil")
 }
 if r.StatusFor("b") == nil {
  t.Fatalf("StatusFor degraded subsystem must be non-nil")
 }
}
```

- [ ] **Step 3: Run the test, expect FAIL**

Run: `go test ./daemon/platform/ -run TestRegistry -v`
Expected: build failure / FAIL (`NewRegistry` undefined).

- [ ] **Step 4: Write `daemon/platform/registry.go`**

```go
// Package platform provides OS-resilience primitives: a data-source health
// registry, capability/version detection, and path/binary resolution. It is
// deliberately Unraid-agnostic (callers supply probe lists) and imports only
// dto + logger, so any layer can use it without import cycles.
package platform

import (
 "sort"
 "sync"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// Notifier is invoked once per state transition for a subsystem.
type Notifier func(dto.SourceStatus)

// Registry is a thread-safe store of per-subsystem source health.
type Registry struct {
 mu       sync.RWMutex
 statuses map[string]dto.SourceStatus
 caps     dto.Capabilities
 notifier Notifier
 clock    func() time.Time
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
 return &Registry{statuses: make(map[string]dto.SourceStatus), clock: time.Now}
}

// SetNotifier sets the transition callback (wired to the event bus by the orchestrator).
func (r *Registry) SetNotifier(n Notifier) { r.mu.Lock(); r.notifier = n; r.mu.Unlock() }

// SetClock overrides the time source (tests).
func (r *Registry) SetClock(f func() time.Time) { r.mu.Lock(); r.clock = f; r.mu.Unlock() }

// SetCapabilities stores the startup capability snapshot.
func (r *Registry) SetCapabilities(c dto.Capabilities) { r.mu.Lock(); r.caps = c; r.mu.Unlock() }

// Capabilities returns the startup capability snapshot.
func (r *Registry) Capabilities() dto.Capabilities { r.mu.RLock(); defer r.mu.RUnlock(); return r.caps }

// Report records a subsystem's source state. On a state transition it logs once
// and invokes the notifier. err may be nil.
func (r *Registry) Report(subsystem string, state dto.SourceState, reason string, err error) {
 r.mu.Lock()
 prev, existed := r.statuses[subsystem]
 status := dto.SourceStatus{
  Subsystem:   subsystem,
  State:       state,
  Reason:      reason,
  LastChecked: r.clock(),
 }
 if err != nil {
  status.LastError = err.Error()
 }
 r.statuses[subsystem] = status
 transition := !existed || prev.State != state
 notifier := r.notifier
 r.mu.Unlock()

 if transition {
  if state == dto.SourceHealthy {
   logger.Info("Resilience: %s source recovered (healthy)", subsystem)
  } else {
   logger.Warning("Resilience: %s source %s: %s", subsystem, state, reason)
  }
  if notifier != nil {
   notifier(status)
  }
 }
}

// Healthy is a convenience for Report(subsystem, healthy, "", nil).
func (r *Registry) Healthy(subsystem string) { r.Report(subsystem, dto.SourceHealthy, "", nil) }

// Get returns the current status for a subsystem.
func (r *Registry) Get(subsystem string) (dto.SourceStatus, bool) {
 r.mu.RLock()
 defer r.mu.RUnlock()
 s, ok := r.statuses[subsystem]
 return s, ok
}

// StatusFor returns a pointer to the status only when NOT healthy (for inline DTO flags).
func (r *Registry) StatusFor(subsystem string) *dto.SourceStatus {
 r.mu.RLock()
 defer r.mu.RUnlock()
 s, ok := r.statuses[subsystem]
 if !ok || s.State == dto.SourceHealthy {
  return nil
 }
 cp := s
 return &cp
}

// Snapshot returns all statuses sorted by subsystem name.
func (r *Registry) Snapshot() []dto.SourceStatus {
 r.mu.RLock()
 out := make([]dto.SourceStatus, 0, len(r.statuses))
 for _, s := range r.statuses {
  out = append(out, s)
 }
 r.mu.RUnlock()
 sort.Slice(out, func(i, j int) bool { return out[i].Subsystem < out[j].Subsystem })
 return out
}

// DegradedCount returns the number of subsystems not in the healthy state.
func (r *Registry) DegradedCount() int {
 r.mu.RLock()
 defer r.mu.RUnlock()
 n := 0
 for _, s := range r.statuses {
  if s.State != dto.SourceHealthy {
   n++
  }
 }
 return n
}

// OverallState returns the worst current state (healthy if empty).
func (r *Registry) OverallState() dto.SourceState {
 r.mu.RLock()
 defer r.mu.RUnlock()
 worst := dto.SourceHealthy
 for _, s := range r.statuses {
  if s.State.Severity() > worst.Severity() {
   worst = s.State
  }
 }
 return worst
}
```

- [ ] **Step 5: Add `dto.Capabilities` to `daemon/dto/source_status.go`** (append):

```go
// Capability is one probed OS capability (a binary or a path).
type Capability struct {
 Name      string `json:"name"`
 Available bool   `json:"available"`
 Target    string `json:"target,omitempty"`
 Detail    string `json:"detail,omitempty"`
}

// Capabilities is the startup probe snapshot.
type Capabilities struct {
 UnraidVersion string       `json:"unraid_version"`
 Items         []Capability `json:"items"`
}
```

- [ ] **Step 6: Run the test, expect PASS**

Run: `go test ./daemon/platform/ -run TestRegistry -v`
Expected: PASS (both tests).

- [ ] **Step 7: Gate + commit**

```bash
go test ./... && go vet ./... && gofmt -l daemon/
git add daemon/dto/source_status.go daemon/platform/registry.go daemon/platform/registry_test.go
git commit -m "feat(platform): add dto.SourceStatus + status Registry with transition notifier"
```

Then `coderabbit review --agent -t uncommitted` (fix findings). (No Unraid deploy yet — no runtime behavior.)

---

### Task 1: `platform.Detector` + `platform.Resolver` + validators

**Goal:** Version detection, capability probing, path/binary existence, and shape-validation helpers exist and are unit-tested. `platform` imports only `dto`, `logger`, and stdlib.

**Files:**

- Create: `daemon/platform/detector.go`, `daemon/platform/resolver.go`, `daemon/platform/validate.go`
- Test: `daemon/platform/detector_test.go`, `daemon/platform/resolver_test.go`, `daemon/platform/validate_test.go`

**Acceptance Criteria:**

- [ ] `Resolver` reports a present temp file as existing and a missing path as absent.
- [ ] `Detect` returns a `dto.Capabilities` with `Available=false` for missing probes.
- [ ] `MissingKeys` returns exactly the absent keys.
- [ ] `go list -deps ./daemon/platform` shows no `services`, `domain`, or `constants` import.

**Verify:** `go test ./daemon/platform/ -v` → PASS

**Steps:**

- [ ] **Step 1: Write `daemon/platform/resolver.go`**

```go
package platform

import (
 "os"
 "os/exec"
 "path/filepath"
)

// PathExists reports whether a filesystem path exists.
func PathExists(path string) bool {
 _, err := os.Stat(path)
 return err == nil
}

// BinaryExists reports whether a binary is available — by absolute path if given,
// otherwise via PATH lookup of its base name.
func BinaryExists(pathOrName string) bool {
 if filepath.IsAbs(pathOrName) {
  if PathExists(pathOrName) {
   return true
  }
 }
 _, err := exec.LookPath(filepath.Base(pathOrName))
 return err == nil
}
```

- [ ] **Step 2: Write `daemon/platform/detector.go`**

```go
package platform

import (
 "os"
 "regexp"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ProbeKind distinguishes a path probe from a binary probe.
type ProbeKind int

const (
 ProbePath ProbeKind = iota
 ProbeBinary
)

// Probe is one capability to check at startup. Name is a stable key; Target is
// the path or binary. Callers (which know Unraid paths) build the probe list,
// keeping this package Unraid-agnostic and cycle-free.
type Probe struct {
 Name   string
 Target string
 Kind   ProbeKind
}

var unraidVersionRe = regexp.MustCompile(`version="?([^"\n]+)"?`)

// DetectUnraidVersion reads /etc/unraid-version; returns "" if unavailable.
func DetectUnraidVersion() string {
 data, err := os.ReadFile("/etc/unraid-version")
 if err != nil {
  return ""
 }
 if m := unraidVersionRe.FindStringSubmatch(string(data)); m != nil {
  return m[1]
 }
 return ""
}

// Detect runs all probes and returns a capability snapshot. Never panics.
func Detect(probes []Probe) dto.Capabilities {
 caps := dto.Capabilities{UnraidVersion: DetectUnraidVersion()}
 for _, p := range probes {
  available := false
  switch p.Kind {
  case ProbeBinary:
   available = BinaryExists(p.Target)
  default:
   available = PathExists(p.Target)
  }
  caps.Items = append(caps.Items, dto.Capability{
   Name:      p.Name,
   Available: available,
   Target:    p.Target,
  })
 }
 return caps
}
```

- [ ] **Step 3: Write `daemon/platform/validate.go`**

```go
package platform

// MissingKeys returns the keys absent from m (or with empty values), preserving
// the requested order. Used by collector shape validators on parsed INI maps.
func MissingKeys(m map[string]string, keys ...string) []string {
 var missing []string
 for _, k := range keys {
  if v, ok := m[k]; !ok || v == "" {
   missing = append(missing, k)
  }
 }
 return missing
}
```

- [ ] **Step 4: Write the tests**

`daemon/platform/resolver_test.go`:

```go
package platform

import (
 "os"
 "path/filepath"
 "testing"
)

func TestPathExists(t *testing.T) {
 dir := t.TempDir()
 f := filepath.Join(dir, "present")
 if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
  t.Fatal(err)
 }
 if !PathExists(f) {
  t.Error("expected present file to exist")
 }
 if PathExists(filepath.Join(dir, "missing")) {
  t.Error("expected missing file to be absent")
 }
}
```

`daemon/platform/detector_test.go`:

```go
package platform

import "testing"

func TestDetectMarksMissingProbes(t *testing.T) {
 caps := Detect([]Probe{{Name: "ghost", Target: "/no/such/path", Kind: ProbePath}})
 if len(caps.Items) != 1 || caps.Items[0].Available {
  t.Fatalf("expected missing probe to be unavailable: %+v", caps.Items)
 }
}
```

`daemon/platform/validate_test.go`:

```go
package platform

import (
 "reflect"
 "testing"
)

func TestMissingKeys(t *testing.T) {
 m := map[string]string{"a": "1", "b": ""}
 got := MissingKeys(m, "a", "b", "c")
 if !reflect.DeepEqual(got, []string{"b", "c"}) {
  t.Fatalf("MissingKeys = %v, want [b c]", got)
 }
}
```

- [ ] **Step 5: Run tests + dependency check**

Run: `go test ./daemon/platform/ -v` → PASS
Run: `go list -deps ./daemon/platform | grep -E 'unraid-management-agent/daemon/(services|domain|constants)$'` → no output (empty).

- [ ] **Step 6: Gate + commit**

```bash
go test ./... && go vet ./...
git add daemon/platform/
git commit -m "feat(platform): add Detector, Resolver, and shape-validation helpers"
```

Then CodeRabbit review; fix findings.

---

### Task 2: Wire registry into Context, main, orchestrator + WS topic

**Goal:** A single `*platform.Registry` lives on `domain.Context`, is created in `main.go`, seeded by the detector with the Unraid probe list, and its notifier publishes `dto.SourceStatus` to a new event-bus topic that the API server broadcasts over WebSocket.

**Files:**

- Modify: `daemon/domain/context.go`, `main.go`, `daemon/services/orchestrator.go`, `daemon/constants/topics.go`, `daemon/services/api/server.go`
- Create: `daemon/services/probes.go`

**Acceptance Criteria:**

- [ ] `o.ctx.Platform` is non-nil during `Run()`; startup logs the detected Unraid version + degraded probe count.
- [ ] A `source_status_changed` transition is published to the hub and appears in the WS broadcast type→topic map.
- [ ] `go build ./...` passes; no import cycle.

**Verify:** `go build ./... && go test ./daemon/...` → PASS

**Steps:**

- [ ] **Step 1: Add the field to `daemon/domain/context.go`** (inside `Context`, after `Hub`):

```go
 Hub                *EventBus
 Platform           *platform.Registry
```

Add the import: `"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"`.

- [ ] **Step 2: Construct it in `main.go`** (in the `appCtx := &domain.Context{...}` literal, after `Hub: ...`):

```go
  Hub:      domain.NewEventBus(1024),
  Platform: platform.NewRegistry(),
```

Add import `"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"`.

- [ ] **Step 3: Add the topic to `daemon/constants/topics.go`** (in the `var (...)` block):

```go
 // TopicSourceStatusChanged fires when a subsystem's data-source health transitions.
 TopicSourceStatusChanged = domain.NewTopic[dto.SourceStatus]("source_status_changed")
```

- [ ] **Step 4: Create `daemon/services/probes.go`** (Unraid-specific probe list — imports `constants` + `platform`, the cycle-safe seam):

```go
package services

import (
 "github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

// resilienceProbes lists the OS capabilities the hot-path subsystems depend on.
// Kept here (not in platform) so platform stays Unraid-agnostic and cycle-free.
func resilienceProbes() []platform.Probe {
 return []platform.Probe{
  {Name: "var.ini", Target: constants.VarIni, Kind: platform.ProbePath},
  {Name: "disks.ini", Target: constants.DisksIni, Kind: platform.ProbePath},
  {Name: "shares.ini", Target: constants.SharesIni, Kind: platform.ProbePath},
  {Name: "mdcmd", Target: constants.MdcmdBin, Kind: platform.ProbeBinary},
  {Name: "smartctl", Target: constants.SmartctlBin, Kind: platform.ProbeBinary},
  {Name: "docker", Target: constants.DockerBin, Kind: platform.ProbeBinary},
  {Name: "virsh", Target: constants.VirshBin, Kind: platform.ProbeBinary},
 }
}
```

- [ ] **Step 5: Seed + wire notifier in `orchestrator.go` `Run()`** (right after `o.collectorManager = NewCollectorManager(...)`, before collectors start):

```go
 // OS-resilience: detect capabilities and wire the status-change notifier to the bus.
 caps := platform.Detect(resilienceProbes())
 o.ctx.Platform.SetCapabilities(caps)
 o.ctx.Platform.SetNotifier(func(s dto.SourceStatus) {
  domain.Publish(o.ctx.Hub, constants.TopicSourceStatusChanged, s)
 })
 degraded := 0
 for _, c := range caps.Items {
  if !c.Available {
   degraded++
  }
 }
 logger.Info("Resilience: Unraid version %q, %d/%d probed capabilities unavailable",
  caps.UnraidVersion, degraded, len(caps.Items))
```

Add imports if missing: `platform`, and ensure `domain`, `constants`, `dto`, `logger` are present (they are).

- [ ] **Step 6: Register the topic for WS broadcast in `daemon/services/api/server.go`**

Find `broadcastTopicNames()` and `buildTypeToTopicMap()`. Add `constants.TopicSourceStatusChanged` to the broadcast topic-name list, and map its type to a topic label:

```go
 // in buildTypeToTopicMap()
 m[reflect.TypeOf(dto.SourceStatus{})] = "source_status_changed"
```

and add `constants.TopicSourceStatusChanged.Name` to whatever slice `broadcastTopicNames()` returns.

- [ ] **Step 7: Build + test + gate + commit**

Run: `go build ./... && go test ./daemon/... -count=1`

```bash
git add daemon/domain/context.go main.go daemon/services/orchestrator.go daemon/constants/topics.go daemon/services/probes.go daemon/services/api/server.go
git commit -m "feat(platform): wire status registry into context/orchestrator + WS source_status_changed topic"
```

CodeRabbit review; fix findings.

---

### Task 3: Shape-validate + report in `system` and `array` collectors

**Goal:** The `system` and `array` collectors validate what they read each run, report status to the registry, still publish best-effort data, and attach the inline `source_status` flag when not healthy.

**Files:**

- Create: `daemon/services/collectors/validators.go`, `daemon/services/collectors/validators_test.go`
- Modify: `daemon/services/collectors/system.go`, `daemon/services/collectors/array.go`, `daemon/dto/system.go`, `daemon/dto/array.go`

**Acceptance Criteria:**

- [ ] `dto.SystemInfo` and `dto.ArrayStatus` have `SourceStatus *dto.SourceStatus json:"source_status,omitempty"`.
- [ ] When `var.ini` is missing required keys, the array collector reports `degraded` with a reason and still publishes.
- [ ] On a successful run, the registry shows `system`/`array` healthy and the inline flag is nil (response unchanged).

**Verify:** `go test ./daemon/services/collectors/ -run TestValidate -v` → PASS

**Steps:**

- [ ] **Step 1: Add inline flag to DTOs.** In `daemon/dto/system.go` add to the `SystemInfo` struct (end of struct):

```go
 SourceStatus *SourceStatus `json:"source_status,omitempty"`
```

Do the same in `daemon/dto/array.go` for `ArrayStatus`.

- [ ] **Step 2: Write `daemon/services/collectors/validators.go`**

```go
package collectors

import (
 "fmt"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

// validateRequiredKeys returns (ok, reason) for a parsed INI map. ok=false with a
// human-readable reason when any required key is missing/empty.
func validateRequiredKeys(parsed map[string]string, keys ...string) (bool, string) {
 missing := platform.MissingKeys(parsed, keys...)
 if len(missing) == 0 {
  return true, ""
 }
 return false, fmt.Sprintf("missing expected keys: %v", missing)
}
```

- [ ] **Step 3: Write the failing test `daemon/services/collectors/validators_test.go`**

```go
package collectors

import "testing"

func TestValidateRequiredKeys(t *testing.T) {
 ok, reason := validateRequiredKeys(map[string]string{"mdState": "STARTED"}, "mdState")
 if !ok || reason != "" {
  t.Fatalf("expected ok, got ok=%v reason=%q", ok, reason)
 }
 ok, reason = validateRequiredKeys(map[string]string{}, "mdState", "mdNumDisks")
 if ok || reason == "" {
  t.Fatalf("expected failure with reason, got ok=%v reason=%q", ok, reason)
 }
}
```

- [ ] **Step 4: Run test, expect FAIL** → `go test ./daemon/services/collectors/ -run TestValidate -v` (undefined symbol).

- [ ] **Step 5: Run test, expect PASS** after Step 2 compiles.

- [ ] **Step 6: Integrate into the array collector** (`daemon/services/collectors/array.go`). Locate where it parses `var.ini` (via `lib.ParseINIFile(constants.VarIni)`). Wrap the parse + publish:

```go
 parsed, err := lib.ParseINIFile(constants.VarIni)
 if err != nil {
  c.ctx.Platform.Report("array", dto.SourceUnavailable, "cannot read var.ini", err)
  return // nothing valid to publish
 }
 if ok, reason := validateRequiredKeys(parsed, "mdState", "mdNumDisks"); !ok {
  c.ctx.Platform.Report("array", dto.SourceDegraded, reason, nil)
 } else {
  c.ctx.Platform.Healthy("array")
 }
 // ... build arrayStatus from parsed (unchanged) ...
 arrayStatus.SourceStatus = c.ctx.Platform.StatusFor("array") // nil when healthy
 domain.Publish(c.ctx.Hub, constants.TopicArrayStatusUpdate, arrayStatus)
```

(Adjust the exact required-key names to those the collector actually relies on; `mdState` + `mdNumDisks` are the canonical array keys in `var.ini`.)

- [ ] **Step 7: Integrate into the system collector** (`daemon/services/collectors/system.go`). The system collector reads `/proc`. Validate the _result shape_ instead of INI keys — after building `systemInfo`:

```go
 switch {
 case systemInfo == nil:
  c.ctx.Platform.Report("system", dto.SourceUnavailable, "no system data", nil)
  return
 case systemInfo.Hostname == "" || systemInfo.CPUCores == 0:
  c.ctx.Platform.Report("system", dto.SourceDegraded, "incomplete system info (hostname/cpu missing)", nil)
 default:
  c.ctx.Platform.Healthy("system")
 }
 systemInfo.SourceStatus = c.ctx.Platform.StatusFor("system")
 domain.Publish(c.ctx.Hub, constants.TopicSystemUpdate, systemInfo)
```

- [ ] **Step 8: Gate + commit**

```bash
go test ./... && go vet ./...
git add daemon/services/collectors/validators.go daemon/services/collectors/validators_test.go daemon/services/collectors/system.go daemon/services/collectors/array.go daemon/dto/system.go daemon/dto/array.go
git commit -m "feat(resilience): shape-validate + report status in system and array collectors"
```

CodeRabbit review; fix findings.

---

### Task 4: Shape-validate + report in `disk` and `shares` collectors

**Goal:** Same pattern for `disk` (source `disks.ini` / smartctl) and `shares` (source `shares.ini`).

**Files:**

- Modify: `daemon/services/collectors/disk.go`, `daemon/services/collectors/shares.go`, `daemon/dto/disk.go`, `daemon/dto/share.go`

**Acceptance Criteria:**

- [ ] `dto.DiskInfo` (list element) and `dto.ShareInfo` carry the inline flag.
- [ ] Disk collector reports `unavailable` if `disks.ini` can't be read, `degraded` if it yields zero disks, else `healthy`; still publishes best-effort.
- [ ] Shares collector reports analogously from `shares.ini`.

**Verify:** `go test ./daemon/services/collectors/ -v` → PASS, plus `go build ./...`.

**Steps:**

- [ ] **Step 1: Inline flags.** Add `SourceStatus *SourceStatus json:"source_status,omitempty"` to `dto.DiskInfo` (`daemon/dto/disk.go`) and `dto.ShareInfo` (`daemon/dto/share.go`).

- [ ] **Step 2: Disk collector** (`daemon/services/collectors/disk.go`). After parsing `disks.ini` and building the disk slice:

```go
 parsed, err := lib.ParseINIFile(constants.DisksIni)
 if err != nil {
  c.ctx.Platform.Report("disk", dto.SourceUnavailable, "cannot read disks.ini", err)
  return
 }
 // ... build disks []dto.DiskInfo (unchanged) ...
 if len(disks) == 0 {
  c.ctx.Platform.Report("disk", dto.SourceDegraded, "disks.ini yielded zero disks", nil)
 } else {
  c.ctx.Platform.Healthy("disk")
 }
 if st := c.ctx.Platform.StatusFor("disk"); st != nil {
  for i := range disks {
   disks[i].SourceStatus = st
  }
 }
 domain.Publish(c.ctx.Hub, constants.TopicDiskListUpdate, disks)
 _ = parsed
```

(If the disk collector derives disks from `disks.ini` keys, keep the existing derivation; only add the validation/report/flag lines.)

- [ ] **Step 3: Shares collector** (`daemon/services/collectors/shares.go`). Same shape against `constants.SharesIni`:

```go
 parsed, err := lib.ParseINIFile(constants.SharesIni)
 if err != nil {
  c.ctx.Platform.Report("shares", dto.SourceUnavailable, "cannot read shares.ini", err)
  return
 }
 // ... build shares (unchanged) ...
 if len(shares) == 0 {
  c.ctx.Platform.Report("shares", dto.SourceDegraded, "shares.ini yielded zero shares", nil)
 } else {
  c.ctx.Platform.Healthy("shares")
 }
 if st := c.ctx.Platform.StatusFor("shares"); st != nil {
  for i := range shares {
   shares[i].SourceStatus = st
  }
 }
 domain.Publish(c.ctx.Hub, constants.TopicShareListUpdate, shares)
 _ = parsed
```

- [ ] **Step 4: Gate + commit**

```bash
go test ./... && go vet ./...
git add daemon/services/collectors/disk.go daemon/services/collectors/shares.go daemon/dto/disk.go daemon/dto/share.go
git commit -m "feat(resilience): shape-validate + report status in disk and shares collectors"
```

CodeRabbit review; fix findings.

---

### Task 5: Shape-validate + report in `docker` and `vm` collectors · USER-GATE checkpoint

**Goal:** Same pattern for `docker` (Docker SDK) and `vm` (libvirt/virsh), then the first on-hardware verification that healthy data still flows and statuses report healthy on a real server.

**USER-ORDERED GATE — NON-SKIPPABLE.** This task was requested by the user in the current conversation ("Test, Build, Verify on Unraid using Ansible and run CodeRabbit"). It MUST NOT be closed by walking around it, declaring it "verified inline", or substituting a cheaper check. Close only after every acceptance criterion has been re-validated on hardware with output captured.

**Files:**

- Modify: `daemon/services/collectors/docker.go`, `daemon/services/collectors/vm.go`, `daemon/dto/container.go`, `daemon/dto/vm.go`

**Acceptance Criteria:**

- [ ] `dto.ContainerInfo` and `dto.VMInfo` carry the inline flag.
- [ ] Docker collector reports `unavailable` when the Docker client/daemon is unreachable; `healthy` otherwise. VM collector reports `unavailable` when libvirt/virsh is unreachable; `healthy` otherwise. Both still publish best-effort.
- [ ] **On Unraid (`192.168.20.21`):** `ansible-playbook ... --tags build,deploy,verify` passes; `curl /api/v1/system` etc. return data with no `source_status` (healthy); agent log shows the resilience startup line.

**Verify:** `go test ./...` PASS **and** `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify` → PLAY RECAP `failed=0`.

**Steps:**

- [ ] **Step 1: Inline flags** on `dto.ContainerInfo` (`daemon/dto/container.go`) and `dto.VMInfo` (`daemon/dto/vm.go`).

- [ ] **Step 2: Docker collector** (`daemon/services/collectors/docker.go`). Where it lists containers via the Docker client:

```go
 containers, err := c.listContainers(ctx) // existing call
 if err != nil {
  c.ctx.Platform.Report("docker", dto.SourceUnavailable, "docker daemon unreachable", err)
  return
 }
 c.ctx.Platform.Healthy("docker")
 if st := c.ctx.Platform.StatusFor("docker"); st != nil {
  for _, ci := range containers {
   ci.SourceStatus = st
  }
 }
 // ... existing publish ...
```

(Match the actual list call/return type; the report/flag lines are the additions.)

- [ ] **Step 3: VM collector** (`daemon/services/collectors/vm.go`). Where it queries libvirt:

```go
 vms, err := c.listVMs() // existing call
 if err != nil {
  c.ctx.Platform.Report("vm", dto.SourceUnavailable, "libvirt unreachable", err)
  return
 }
 c.ctx.Platform.Healthy("vm")
 if st := c.ctx.Platform.StatusFor("vm"); st != nil {
  for _, vm := range vms {
   vm.SourceStatus = st
  }
 }
 // ... existing publish ...
```

> Note: VM being absent on a host with no VMs is normal — treat "no VMs configured" as `healthy`, only `unavailable` on an actual libvirt connection error.

- [ ] **Step 4: Local gate + commit**

```bash
go test ./... && go vet ./... && make local
git add daemon/services/collectors/docker.go daemon/services/collectors/vm.go daemon/dto/container.go daemon/dto/vm.go
git commit -m "feat(resilience): shape-validate + report status in docker and vm collectors"
```

- [ ] **Step 5: Deploy + verify on Unraid (the gate)**

```bash
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify
```

Expected: `PLAY RECAP ... failed=0`. Then capture proof:

```bash
ansible unraid -i ansible/inventory.yml -m raw -a "grep -i resilience /var/log/unraid-management-agent.log | tail -3"
curl -s http://192.168.20.21:8043/api/v1/system | grep -c source_status   # expect 0 (healthy)
```

- [ ] **Step 6: CodeRabbit review** `coderabbit review --agent -t uncommitted`; fix findings; re-commit if needed.

```json:metadata
{"files": ["daemon/services/collectors/docker.go","daemon/services/collectors/vm.go","daemon/dto/container.go","daemon/dto/vm.go"], "verifyCommand": "ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["ContainerInfo and VMInfo carry inline source_status flag","docker/vm collectors report unavailable on client errors, healthy otherwise, still publish","Unraid verify role passes failed=0 and /api/v1/system has no source_status when healthy"], "userGate": true, "tags": ["user-gate"], "gateScope": "this-step"}
```

---

### Task 6: Capability-gate the array/disk/docker/vm controllers

**Goal:** Control operations check capability _before_ acting and return a clear `"<subsystem> unavailable: <reason>"` error instead of a cryptic shell failure.

**Files:**

- Modify: `daemon/services/controllers/array.go`, `daemon/services/controllers/docker.go`, `daemon/services/controllers/vm.go` (+ disk spin controller wherever `disk_spin_*` lives)
- Test: `daemon/services/controllers/array_test.go` (add a gating test)

**Acceptance Criteria:**

- [ ] Array control returns a typed "unavailable" error when neither `/proc/mdcmd` nor the `mdcmd` binary is present (simulated in test via a seam).
- [ ] Docker/VM/disk control return clear unavailable errors when their binary/socket is absent.
- [ ] Existing happy-path tests still pass.

**Verify:** `go test ./daemon/services/controllers/ -v` → PASS

**Steps:**

- [ ] **Step 1: Add a guard helper** in `daemon/services/controllers/` (new `guard.go`):

```go
package controllers

import (
 "fmt"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

// requireBinary returns a typed error if the binary is unavailable.
func requireBinary(subsystem, binaryPath string) error {
 if !platform.BinaryExists(binaryPath) {
  return fmt.Errorf("%s unavailable: %s not found", subsystem, binaryPath)
 }
 return nil
}
```

- [ ] **Step 2: Gate array control.** In `array.go`, at the top of the exported start/stop/parity entrypoints, before `mdcmdExec`:

```go
 if !platform.PathExists("/proc/mdcmd") {
  if err := requireBinary("array", constants.MdcmdBin); err != nil {
   return err
  }
 }
```

- [ ] **Step 3: Gate docker/vm/disk control** similarly at each exported control entrypoint:

```go
 if err := requireBinary("docker", constants.DockerBin); err != nil { return err }   // docker.go
 if err := requireBinary("vm", constants.VirshBin); err != nil { return err }        // vm.go
 if err := requireBinary("disk", constants.SmartctlBin); err != nil { return err }   // disk spin controller (uses smartctl/hdparm — match actual binary)
```

- [ ] **Step 4: Add a gating test** in `array_test.go`:

```go
func TestArrayControlGatedWhenUnavailable(t *testing.T) {
 // With a bogus binary path the guard must refuse. Validate the helper directly:
 if err := requireBinary("array", "/nonexistent/mdcmd"); err == nil {
  t.Fatal("expected unavailable error for missing binary")
 }
}
```

- [ ] **Step 5: Gate + commit**

```bash
go test ./... && go vet ./...
git add daemon/services/controllers/
git commit -m "feat(resilience): capability-gate array/disk/docker/vm control operations"
```

CodeRabbit review; fix findings.

---

### Task 7: REST `/api/v1/diagnostics/self-test` + health-report summary · USER-GATE checkpoint

**Goal:** A REST endpoint returns the full capability + per-subsystem status snapshot, the health report includes a degraded summary, and it's verified healthy on hardware.

**USER-ORDERED GATE — NON-SKIPPABLE.** Requested by the user (verify on Unraid). Close only after the live self-test returns `overall_state: healthy` on `192.168.20.21`, with output captured.

**Files:**

- Create: `daemon/services/api/diagnostics.go`, `daemon/services/api/diagnostics_test.go`
- Modify: `daemon/services/api/server.go` (route), `daemon/services/api/handlers.go` (health report), `ansible/roles/verify/tasks/*.yml`

**Acceptance Criteria:**

- [ ] `GET /api/v1/diagnostics/self-test` returns `{unraid_version, overall_state, capabilities, subsystems[], timestamp}` (HTTP 200).
- [ ] `/api/v1/health/report` includes `degraded_subsystems` (count + names).
- [ ] **On Unraid:** the endpoint returns `overall_state: "healthy"`; the ansible verify role asserts it and passes.

**Verify:** `go test ./daemon/services/api/ -run TestSelfTest -v` PASS **and** live `curl … | jq .overall_state` → `"healthy"`.

**Steps:**

- [ ] **Step 1: Response DTO + handler `daemon/services/api/diagnostics.go`**

```go
package api

import (
 "net/http"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

type selfTestResponse struct {
 UnraidVersion string             `json:"unraid_version"`
 OverallState  dto.SourceState    `json:"overall_state"`
 Capabilities  dto.Capabilities   `json:"capabilities"`
 Subsystems    []dto.SourceStatus `json:"subsystems"`
 Timestamp     time.Time          `json:"timestamp"`
}

func (s *Server) handleSelfTest(w http.ResponseWriter, _ *http.Request) {
 reg := s.ctx.Platform
 respondJSON(w, http.StatusOK, selfTestResponse{
  UnraidVersion: reg.Capabilities().UnraidVersion,
  OverallState:  reg.OverallState(),
  Capabilities:  reg.Capabilities(),
  Subsystems:    reg.Snapshot(),
  Timestamp:     time.Now(),
 })
}
```

- [ ] **Step 2: Register route** in `server.go` (with the other GET routes):

```go
 api.HandleFunc("/diagnostics/self-test", s.handleSelfTest).Methods("GET")
```

- [ ] **Step 3: Health-report summary.** In the existing health-report handler (`handlers.go`), add to the response payload:

```go
 "degraded_subsystems": map[string]any{
  "count": s.ctx.Platform.DegradedCount(),
  "items": s.ctx.Platform.Snapshot(),
 },
```

- [ ] **Step 4: Test `daemon/services/api/diagnostics_test.go`**

```go
package api

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

func TestSelfTestEndpoint(t *testing.T) {
 reg := platform.NewRegistry()
 reg.Healthy("system")
 reg.Report("array", dto.SourceDegraded, "stale", nil)
 s := &Server{ctx: &domain.Context{Platform: reg}}

 req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/self-test", nil)
 rec := httptest.NewRecorder()
 s.handleSelfTest(rec, req)

 if rec.Code != http.StatusOK {
  t.Fatalf("status = %d, want 200", rec.Code)
 }
 var out selfTestResponse
 if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
  t.Fatal(err)
 }
 if out.OverallState != dto.SourceDegraded || len(out.Subsystems) != 2 {
  t.Fatalf("unexpected self-test: %+v", out)
 }
}
```

(If `Server{ctx:...}` needs more non-nil fields to construct, use the package's existing test constructor instead and inject the registry.)

- [ ] **Step 5: Ansible verify assertion.** Add to a verify task file (e.g. `ansible/roles/verify/tasks/main.yml`):

```yaml
- name: Verify diagnostics self-test
  delegate_to: localhost
  ansible.builtin.uri:
    url: "http://{{ ansible_host }}:{{ api_port }}/api/v1/diagnostics/self-test"
    method: GET
    status_code: 200
    return_content: true
  register: selftest_result

- name: Assert self-test overall_state is healthy
  ansible.builtin.assert:
    that:
      - selftest_result.json.overall_state == "healthy"
    fail_msg: "Self-test reports degraded: {{ selftest_result.json.subsystems | default([]) }}"
    success_msg: "Self-test healthy (Unraid {{ selftest_result.json.unraid_version }})"
```

- [ ] **Step 6: Local gate, deploy + verify (the gate), CodeRabbit**

```bash
go test ./... && make local
git add daemon/services/api/diagnostics.go daemon/services/api/diagnostics_test.go daemon/services/api/server.go daemon/services/api/handlers.go ansible/roles/verify/tasks/
git commit -m "feat(resilience): add /diagnostics/self-test endpoint + health-report degraded summary"
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify
curl -s http://192.168.20.21:8043/api/v1/diagnostics/self-test | python3 -m json.tool
coderabbit review --agent -t uncommitted
```

Expected: verify `failed=0`; `overall_state: "healthy"`.

```json:metadata
{"files": ["daemon/services/api/diagnostics.go","daemon/services/api/server.go","daemon/services/api/handlers.go","ansible/roles/verify/tasks/main.yml"], "verifyCommand": "ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["GET /api/v1/diagnostics/self-test returns 200 with unraid_version/overall_state/capabilities/subsystems","health/report includes degraded_subsystems count+items","live self-test overall_state == healthy on 192.168.20.21"], "userGate": true, "tags": ["user-gate"], "gateScope": "this-step"}
```

---

### Task 8: MCP `run_self_test` tool + Prometheus gauges

**Goal:** AI clients can call `run_self_test`; Prometheus exposes per-subsystem status + degraded count.

**Files:**

- Modify: `daemon/services/mcp/server.go`, `daemon/services/api/metrics.go`

**Acceptance Criteria:**

- [ ] `run_self_test` (read-only) returns the same payload as the REST endpoint → MCP tool count becomes 122.
- [ ] `/metrics` exposes `unraid_subsystem_status{subsystem="…"}` (0/1/2) and `unraid_degraded_subsystem_count`.

**Verify:** `go test ./daemon/services/mcp/ ./daemon/services/api/ -count=1` PASS; live `curl /metrics | grep unraid_subsystem_status`.

**Steps:**

- [ ] **Step 1: Register the MCP tool** in `server.go` `registerMonitoringTools()`:

```go
 mcp.AddTool(s.mcpServer, &mcp.Tool{
  Name:        "run_self_test",
  Description: "Run a self-test of the agent's data sources: returns the detected Unraid version, overall health, probed capabilities, and per-subsystem source status (healthy/degraded/unavailable). Use to check whether an OS update has broken any collector.",
  Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
 }, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
  reg := s.ctx.Platform
  return jsonResult(map[string]any{
   "unraid_version": reg.Capabilities().UnraidVersion,
   "overall_state":  reg.OverallState(),
   "capabilities":   reg.Capabilities(),
   "subsystems":     reg.Snapshot(),
  })
 })
```

- [ ] **Step 2: Prometheus gauges** in `metrics.go` — add to the `var (...)` block:

```go
 subsystemStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
  Name: "unraid_subsystem_status",
  Help: "Data-source health per subsystem (0=healthy,1=degraded,2=unavailable)",
 }, []string{"subsystem"})
 degradedSubsystemCount = prometheus.NewGauge(prometheus.GaugeOpts{
  Name: "unraid_degraded_subsystem_count",
  Help: "Number of subsystems whose data source is not healthy",
 })
```

Register them in `init()` (`metricsRegistry.MustRegister(... subsystemStatus, degradedSubsystemCount)`), and update them inside `updateMetrics()`:

```go
 for _, st := range s.ctx.Platform.Snapshot() {
  subsystemStatus.WithLabelValues(st.Subsystem).Set(float64(st.State.Severity()))
 }
 degradedSubsystemCount.Set(float64(s.ctx.Platform.DegradedCount()))
```

- [ ] **Step 3: Gate + commit**

```bash
go test ./... && go vet ./... && make local
git add daemon/services/mcp/server.go daemon/services/api/metrics.go
git commit -m "feat(resilience): add run_self_test MCP tool + Prometheus subsystem-status gauges"
```

CodeRabbit review; fix findings. (Hardware verify folded into Task 9's checkpoint.)

---

### Task 9: Auto-alert on degradation (`subsystem_degraded`) · USER-GATE checkpoint

**Goal:** When any subsystem is degraded/unavailable, a built-in enabled alert fires through the existing alerting engine — verified by inducing a degradation on hardware.

**USER-ORDERED GATE — NON-SKIPPABLE.** Requested by the user (verify on Unraid). Close only after an induced degradation produces a fired alert on the live server, with output captured.

**Files:**

- Modify: `daemon/dto/alert.go` (AlertEnv), the AlertEnv builder (locate via grep), `daemon/services/alerting/templates.go` or the default-rules seed.

**Acceptance Criteria:**

- [ ] `AlertEnv.DegradedSubsystemCount` exists and is populated from `ctx.Platform.DegradedCount()`.
- [ ] A built-in **enabled** rule `subsystem_degraded` (`DegradedSubsystemCount > 0`, severity warning) ships by default.
- [ ] **On Unraid:** inducing a degradation (e.g. temporarily point a probe at a missing path via a test build, or stop a monitored service) flips self-test to degraded AND the alert appears in `get_firing_alerts` / `/api/v1/alerts`.

**Verify:** `go test ./daemon/services/alerting/ -v` PASS; live: induce degradation → `curl /api/v1/alerts/firing` shows `subsystem_degraded`.

**Steps:**

- [ ] **Step 1: Add the field** to `dto.AlertEnv` (`daemon/dto/alert.go`):

```go
 DegradedSubsystemCount int `expr:"DegradedSubsystemCount"`
```

- [ ] **Step 2: Populate it.** Find the AlertEnv builder:

```bash
grep -rn "AlertEnv{" daemon/services/alerting/
```

In that builder, set `env.DegradedSubsystemCount = <registry>.DegradedCount()` (thread the `*platform.Registry` in via the engine's existing `ctx`/provider — the engine already has access to caches; add the registry the same way).

- [ ] **Step 3: Ship the built-in rule.** Add to the default rules (the enabled set, not the disabled templates). If defaults are seeded in `alerting.NewStore`/engine init, add:

```go
 {ID: "subsystem-degraded", Name: "Agent data source degraded", Expression: "DegradedSubsystemCount > 0", Severity: "warning", Enabled: true}
```

If only `AlertRuleTemplates()` exists, also add it there with `Enabled: true` so it is active out of the box.

- [ ] **Step 4: Unit test** in `daemon/services/alerting/` asserting the expression compiles and triggers when `DegradedSubsystemCount=1` and not when `0` (mirror existing evaluator tests).

- [ ] **Step 5: Gate + deploy + induce + verify**

```bash
go test ./... && make local
git add daemon/dto/alert.go daemon/services/alerting/
git commit -m "feat(resilience): built-in subsystem_degraded alert + AlertEnv degraded count"
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify
```

Induce a degradation safely (non-destructive): temporarily disable a monitored service the docker/vm collector depends on, OR deploy a one-off build whose probe list points one entry at a bogus path; then:

```bash
curl -s http://192.168.20.21:8043/api/v1/diagnostics/self-test | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["overall_state"])'   # degraded
curl -s http://192.168.20.21:8043/api/v1/alerts/firing | grep subsystem
```

Restore the induced condition and confirm self-test returns to healthy and the alert resolves.

- [ ] **Step 6: CodeRabbit review**; fix findings.

```json:metadata
{"files": ["daemon/dto/alert.go","daemon/services/alerting/templates.go"], "verifyCommand": "ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["AlertEnv.DegradedSubsystemCount populated from registry","built-in enabled subsystem_degraded rule ships","induced degradation on hardware fires the alert and resolves on recovery"], "userGate": true, "tags": ["user-gate"], "gateScope": "this-step"}
```

---

### Task 10: Golden fixtures + breakage tests + docs + final verify · USER-GATE checkpoint

**Goal:** Capture sanitized fixtures, add the crux breakage-simulation tests, wire CI, update docs, do a final full hardware verification + CodeRabbit, and only then update the CHANGELOG.

**USER-ORDERED GATE — NON-SKIPPABLE.** Requested by the user ("once done and tested and everything is working update the CHANGELOG.md"). The CHANGELOG entry is the LAST action and only after full hardware verification + clean CodeRabbit, with output captured.

**Files:**

- Create: `daemon/services/collectors/testdata/fixtures/unraid-<ver>/{var.ini,disks.ini,shares.ini,network.ini}`, `daemon/services/collectors/fixtures_test.go`
- Modify: `docs/guides/configuration.md`, `docs/integrations/mcp.md` (121→122 tools; add `run_self_test`; add self-test endpoint), `AGENTS.md`, `skills/unraid-management-agent/references/mcp-tools.md` + `diagnostics.md`, `CHANGELOG.md` (LAST)

**Acceptance Criteria:**

- [ ] Fixtures captured from the live box are sanitized (no GUID/license/serials/WG keys) and committed.
- [ ] Parser+validator tests pass against fixtures; **breakage-simulation tests** assert degraded/unavailable + best-effort + no panic on malformed/empty/missing fixtures.
- [ ] Docs updated; MCP tool count corrected to 122.
- [ ] **Final on Unraid:** full `--tags build,deploy,verify` passes (incl. self-test healthy); CodeRabbit clean; **then** CHANGELOG updated.

**Verify:** `go test ./daemon/services/collectors/ -run TestFixtures -v` PASS; final `ansible-playbook … --tags build,deploy,verify` `failed=0`.

**Steps:**

- [ ] **Step 1: Capture + sanitize fixtures** from the live server:

```bash
V=$(ansible unraid -i ansible/inventory.yml -m raw -a "cat /etc/unraid-version" 2>/dev/null | grep -oE '[0-9]+\.[0-9.]+' | head -1)
mkdir -p daemon/services/collectors/testdata/fixtures/unraid-$V
for f in var.ini disks.ini shares.ini network.ini; do
  ansible unraid -i ansible/inventory.yml -m raw -a "cat /var/local/emhttp/$f" 2>/dev/null \
    | grep -vE 'CHANGED|SUCCESS|shared connection' \
    | sed -E 's/(regGUID|regTo|regKey|flashGUID|serial|csrf_token)=.*/\1=REDACTED/I' \
    > daemon/services/collectors/testdata/fixtures/unraid-$V/$f
done
```

Manually review each file for any residual secrets before committing.

- [ ] **Step 2: Fixture + breakage tests `daemon/services/collectors/fixtures_test.go`**

```go
package collectors

import (
 "os"
 "path/filepath"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
)

// TestFixturesParseAndValidate runs the array validator against every captured
// fixture dir; each must contain a parseable var.ini with the required keys.
func TestFixturesParseAndValidate(t *testing.T) {
 dirs, _ := filepath.Glob("testdata/fixtures/unraid-*")
 if len(dirs) == 0 {
  t.Skip("no fixtures captured yet")
 }
 for _, d := range dirs {
  parsed, err := lib.ParseINIFile(filepath.Join(d, "var.ini"))
  if err != nil {
   t.Fatalf("%s: parse var.ini: %v", d, err)
  }
  if ok, reason := validateRequiredKeys(parsed, "mdState", "mdNumDisks"); !ok {
   t.Errorf("%s: var.ini failed validation: %s", d, reason)
  }
 }
}

// TestFixturesBreakage is the crux: malformed/empty input must validate as
// not-ok (degraded) WITHOUT panicking.
func TestFixturesBreakage(t *testing.T) {
 tmp := filepath.Join(t.TempDir(), "var.ini")
 if err := os.WriteFile(tmp, []byte("garbage=1\n"), 0o600); err != nil {
  t.Fatal(err)
 }
 parsed, err := lib.ParseINIFile(tmp)
 if err != nil {
  t.Fatalf("parse should not error on malformed-but-valid-ini: %v", err)
 }
 if ok, _ := validateRequiredKeys(parsed, "mdState", "mdNumDisks"); ok {
  t.Fatal("expected validation to fail on garbage var.ini")
 }
}
```

- [ ] **Step 3: CI.** Confirm `go test ./...` runs in the existing GitHub Actions workflow (it already runs the suite); fixtures live under `testdata/` so they ship with the package automatically. No extra CI wiring needed beyond confirming the job runs `go test ./...`.

- [ ] **Step 4: Docs.** Update:

  - `docs/integrations/mcp.md`: "121 total" → "122 total"; add `run_self_test` to the monitoring tools table; add `GET /api/v1/diagnostics/self-test` to the REST surface.
  - `skills/unraid-management-agent/references/mcp-tools.md` (122; add `run_self_test`) and `diagnostics.md` (mention the self-test tool); bump the SKILL.md "121 tools" mentions.
  - `docs/guides/configuration.md`: add an "OS-resilience / self-test" subsection (endpoint, Prometheus metrics, the `subsystem_degraded` alert).
  - `AGENTS.md`: note the `daemon/platform` package in the structure + the MCP count.

- [ ] **Step 5: FINAL gate**

```bash
go test ./... && go vet ./... && gofmt -l daemon/ && make local
git add daemon/services/collectors/testdata daemon/services/collectors/fixtures_test.go docs/ skills/ AGENTS.md
git commit -m "test(resilience): golden fixtures + breakage tests; docs for self-test + 122 tools"
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify   # failed=0, self-test healthy
coderabbit review --agent -t uncommitted   # clean
```

- [ ] **Step 6: CHANGELOG (LAST, only after the above all pass)** — add under a new/existing Unreleased (or next version) section:

```markdown
### Added

- **OS-resilience & self-diagnostics** (sub-project B) — the agent now detects
  when an Unraid data source breaks (moved path, changed format, missing binary)
  via capability/shape probing, degrades gracefully (serves best-effort data,
  never silently wrong/empty), and surfaces it: `GET /api/v1/diagnostics/self-test`,
  the `run_self_test` MCP tool, inline `source_status` flags, a
  `source_status_changed` WebSocket event, `unraid_subsystem_status` /
  `unraid_degraded_subsystem_count` Prometheus metrics, and a built-in
  `subsystem_degraded` alert. Self-contained — no dependency on the official
  Unraid API. Verified live on Unraid.
```

```bash
git add CHANGELOG.md && git commit -m "docs(changelog): OS-resilience & self-diagnostics (sub-project B)"
```

```json:metadata
{"files": ["daemon/services/collectors/fixtures_test.go","docs/integrations/mcp.md","CHANGELOG.md"], "verifyCommand": "ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["sanitized fixtures committed","breakage-simulation tests pass (degraded + no panic)","docs updated incl. 122 MCP tools","final Unraid verify passes + CodeRabbit clean, THEN CHANGELOG updated last"], "userGate": true, "tags": ["user-gate"], "gateScope": "whole-plan"}
```

---

## Self-review notes

- **Spec coverage:** registry/detector/resolver (T0,T1) ✓; context/orchestrator wiring + WS topic (T2) ✓; 6 collectors (T3,T4,T5) ✓; 4 controllers (T6) ✓; REST self-test + health summary (T7) ✓; MCP tool + Prometheus (T8) ✓; WS event (T2+T8) ✓; alerting (T9) ✓; fixtures + breakage tests + docs (T10) ✓; mandatory verify gate on every task ✓; no official-API dependency ✓; healthy responses unchanged (omitempty + StatusFor returns nil when healthy) ✓.
- **Cycle-safety:** `platform` imports only `dto`+`logger`+stdlib; Unraid probe list lives in `daemon/services/probes.go` (imports `constants`+`platform`), so `domain→platform→dto` has no back-edge. Verified by the `go list -deps` check in T1.
- **Type consistency:** `SourceState`/`SourceStatus`/`Capabilities` defined in T0–T1 and used identically in T2–T10; `Registry` methods (`Report`, `Healthy`, `StatusFor`, `Snapshot`, `DegradedCount`, `OverallState`, `Capabilities`, `SetNotifier`, `SetCapabilities`) referenced consistently.
- **Adjust-at-implementation flags:** exact required INI key names per collector (T3/T4) and the AlertEnv builder location (T9) are marked with `grep`/verification steps rather than guessed.
