# Continuous Container Update Detection — Design

**Date:** 2026-05-30
**Status:** Approved (design)
**Scope:** First slice of a larger "smarter agent" initiative. Follow-on slices (AI remediation toolkit, Prometheus exporter + smart alerts, coverage-gap bundle) are separate specs.

## Problem

The plugin already has the data model and logic to detect Docker container updates
(`dto.ContainerUpdateInfo`, `DockerController.CheckContainerUpdate` /
`CheckAllContainerUpdates` using `DistributionInspect` for bandwidth-free registry
digest comparison). **But it is on-demand only.** The continuous Docker collector
(`docker.go`) never populates update status, so `GET /docker`, `list_containers`,
and the WebSocket stream never tell a user or AI agent what is outdated without a
separate explicit check. There is no push event when an update appears and no alert
rule can fire on it.

The official Unraid GraphQL API, by contrast, exposes `isUpdateAvailable` on every
`DockerContainer`, a `containerUpdateStatuses` query, and a `refreshDockerDigests`
mutation. This slice closes that gap.

## Key Constraint

`DistributionInspect` is a registry manifest call. It is bandwidth-free (no image
pull) but still hits the registry over the network, and **Docker Hub rate-limits
anonymous manifest requests**. Update checks therefore **cannot** run at the Docker
collector's 30s cadence. They require their own slow cadence (hours), low
concurrency, and startup staggering. This constraint drives the architecture.

## Approach (selected)

**Dedicated `DockerUpdate` collector** running on an independent long interval. The
existing 30s Docker collector is untouched. Update status is merged into each
`ContainerInfo` at API read time, keeping the two collectors fully decoupled. No
database — update status lives in memory and is re-checked after restart (Tier-0
persistence model).

Rejected alternatives:
- **Sub-cadence inside the Docker collector** — couples a network/rate-limit-bound
  operation into the CPU-bound stats loop; harder to configure independently.
- **On-demand + cache only** — does not deliver continuous detection; defeats the goal.

## Architecture

```
DockerUpdate collector (every 6h, staggered start)
   └─ CheckAllContainerUpdates()  ── publishes ──▶ topic: docker_updates_update
                                                        │
                          ┌─────────────────────────────┼───────────────────────┐
                          ▼                             ▼                         ▼
              API: dockerUpdatesCache          WebSocket Hub             Alert engine
              (map[id]ContainerUpdateInfo)   (event: docker_updates,    (metric:
                          │                   broadcast on change)       docker.updates_available)
                          ▼
        Read-merge into ContainerInfo on
        GET /docker, /docker/{id},
        MCP list_containers / get_container_info
```

## Components

### 1. `DockerUpdate` collector — `daemon/services/collectors/docker_update.go`
- Standard collector pattern: goroutine, `defer/recover` panic wrappers, `ctx.Done()`.
- **Startup stagger:** wait 30–60s after boot before first `Collect()` to avoid
  piling onto startup, then tick on `IntervalDockerUpdate`.
- `Collect()` calls existing `DockerController.CheckAllContainerUpdates()`
  (semaphore of 5 already limits concurrency), builds a
  `map[containerID]dto.ContainerUpdateInfo`, and publishes the full
  `dto.ContainerUpdatesResult` on topic `docker_updates_update`.
- Registered in `collector_manager.go` `RegisterAllCollectors()`.

### 2. Constants — `daemon/constants/const.go`
- `IntervalDockerUpdate = 21600` (6 hours). Adjustable at runtime via the existing
  `PATCH /collectors/{name}/interval` mechanism for users on metered/rate-limited links.
- Event topic constant `docker_updates_update`.

### 3. Data model — `daemon/dto/docker.go`
Extend `ContainerInfo`:
```go
UpdateStatus    string     `json:"update_status" example:"up_to_date"` // up_to_date | update_available | unknown
UpdateAvailable *bool      `json:"update_available,omitempty"`          // nil = not yet checked / registry unreachable
UpdateChecked   *time.Time `json:"update_checked,omitempty"`
```
The tri-state (`*bool` + `"unknown"`) lets us honestly distinguish "no update" from
"haven't checked yet / registry unreachable". `DistributionInspect` legitimately
fails for private registries and rate-limits; we must never show "up to date" when
we do not know.

### 4. API server — `daemon/services/api/server.go`, `handlers.go`
- New `dockerUpdatesCache map[string]dto.ContainerUpdateInfo` guarded by the existing
  `cacheMutex`, populated from the `docker_updates_update` subscription (added to both
  `Hub.Sub()` and the `subscribeToEvents()` switch).
- **Read-merge:** `handleGetDocker` / `handleGetContainer` copy the container list
  under `RLock` and overlay each container's update fields from `dockerUpdatesCache`.
  Containers absent from the cache report `update_status: "unknown"`. The 30s
  `containerListCache` is never mutated — the merge is a pure read-side join.

### 5. REST endpoints
- `GET /api/v1/docker`, `/docker/{id}` — now include update fields via merge.
- `GET /api/v1/docker/updates` — **repointed to serve the cached result** (instant,
  no registry traffic) instead of triggering a live check.
- `POST /api/v1/docker/updates/refresh` — **new**: force an immediate out-of-band
  re-check so users/agents are not stuck waiting up to 6h. Mirrors Unraid's
  `refreshDockerDigests`.

### 6. WebSocket — `daemon/services/api/websocket.go`
- Subscribe hub to `docker_updates_update`; broadcast as event `docker_updates` with
  the `ContainerUpdatesResult`.
- **Change-detection:** only broadcast when the result differs from the last published
  one, to avoid a 6-hourly no-op event.

### 7. Alerting + notifications
- Expose aggregate metric `docker.updates_available` (count) to the existing alert
  engine so users can create a rule "alert when `updates_available > 0`".
- On a **transition** (container goes up-to-date → update-available), optionally raise
  an Unraid notification via `NotificationController`
  (e.g. *"3 container updates available: plex, sonarr, radarr"*).
  **Opt-in** via a settings toggle to avoid notification spam.

### 8. MCP — `daemon/services/mcp/server.go`
- `check_container_update` / `check_container_updates` remain (live, on-demand).
- `list_containers` / `get_container_info` output gains the cached update fields.
- New `refresh_container_updates` tool mapping to `POST /docker/updates/refresh`.
- Update `docs/integrations/mcp.md`.

## Data Flow

1. DockerUpdate collector ticks (every 6h) → `CheckAllContainerUpdates()` →
   `ContainerUpdatesResult`.
2. Publishes to `docker_updates_update`.
3. API server updates `dockerUpdatesCache`; alert engine updates
   `docker.updates_available`; WebSocket hub broadcasts `docker_updates` if changed;
   notification raised on new-update transition (if opt-in enabled).
4. `GET /docker` merges cached update status into each `ContainerInfo` at read time.

## Error Handling

- `DistributionInspect` failure (auth/private registry/rate-limit/registry down) →
  that container reports `update_status: "unknown"` (`UpdateAvailable == nil`), never
  a false "up to date". Already handled in `CheckContainerUpdate`'s fallback path;
  preserve it.
- Collector panics caught by the standard `defer/recover` wrapper.
- Manual refresh while a scheduled check is in flight: guard with a mutex/flag so only
  one check runs at a time (the controller does the heavy work; avoid concurrent
  registry storms).

## Persistence

None. Update status is in-memory; lost on restart and re-checked on the next cycle
(after the startup stagger). Matches the plugin's existing in-memory cache model and
the Tier-0 low-overhead approach — no USB-flash writes, no DB dependency.

## Testing

Table-driven tests (mock the controller — no real registry calls):
- Merge logic: cached / uncached / unknown cases.
- Tri-state correctness when `DistributionInspect` fails.
- WebSocket change-detection (no broadcast on identical result).
- Alert metric count (`docker.updates_available`).
- Notification raised only on up-to-date → update-available transition.

## Out of Scope (follow-on specs)

- Auto-update on schedule / self-healing (separate "Automation & self-healing" slice).
- `isRebuildReady` semantics for `network_mode: container:<x>` chains.
- Prometheus exporter and historical trend storage.
- Plugin/OS update availability.

## Acceptance Criteria

- [ ] `GET /docker` and `list_containers` return `update_status` for every container
      without any explicit check call.
- [ ] A new collector checks updates on an independent, runtime-configurable interval
      (default 6h) with a staggered start.
- [ ] `GET /docker/updates` serves cached data instantly; `POST /docker/updates/refresh`
      forces a fresh check.
- [ ] WebSocket emits `docker_updates` only when status changes.
- [ ] An alert rule can fire on `docker.updates_available > 0`.
- [ ] Containers with unreachable/private registries report `"unknown"`, never a false
      "up to date".
- [ ] Tests cover merge, tri-state, change-detection, alert metric, and notification
      transition. `make test` and `make pre-commit-run` pass.
- [ ] CHANGELOG.md and `docs/integrations/mcp.md` updated.
