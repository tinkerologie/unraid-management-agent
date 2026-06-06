# OS-Resilience & Compatibility Hardening — Design

**Date:** 2026-06-06
**Sub-project:** B (of a 3-part effort: A = Security/Access Control [descoped — LAN-only], B = OS-Resilience, C = Feature Parity)
**Status:** Approved design — ready for implementation planning

## Problem

The agent reads Unraid state directly (42 hardcoded paths in `daemon/constants/const.go`; parsing `var.ini`, `disks.ini`, `shares.ini`, etc.) and controls the system via binaries/`/proc` writes (`mdcmd`, `virsh`, Docker SDK, libvirt). When an Unraid OS update moves a path, changes a file format, or removes a binary, a collector can **silently emit empty or wrong data** as if healthy, and a controller can fail with a cryptic error. The goal: make the agent **detect** such breakage, **degrade gracefully and surface it** (never silently lie), and **catch it in tests** before it ships — without a large rewrite, and without depending on the official Unraid GraphQL API.

## Decisions (locked during brainstorming)

| Decision                     | Choice                                                                                                                                                     |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Scope                        | **Targeted hardening** — no full rewrite                                                                                                                   |
| Structure                    | **Approach 3 (Hybrid)** — status/detection backbone + thin resolver adopted by highest-risk subsystems first                                               |
| Runtime behavior on breakage | **Report + flag, keep best-effort data.** Never silently return wrong/empty data as healthy                                                                |
| Detection method             | **Capability + shape probing** (react to reality); Unraid version recorded as informational only — no version-string gating                                |
| Surfacing                    | Dedicated self-test surface (REST + MCP) **and** inline per-subsystem flags **and** auto-alert via the existing alerting engine **and** Prometheus metrics |
| External dependency          | **None** — stay self-contained (no official GraphQL API fallback)                                                                                          |

## Non-goals

- No authentication/RBAC (sub-project A, descoped — LAN-only deployment).
- No full OS-adapter rewrite; subsystems outside the hot path keep using `constants` unchanged.
- No dependency on the official Unraid API / Connect plugin.
- No new control features (that is sub-project C, which will build on this).

## Architecture

A new self-contained package **`daemon/platform`** is the resilience backbone. It imports only `dto`, `constants`, `lib`, `logger` (never `services/` or `domain`), so any layer can use it without import cycles.

### Components

**1. `dto.SourceStatus`** (placed in the leaf `daemon/dto` package to avoid cycles):

```go
type SourceState string // "healthy" | "degraded" | "unavailable"

type SourceStatus struct {
    Subsystem   string     `json:"subsystem"`
    State       SourceState `json:"state"`
    Reason      string     `json:"reason,omitempty"`
    LastChecked time.Time  `json:"last_checked"`
    LastError   string     `json:"last_error,omitempty"`
}
```

- `healthy` — source read **and** shape-validated OK.
- `degraded` — source read but a sanity/shape check failed; **best-effort data is still served**.
- `unavailable` — source/binary absent.

**2. `platform.Registry`** — thread-safe store of the current `dto.SourceStatus` per subsystem (`Set`, `Get`, `Snapshot`, `DegradedCount`). On a **state transition** it invokes an **injected notifier callback** `func(dto.SourceStatus)` (wired by the orchestrator to the event bus), keeping `platform` decoupled from `domain`. One `Registry` instance is created at startup and shared via `domain.Context`.

**3. `platform.Detector`** — runs once at startup: records the Unraid version (`/etc/unraid-version`, informational) and seeds the registry by probing the capabilities the hot-path subsystems need. Never panics; a probe failure records `unavailable`.

**4. `platform.Resolver`** — thin helpers `Resolve(path) (string, bool)` and `RequireBinary(name) (string, bool)` that wrap existing `constants` paths with an existence/capability check and record status. Adopted by the highest-risk subsystems first; everything else keeps using `constants`.

**Key principle:** `platform` only _observes and reports_. It never changes what a collector returns — best-effort data still flows; the status rides alongside it.

## Collector & controller integration

**Collectors (adopted ones report each run).** After parsing, a collector runs a small **shape validator** and reports to the registry:

- parse + validation OK → `healthy`;
- parsed but a sanity/shape check failed (e.g. `var.ini` missing expected keys, `disks.ini` yields zero disks) → `degraded` + reason, **still publishes valid partial data**;
- source/binary absent → `unavailable`.

Shape validators are tiny per-subsystem functions reusing `lib.ParseINIFile`, living beside each collector.

**Inline flag.** Key subsystem DTOs gain `SourceStatus *dto.SourceStatus json:"source_status,omitempty"`, populated **only when not healthy** — so healthy responses are byte-identical to today (backward-compatible).

**Controllers (capability gating).** Control ops check `platform.RequireBinary`/capability _before_ acting and return a clear `"<subsystem> unavailable: <reason>"` error instead of a cryptic shell failure — extending the existing native-with-fallback pattern (e.g. `/proc/mdcmd` → `mdcmd` binary).

**Adoption scope (highest-risk first):** the `system`, `array`, `disk`, `shares`, `docker`, and `vm` collectors + the `array`/`disk`/`docker`/`vm` controllers. Everything else is unchanged and can adopt later.

## Surfacing

All surfaces read the one shared registry, so they never disagree:

- **REST:** `GET /api/v1/diagnostics/self-test` → `{ unraid_version, overall_state, capabilities{…}, subsystems:[dto.SourceStatus…], timestamp }`. `overall_state` = worst subsystem state.
- **MCP:** new read-only tool `run_self_test` (tool #122 — bump skill/doc counts). Existing `get_health_status` / `get_diagnostic_summary` and `/api/v1/health/report` gain a `degraded_subsystems` summary.
- **WebSocket:** the `source_status_changed` transition event is broadcast (push) so dashboards update live.
- **Prometheus:** `unraid_subsystem_status{subsystem="…"}` gauge (0/1/2 = healthy/degraded/unavailable) + `unraid_degraded_subsystem_count`.

**Alerting:** feed `degraded_subsystem_count` (+ per-subsystem booleans) into the alerting engine's evaluation context and ship a **built-in, enabled-by-default template `subsystem_degraded`** (`degraded_subsystem_count > 0`, severity warning), routed through the existing alert/notification dispatch. (Exact metric-injection point pinned during plan-writing against the alerting engine's snapshot source.)

## Error handling

- Probes and validators **never panic** (defensive coding + existing collector panic-recovery). Failures become statuses, not crashes.
- A degraded/unavailable source is logged **once per transition** (not every run) to avoid log spam; the registry tracks transitions.
- Control-op capability failures return typed, human-readable errors.

## Testing

- **Golden-fixture parser tests.** Capture real, **sanitized** state files from the live Unraid (192.168.20.21) into `testdata/fixtures/unraid-<version>/` (`var.ini`, `disks.ini`, `shares.ini`, `network.ini`, …; redact GUID/license/serials/WireGuard keys). Parsers + shape validators run against every fixture dir; a new Unraid version is just a new dir.
- **Breakage-simulation tests (the crux).** Point each parser at malformed/empty/renamed fixtures and assert it reports `degraded`/`unavailable` with a reason and **still returns best-effort partial data — never panics, never reports healthy.**
- **Unit tests** for `platform`: detector probes against temp dirs/missing binaries → `unavailable`; registry transitions invoke the notifier exactly once per transition; resolver existence checks.
- **Live verification:** extend the ansible `verify` role to assert `GET /diagnostics/self-test` → `overall_state: healthy` on the real box, alongside existing endpoint checks.

## Rollout (incremental — each step shippable)

1. `platform` pkg + `dto.SourceStatus` + registry/detector/resolver + unit tests.
2. Wire registry into `domain.Context` + orchestrator (notifier→event bus; startup detection).
3. Adopt the 6 hot collectors one at a time (validator + status + inline flag + fixtures).
4. Capability-gate the 4 hot controllers.
5. Surfaces: REST self-test, MCP tool, health integration, WS event, Prometheus metrics.
6. Alerting template + metric injection.
7. Golden-fixture harness + CI + docs (configuration.md, mcp.md counts, AGENTS.md) + CHANGELOG.
8. Deploy to Unraid + verify (healthy self-test; simulate a degradation; confirm the alert).

## Compatibility

Healthy responses are byte-identical to today (inline flag is `omitempty`, populated only when not healthy). The new endpoint, MCP tool, Prometheus metrics, WS event, and alert template are all additive. **No breaking changes.**

## Success criteria

- An induced breakage (missing/renamed path, malformed file, removed binary) results in a `degraded`/`unavailable` status with a reason, an inline flag, a self-test entry, a Prometheus signal, and a fired `subsystem_degraded` alert — **not** silent wrong/empty data and **not** a crash.
- Best-effort valid data continues to flow during degradation.
- Healthy-state API responses are unchanged from the prior release.
- Golden-fixture + breakage-simulation tests pass in CI; live self-test reports healthy on the real server.
