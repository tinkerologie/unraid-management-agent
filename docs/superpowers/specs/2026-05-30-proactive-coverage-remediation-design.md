# Proactive Intelligence + Coverage Gaps + AI Remediation Toolkit — Design

**Date:** 2026-05-30
**Status:** Approved (design)
**Scope:** Three brainstormed areas combined into one phased plan. Builds on the merged continuous-container-update work (collector pattern, alert metric pattern, read-merge cache pattern).

## Goal

1. **Proactive/predictive intelligence** — built-in trend alerts (disk-fill ETA, temp climbing, container flapping, predictive SMART) via a low-overhead in-memory ring-buffer history. No database.
2. **Coverage gaps** — container network I/O, Docker networks listing, continuous plugin/OS update status, detailed mover stats.
3. **AI remediation toolkit** — a unified health-and-remediation MCP tool (recommend + execute-with-confirm), NL queries over the new history, plus root-cause/runbook helpers.

## Approved decisions

- **Remediation toolkit:** recommend **and** execute, but only when the caller passes an explicit `confirm: true`. No auto-execution. Reuses existing controller actions.
- **OS updates:** **best-effort, local-file only** — read whatever Unraid's own update check wrote locally; degrade to `unknown` if absent. **No external network calls.**
- **Mover stats:** **conservative** — running-state + schedule + last-run duration/files/bytes parsed from `/var/log/mover.log`; best-effort live throughput. No fragile internals.

## Key overhead constraint (re-affirmed)

The history layer is **Tier-0**: bounded in-memory ring buffers sampled on the existing 15s alert eval tick. A few MB RAM, zero disk writes (never the USB boot flash), negligible CPU. Lost on restart and rebuilt — acceptable for alerting.

---

## Architecture (3 phases, one plan)

```
Phase A: MetricsHistory sampler (alerting pkg)
   └─ on each 15s eval tick: record bounded ring buffers (global + per-disk + per-container)
   └─ Derive() → new trend fields on dto.AlertEnv → evaluator (existing) → alerts
   └─ Query API → used by Phase C history tool

Phase B: coverage collectors/fields
   ├─ docker.go: + RestartCount, + NetworkRX/TX (from /proc/<pid>/net/dev)
   ├─ docker_networks collector → topic/cache/REST/MCP
   ├─ plugin_update collector (mirror docker_update) → topic/cache/REST/alert metric
   ├─ os_update collector (best-effort local file) → topic/cache/REST
   └─ mover stats (parse mover.log + var.ini) → topic/cache/REST

Phase C: remediation pkg + MCP tools (build on A + existing diagnostics/controllers)
   ├─ system_health_report (aggregate + recommend [+ execute w/ confirm])
   ├─ query_metric_history (NL queries over Phase-A buffers)
   └─ root-cause + runbook helpers
```

Phase A is a prerequisite for the Phase C history tool. Phases B is independent and can interleave.

---

## Phase A — Proactive / predictive intelligence

### A1. `MetricsHistory` sampler — `daemon/services/alerting/history.go`
- Bounded ring buffers of `(timestamp, value)`:
  - **Global series:** `cpu_temp`, `ram_used_pct`, `array_used_pct`, `array_free_bytes`.
  - **Per-disk series** (keyed by disk ID): `temp`, `reallocated_sectors`, `pending_sectors`, `used_pct`.
  - **Per-container series** (keyed by container ID): `restart_count`.
- Capacity: max N samples (default 240 = 1h at 15s) **and** max age (default 1h), whichever first. Per-entity maps pruned when an entity disappears.
- `Record(provider DataProvider)` — called by the engine each eval tick before `buildEnv`.
- `Derive()` — returns computed trend values (see A2) using least-squares slope over each series.
- Thread-safe (`sync.RWMutex`) because the query API (Phase C) reads concurrently.
- Helpers: `slope(series) float64` (value-units per second), `etaToThreshold(series, threshold) float64` (hours until projected to reach threshold; returns a sentinel like `-1` when not trending toward it).

### A2. New `dto.AlertEnv` trend fields (with `expr:"..."` tags)
- `ArrayFillETAHours float64` — projected hours until array reaches ~100% (from `array_used_pct` slope).
- `MaxDiskFillETAHours float64` — worst-case per-disk fill ETA.
- `CPUTempSlopePerMin float64` — °C/min.
- `MaxDiskTempSlopePerMin float64` — worst per-disk °C/min.
- `MaxContainerRestartsPerHour float64` — worst container restart rate (flapping).
- `MaxReallocatedSectors int` — worst current reallocated-sector raw value across disks (point-in-time).
- `MaxPendingSectors int` — worst current pending-sector raw value.
- `DiskErrorsIncreasing bool` — any disk's reallocated/pending/SMARTErrors increased over the window.

### A3. Engine wiring — `daemon/services/alerting/engine.go`
- Engine owns a `*MetricsHistory`. In the eval loop: `history.Record(provider)` → `env := buildEnv()` → overlay `history.Derive()` fields onto `env` → evaluate.
- `NewEngine` constructs the history with configurable window (constants, defaults above).

### A4. Collector addition — `RestartCount` on `ContainerInfo`
- `daemon/dto/docker.go`: add `RestartCount int json:"restart_count"`.
- `daemon/services/collectors/docker.go`: in the inspect loop, set from `inspectData.RestartCount` (the inspect already runs for running containers).

### A5. Default trend alert rule templates (disabled by default)
- Ship a small set the user can enable: e.g. `ArrayFillETAHours > 0 && ArrayFillETAHours < 72`, `MaxDiskTempSlopePerMin > 1`, `MaxContainerRestartsPerHour >= 5`, `MaxReallocatedSectors > 0`, `DiskErrorsIncreasing`. Provided via an MCP/REST helper that lists template rules (not auto-created).

---

## Phase B — Coverage gaps

### B1. Container network I/O — `daemon/services/collectors/docker.go`
- Extract `pid := inspectData.State.Pid` in the running-container inspect loop.
- New `getNetworkFromProc(pid int, fullID string, cont *dto.ContainerInfo)`: read `/proc/<pid>/net/dev`, sum RX/TX bytes across non-loopback interfaces → `cont.NetworkRX`/`cont.NetworkTX`.
- New `prevNet map[string]netSnapshot` (mirrors `prevCPU`) for per-sec rates; add `NetworkRXBytesPerSec`/`NetworkTXBytesPerSec` to `ContainerInfo`. First sample → rates 0. Prune stale on the existing prune path.
- Graceful: pid 0 / unreadable proc → leave zero, no error spam.

### B2. Docker networks listing
- DTO `dto.DockerNetworkInfo` (`daemon/dto/docker_network.go`): ID, Name, Driver, Scope, Internal, Attachable, IPAM (subnet/gateway), connected container names/IPs, Created, Labels.
- New `DockerNetworksCollector` (`daemon/services/collectors/docker_networks.go`): `client.NetworkList` + `NetworkInspect`; topic `TopicDockerNetworksUpdate`; long-ish interval (e.g. 60s — networks change rarely). Registered in manager + main intervals + CLI flag.
- Cache binding + `GET /api/v1/docker/networks` handler + MCP `list_docker_networks` tool. Swagger.

### B3. Continuous plugin updates — mirror `docker_update`
- `PluginUpdateCollector` (`daemon/services/collectors/plugin_update.go`): `CheckFn` injected (factory) → `controllers.NewPluginController().CheckPluginUpdates()`; dedupe signature over (plugin, version); opt-in `NotifyFn`; topic `TopicPluginUpdatesUpdate`; default interval 1h (`IntervalPluginUpdate`).
- Cache (`*dto.PluginList` or a `PluginUpdatesResult`) + repoint `GET /api/v1/plugins`/`/updates` to serve cache + `POST /plugins/updates/refresh`.
- Alert metric `PluginUpdatesAvailable int` (`expr` tag) populated in `buildEnv` from the cache (via a new `DataProvider.GetPluginUpdatesCache()` accessor — additive interface change).
- MCP: keep on-demand `check_plugin_updates`; add `refresh_plugin_updates`.

### B4. OS update availability — best-effort local file
- `OSUpdateCollector` (`daemon/services/collectors/os_update.go`): reads Unraid's locally-cached update-check result if present (the dynamix update check writes a status; exact path verified at implementation — candidates under `/tmp/` or `/var/local/emhttp/`). If no local data → report `UpdateAvailable=false, Status="unknown"`. **No external HTTP.**
- DTO `dto.OSUpdateStatus`: CurrentVersion, LatestVersion, UpdateAvailable, Status (`up_to_date`/`update_available`/`unknown`), Timestamp. Topic `TopicOSUpdateUpdate`, cache, `GET /api/v1/os/update`. Long interval (daily). Opt-in notify on transition.
- Honest degradation: if the local file can't be located/parsed, ship the collector returning `unknown` and document it; never fabricate a latest version.

### B5. Detailed mover stats — conservative
- DTO `dto.MoverStatus`: Active (running), Schedule, LastRunStart, LastRunFinish, LastRunDurationSeconds, LastRunFilesMoved, LastRunBytesMoved, CurrentThroughputMBs (best-effort, only while active), Timestamp.
- `MoverCollector` (`daemon/services/collectors/mover.go`): Active from `var.ini` (`shareMoverActive`) and/or process check; parse `/var/log/mover.log` for the most recent run’s start/finish timestamps → duration, and files/bytes if the log records them (mover `-v`); throughput best-effort by sampling array/cache write deltas while active. Topic, cache, `GET /api/v1/mover`. Short-ish interval while active, long while idle (or fixed 30s). MCP `get_mover_status`.

---

## Phase C — AI remediation toolkit

### C1. Shared remediation executor — `daemon/services/remediation/` (new, small)
- `Executor` mapping action strings (`restart_container`/`stop_container`/`start_container`/`restart_vm`/`stop_vm`/`start_vm`/`force_stop_vm`) → existing `controllers`. Returns `(ok, durationMs, err)`.
- **Reuses existing controllers; does not refactor dispatcher.go/watchdog yet** (noted as future dedup) to avoid regressing the merged alert/watchdog paths.

### C2. `system_health_report` MCP tool (+ `GET /api/v1/health/report`)
- Aggregates: `get_diagnostic_summary` data + `GetHealthStatus()` + firing alerts (`alerts/firing`) + watchdog statuses + Phase-A derived trends/predictive signals.
- Returns a structured report: prioritized findings (severity-ranked) each with a human explanation and **recommended actions** (action strings + targets).
- Optional input `confirm: true` + an explicit `actions: []` list → executes those via C1 and returns per-action results. Without `confirm`, it only recommends. Validates every target via existing `lib.Validate*` before executing.

### C3. `query_metric_history` MCP tool (+ `GET /api/v1/metrics/history`)
- Reads the Phase-A ring buffers: params = series name (+ optional entity id), window. Returns the samples + computed slope/min/max/avg. Enables "CPU temp last hour", "disk X reallocated-sector trend".

### C4. Root-cause + runbook helpers (MCP)
- `find_root_cause` (prompt-style, like existing diagnostics) that correlates current signals (high CPU → top container by CPU; slow array → parity running; high temp slope → which disk).
- `list_runbooks` / `run_runbook` — named remediation sequences (e.g. `restart_unhealthy_containers`, `update_outdated_containers`) that, with `confirm`, execute via C1. Runbooks are a static, reviewed set.

---

## Error handling & safety

- All new collectors: standard `defer/recover` panic wrappers, `ctx.Done()` handling, startup stagger where they hit external surfaces (plugin/os checks).
- Network/proc reads: missing files → zero values, debug-log only (no error spam).
- Remediation execution: gated behind explicit `confirm`; every target validated; errors logged server-side, generic message to client; no destructive defaults.
- History sampler: bounded memory (count + age cap); per-entity maps pruned; never persisted to disk.
- OS-update & mover: degrade gracefully to `unknown`/idle; never fabricate data or make external calls (OS) beyond reading local files/logs.

## Persistence

None new. History is in-memory (Tier-0). Update/network/mover caches follow the existing atomic-pointer cache pattern.

## Testing

Table-driven, mock controllers/providers (no real Docker/registry/network):
- History: slope/ETA math (known series → expected slope/ETA), ring-buffer bounding (count + age), per-entity prune.
- AlertEnv trend fields populated from a synthetic history.
- Container network parse (`/proc/net/dev` sample fixture → RX/TX + rates; first-sample zero).
- Docker networks mapping (mock NetworkList).
- plugin_update / os_update collectors: publish-on-change + dedupe + unknown degradation (mock CheckFn / fixture files).
- mover: log-fixture parse (duration/files/bytes), idle vs active.
- remediation executor: action→controller dispatch + validation rejects bad targets; `system_health_report` recommend vs execute-with-confirm.
- `query_metric_history`: returns samples + stats.

## Out of scope (future)

- Refactoring dispatcher.go/watchdog to use the shared executor (dedup).
- Persisting history across restarts.
- External OS-version checks.
- Auto-execution of remediation without confirm.

## Acceptance criteria

- [ ] Ring-buffer history samples on the eval tick; bounded (count+age); no disk writes; prunes vanished entities.
- [ ] New trend AlertEnv fields populate and are alertable (`ArrayFillETAHours`, `MaxDiskTempSlopePerMin`, `MaxContainerRestartsPerHour`, `MaxReallocatedSectors`, `DiskErrorsIncreasing`, etc.).
- [ ] `RestartCount` collected; container network RX/TX + per-sec populated from `/proc/<pid>/net/dev`.
- [ ] `GET /docker/networks` + MCP tool return networks.
- [ ] Continuous plugin updates: collector + cached `GET /plugins` + `refresh` + `PluginUpdatesAvailable` alert metric.
- [ ] OS update status served (best-effort, `unknown` when no local data; no external calls).
- [ ] Mover status: active/schedule/last-run duration+files+bytes from log; `GET /mover` + MCP.
- [ ] `system_health_report` aggregates signals + recommends; executes only with explicit `confirm`, validating targets.
- [ ] `query_metric_history` returns series + stats.
- [ ] Root-cause + runbook tools present.
- [ ] `make test` and feature-file lint pass; swagger regenerated; CHANGELOG + mcp.md updated; deploy+verify on Unraid clean.
