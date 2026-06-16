# Defer to External Fan Controllers — Design

**Date:** 2026-06-16
**Status:** Approved (brainstorming)
**Author:** Ruaan Deysel (with Claude Code)
**Issue:** [#128](https://github.com/ruaan-deysel/unraid-management-agent/issues/128)

## Problem

When a user runs a third-party fan-control plugin (FanCTRL Plus or Dynamix Auto
Fan Control) the agent can fight it for control of the same hwmon sysfs files
(`pwmN_enable`, `pwmN`). The user reports that "every time this plugin has
updated automatically, all fans go to 100%."

Investigation on the live server (Unraid 7.3.1) confirmed:

- **FanCTRL Plus** is installed and active (`service="1"`, `array_monitor.sh`
  running). Its loop owns a fan by writing `1 > pwmN_enable` (manual) then
  `pwm_val > pwmN` (`fanctrlplus_loop.sh:146-147`).
- **Dynamix Auto Fan Control** is installed but disabled (`service="0"`); when
  active it does the same (`autofan:267-268`).
- The agent's own `control_enabled` is `false` (active control is OFF).

Every _active-control_ path in the agent (`SetSpeed`, the curve engine,
`EmergencyFullSpeed`) is correctly gated behind `control_enabled`. **One write
path is not gated:** `FanController.Shutdown()` → `FanSafetyGuard.RestoreAll()`
(`orchestrator.go:262`, `fan_safety.go:84`). On **every daemon shutdown** —
including the plugin auto-updates the user describes — it writes `pwmN_enable`
to _every controllable fan_, even when the user never enabled agent fan control.
That write yanks the enable-mode out from under FanCTRL Plus. This is the root
cause of the reported symptom.

Separately, the agent has no awareness of a third-party fan controller and no way
to "not touch fan speeds," which is the issue's explicit ask.

## Goal

1. The agent must never write to fan sysfs that it didn't change (fix the
   unguarded shutdown write).
2. When a known third-party fan controller is installed **and enabled**, the
   agent automatically stays **monitor-only** and refuses all fan writes.
3. The deferral is **visible** in the API / dashboard / MQTT so the user can
   confirm "the agent is not touching my fans."

## Non-Goals (YAGNI)

- **A user-facing override flag** to force agent control over an active plugin.
  Not requested; a power user disables the other plugin instead.
- **Generic "any fan in manual mode" heuristic.** Detection is limited to the
  two named plugins to avoid false positives.
- **Changing how the agent controls fans when no plugin is present.** Untouched.
- **Detecting every fan plugin in existence.** Only FanCTRL Plus and Dynamix Auto
  Fan Control, the two the issue concerns.

## Decisions (from brainstorming)

- **Behavior on conflict:** auto stand-down — monitor-only, refuse _all_ fan
  writes (including the emergency 100% override; the active plugin owns safety).
- **Detection:** named plugins via config + process (not bare install-dir
  presence, which would flag a disabled plugin).

## Design

### 1. Detection — `lib.DetectExternalFanControl()`

A shared function (used by both the collector and the controller) returning:

```go
type ExternalFanControl struct {
    Active      bool     `json:"active"`
    Controllers []string `json:"controllers,omitempty"` // human names, e.g. "FanCTRL Plus"
}
```

A controller is active when its plugin is **installed AND enabled**:

| Plugin                   | Installed (dir)                                    | Enabled signal                                                                                                                             |
| ------------------------ | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| FanCTRL Plus             | `/usr/local/emhttp/plugins/fanctrlplus`            | any `/boot/config/plugins/fanctrlplus/*.cfg` contains `service="1"` **OR** a `fanctrlplus_loop.sh` / `array_monitor.sh` process is running |
| Dynamix Auto Fan Control | `/usr/local/emhttp/plugins/dynamix.system.autofan` | any `/boot/config/plugins/dynamix.system.autofan/*.cfg` contains `service="1"` **OR** an `autofan` process is running                      |

- **Cheap path:** the config-file check (a few `stat`/read calls) short-circuits;
  the `/proc` cmdline scan runs only when no enabled cfg is found.
- **Test seam:** the sysfs/flash/proc roots are package-level variables (or
  function parameters) overridable in tests so detection can point at temp dirs.

### 2. Stand-down in `FanController`

- `Initialize()` runs detection once, stores the result, and logs clearly when
  active: `Fan control: Detected active third-party fan control (FanCTRL Plus);
staying monitor-only and will not modify fan speeds`.
- When active, the write methods refuse with an explanatory error:
  `SetSpeed`, `SetMode`, `SetProfile`, `RestoreDefaults`.
- `GetStatus()` skips `EmergencyFullSpeed` when active (it still logs the
  critical-temperature warning so the condition is visible — it just doesn't
  write).
- The curve engine needs no separate guard: it only starts via `SetProfile`,
  which is now refused.

### 3. Restore only what the agent changed (`HwmonProvider` + `RestoreAll`)

- `HwmonProvider` records the fan IDs it has written to (`SetPWM` / `SetMode`
  mark the fan; a mutex-guarded `map[string]bool`; `ModifiedFans()` accessor).
  These two methods are the single funnel for every agent fan write.
- `FanSafetyGuard.RestoreAll()` iterates `hwmon.ModifiedFans()` instead of every
  captured fan, restoring each one's captured original mode/PWM.
- `CaptureState()` stays as-is (read-only, captures originals up front), so an
  original is always available for any fan that later gets modified.

Result: when the agent never modifies a fan (control disabled _or_ external
control active), `ModifiedFans()` is empty and shutdown writes nothing.

### 4. Surface the state

- Add `ExternalControl *ExternalFanControl` to `dto.FanControlStatus`.
- `FanController.GetStatus()` populates it (from the cached Initialize result).
- `FanControlCollector.Collect()` populates it via the same `lib` function so the
  WebSocket / MQTT / dashboard feed reflects deferral even with control disabled.

## Data Flow

```
Initialize ──> lib.DetectExternalFanControl() ──> store on FanController
                                              └──> log if active

API/MCP write ──> FanController.Set* ──> if external.Active: return error
                                    └──> else hwmon.Set* (marks modified)

Collector tick ──> lib.DiscoverHwmonFans() + lib.DetectExternalFanControl()
                                          └──> publish FanControlStatus (incl. ExternalControl)

Shutdown ──> RestoreAll() ──> for id in hwmon.ModifiedFans(): restore original
```

## Error Handling

- Refused writes return a clear error naming the active controller, so the API
  surfaces _why_ (e.g. `fan control deferred to active plugin: FanCTRL Plus`).
- Detection is best-effort: any unreadable cfg / proc entry is skipped, never
  fatal. Detection failure defaults to "not active" (agent behaves as today),
  which is the safe direction because #3 already prevents stray shutdown writes.

## Testing

- `lib` detection: table-driven over temp dirs — installed+enabled,
  installed+disabled (`service="0"`), not installed, enabled-by-process-only.
- `RestoreAll`: restores only fans recorded as modified; writes nothing when none
  were modified.
- `FanController`: each write method refuses when external control is active;
  `GetStatus` reports `ExternalControl.Active`.

## Affected Files

- `daemon/lib/fancontrol_external.go` (new) + test
- `daemon/dto/fan.go` (DTO field)
- `daemon/services/controllers/fan.go` (detect, refuse, status)
- `daemon/services/controllers/fan_hwmon.go` (modified-fan tracking)
- `daemon/services/controllers/fan_safety.go` (`RestoreAll` uses modified set)
- `daemon/services/collectors/fancontrol.go` (publish external status)
- `CHANGELOG.md`
