# Drive-Aware Fan Curves + Sensor Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a fan curve read its temperature from a selected set of drives (max of the active ones, from `disks.ini`), fail over to a per-profile hwmon sensor when those drives are spun down, and expose a discovery endpoint listing every hwmon sensor and drive.

**Architecture:** A tagged `FanTempSource` value object describes each curve's input. The curve engine gains a `resolveTemp` step (primary hwmon-or-drives → per-profile hwmon fallback → "no reading"). Drive temps come from a new injected `DriveTempProvider` backed by a shared `lib.ReadDiskTemps()` parser. A read-only `GET /fans/sensors` endpoint and `get_fan_sensors` MCP tool enumerate sensors and drives. The emergency thermal cutoff stays hwmon-only.

**Tech Stack:** Go 1.26, gorilla/mux, official MCP Go SDK, sysfs hwmon, Unraid `disks.ini`. Spec: `docs/superpowers/specs/2026-06-07-drive-aware-fan-curves-design.md`.

---

## File Structure

| File                                        | Responsibility                                                                                                         |
| ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `daemon/lib/disktemp.go` (new)              | Parse `disks.ini` → `map[id]DiskTemp` (id, device, temp, spun-down)                                                    |
| `daemon/lib/hwmon.go`                       | Add `DiscoverHwmonTempSensors()` (path + label + value + plausibility)                                                 |
| `daemon/dto/fan.go`                         | `FanTempSource`(+`Type`), catalog DTOs, extend `FanDevice` + `FanProfileRequest`                                       |
| `daemon/lib/validation.go`                  | `ValidateFanTempSource`                                                                                                |
| `daemon/services/controllers/fan_curves.go` | `fanCurveAssignment.Source`, `DriveTempProvider`, `resolveTemp`, `AssignProfile`, `applyCurves`, `GetAssignmentSource` |
| `daemon/services/controllers/fan.go`        | Update restore loop, `SetProfile` signature, `GetStatus` source annotation, `GetSensorCatalog`                         |
| `daemon/services/controllers/fan_config.go` | Custom `UnmarshalJSON` migration for legacy assignments                                                                |
| `daemon/services/api/handlers.go`           | `handleFanSensors`; source resolution in assign handler                                                                |
| `daemon/services/api/server.go`             | Register `GET /fans/sensors`                                                                                           |
| `daemon/dto/mcp.go`                         | Extend `MCPFanProfileArgs` with source fields                                                                          |
| `daemon/services/mcp/server.go`             | `get_fan_sensors` tool; source params on `set_fan_profile`                                                             |
| `CHANGELOG.md`                              | Entry (last)                                                                                                           |

---

### Task 1: Drive-temperature parser (`lib.ReadDiskTemps`)

**Goal:** A standalone, fixture-testable parser of `disks.ini` returning each disk's temperature and spun-down state.

**Files:**

- Create: `daemon/lib/disktemp.go`
- Test: `daemon/lib/disktemp_test.go`

**Acceptance Criteria:**

- [ ] `ReadDiskTemps(path)` parses section headers `["disk1"]` → ID `disk1`, plus `device=` and `temp=`.
- [ ] `temp="*"` or empty → `SpunDown:true`, `TempC:0`. Numeric temp → `SpunDown:false`, parsed value.
- [ ] Missing file → returns a non-nil error and an empty (non-nil) map.
- [ ] A package-level `ReadDiskTemps()` (no args) reads the real `/boot/config/disks.ini` via a const default path.

**Verify:** `go test ./daemon/lib/ -run TestReadDiskTemps -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** — `daemon/lib/disktemp_test.go`

```go
package lib

import (
 "os"
 "path/filepath"
 "testing"
)

func TestReadDiskTempsFromFile(t *testing.T) {
 const sample = `["disk1"]
name="disk1"
device="sdb"
temp="38"
["disk2"]
device="sdc"
temp="*"
["cache"]
device="nvme0n1"
temp=""
`
 dir := t.TempDir()
 p := filepath.Join(dir, "disks.ini")
 if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
  t.Fatal(err)
 }

 got, err := ReadDiskTempsFromFile(p)
 if err != nil {
  t.Fatalf("unexpected error: %v", err)
 }

 if d := got["disk1"]; d.Device != "sdb" || d.TempC != 38 || d.SpunDown {
  t.Errorf("disk1: got %+v, want device=sdb temp=38 spundown=false", d)
 }
 if d := got["disk2"]; !d.SpunDown || d.TempC != 0 {
  t.Errorf("disk2 (temp=*): got %+v, want spundown=true temp=0", d)
 }
 if d := got["cache"]; !d.SpunDown {
  t.Errorf("cache (empty temp): got %+v, want spundown=true", d)
 }
}

func TestReadDiskTempsMissingFile(t *testing.T) {
 got, err := ReadDiskTempsFromFile(filepath.Join(t.TempDir(), "nope.ini"))
 if err == nil {
  t.Fatal("expected error for missing file")
 }
 if got == nil {
  t.Fatal("expected non-nil (empty) map even on error")
 }
}
```

- [ ] **Step 2: Run test, confirm it fails** — `go test ./daemon/lib/ -run TestReadDiskTemps -v` → FAIL (undefined: ReadDiskTempsFromFile)

- [ ] **Step 3: Implement** — `daemon/lib/disktemp.go`

```go
package lib

import (
 "bufio"
 "fmt"
 "os"
 "strconv"
 "strings"
)

// DiskTempsPath is the default Unraid disks.ini location.
const DiskTempsPath = "/boot/config/disks.ini"

// DiskTemp is the temperature and spin state of a single Unraid disk,
// parsed from disks.ini. It never wakes a drive: Unraid writes "*" for a
// spun-down disk, which this maps to SpunDown=true.
type DiskTemp struct {
 ID       string  // disks.ini section name: "disk1", "cache", "parity"
 Device   string  // "sdb"
 TempC    float64 // 0 when unavailable
 SpunDown bool    // disks.ini temp == "*" or empty
}

// ReadDiskTemps parses the default disks.ini (/boot/config/disks.ini).
func ReadDiskTemps() (map[string]DiskTemp, error) {
 return ReadDiskTempsFromFile(DiskTempsPath)
}

// ReadDiskTempsFromFile parses the given disks.ini and returns temps keyed by
// disk ID (the section-header name). The returned map is always non-nil.
func ReadDiskTempsFromFile(path string) (map[string]DiskTemp, error) {
 result := make(map[string]DiskTemp)

 // #nosec G304 -- path is a fixed const in production; tests pass a temp file.
 file, err := os.Open(path)
 if err != nil {
  return result, fmt.Errorf("open disks.ini: %w", err)
 }
 defer func() { _ = file.Close() }()

 var cur *DiskTemp
 flush := func() {
  if cur != nil && cur.ID != "" {
   result[cur.ID] = *cur
  }
 }

 scanner := bufio.NewScanner(file)
 for scanner.Scan() {
  line := strings.TrimSpace(scanner.Text())

  if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
   flush()
   id := strings.Trim(line, "[]")
   id = strings.Trim(id, `"`)
   cur = &DiskTemp{ID: id}
   continue
  }

  if cur == nil || !strings.Contains(line, "=") {
   continue
  }
  parts := strings.SplitN(line, "=", 2)
  key := strings.TrimSpace(parts[0])
  val := strings.Trim(strings.TrimSpace(parts[1]), `"`)

  switch key {
  case "device":
   cur.Device = val
  case "temp":
   // Unraid writes "*" (or empty) for a spun-down disk — do NOT wake it.
   if val == "*" || val == "" {
    cur.SpunDown = true
   } else if t, perr := strconv.ParseFloat(val, 64); perr == nil {
    cur.TempC = t
   }
  }
 }
 flush()

 if err := scanner.Err(); err != nil {
  return result, fmt.Errorf("scan disks.ini: %w", err)
 }
 return result, nil
}
```

- [ ] **Step 4: Run test, confirm PASS** — `go test ./daemon/lib/ -run TestReadDiskTemps -v` → PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/lib/disktemp.go daemon/lib/disktemp_test.go
git commit -m "feat(lib): add disks.ini drive-temperature parser"
```

---

### Task 2: hwmon temp-sensor discovery (`lib.DiscoverHwmonTempSensors`)

**Goal:** Enumerate every hwmon `tempN_input` with its path, label, current value, and a plausibility flag — including implausible/unreliable sensors (flagged, not hidden).

**Files:**

- Modify: `daemon/lib/hwmon.go`
- Test: `daemon/lib/hwmon_test.go`

**Acceptance Criteria:**

- [ ] Returns one entry per readable `hwmon*/tempN_input` with `Path`, `Label` (from `tempN_label`, else `hwmonX_tempN`), `TempC`, `Plausible`.
- [ ] `Plausible` is `false` for readings failing `IsPlausibleTempC` OR carrying an unreliable label (`isUnreliableTempLabel`); the entry is still returned.
- [ ] Reuses existing `HwmonBasePath`, `MaxHwmonDevices`, `ReadSysfsInt`, `ReadSysfsString`, `IsPlausibleTempC`, `isUnreliableTempLabel`.

**Verify:** `go test ./daemon/lib/ -run TestDiscoverHwmonTempSensors -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** — append to `daemon/lib/hwmon_test.go` (create if absent with `package lib`)

```go
func TestDiscoverHwmonTempSensorsPlausibility(t *testing.T) {
 // Pure-logic guard: ensures unreliable labels and out-of-range temps are
 // flagged (not silently dropped). Real sysfs scanning is integration-tested
 // on hardware.
 if classifyTempSensorPlausible("Tctl", 45.0) != true {
  t.Errorf("normal CPU sensor should be plausible")
 }
 if classifyTempSensorPlausible("AUXTIN", 45.0) != false {
  t.Errorf("unreliable label should be flagged implausible")
 }
 if classifyTempSensorPlausible("Core 0", 200.0) != false {
  t.Errorf("out-of-range temp should be flagged implausible")
 }
}
```

- [ ] **Step 2: Run test, confirm it fails** — `go test ./daemon/lib/ -run TestDiscoverHwmonTempSensors -v` → FAIL (undefined: classifyTempSensorPlausible)

- [ ] **Step 3: Implement** — append to `daemon/lib/hwmon.go`

```go
// HwmonTempSensor describes one discovered hwmon temperature input.
type HwmonTempSensor struct {
 Path      string
 Label     string
 TempC     float64
 Plausible bool
}

// classifyTempSensorPlausible reports whether a sensor reading is trustworthy
// for fan-curve use: within range AND not a known-unreliable label.
func classifyTempSensorPlausible(label string, tempC float64) bool {
 if !IsPlausibleTempC(tempC) {
  return false
 }
 if label != "" && isUnreliableTempLabel(label) {
  return false
 }
 return true
}

// DiscoverHwmonTempSensors enumerates all readable hwmon temperature inputs.
// Implausible / unreliable sensors are INCLUDED but flagged Plausible=false,
// so callers see the full picture rather than a silently-filtered subset.
func DiscoverHwmonTempSensors() []HwmonTempSensor {
 var sensors []HwmonTempSensor
 for i := range MaxHwmonDevices {
  hwmonDir := filepath.Join(HwmonBasePath, fmt.Sprintf("hwmon%d", i))
  for j := 1; j <= 20; j++ {
   tempPath := filepath.Join(hwmonDir, fmt.Sprintf("temp%d_input", j))
   raw := ReadSysfsInt(tempPath)
   if raw == 0 {
    continue
   }
   tempC := float64(raw) / 1000.0

   labelPath := filepath.Join(hwmonDir, fmt.Sprintf("temp%d_label", j))
   label := ReadSysfsString(labelPath)
   display := label
   if display == "" {
    display = fmt.Sprintf("hwmon%d_temp%d", i, j)
   }

   sensors = append(sensors, HwmonTempSensor{
    Path:      tempPath,
    Label:     display,
    TempC:     tempC,
    Plausible: classifyTempSensorPlausible(label, tempC),
   })
  }
 }
 return sensors
}
```

- [ ] **Step 4: Run test, confirm PASS** — `go test ./daemon/lib/ -run TestDiscoverHwmonTempSensors -v` → PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/lib/hwmon.go daemon/lib/hwmon_test.go
git commit -m "feat(lib): add hwmon temperature-sensor discovery"
```

---

### Task 3: DTOs + validation for temperature sources

**Goal:** Define `FanTempSource`, catalog DTOs, extend `FanDevice` and `FanProfileRequest`, and add source validation.

**Files:**

- Modify: `daemon/dto/fan.go`
- Modify: `daemon/lib/validation.go`
- Test: `daemon/lib/validation_test.go`

**Acceptance Criteria:**

- [ ] `dto.FanTempSource{Type, SensorPath, DriveIDs, FallbackSensorPath}` and `FanTempSourceType` consts (`hwmon`, `drives`) exist.
- [ ] `dto.FanDevice` gains `TempSource *FanTempSource` (omitempty); `dto.FanProfileRequest` gains `Source *FanTempSource`.
- [ ] `dto.FanSensorCatalog`, `AvailableTempSensor`, `AvailableDriveSensor` exist.
- [ ] `lib.ValidateFanTempSource` rejects unknown `Type`, `drives` with empty `DriveIDs`, and sensor paths not under `/sys/class/hwmon/` (or containing `..`).

**Verify:** `go test ./daemon/lib/ -run TestValidateFanTempSource -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** — append to `daemon/lib/validation_test.go`

```go
func TestValidateFanTempSource(t *testing.T) {
 tests := []struct {
  name    string
  src     dto.FanTempSource
  wantErr bool
 }{
  {"hwmon ok", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/sys/class/hwmon/hwmon0/temp1_input"}, false},
  {"drives ok", dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}, FallbackSensorPath: "/sys/class/hwmon/hwmon0/temp1_input"}, false},
  {"bad type", dto.FanTempSource{Type: "bogus"}, true},
  {"drives empty list", dto.FanTempSource{Type: dto.FanTempSourceDrives}, true},
  {"path traversal", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/sys/class/hwmon/../../etc/passwd"}, true},
  {"path outside hwmon", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/etc/passwd"}, true},
  {"bad fallback path", dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}, FallbackSensorPath: "/etc/shadow"}, true},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   err := ValidateFanTempSource(tt.src)
   if (err != nil) != tt.wantErr {
    t.Errorf("ValidateFanTempSource(%+v) err=%v wantErr=%v", tt.src, err, tt.wantErr)
   }
  })
 }
}
```

- [ ] **Step 2: Run test, confirm it fails** — FAIL (undefined: ValidateFanTempSource and/or new dto types)

- [ ] **Step 3a: Add DTOs** — in `daemon/dto/fan.go`, after the `FanControlMethod` consts block add:

```go
// FanTempSourceType identifies where a fan curve reads its temperature.
type FanTempSourceType string

const (
 // FanTempSourceHwmon reads a single hwmon sysfs temperature input.
 FanTempSourceHwmon FanTempSourceType = "hwmon"
 // FanTempSourceDrives reads the max temperature across selected active drives.
 FanTempSourceDrives FanTempSourceType = "drives"
)

// FanTempSource describes a fan curve's temperature input. For Type=="drives"
// the engine uses the max temperature of the active (non-standby) DriveIDs and
// falls back to FallbackSensorPath when they are all spun down.
type FanTempSource struct {
 Type               FanTempSourceType `json:"type" example:"drives"`
 SensorPath         string            `json:"sensor_path,omitempty" example:"/sys/class/hwmon/hwmon0/temp1_input"`
 DriveIDs           []string          `json:"drive_ids,omitempty" example:"disk1,disk2"`
 FallbackSensorPath string            `json:"fallback_sensor_path,omitempty" example:"/sys/class/hwmon/hwmon0/temp1_input"`
}

// AvailableTempSensor is a discoverable hwmon temperature sensor.
type AvailableTempSensor struct {
 Path      string  `json:"path" example:"/sys/class/hwmon/hwmon0/temp1_input"`
 Label     string  `json:"label,omitempty" example:"Tctl"`
 TempC     float64 `json:"temp_celsius" example:"45"`
 Plausible bool    `json:"plausible" example:"true"`
}

// AvailableDriveSensor is a discoverable drive temperature source.
type AvailableDriveSensor struct {
 ID       string  `json:"id" example:"disk1"`
 Device   string  `json:"device,omitempty" example:"sdb"`
 TempC    float64 `json:"temp_celsius" example:"38"`
 SpunDown bool    `json:"spun_down" example:"false"`
}

// FanSensorCatalog lists everything a fan curve can be pointed at.
type FanSensorCatalog struct {
 HwmonSensors []AvailableTempSensor  `json:"hwmon_sensors"`
 Drives       []AvailableDriveSensor `json:"drives"`
 Timestamp    time.Time              `json:"timestamp"`
}
```

- [ ] **Step 3b: Extend FanDevice** — add field to the `FanDevice` struct (after `TempSensorPath`):

```go
 TempSource     *FanTempSource `json:"temp_source,omitempty"`
```

- [ ] **Step 3c: Extend FanProfileRequest** — add field to the `FanProfileRequest` struct (after `TempSensorPath`):

```go
 Source         *FanTempSource `json:"source,omitempty"`
```

- [ ] **Step 3d: Add validator** — in `daemon/lib/validation.go` add (ensure `dto` and `strings` are imported):

```go
// ValidateFanTempSource validates a fan curve temperature source.
func ValidateFanTempSource(src dto.FanTempSource) error {
 switch src.Type {
 case dto.FanTempSourceHwmon:
  if err := validateHwmonSensorPath(src.SensorPath); err != nil {
   return err
  }
 case dto.FanTempSourceDrives:
  if len(src.DriveIDs) == 0 {
   return errors.New("drives source requires at least one drive ID")
  }
 default:
  return fmt.Errorf("invalid temperature source type: %q", src.Type)
 }
 // Fallback is optional, but if set it must be a valid hwmon path.
 if src.FallbackSensorPath != "" {
  if err := validateHwmonSensorPath(src.FallbackSensorPath); err != nil {
   return err
  }
 }
 return nil
}

// validateHwmonSensorPath ensures a sysfs path is under /sys/class/hwmon and
// free of directory traversal.
func validateHwmonSensorPath(path string) error {
 if path == "" {
  return errors.New("hwmon sensor path cannot be empty")
 }
 if strings.Contains(path, "..") || strings.Contains(path, "\x00") {
  return errors.New("invalid hwmon sensor path: traversal or null byte")
 }
 if !strings.HasPrefix(path, "/sys/class/hwmon/") {
  return errors.New("hwmon sensor path must be under /sys/class/hwmon/")
 }
 return nil
}
```

(If `fmt` is not yet imported in `validation.go`, add it.)

- [ ] **Step 4: Run test, confirm PASS** — `go test ./daemon/lib/ -run TestValidateFanTempSource -v` → PASS; also `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add daemon/dto/fan.go daemon/lib/validation.go daemon/lib/validation_test.go
git commit -m "feat(dto): add FanTempSource, sensor-catalog DTOs, and validation"
```

---

### Task 4: Curve engine — drive provider, resolveTemp, fallback

**Goal:** Make the curve engine resolve a `FanTempSource` (hwmon or max-of-active-drives) with per-profile fallback and log-once-per-transition, updating all callers so the package compiles and existing tests pass.

**Files:**

- Modify: `daemon/services/controllers/fan_curves.go`
- Modify: `daemon/services/controllers/fan.go` (restore loop + `GetStatus` annotation; keep `SetProfile` external signature for now)
- Test: `daemon/services/controllers/fan_curves_test.go`

**Acceptance Criteria:**

- [ ] `fanCurveAssignment` is `{ProfileName string; Source dto.FanTempSource}` with `json:"profile_name"` / `json:"source"` tags.
- [ ] `AssignProfile(fanID, profileName string, source dto.FanTempSource)` replaces the old 3-string signature; all callers updated.
- [ ] `FanCurveEngine` has an injected `DriveTempProvider`; `NewFanCurveEngine` accepts/sets a default backed by `lib.ReadDiskTemps`.
- [ ] `resolveTemp` returns `(tempC, true)` from the primary source, else the fallback, else `(0,false)`. For `drives` it takes the max of active, plausible drives.
- [ ] All-spun-down→fallback transition logs **once** (and recovery once), proven by a test capturing log output.
- [ ] `GetAssignmentSource(fanID) (dto.FanTempSource, bool)` exists; `GetStatus` populates `FanDevice.TempSource` and (for hwmon sources) `TempSensorPath`.

**Verify:** `go test ./daemon/services/controllers/ -run 'TestResolveTemp|TestDriveSourceFallbackLogsOnce' -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing tests** — `daemon/services/controllers/fan_curves_test.go`

```go
package controllers

import (
 "bytes"
 "log"
 "os"
 "strings"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type fakeDriveTemps struct{ m map[string]lib.DiskTemp }

func (f fakeDriveTemps) DriveTemps() (map[string]lib.DiskTemp, error) { return f.m, nil }

func newTestEngine(drives map[string]lib.DiskTemp) *FanCurveEngine {
 e := NewFanCurveEngine(nil, NewFanSafetyGuard(nil, dto.FanSafetyConfig{}))
 e.drives = fakeDriveTemps{m: drives}
 return e
}

func TestResolveTempDrivesMaxOfActive(t *testing.T) {
 e := newTestEngine(map[string]lib.DiskTemp{
  "disk1": {ID: "disk1", TempC: 36},
  "disk2": {ID: "disk2", TempC: 41},
  "disk3": {ID: "disk3", SpunDown: true}, // excluded
 })
 src := dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1", "disk2", "disk3"}}
 got, ok := e.resolveTemp(src)
 if !ok || got != 41 {
  t.Fatalf("max-of-active: got (%v,%v), want (41,true)", got, ok)
 }
}

func TestResolveTempAllSpunDownNoFallback(t *testing.T) {
 e := newTestEngine(map[string]lib.DiskTemp{"disk1": {ID: "disk1", SpunDown: true}})
 src := dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}}
 if _, ok := e.resolveTemp(src); ok {
  t.Fatal("all spun down with no fallback should yield ok=false")
 }
}

func TestDriveSourceFallbackLogsOnce(t *testing.T) {
 var buf bytes.Buffer
 prev := logger.GetLevel()
 log.SetOutput(&buf)
 logger.SetLevel(logger.LevelInfo)
 t.Cleanup(func() { log.SetOutput(os.Stderr); logger.SetLevel(prev) })

 e := newTestEngine(map[string]lib.DiskTemp{"disk1": {ID: "disk1", SpunDown: true}})
 src := dto.FanTempSource{
  Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"},
  FallbackSensorPath: "/sys/class/hwmon/hwmon0/temp1_input", // may read 0 in CI; logging is what we assert
 }
 for i := 0; i < 3; i++ {
  e.resolveTempForFan("hwmon0_fan1", src)
 }
 if n := strings.Count(buf.String(), "falling back"); n != 1 {
  t.Errorf("expected fallback logged once across 3 calls, got %d", n)
 }
}
```

- [ ] **Step 2: Run tests, confirm they fail** — FAIL (undefined: `e.drives`, `resolveTemp`, new `NewFanCurveEngine` shape)

- [ ] **Step 3a: Replace assignment struct + engine fields + constructor** — in `daemon/services/controllers/fan_curves.go`:

```go
// fanCurveAssignment links a fan to a profile and a temperature source.
type fanCurveAssignment struct {
 ProfileName string            `json:"profile_name"`
 Source      dto.FanTempSource `json:"source"`
}

// DriveTempProvider supplies per-disk temperatures (from disks.ini).
type DriveTempProvider interface {
 DriveTemps() (map[string]lib.DiskTemp, error)
}

// defaultDriveTempProvider reads the real disks.ini.
type defaultDriveTempProvider struct{}

func (defaultDriveTempProvider) DriveTemps() (map[string]lib.DiskTemp, error) {
 return lib.ReadDiskTemps()
}

// FanCurveEngine evaluates temperature→speed curves and applies PWM changes.
type FanCurveEngine struct {
 mu          sync.RWMutex
 profiles    map[string]dto.FanProfile
 assignments map[string]fanCurveAssignment // keyed by fan ID
 hwmon       *HwmonProvider
 safety      *FanSafetyGuard
 drives      DriveTempProvider
 driveStandby map[string]bool // fanID → currently in all-spun-down fallback (log-once)
 cancel      context.CancelFunc
 running     bool
}

// NewFanCurveEngine creates a curve engine with built-in profiles.
func NewFanCurveEngine(hwmon *HwmonProvider, safety *FanSafetyGuard) *FanCurveEngine {
 profileMap := make(map[string]dto.FanProfile)
 for _, p := range builtInProfiles() {
  profileMap[p.Name] = p
 }

 return &FanCurveEngine{
  profiles:     profileMap,
  assignments:  make(map[string]fanCurveAssignment),
  hwmon:        hwmon,
  safety:       safety,
  drives:       defaultDriveTempProvider{},
  driveStandby: make(map[string]bool),
 }
}
```

- [ ] **Step 3b: Replace `AssignProfile`, add `GetAssignmentSource`** — in `fan_curves.go`:

```go
// AssignProfile links a fan to a named profile and temperature source.
func (e *FanCurveEngine) AssignProfile(fanID, profileName string, source dto.FanTempSource) error {
 e.mu.Lock()
 defer e.mu.Unlock()

 if _, ok := e.profiles[profileName]; !ok {
  return &fanError{msg: "profile not found: " + profileName}
 }

 e.assignments[fanID] = fanCurveAssignment{ProfileName: profileName, Source: source}
 return nil
}

// GetAssignmentSource returns the temperature source for a fan, if assigned.
func (e *FanCurveEngine) GetAssignmentSource(fanID string) (dto.FanTempSource, bool) {
 e.mu.RLock()
 defer e.mu.RUnlock()
 a, ok := e.assignments[fanID]
 if !ok {
  return dto.FanTempSource{}, false
 }
 return a.Source, true
}
```

- [ ] **Step 3c: Add `resolveTemp` + helpers, rewrite `applyCurves` temp block** — in `fan_curves.go`:

```go
// resolveTemp returns the curve input temperature for a source. It tries the
// primary source (hwmon sensor or max-of-active-drives), then the per-profile
// hwmon fallback, then reports no reading.
func (e *FanCurveEngine) resolveTemp(src dto.FanTempSource) (float64, bool) {
 switch src.Type {
 case dto.FanTempSourceHwmon:
  if t, ok := readHwmonTemp(src.SensorPath); ok {
   return t, true
  }
 case dto.FanTempSourceDrives:
  if t, ok := e.maxActiveDriveTemp(src.DriveIDs); ok {
   return t, true
  }
 }
 // Fallback.
 if t, ok := readHwmonTemp(src.FallbackSensorPath); ok {
  return t, true
 }
 return 0, false
}

// maxActiveDriveTemp returns the highest plausible temperature among the
// selected drives that are present and not spun down.
func (e *FanCurveEngine) maxActiveDriveTemp(ids []string) (float64, bool) {
 temps, err := e.drives.DriveTemps()
 if err != nil {
  return 0, false
 }
 maxT := 0.0
 found := false
 for _, id := range ids {
  d, ok := temps[id]
  if !ok || d.SpunDown || !lib.IsPlausibleTempC(d.TempC) {
   continue
  }
  if !found || d.TempC > maxT {
   maxT = d.TempC
   found = true
  }
 }
 return maxT, found
}

// readHwmonTemp reads a single hwmon sysfs temp input in °C.
func readHwmonTemp(path string) (float64, bool) {
 if path == "" {
  return 0, false
 }
 raw := lib.ReadSysfsInt(path)
 if raw <= 0 {
  return 0, false
 }
 t := float64(raw) / 1000.0
 if !lib.IsPlausibleTempC(t) {
  return 0, false
 }
 return t, true
}
```

Add `resolveTempForFan`, a per-fan wrapper that resolves the source and logs the drive-spun-down→fallback transition once. The fallback trigger is "the drives primary is unavailable" — independent of whether the fallback sensor itself reads a value — so a spun-down array logs exactly one transition line:

```go
// resolveTempForFan resolves a fan's source and logs the drive-spun-down
// fallback transition once per fan. Returns the resolved temperature.
func (e *FanCurveEngine) resolveTempForFan(fanID string, src dto.FanTempSource) (float64, bool) {
 tempC, ok := e.resolveTemp(src)
 if src.Type == dto.FanTempSourceDrives {
  _, primaryOK := e.maxActiveDriveTemp(src.DriveIDs)
  e.noteDriveFallback(fanID, !primaryOK)
 }
 return tempC, ok
}
```

In `applyCurves`, replace the inline temp block (the `tempC := 0.0 … if tempC == 0 || !lib.IsPlausibleTempC(tempC) { continue }` section) with:

```go
  // Resolve the curve input from the assignment's source.
  tempC, ok := e.resolveTempForFan(fanID, assignment.Source)
  if !ok {
   continue // no valid reading — hold last PWM (existing safe behavior)
  }
```

Add the transition logger. `applyCurves` copies assignments and does not hold `e.mu` during the loop, so guard `driveStandby` with its own short critical section:

```go
// noteDriveFallback logs the first transition into and out of the
// all-spun-down fallback state for a fan, avoiding per-poll log spam.
func (e *FanCurveEngine) noteDriveFallback(fanID string, inFallback bool) {
 e.mu.Lock()
 defer e.mu.Unlock()
 was := e.driveStandby[fanID]
 switch {
 case inFallback && !was:
  logger.Info("Fan curve: %s — selected drives spun down, falling back to fallback sensor", fanID)
  e.driveStandby[fanID] = true
 case !inFallback && was:
  logger.Info("Fan curve: %s — drives active again, leaving fallback", fanID)
  delete(e.driveStandby, fanID)
 }
}
```

- [ ] **Step 3d: Update callers in `fan.go`** — restore loop (~line 79) becomes:

```go
 for fanID, assignment := range cfgData.Assignments {
  if assignErr := c.curves.AssignProfile(fanID, assignment.ProfileName, assignment.Source); assignErr != nil {
   logger.Warning("Fan control: Failed to restore assignment for %s: %v", fanID, assignErr)
  }
 }
```

In `SetProfile` (keep its current `(fanID, profileName, tempSensorPath string)` signature for this task — Task 6 changes it), build a source from the legacy path:

```go
 src := dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: tempSensorPath}
 if err := c.curves.AssignProfile(fanID, profileName, src); err != nil {
  return fmt.Errorf("assign profile: %w", err)
 }
```

In `GetStatus`, replace the annotation loop:

```go
 for i := range fans {
  if src, ok := c.curves.GetAssignmentSource(fans[i].ID); ok {
   if profileName, pok := c.curves.GetAssignment(fans[i].ID); pok {
    fans[i].ActiveProfile = profileName
   }
   s := src
   fans[i].TempSource = &s
   if src.Type == dto.FanTempSourceHwmon {
    fans[i].TempSensorPath = src.SensorPath
   }
  }
 }
```

- [ ] **Step 4: Run tests, confirm PASS** — `go test ./daemon/services/controllers/ -v` → PASS (new + existing)

- [ ] **Step 5: Commit**

```bash
git add daemon/services/controllers/fan_curves.go daemon/services/controllers/fan.go daemon/services/controllers/fan_curves_test.go
git commit -m "feat(fan): resolve drive/hwmon temp sources with per-profile fallback"
```

---

### Task 5: Config persistence migration (legacy → Source)

**Goal:** Load existing `fancontrol.json` files (old flat `{ProfileName, TempSensorPath}`) into the new `Source` shape, and save in the new shape.

**Files:**

- Modify: `daemon/services/controllers/fan_config.go`
- Test: `daemon/services/controllers/fan_config_test.go`

**Acceptance Criteria:**

- [ ] A legacy assignment `{"ProfileName":"balanced","TempSensorPath":"/sys/class/hwmon/hwmon0/temp1_input"}` loads as `Source{Type:hwmon, SensorPath:...}`.
- [ ] A new-shape assignment `{"profile_name":"balanced","source":{"type":"drives","drive_ids":["disk1"]}}` loads unchanged.
- [ ] `Save` then `Load` round-trips a `drives` source.

**Verify:** `go test ./daemon/services/controllers/ -run 'TestFanConfigMigration|TestFanConfigRoundTrip' -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** — `daemon/services/controllers/fan_config_test.go`

```go
package controllers

import (
 "encoding/json"
 "path/filepath"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestFanConfigMigrationLegacyAssignment(t *testing.T) {
 var a fanCurveAssignment
 legacy := []byte(`{"ProfileName":"balanced","TempSensorPath":"/sys/class/hwmon/hwmon0/temp1_input"}`)
 if err := json.Unmarshal(legacy, &a); err != nil {
  t.Fatal(err)
 }
 if a.ProfileName != "balanced" || a.Source.Type != dto.FanTempSourceHwmon ||
  a.Source.SensorPath != "/sys/class/hwmon/hwmon0/temp1_input" {
  t.Fatalf("legacy migration failed: %+v", a)
 }
}

func TestFanConfigNewShapeAssignment(t *testing.T) {
 var a fanCurveAssignment
 newShape := []byte(`{"profile_name":"balanced","source":{"type":"drives","drive_ids":["disk1"]}}`)
 if err := json.Unmarshal(newShape, &a); err != nil {
  t.Fatal(err)
 }
 if a.Source.Type != dto.FanTempSourceDrives || len(a.Source.DriveIDs) != 1 {
  t.Fatalf("new-shape parse failed: %+v", a)
 }
}

func TestFanConfigRoundTrip(t *testing.T) {
 store := NewFanConfigStore(t.TempDir())
 in := fanConfigData{
  Config: defaultFanControlConfig(),
  Assignments: map[string]fanCurveAssignment{
   "hwmon0_fan1": {ProfileName: "balanced", Source: dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}}},
  },
 }
 if err := store.Save(in); err != nil {
  t.Fatal(err)
 }
 out, err := store.Load()
 if err != nil {
  t.Fatal(err)
 }
 got := out.Assignments["hwmon0_fan1"]
 if got.Source.Type != dto.FanTempSourceDrives || got.Source.DriveIDs[0] != "disk1" {
  t.Fatalf("round-trip failed: %+v", got)
 }
 _ = filepath.Base(store.filePath)
}
```

- [ ] **Step 2: Run test, confirm it fails** — `TestFanConfigMigrationLegacyAssignment` FAILs (legacy keys ignored → empty Source).

- [ ] **Step 3: Implement custom UnmarshalJSON** — in `daemon/services/controllers/fan_config.go` add (and `import "encoding/json"` already present):

```go
// UnmarshalJSON accepts both the current shape ({profile_name, source}) and the
// legacy flat shape ({ProfileName, TempSensorPath}) so existing fancontrol.json
// files keep working. Legacy TempSensorPath maps to a hwmon source.
func (a *fanCurveAssignment) UnmarshalJSON(data []byte) error {
 // New shape first.
 type newShape struct {
  ProfileName string            `json:"profile_name"`
  Source      dto.FanTempSource `json:"source"`
 }
 var ns newShape
 if err := json.Unmarshal(data, &ns); err != nil {
  return err
 }

 // Legacy shape (capitalized keys from the old default Go encoding).
 type legacyShape struct {
  ProfileName    string `json:"ProfileName"`
  TempSensorPath string `json:"TempSensorPath"`
 }
 var ls legacyShape
 _ = json.Unmarshal(data, &ls)

 a.ProfileName = ns.ProfileName
 if a.ProfileName == "" {
  a.ProfileName = ls.ProfileName
 }

 a.Source = ns.Source
 if a.Source.Type == "" && ls.TempSensorPath != "" {
  a.Source = dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: ls.TempSensorPath}
 }
 return nil
}
```

- [ ] **Step 4: Run tests, confirm PASS** — `go test ./daemon/services/controllers/ -run 'TestFanConfig' -v` → PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/services/controllers/fan_config.go daemon/services/controllers/fan_config_test.go
git commit -m "feat(fan): migrate legacy fancontrol.json assignments to Source shape"
```

---

### Task 6: REST — `GET /fans/sensors` + source-aware assign

**Goal:** Expose the discovery endpoint and accept a `Source` (or legacy `temp_sensor_path`) on profile assignment, validated.

**Files:**

- Modify: `daemon/services/controllers/fan.go` (change `SetProfile` to take `dto.FanTempSource`; add `GetSensorCatalog`)
- Modify: `daemon/services/api/handlers.go` (`handleSetFanProfile` source resolution; new `handleFanSensors`)
- Modify: `daemon/services/api/server.go` (register route)
- Test: `daemon/services/api/fan_sensors_test.go`

**Acceptance Criteria:**

- [ ] `GET /api/v1/fans/sensors` returns `dto.FanSensorCatalog` (200) with `hwmon_sensors` and `drives` arrays.
- [ ] `POST /fans/profile` with a `source` object validates via `lib.ValidateFanTempSource` (400 on invalid) and assigns it; with only legacy `temp_sensor_path` it synthesizes a hwmon source.
- [ ] `FanController.SetProfile(fanID, profileName string, source dto.FanTempSource)` is the new signature; `GetSensorCatalog()` returns the catalog from `lib.DiscoverHwmonTempSensors` + `lib.ReadDiskTemps`.

**Verify:** `go test ./daemon/services/api/ -run 'TestFanSensors|TestSetFanProfileSource' -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** — `daemon/services/api/fan_sensors_test.go`

```go
package api

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestFanSensorsEndpointSchema(t *testing.T) {
 server := NewServer(&domain.Context{Config: domain.Config{Port: 8043}})
 if server.fanController == nil {
  t.Skip("fan controller not wired in test context")
 }
 req := httptest.NewRequest(http.MethodGet, "/api/v1/fans/sensors", nil)
 w := httptest.NewRecorder()
 server.handleFanSensors(w, req)
 if w.Code != http.StatusOK {
  t.Fatalf("status: got %d want 200", w.Code)
 }
 var cat dto.FanSensorCatalog
 if err := json.Unmarshal(w.Body.Bytes(), &cat); err != nil {
  t.Fatalf("decode catalog: %v", err)
 }
}

func TestSetFanProfileSourceValidation(t *testing.T) {
 server := NewServer(&domain.Context{Config: domain.Config{Port: 8043}})
 body, _ := json.Marshal(dto.FanProfileRequest{
  FanID:       "hwmon0_fan1",
  ProfileName: "balanced",
  Source:      &dto.FanTempSource{Type: "bogus"},
 })
 req := httptest.NewRequest(http.MethodPost, "/api/v1/fans/profile", bytes.NewReader(body))
 w := httptest.NewRecorder()
 server.handleSetFanProfile(w, req)
 if w.Code != http.StatusBadRequest {
  t.Fatalf("invalid source: got %d want 400", w.Code)
 }
}
```

(Add the `domain` import; mirror the import list of `metrics_test.go`.)

- [ ] **Step 2: Run test, confirm it fails** — FAIL (undefined: `handleFanSensors`; `Source` field handling)

- [ ] **Step 3a: `FanController` changes** — in `daemon/services/controllers/fan.go`:

Change `SetProfile` signature and body to take a source:

```go
func (c *FanController) SetProfile(fanID, profileName string, source dto.FanTempSource) error {
 if err := lib.ValidateFanID(fanID); err != nil {
  return err
 }
 c.mu.Lock()
 defer c.mu.Unlock()

 if !c.config.ControlEnabled {
  return fmt.Errorf("fan control is not enabled; enable it via the configuration endpoint first")
 }
 if err := c.hwmon.SetMode(fanID, dto.FanModeManual); err != nil {
  return fmt.Errorf("set manual mode for profile: %w", err)
 }
 if err := c.curves.AssignProfile(fanID, profileName, source); err != nil {
  return fmt.Errorf("assign profile: %w", err)
 }
 if !c.curves.running {
  c.curves.Start(time.Duration(c.config.PollInterval) * time.Second)
 }
 c.saveConfigLocked()
 logger.Info("Fan control: Assigned profile %q to %s (source=%s)", profileName, fanID, source.Type)
 return nil
}

// GetSensorCatalog returns the hwmon sensors and drives available as fan-curve
// temperature sources.
func (c *FanController) GetSensorCatalog() dto.FanSensorCatalog {
 cat := dto.FanSensorCatalog{Timestamp: time.Now()}
 for _, s := range lib.DiscoverHwmonTempSensors() {
  cat.HwmonSensors = append(cat.HwmonSensors, dto.AvailableTempSensor{
   Path: s.Path, Label: s.Label, TempC: s.TempC, Plausible: s.Plausible,
  })
 }
 if drives, err := lib.ReadDiskTemps(); err == nil {
  for _, d := range drives {
   cat.Drives = append(cat.Drives, dto.AvailableDriveSensor{
    ID: d.ID, Device: d.Device, TempC: d.TempC, SpunDown: d.SpunDown,
   })
  }
 }
 return cat
}
```

Update the internal `SetProfile` caller note: the restore loop in `Init` already calls `AssignProfile` directly (Task 4) — no change here.

- [ ] **Step 3b: API handlers** — in `daemon/services/api/handlers.go` rewrite the assign call inside `handleSetFanProfile`:

```go
 // Resolve temperature source: explicit Source wins; else legacy path.
 var source dto.FanTempSource
 switch {
 case req.Source != nil:
  source = *req.Source
 case req.TempSensorPath != "":
  source = dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: req.TempSensorPath}
 default:
  source = dto.FanTempSource{Type: dto.FanTempSourceHwmon}
 }
 if req.Source != nil || req.TempSensorPath != "" {
  if err := lib.ValidateFanTempSource(source); err != nil {
   respondWithError(w, http.StatusBadRequest, err.Error())
   return
  }
 }

 if err := s.fanController.SetProfile(req.FanID, req.ProfileName, source); err != nil {
  logger.Error("API: Failed to assign fan profile: %v", err)
  respondWithError(w, http.StatusInternalServerError, "Failed to assign fan profile")
  return
 }
```

Add the new handler (place near the other fan handlers):

```go
// handleFanSensors godoc
//
// @Summary  List fan temperature sources
// @Description List available hwmon temperature sensors and drives for fan-curve assignment
// @Tags   Fans
// @Produce  json
// @Success  200 {object} dto.FanSensorCatalog "Available sensors and drives"
// @Failure  503 {object} dto.Response   "Fan controller not initialized"
// @Router   /fans/sensors [get]
func (s *Server) handleFanSensors(w http.ResponseWriter, _ *http.Request) {
 if s.fanController == nil {
  respondWithError(w, http.StatusServiceUnavailable, "Fan controller not initialized")
  return
 }
 respondJSON(w, http.StatusOK, s.fanController.GetSensorCatalog())
}
```

(Ensure `lib` and `dto` are imported in `handlers.go` — they already are.)

- [ ] **Step 3c: Register route** — in `daemon/services/api/server.go`, next to the other `/fans` routes (~line 320):

```go
 api.HandleFunc("/fans/sensors", s.handleFanSensors).Methods("GET")
```

- [ ] **Step 4: Run tests, confirm PASS** — `go test ./daemon/services/api/ -run 'TestFanSensors|TestSetFanProfileSource' -v` → PASS; `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add daemon/services/controllers/fan.go daemon/services/api/handlers.go daemon/services/api/server.go daemon/services/api/fan_sensors_test.go
git commit -m "feat(api): GET /fans/sensors + source-aware fan profile assignment"
```

---

### Task 7: MCP — `get_fan_sensors` tool + source-aware `set_fan_profile`

**Goal:** Add the read-only `get_fan_sensors` MCP tool and extend `set_fan_profile` to accept a structured source while still accepting the legacy `temp_sensor_path`.

**Files:**

- Modify: `daemon/dto/mcp.go` (extend `MCPFanProfileArgs`)
- Modify: `daemon/services/mcp/server.go` (`set_fan_profile` handler + new tool)
- Test: `daemon/services/mcp/fan_sensors_test.go`

**Acceptance Criteria:**

- [ ] `MCPFanProfileArgs` gains `SourceType`, `DriveIDs`, `FallbackSensorPath` (all omitempty) with jsonschema descriptions.
- [ ] `set_fan_profile` builds a `dto.FanTempSource`: if `SourceType=="drives"` use `DriveIDs`+`FallbackSensorPath`; if `temp_sensor_path` or `SourceType=="hwmon"` use the hwmon path; validates via `lib.ValidateFanTempSource`.
- [ ] `get_fan_sensors` (ReadOnlyHint) returns `FanController.GetSensorCatalog()` as JSON.
- [ ] A test asserts both tools are registered (tool list contains `get_fan_sensors`).

**Verify:** `go test ./daemon/services/mcp/ -run TestFanSensorsTool -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test** — `daemon/services/mcp/fan_sensors_test.go`

```go
package mcp

import (
 "strings"
 "testing"
)

func TestFanSensorsToolRegistered(t *testing.T) {
 names := registeredToolNames(t) // helper used by existing MCP tests
 if !contains(names, "get_fan_sensors") {
  t.Errorf("get_fan_sensors not registered; got %s", strings.Join(names, ","))
 }
}
```

(If the existing MCP test suite exposes a different registration-introspection helper, use that one; mirror an existing `*_test.go` in `daemon/services/mcp/`. The assertion is: `get_fan_sensors` is among registered tools.)

- [ ] **Step 2: Run test, confirm it fails** — FAIL (tool not registered)

- [ ] **Step 3a: Extend args** — in `daemon/dto/mcp.go`, add to `MCPFanProfileArgs`:

```go
 SourceType         string   `json:"source_type,omitempty" jsonschema:"Temperature source type: 'hwmon' (single sensor) or 'drives' (max of selected drives)"`
 DriveIDs           []string `json:"drive_ids,omitempty" jsonschema:"Drive IDs (e.g. disk1, cache) when source_type=drives; engine uses the max temp of active drives"`
 FallbackSensorPath string   `json:"fallback_sensor_path,omitempty" jsonschema:"Hwmon sensor path used when source_type=drives and all selected drives are spun down"`
```

- [ ] **Step 3b: Update `set_fan_profile` handler** — in `daemon/services/mcp/server.go` replace the `SetProfile` call body:

```go
  var source dto.FanTempSource
  switch args.SourceType {
  case string(dto.FanTempSourceDrives):
   source = dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: args.DriveIDs, FallbackSensorPath: args.FallbackSensorPath}
  default:
   source = dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: args.TempSensorPath}
  }
  if err := lib.ValidateFanTempSource(source); err != nil {
   return textResult(fmt.Sprintf("Invalid temperature source: %v", err)), nil, nil
  }
  logger.Info("MCP: Set fan profile '%s' for '%s' (source=%s)", args.ProfileName, args.FanID, source.Type)
  if err := s.fanController.SetProfile(args.FanID, args.ProfileName, source); err != nil {
   return textResult(fmt.Sprintf("Failed to assign fan profile: %v", err)), nil, nil
  }
  return textResult(fmt.Sprintf("Profile %s assigned to fan %s", args.ProfileName, args.FanID)), nil, nil
```

Also update the `set_fan_profile` tool `Description` to mention drive sources, and ensure `lib` is imported in `server.go`.

- [ ] **Step 3c: Register `get_fan_sensors`** — in `registerFanControlTools()`, after `get_fan_status`:

```go
 mcp.AddTool(s.mcpServer, &mcp.Tool{
  Name:        "get_fan_sensors",
  Description: "List available hwmon temperature sensors and drives that can be used as fan-curve temperature sources",
  Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
 }, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
  if s.fanController == nil {
   return textResult("Fan controller not initialized"), nil, nil
  }
  return jsonResult(s.fanController.GetSensorCatalog())
 })
```

- [ ] **Step 4: Run tests, confirm PASS** — `go test ./daemon/services/mcp/ -v` → PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/dto/mcp.go daemon/services/mcp/server.go daemon/services/mcp/fan_sensors_test.go
git commit -m "feat(mcp): add get_fan_sensors and drive-source set_fan_profile"
```

---

### Task 8: Full-suite, hardware verify, CodeRabbit, CHANGELOG

**Goal:** Prove the feature works end-to-end on the live Unraid server and record it in the changelog.

> **USER-ORDERED GATE — NON-SKIPPABLE.** This task was requested by the user in the current conversation. It MUST NOT be closed by walking around it, by declaring it "verified inline", or by substituting a cheaper check. Close only after every item in `acceptanceCriteria` has been re-validated independently, with output captured.

**Files:**

- Modify: `CHANGELOG.md`

**Acceptance Criteria:**

- [ ] `go test ./...` passes and `make local` builds (with regenerated swagger).
- [ ] `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify` reports `failed=0`.
- [ ] Live `curl http://192.168.20.21:8043/api/v1/fans/sensors` returns a JSON body containing both `hwmon_sensors` and `drives` arrays (drives non-empty on this server).
- [ ] A live `POST /fans/profile` with a `drives` source (e.g. `{"fan_id":"<real controllable fan>","profile_name":"balanced","source":{"type":"drives","drive_ids":["disk1"],"fallback_sensor_path":"<a real hwmon path from the catalog>"}}`) returns success, and `GET /fans` shows that fan's `temp_source.type == "drives"`. (Use a real controllable fan ID; if none is controllable, capture the catalog + a 400/Service response and note the hardware limitation instead — do NOT fabricate success.)
- [ ] `coderabbit review --agent --base main` reports 0 findings (or only addressed ones).
- [ ] `CHANGELOG.md` gets an "Added" entry under the next version section, written **last**.

**Verify:** `go test ./... && make local` → builds; then the Ansible + curl + CodeRabbit commands above.

**Steps:**

- [ ] **Step 1: Full suite + build**

```bash
go test ./... && make local
```

- [ ] **Step 2: Deploy + verify on Unraid**

```bash
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify
```

Expected: `failed=0`.

- [ ] **Step 3: Live discovery + drive-source assignment**

```bash
curl -s http://192.168.20.21:8043/api/v1/fans/sensors | head -c 2000
# pick a real controllable fan from GET /fans and a real hwmon path from the catalog, then:
# curl -s -X POST http://192.168.20.21:8043/api/v1/fans/profile -H 'Content-Type: application/json' \
#   -d '{"fan_id":"<fan>","profile_name":"balanced","source":{"type":"drives","drive_ids":["disk1"],"fallback_sensor_path":"<hwmon path>"}}'
# curl -s http://192.168.20.21:8043/api/v1/fans | python3 -c 'import sys,json;...'  # confirm temp_source.type==drives
```

Restore afterward with `POST /fans/defaults` so the live box is left as found.

- [ ] **Step 4: CodeRabbit**

```bash
coderabbit review --agent --base main
```

Fix any Critical/Warning findings, re-run until clean.

- [ ] **Step 5: Update CHANGELOG.md (LAST)** — add under the next version's `### Added`:

```markdown
- **Drive-aware fan curves + sensor discovery** — a fan curve can now read its
  temperature from a selected set of drives (max of the active ones, sourced
  non-destructively from `disks.ini` — spun-down drives are skipped, never
  woken) and fall back to a per-profile hwmon sensor when those drives are spun
  down. New read-only `GET /api/v1/fans/sensors` endpoint and `get_fan_sensors`
  MCP tool list every hwmon temperature sensor (path, label, value, plausibility
  flag) and drive (id, device, temp, spin state) available as a curve source.
  `POST /fans/profile` and `set_fan_profile` accept a structured `source`
  (`hwmon` or `drives` + optional fallback) while remaining backward compatible
  with the legacy `temp_sensor_path`. Existing `fancontrol.json` assignments are
  migrated transparently. The emergency thermal cutoff remains hwmon-only by
  design. MCP tool count → 126. Verified live on Unraid 7.3.1.
```

- [ ] **Step 6: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): drive-aware fan curves + sensor discovery"
```

---

## Notes for the Implementer

- **Branch:** create `feat/drive-aware-fan-curves` off `main` before Task 1; do not work on `main`.
- **Safety invariant:** never route drive temperatures into `CheckTemperatureSafety`/`ReadMaxHwmonTemp`. The emergency cutoff stays hwmon-only.
- **Non-destructive on hardware:** the Unraid box runs live VMs/containers. Only touch controllable fans, and restore defaults after testing.
- **prettier hook:** the pre-commit prettier hook reformats `.md`/`.json` and aborts the commit; if that happens, `git add` the reformatted files and re-commit.
