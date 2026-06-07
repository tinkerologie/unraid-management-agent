# Drive-Aware Fan Curves + Sensor Discovery — Design

**Date:** 2026-06-07
**Status:** Approved (brainstorming)
**Author:** Ruaan Deysel (with Claude Code)

## Problem

The fan-control subsystem has three gaps relative to what users expect (and what
the `SimonFair/IPMI-unRAID` plugin offers):

1. **No sensor discovery.** To drive a fan curve, a user assigns a profile and
   links a temperature sensor by its raw sysfs path
   (`/sys/class/hwmon/hwmonX/tempN_input`). There is no API that enumerates the
   available sensors, so the user must SSH in and inspect `/sys/class/hwmon`
   manually. The system collector reads hwmon temps into a `label → value` map
   but discards the sysfs **path**, so a value is visible but not the path
   needed for assignment.

2. **No drive-temperature source.** Fan control only ever reads hwmon
   (`/sys/class/hwmon`). Drive temperatures live entirely in the disk collector
   (sourced from Unraid's `disks.ini`) and are never fed into fan curves. A user
   cannot cool drives based on drive temperature.

3. **No spun-down failover.** The IPMI plugin lets users select drives, computes
   the max temperature across them, and fails over to a secondary sensor when
   drives are spun down. None of this exists here, even though the disk side
   already detects standby drives (`SpinState == "standby"`, `disks.ini` temp
   `"*"`).

## Goal

Let a fan curve read its temperature from a **selected set of drives** (max of
the active ones, sourced non-destructively from `disks.ini`), fail over to a
**per-profile hwmon sensor** when those drives are spun down, and expose a
**discovery endpoint** so users can see every hwmon sensor and drive they can
curve against.

## Non-Goals (YAGNI)

- **Named drive groups** (`all-array`, `all-pool`). Explicit per-drive selection
  ships first; groups are a possible future enhancement.
- **smartctl as a drive-temp source.** `disks.ini` only — no wake risk, already
  maintained by Unraid, zero extra subprocess I/O. (Drives not present in
  `disks.ini`, e.g. unassigned/JBOD, are simply not selectable for now.)
- **A generic virtual-sensor registry.** Over-engineered for "max of N drives
  with one fallback."
- **Changing the emergency thermal cutoff.** It stays hwmon-only (see Safety).
- **Refactoring the disk collector** to use the new shared parser. We only _add_
  the `lib` helper; rewiring the working collector is out of scope.

## Decisions (from brainstorming)

| Decision               | Choice                                             |
| ---------------------- | -------------------------------------------------- |
| Drive-temp data source | `disks.ini` (Unraid-native, no wake risk)          |
| Failover scope         | Per-profile fallback hwmon sensor                  |
| Drive selection model  | Explicit drive list → max of **active** drives     |
| Discovery scope        | One endpoint: hwmon sensors **and** drives         |
| Temp-source model      | Tagged `TempSource` on the assignment (Approach A) |

## Architecture

A new `FanTempSource` value object describes a curve's temperature input. The
curve engine gains a `resolveTemp(src)` step that tries the primary source
(hwmon sensor _or_ max-of-active-drives), then a per-profile hwmon fallback,
then reports "no reading" (the existing safe "hold last PWM" behavior). Drive
temperatures come from a new injected `DriveTempProvider` backed by a shared
`lib.ReadDiskTemps()` parser of `disks.ini`. A read-only discovery endpoint and
MCP tool enumerate hwmon sensors and drives. The emergency thermal cutoff is
untouched and remains hwmon-only.

### Data flow

```
poll tick
  └─ applyCurves()
       └─ for each assignment:
            tempC, ok := resolveTemp(assignment.Source)
              ├─ primary: hwmon SensorPath  → ReadSysfsInt + IsPlausibleTempC
              │           OR drives DriveIDs → max(active, plausible) via DriveTempProvider
              ├─ fallback: FallbackSensorPath (hwmon) when primary invalid
              └─ else ok=false → continue (hold last PWM)
            targetPct = interpolateSpeed(profile, tempC)
            targetPct = safety.ValidatePWM(targetPct)
            hwmon.SetPWM(fanID, PctToPWM(targetPct))
```

## Components

### 1. Data model (`daemon/dto/fan.go`)

```go
type FanTempSourceType string
const (
    FanTempSourceHwmon  FanTempSourceType = "hwmon"
    FanTempSourceDrives FanTempSourceType = "drives"
)

type FanTempSource struct {
    Type               FanTempSourceType `json:"type"`
    SensorPath         string            `json:"sensor_path,omitempty"`          // Type==hwmon
    DriveIDs           []string          `json:"drive_ids,omitempty"`            // Type==drives (max of active)
    FallbackSensorPath string            `json:"fallback_sensor_path,omitempty"` // hwmon path; used when primary invalid
}
```

- `dto.FanDevice` keeps `TempSensorPath` (populated only for hwmon sources, for
  back-compat display) and gains `TempSource *FanTempSource` (omitempty) so the
  full source is visible in `GET /fans`.

### 2. Internal assignment (`daemon/services/controllers/fan_curves.go`)

`fanCurveAssignment` changes from `{ProfileName, TempSensorPath string}` to:

```go
type fanCurveAssignment struct {
    ProfileName string             `json:"profile_name"`
    Source      dto.FanTempSource  `json:"source"`
}
```

Explicit `json` tags stabilize the on-disk keys (snake_case) going forward.

### 3. Drive-temperature provider

New shared parser `daemon/lib/disktemp.go`:

```go
type DiskTemp struct {
    ID       string  // disks.ini section name: "disk1", "cache", "parity"
    Device   string  // "sdb"
    TempC    float64 // 0 if unavailable
    SpunDown bool    // disks.ini temp == "*" or empty
}

func ReadDiskTemps() (map[string]DiskTemp, error) // parses /boot/config/disks.ini
```

Reuses the proven `disks.ini` rules from `collectors/disk.go:266` (the `"*"` /
empty → spun-down sentinel).

Provider interface injected into the engine (mirrors the existing `hwmon`
injection), so tests can supply a fake (spun-down simulation) without touching
the filesystem:

```go
type DriveTempProvider interface {
    DriveTemps() (map[string]lib.DiskTemp, error)
}
```

Default implementation wraps `lib.ReadDiskTemps()`.

**Aggregation for a `drives` source:** `max(TempC)` over `DriveIDs` that are
present, not `SpunDown`, and pass `IsPlausibleTempC`. Empty after filtering →
primary invalid → fallback.

### 4. Resolution & fallback (`fan_curves.go`)

```go
func (e *FanCurveEngine) resolveTemp(src dto.FanTempSource) (tempC float64, ok bool)
```

Order:

1. **Primary** — hwmon (`ReadSysfsInt` + `IsPlausibleTempC`) or drives
   (max-of-active).
2. **Fallback** — `FallbackSensorPath` read as hwmon when primary invalid.
3. **No reading** — both fail → `ok == false`.

`applyCurves` replaces its inline temp block with
`tempC, ok := e.resolveTemp(assignment.Source); if !ok { continue }`.
"Hold last PWM on no reading" is the **unchanged** existing behavior. Downstream
(`interpolateSpeed` → `ValidatePWM` → `SetPWM`) is untouched.

**Observability:** when a `drives` source falls back because every selected drive
is spun down, log **once per transition** into/out of that state (reusing the
log-once pattern from `fan_safety.go` / `fan_safety_test.go`), so a nightly
array spin-down does not spam the log every poll interval.

### 5. Discovery endpoint & DTO

`GET /api/v1/fans/sensors` → `dto.FanSensorCatalog`:

```go
type AvailableTempSensor struct {
    Path      string  `json:"path"`
    Label     string  `json:"label,omitempty"`
    TempC     float64 `json:"temp_celsius"`
    Plausible bool    `json:"plausible"` // IsPlausibleTempC && not an unreliable label
}

type AvailableDriveSensor struct {
    ID       string  `json:"id"`
    Device   string  `json:"device,omitempty"`
    TempC    float64 `json:"temp_celsius"`
    SpunDown bool    `json:"spun_down"`
}

type FanSensorCatalog struct {
    HwmonSensors []AvailableTempSensor  `json:"hwmon_sensors"`
    Drives       []AvailableDriveSensor `json:"drives"`
    Timestamp    time.Time              `json:"timestamp"`
}
```

New `lib.DiscoverHwmonTempSensors()` walks `hwmon*/temp*_input`, reads
`temp*_label`, and sets `Plausible` (reusing `IsPlausibleTempC` + the existing
unreliable-label filter). Unreliable/implausible sensors are **included but
flagged `Plausible:false`**, so the user sees the full picture rather than a
silently filtered subset. Drives come from `lib.ReadDiskTemps()`.

MCP tool `get_fan_sensors` (read-only) returns the same catalog.

### 6. Assignment surface

REST `dto.FanProfileRequest` extended:

```go
type FanProfileRequest struct {
    FanID          string         `json:"fan_id"`
    ProfileName    string         `json:"profile_name"`
    TempSensorPath string         `json:"temp_sensor_path,omitempty"` // legacy hwmon shorthand
    Source         *FanTempSource `json:"source,omitempty"`           // preferred
}
```

Handler resolution: `Source` if present; else synthesize
`{Type: hwmon, SensorPath: TempSensorPath}` from the legacy field; else no
source. `AssignProfile(fanID, profileName string, source dto.FanTempSource)`
replaces the old `(fanID, profileName, tempSensorPath string)`; the single
caller at `fan.go:80` updates.

MCP `set_fan_profile` gains optional source params (`type`, `drive_ids`,
`sensor_path`, `fallback_sensor_path`) while still accepting `temp_sensor_path`.
Net MCP tool delta: `get_fan_sensors` only → **126** tools.

### 7. Validation (`daemon/lib/validation.go` style)

- `Type` ∈ {`hwmon`, `drives`}.
- `drives` requires non-empty `DriveIDs`.
- Sensor paths (`SensorPath`, `FallbackSensorPath`) must resolve under
  `/sys/class/hwmon/` (path-traversal guard, matching existing sysfs validation).
- Drive IDs are sanity-checked against the `disks.ini` catalog as a **warning**,
  not a hard failure (the array composition can change).

### 8. Persistence & migration (`fan_config.go`)

- Assignments serialize in the new shape
  `{"profile_name": …, "source": {…}}`.
- **Load-path migration:** the persisted assignment type accepts both the legacy
  flat shape `{"ProfileName": …, "TempSensorPath": …}` and the new shape (custom
  `UnmarshalJSON` or a one-pass migrate). Legacy `TempSensorPath` →
  `Source{Type: hwmon, SensorPath: …}`. Existing installs keep working untouched
  and are rewritten in the new shape on next save.

## Safety

The emergency full-speed trigger (`CheckTemperatureSafety` →
`ReadMaxHwmonTemp`, default ≥ 90 °C) stays **hwmon-only**. Drive temperatures
drive _curves_, never the thermal-runaway cutoff — a spun-down array must never
be able to suppress the emergency path. This is a deliberate, documented
invariant.

## Error Handling

- `disks.ini` missing/unreadable → `ReadDiskTemps` returns an error; a `drives`
  source treats it as "no active drive reading" and falls back (no crash, no
  log spam — gated by the once-per-transition rule).
- All-spun-down with no fallback configured → no reading → hold last PWM
  (existing safe default).
- Invalid/implausible sensor reads → filtered by `IsPlausibleTempC`, treated as
  no reading.

## Testing

- `lib/disktemp_test.go` — fixture `disks.ini` parse: normal temps, `"*"`
  spun-down, empty value, missing file → error.
- `lib` hwmon-discovery test — label resolution, plausibility flags (incl. an
  unreliable-label and an out-of-range sensor flagged `false`).
- `fan_curves_test.go` — table-driven `resolveTemp`: hwmon primary;
  drives max-of-active; all-spun-down → fallback; fallback-also-invalid →
  no-reading; log-once-per-transition (pattern from `fan_safety_test.go`).
- Config migration test — legacy flat JSON unmarshals into new `Source`.
- API test — `GET /fans/sensors` schema; assign with `source` and with legacy
  `temp_sensor_path`.
- MCP test — `get_fan_sensors` registered; `set_fan_profile` accepts a source.

## Verification Gate (mandatory standing rule)

`go test ./...` + `make local` → `ansible-playbook -i ansible/inventory.yml
ansible/deploy.yml --tags build,deploy,verify` (failed=0) + live
`GET /fans/sensors` returns sensors & drives **and** a real drive-source profile
assignment succeeds on Unraid 7.3.1 → `coderabbit review --agent --base main`
clean → update `CHANGELOG.md` **last** (under the next version section).

## Affected Files

| File                                        | Change                                                                                                      |
| ------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `daemon/dto/fan.go`                         | Add `FanTempSource(Type)`, catalog DTOs; extend `FanDevice`, `FanProfileRequest`                            |
| `daemon/lib/disktemp.go`                    | **New** — `DiskTemp`, `ReadDiskTemps()`                                                                     |
| `daemon/lib/hwmon.go`                       | Add `DiscoverHwmonTempSensors()`                                                                            |
| `daemon/lib/validation.go`                  | Add `FanTempSource` validation                                                                              |
| `daemon/services/controllers/fan_curves.go` | `fanCurveAssignment.Source`, `resolveTemp`, `DriveTempProvider`, updated `AssignProfile`, log-once fallback |
| `daemon/services/controllers/fan.go`        | Update `AssignProfile` caller                                                                               |
| `daemon/services/controllers/fan_config.go` | New assignment shape + load-path migration                                                                  |
| `daemon/services/api/handlers.go`           | `handleFanSensors` handler; source resolution in assign handler                                             |
| `daemon/services/api/server.go`             | Register `api.HandleFunc("/fans/sensors", …).Methods("GET")` (near line 320)                                |
| `daemon/services/mcp/server.go`             | `get_fan_sensors` tool; source params on `set_fan_profile`                                                  |
| `CHANGELOG.md`                              | Entry (last)                                                                                                |
