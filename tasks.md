# Enhancement Tasks: Unraid Management Agent

Prioritized backlog of planned enhancements. Each feature includes researched Go libraries with verified versions, import paths, and GitHub URLs.

**Legend:**

- `[P]` — Can be implemented in parallel with other items in the same phase
- Priority: 🔴 High · 🟡 Medium · 🟢 Low
- Effort: S (days) · M (1–2 weeks) · L (3+ weeks)

---

## Already Implemented ✅

- [x] Prometheus metrics endpoint (`GET /metrics` — 41 metrics)
- [x] Grafana integration (`docs/integrations/grafana.md`)
- [x] MQTT publishing (`daemon/services/mqtt/`)
- [x] Plugin management — check + update (`daemon/services/controllers/plugin.go`)
- [x] Service management (`daemon/services/controllers/service.go`)
- [x] Process management (`daemon/services/controllers/process.go`)
- [x] ZFS pools / datasets / snapshots / ARC stats
- [x] Parity history collector
- [x] NUT UPS support (alongside apcupsd)
- [x] User scripts execution
- [x] File watcher collector
- [x] Settings + config collectors

---

## Dependency Summary (New Libraries)

All libraries verified as pure Go (no CGO) unless noted.

| Feature | Library | Import Path | Version | GitHub |
|---|---|---|---|---|
| Alerting rules engine | expr-lang/expr | `github.com/expr-lang/expr` | v1.17.8 | <https://github.com/expr-lang/expr> |
| Notification dispatch (all-in-one) | nicholas-fedor/shoutrrr | `github.com/nicholas-fedor/shoutrrr` | v0.13.2 | <https://github.com/nicholas-fedor/shoutrrr> |
| Time-series storage | bbolt | `go.etcd.io/bbolt` | v1.4.3 | <https://github.com/etcd-io/bbolt> |
| Cron scheduler | gocron v2 | `github.com/go-co-op/gocron/v2` | v2.19.1 | <https://github.com/go-co-op/gocron> |
| Git backup | go-git v5 | `github.com/go-git/go-git/v5` | v5.16.5 | <https://github.com/go-git/go-git> |
| Speedtest | speedtest-go | `github.com/showwin/speedtest-go/speedtest` | v1.7.10 | <https://github.com/showwin/speedtest-go> |

**Not used (reasons):**

- `robfig/cron v3` — last release Jan 2020, 111 open PRs, effectively unmaintained; use `gocron v2` instead
- `mattn/go-sqlite3` — requires CGO; breaks cross-compilation for linux/amd64 from macOS
- `github.com/nakabonne/tstorage` — last release March 2023, maintenance uncertain; use bbolt instead
- `github.com/prometheus/prometheus/tsdb` — massive dependency tree, designed for Prometheus itself, overkill for this use case
- `nikoksr/notify` — pulls entire AWS SDK / Firebase / Twilio into `go.mod` even for HTTP-only use

---

## Phase 1: Quick Wins (High Value, Low Effort)

### 1.1 — AI-Powered Diagnostic Prompts (MCP) 🔴 · Effort: S

**What:** Add higher-level MCP `Prompts` that guide an AI agent through structured diagnostic workflows. No new collectors, no new libraries — leverages existing 54 tools and cache data.

**No new dependencies required.**

- [ ] T001 [P] Add `diagnose_disk_health` prompt in `daemon/services/mcp/server.go` — walks through SMART data, temps, reallocated sectors, error rates and produces a plain-English verdict
- [ ] T002 [P] Add `diagnose_performance_issue` prompt — correlates CPU, RAM, Docker resource usage, and running VM count to identify bottlenecks
- [ ] T003 [P] Add `suggest_maintenance` prompt — reviews parity history, disk ages, array errors, and temps to generate a prioritised maintenance checklist
- [ ] T004 [P] Add `explain_array_state` prompt — translates raw array status into human-readable context (e.g. why parity is running, what a degraded state means)
- [ ] T005 Register all prompts in `registerPrompts()` in `daemon/services/mcp/server.go`
- [ ] T006 Add tests in `daemon/services/mcp/tools_test.go`

**Checkpoint:** An AI agent can call `diagnose_disk_health` and receive a structured verdict using only existing cached data.

---

### 1.2 — Docker Image Update Detection 🔴 · Effort: S

**What:** Detect when running containers have newer image digests available on their registry. Detection only — no auto-update by default. Uses the Docker Engine SDK already present in the project (`github.com/moby/moby/client`).

**No new dependencies required** — uses existing `github.com/moby/moby/client`.

- [ ] T007 Add `ImageUpdateStatus` fields to `daemon/dto/docker.go`:

  ```go
  UpdateAvailable bool   `json:"update_available"`
  LatestDigest    string `json:"latest_digest,omitempty"`
  CurrentDigest   string `json:"current_digest,omitempty"`
  ```

- [ ] T008 Implement `CheckImageUpdate(containerID string)` in `daemon/services/controllers/docker.go` — uses Docker SDK `ImageInspectWithRaw` for local digest and `DistributionInspect` for remote digest comparison
- [ ] T009 Add `GET /api/v1/docker/updates` endpoint in `daemon/services/api/handlers.go` — returns all containers with update availability status
- [ ] T010 Add `POST /api/v1/docker/{id}/update` endpoint to pull latest image and recreate container
- [ ] T011 [P] Add `check_container_updates` MCP tool (read-only) in `daemon/services/mcp/server.go`
- [ ] T012 [P] Add `update_container_image` MCP control tool in `daemon/services/mcp/server.go`
- [ ] T013 Add registry auth config support for private registries in `daemon/domain/config.go`
- [ ] T014 Add tests in `daemon/services/controllers/docker_test.go`

**Checkpoint:** `GET /api/v1/docker/updates` returns a list of containers with `update_available: true/false`.

---

## Phase 2: Core Infrastructure (Enables Multiple Features)

### 2.1 — Threshold-Based Alerting Engine 🔴 · Effort: M

**What:** A rule engine that evaluates user-defined conditions against collector data and fires configured alert channels (ntfy, Gotify, Discord, Slack, generic webhook, Unraid notification). Reads from existing `CacheProvider` — no new collectors needed.

**New dependencies:**

```
github.com/expr-lang/expr v1.17.8          # type-safe expression evaluator, zero runtime deps
github.com/nicholas-fedor/shoutrrr v0.13.2  # unified notification dispatch (ntfy/Gotify/Discord/Slack/webhook)
```

**Why `expr-lang/expr`?** Compiles alert expressions like `"CPU > 90 || DiskUsedPct > 95"` to bytecode, statically type-checked against your existing `dto.*` structs at rule creation time. Zero runtime dependencies. Used in production by Google Cloud, Uber, and Argo Workflows.

**Why `nicholas-fedor/shoutrrr`?** Single URL-based interface for all notification services — `ntfy://ntfy.sh/topic`, `gotify://host/token`, `discord://token@channelID`, `slack://hooks/...`. Actively maintained (v0.13.2 released Feb 2026), requires Go 1.25 which matches this project. Avoids implementing 4 separate HTTP clients.

- [ ] T015 Define `AlertRule` struct in new `daemon/dto/alert.go`:

  ```go
  type AlertRule struct {
      ID         string        `json:"id"`
      Name       string        `json:"name"`
      Expression string        `json:"expression"` // e.g. "CPU > 90 && ArrayState == 'Started'"
      Duration   time.Duration `json:"duration"`   // must be true for this long before firing
      Severity   string        `json:"severity"`   // "info", "warning", "critical"
      Channels   []string      `json:"channels"`   // shoutrrr URLs
      Enabled    bool          `json:"enabled"`
  }
  ```

- [ ] T016 Define `AlertEvent` struct in `daemon/dto/alert.go` for alert history
- [ ] T017 Create `daemon/services/alerting/` package
- [ ] T018 Implement `RuleEvaluator` using `github.com/expr-lang/expr`:
  - Compile expressions against a typed `AlertEnv` struct that maps fields from `dto.SystemInfo`, `dto.DiskInfo`, etc.
  - Track firing state per rule with duration support to prevent flapping
  - Emit `AlertEvent` on state transitions (firing → resolved)
- [ ] T019 Implement `Dispatcher` using `github.com/nicholas-fedor/shoutrrr`:
  - Send via shoutrrr URL: `ntfy://ntfy.sh/my-topic`, `gotify://host/token?priority=8`, `discord://token@channelID`
  - Route `"unraid"` channel type to existing `controllers/notification.go`
  - Retry with backoff on transient failures
- [ ] T020 Store alert rules as JSON in `/boot/config/plugins/unraid-management-agent/alerts.json`
- [ ] T021 Add REST endpoints in `daemon/services/api/handlers.go`:
  - `GET /api/v1/alerts/rules` — list rules
  - `POST /api/v1/alerts/rules` — create rule
  - `PUT /api/v1/alerts/rules/{id}` — update rule
  - `DELETE /api/v1/alerts/rules/{id}` — delete rule
  - `GET /api/v1/alerts/history` — recent alert events
  - `GET /api/v1/alerts/status` — current firing alerts
- [ ] T022 Add MCP tools: `list_alert_rules`, `create_alert_rule`, `delete_alert_rule`, `get_alert_history`, `get_firing_alerts`
- [ ] T023 Wire `RuleEvaluator` into `daemon/services/orchestrator.go` — runs on 15s tick via `CacheProvider`
- [ ] T024 Add tests in `daemon/services/alerting/`

**Checkpoint:** Create rule `"DiskTemp > 55"` via POST API → stop a fan → verify ntfy notification received.

---

### 2.2 — Historical Metrics Storage 🔴 · Effort: M

**What:** Persist time-series snapshots of all collector data using bbolt (embedded key-value DB). Enables trend queries like "what was RAM usage yesterday?" and anomaly detection.

**New dependency:**

```
go.etcd.io/bbolt v1.4.3  # pure Go embedded KV DB, single file, no CGO, used by Consul/InfluxDB/NATS
```

**Why bbolt over alternatives?**

- `BadgerDB v4` (LSM-tree) — better write throughput but ~3x more complex; overkill for this use case
- `SQLite` (`mattn/go-sqlite3`) — **requires CGO**, breaks cross-compilation for `linux/amd64`
- `tstorage` — purpose-built time-series DB but last release March 2023, maintenance uncertain
- `Prometheus TSDB` — massive dependency tree, designed for Prometheus, not for embedding in a plugin

bbolt's B+tree structure is ideal for range scans by timestamp prefix (key = `topic/unix_timestamp_ns`). Single file on `/boot/config/plugins/unraid-management-agent/history.db`.

- [ ] T025 Create `daemon/services/history/` package with `Store` struct wrapping `go.etcd.io/bbolt`
- [ ] T026 Design key scheme: bucket = topic name (e.g. `system_update`), key = 8-byte big-endian nanosecond timestamp, value = JSON-encoded snapshot
- [ ] T027 Implement `Record(topic string, data any) error` — marshals to JSON, writes to bbolt
- [ ] T028 Implement `QueryRange(topic string, from, to time.Time) ([]Snapshot, error)` — cursor scan by timestamp range
- [ ] T029 Implement `Prune(retention time.Duration) error` — deletes entries older than retention (default 7 days); run on daily ticker
- [ ] T030 Subscribe history store to all collector event topics in `daemon/services/api/server.go` alongside existing cache updates
- [ ] T031 Add REST endpoints in `daemon/services/api/handlers.go`:
  - `GET /api/v1/history/topics` — list available metric topics
  - `GET /api/v1/history/{topic}?from=&to=&resolution=` — returns sampled data points
- [ ] T032 Add MCP tools: `get_metric_history(topic, duration)`, `compare_metric_trend(topic, baseline_duration)`
- [ ] T033 Expose retention config in `daemon/domain/config.go`
- [ ] T034 Add tests in `daemon/services/history/`

**Checkpoint:** `GET /api/v1/history/system_update?from=-24h` returns hourly CPU/RAM snapshots from the past 24 hours.

---

## Phase 3: Automation & Operations

### 3.1 — Custom Health Checks / Watchdog 🟡 · Effort: M

**What:** User-defined health check probes (HTTP, TCP, container-state) with optional auto-remediation actions. Actively tests reachability — complements alerting (which is threshold-based, not probe-based).

**No new dependencies required** — uses `net/http` stdlib for HTTP probes, `net.DialTimeout` for TCP, existing Docker SDK for container probes.

- [ ] T035 Define `HealthCheck` struct in `daemon/dto/healthcheck.go`:

  ```go
  type HealthCheck struct {
      ID           string        `json:"id"`
      Name         string        `json:"name"`
      Type         string        `json:"type"`     // "http", "tcp", "container"
      Target       string        `json:"target"`   // URL, "host:port", or container name
      Interval     time.Duration `json:"interval"`
      Timeout      time.Duration `json:"timeout"`
      SuccessCode  int           `json:"success_code,omitempty"` // for HTTP
      OnFail       string        `json:"on_fail"`  // "notify", "restart_container:<name>", "webhook:<url>"
      Enabled      bool          `json:"enabled"`
  }
  ```

- [ ] T036 Create `daemon/services/watchdog/` package
- [ ] T037 [P] Implement HTTP probe — `GET` with timeout, check expected status code
- [ ] T038 [P] Implement TCP probe — `net.DialTimeout` to host:port
- [ ] T039 [P] Implement container-state probe — check container is `running` via existing Docker SDK
- [ ] T040 Implement remediation actions: `restart_container` (via `controllers/docker.go`), `notify` (via `controllers/notification.go`), `webhook` (stdlib `net/http` POST)
- [ ] T041 Store health check configs in `/boot/config/plugins/unraid-management-agent/healthchecks.json`
- [ ] T042 Add REST endpoints: `GET/POST/PUT/DELETE /api/v1/healthchecks` and `GET /api/v1/healthchecks/status`
- [ ] T043 Add MCP tools: `list_health_checks`, `create_health_check`, `get_health_check_status`, `run_health_check`
- [ ] T044 Wire watchdog into `daemon/services/orchestrator.go` with its own goroutine and context cancellation
- [ ] T045 Add tests in `daemon/services/watchdog/`

**Checkpoint:** Create HTTP check for Plex. Stop Plex container. Verify watchdog detects failure, restarts container, and logs the remediation action.

---

### 3.2 — Native Scheduled Tasks 🟡 · Effort: M

**What:** Built-in cron-style scheduler. Runs shell commands or user scripts on a schedule without requiring the Community Applications User Scripts plugin.

**New dependency:**

```
github.com/go-co-op/gocron/v2 v2.19.1  # actively maintained scheduler (last release Jan 2026)
```

**Why gocron v2 over robfig/cron v3?** `robfig/cron v3` was last released in January 2020 with 111 open issues and no active maintenance. `gocron v2` was released v2.19.1 in January 2026, supports cron expressions, duration intervals, calendar-based scheduling, distributed locking, singleton mode (skip if still running), and context-aware execution with panic recovery — all features needed for a production scheduler in a daemon.

**Why not go-quartz?** `github.com/reugn/go-quartz` v0.15.2 (Sep 2025) is a good zero-dependency option but gocron v2 has 445+ importers vs go-quartz's smaller ecosystem, and gocron's API maps more naturally to what Unraid users expect (simple cron expressions + duration intervals).

- [ ] T046 Define `ScheduledTask` struct in `daemon/dto/schedule.go`:

  ```go
  type ScheduledTask struct {
      ID          string    `json:"id"`
      Name        string    `json:"name"`
      Schedule    string    `json:"schedule"`    // cron expr or "@every 1h"
      Command     string    `json:"command"`     // shell command
      WorkDir     string    `json:"work_dir,omitempty"`
      Enabled     bool      `json:"enabled"`
      LastRun     time.Time `json:"last_run,omitempty"`
      LastExitCode int      `json:"last_exit_code,omitempty"`
      LastOutput  string    `json:"last_output,omitempty"`
  }
  ```

- [ ] T047 Create `daemon/services/scheduler/` package using `github.com/go-co-op/gocron/v2`
- [ ] T048 Implement `Scheduler` — loads tasks from config, registers with gocron, runs with `gocron.WithSingletonMode()` to skip overlapping runs
- [ ] T049 Capture stdout/stderr per run; store last N runs per task (reuse history bbolt store from T025 if implemented)
- [ ] T050 Add REST endpoints in `daemon/services/api/handlers.go`:
  - `GET/POST/PUT/DELETE /api/v1/schedule`
  - `POST /api/v1/schedule/{id}/run` — manual trigger
  - `GET /api/v1/schedule/{id}/history` — last N runs with output
- [ ] T051 Add MCP tools: `list_scheduled_tasks`, `create_scheduled_task`, `run_task_now`, `get_task_run_history`
- [ ] T052 Add tests in `daemon/services/scheduler/`

**Checkpoint:** Create task via API: `schedule=@every 5m`, `command=df -h /mnt/user`. Trigger via `POST /api/v1/schedule/{id}/run`. Retrieve captured output.

---

## Phase 4: Data & Configuration Management

### 4.1 — Config Backup & Git Versioning 🟡 · Effort: M

**What:** Periodically snapshot Unraid config files (docker templates, user scripts, plugin configs, share/network settings) to a local or remote Git repository. Exposes restore-from-commit endpoint.

**New dependency:**

```
github.com/go-git/go-git/v5 v5.16.5  # pure Go git client, no CGO, supports SSH keys + GitHub PAT push
```

**Why go-git v5 over alternatives?**

- Pure Go — no CGO, cross-compiles cleanly for `linux/amd64` from macOS
- Supports GitHub/GitLab via SSH key (`ssh.NewPublicKeysFromFile`) and PAT (`http.BasicAuth`)
- Covers add/commit/push completely — the only operations needed for config backup
- Known limitations (no rebase, no stash, no hooks) are irrelevant to a backup workflow
- `git2go` (libgit2 bindings) requires CGO and was last tagged October 2022 — eliminated
- Shelling out to `git` binary requires `git` installed on Unraid (not guaranteed)

- [ ] T053 Define `BackupConfig` in `daemon/domain/config.go`:

  ```go
  type BackupConfig struct {
      Enabled      bool     `json:"enabled"`
      Schedule     string   `json:"schedule"`      // cron expression
      Paths        []string `json:"paths"`          // dirs/files to back up
      GitRemote    string   `json:"git_remote,omitempty"`
      GitBranch    string   `json:"git_branch"`     // default: "main"
      SSHKeyPath   string   `json:"ssh_key_path,omitempty"`
      GitHubToken  string   `json:"github_token,omitempty"` // PAT for HTTPS push
  }
  ```

- [ ] T054 Create `daemon/services/backup/` package
- [ ] T055 Implement `GitBackup.Run()` using `github.com/go-git/go-git/v5`:
  - `git.PlainInit` or `git.PlainOpen` on backup directory
  - `worktree.AddWithOptions` to stage configured paths
  - Skip commit if `git diff --stat` shows no changes (idempotent)
  - `worktree.Commit` with timestamp message
  - Optional `repo.Push` with SSH or PAT auth
- [ ] T056 Add `daemon/services/collectors/backup.go` collector — runs on configured schedule, publishes `backup_status_update` event
- [ ] T057 Subscribe backup topic in `daemon/services/api/server.go`
- [ ] T058 Add REST endpoints in `daemon/services/api/handlers.go`:
  - `GET /api/v1/backup/status` — last backup time, commit hash, dirty files count
  - `POST /api/v1/backup/run` — trigger manual backup
  - `GET /api/v1/backup/history` — list recent commits with hash + timestamp
  - `POST /api/v1/backup/restore/{commit}` — restore files from a specific commit hash
- [ ] T059 Add MCP tools: `get_backup_status`, `run_backup`, `list_backup_history`, `restore_config_from_backup`
- [ ] T060 Add tests in `daemon/services/backup/`

**Checkpoint:** Modify a share config file. Call `POST /api/v1/backup/run`. Verify new commit in `GET /api/v1/backup/history` with the expected file diff.

---

## Phase 5: Advanced / Long-Horizon

### 5.1 — Speedtest & Network Diagnostics 🟢 · Effort: S

**What:** Periodic internet speed tests stored historically. Runs against official Speedtest.net infrastructure using a pure Go implementation — no external binary required.

**New dependency:**

```
github.com/showwin/speedtest-go v1.7.10  # pure Go, uses Speedtest.net servers, no binary dependency
```

**Why showwin/speedtest-go?** Only actively maintained pure-Go Speedtest.net client (v1.7.10, Dec 2024). Implements the Speedtest.net protocol natively — same servers as the official CLI. Supports context cancellation (critical for a daemon). No CGO, no external binary. Alternatives (`go-fast` by ddo) target fast.com and are unmaintained since 2019.

- [ ] T061 Add `SpeedtestResult` to `daemon/dto/network.go`:

  ```go
  type SpeedtestResult struct {
      DownloadMbps float64   `json:"download_mbps"`
      UploadMbps   float64   `json:"upload_mbps"`
      LatencyMs    float64   `json:"latency_ms"`
      Server       string    `json:"server"`
      Timestamp    time.Time `json:"timestamp"`
  }
  ```

- [ ] T062 Add `daemon/services/collectors/speedtest.go` collector — runs every 6h by default (configurable), stores results via history store
- [ ] T063 Add REST endpoints:
  - `GET /api/v1/network/speedtest` — last stored result
  - `POST /api/v1/network/speedtest/run` — trigger on-demand (runs with 60s context timeout)
- [ ] T064 Add MCP tools: `run_speedtest`, `get_speedtest_history`
- [ ] T065 Wire collector into `daemon/services/orchestrator.go`
- [ ] T066 Add tests in `daemon/services/collectors/speedtest_test.go`

---

### 5.2 — Multi-Server Federation 🟢 · Effort: L

**What:** One agent acts as federation hub, pulling data from satellite agents via their REST APIs. Single MCP endpoint gives an AI agent a unified view and control plane across a fleet.

**No new dependencies required** — uses stdlib `net/http` for agent-to-agent communication.

- [ ] T067 Define `RemoteAgent` struct in new `daemon/dto/federation.go`:

  ```go
  type RemoteAgent struct {
      ID       string    `json:"id"`
      Name     string    `json:"name"`
      URL      string    `json:"url"`       // base URL of remote agent
      APIKey   string    `json:"api_key,omitempty"`
      LastSeen time.Time `json:"last_seen"`
      Status   string    `json:"status"`   // "online", "offline", "unknown"
  }
  ```

- [ ] T068 Create `daemon/services/federation/` package — HTTP client wrapper that calls remote agent REST endpoints
- [ ] T069 Add agent registry endpoints: `GET/POST/DELETE /api/v1/federation/agents`
- [ ] T070 Implement federated cache — polls registered agents on their collector intervals, tags data with `agent_id`
- [ ] T071 Add federated REST endpoints: `GET /api/v1/federation/agents/{id}/system`, `/docker`, `/array`, etc.
- [ ] T072 Add federated MCP tools accepting `agent_id` parameter to target a specific server
- [ ] T073 Add tests in `daemon/services/federation/`

**Checkpoint:** Register a second Unraid agent. Query `GET /api/v1/federation/agents/{id}/docker` from the primary and receive the secondary server's container list.

---

## Dependencies & Suggested Implementation Order

```
Phase 1 (T001–T014)  ──── No new dependencies. Start immediately.
        │
        ├── 1.1 MCP Prompts (T001–T006)     ─── [P] Independent
        └── 1.2 Docker Updates (T007–T014)  ─── [P] Independent

Phase 2 (T015–T034)  ──── Add bbolt + expr-lang/expr + shoutrrr
        │
        ├── 2.1 Alerting Engine (T015–T024) ─── [P] Independent of History
        └── 2.2 History Storage (T025–T034) ─── [P] Independent of Alerting
                                                  (Alerting can use History for alert log)

Phase 3 (T035–T052)  ──── Add gocron v2. Can reuse alerting channels (T019) + history store (T025)
        │
        ├── 3.1 Watchdog (T035–T045)        ─── [P] Independent
        └── 3.2 Scheduler (T046–T052)       ─── [P] Independent

Phase 4 (T053–T060)  ──── Add go-git v5. Can reuse scheduler (T047) for backup schedule.

Phase 5 (T061–T073)  ──── Add speedtest-go. Federation has no new deps.
        │
        ├── 5.1 Speedtest (T061–T066)       ─── [P] Independent
        └── 5.2 Federation (T067–T073)      ─── [P] Independent
```

### Files touched per feature

| Feature | New Packages | Modified Files |
|---|---|---|
| MCP Diagnostic Prompts | none | `mcp/server.go` |
| Docker Update Detection | none | `dto/docker.go`, `controllers/docker.go`, `api/handlers.go`, `mcp/server.go` |
| Alerting Engine | `services/alerting/`, `dto/alert.go` | `orchestrator.go`, `api/server.go`, `api/handlers.go`, `go.mod` |
| Historical Metrics | `services/history/` | `api/server.go`, `api/handlers.go`, `orchestrator.go`, `go.mod` |
| Health Checks / Watchdog | `services/watchdog/`, `dto/healthcheck.go` | `orchestrator.go`, `api/handlers.go` |
| Scheduled Tasks | `services/scheduler/`, `dto/schedule.go` | `orchestrator.go`, `api/handlers.go`, `go.mod` |
| Config Backup + Git | `services/backup/`, `collectors/backup.go` | `orchestrator.go`, `api/handlers.go`, `domain/config.go`, `go.mod` |
| Speedtest | `collectors/speedtest.go` | `dto/network.go`, `api/handlers.go`, `orchestrator.go`, `go.mod` |
| Multi-Server Federation | `services/federation/`, `dto/federation.go` | `orchestrator.go`, `api/handlers.go`, `mcp/server.go` |

---

## Coding Standards (applies to all features)

- All new REST endpoints: `RLock/RUnlock` for cache reads, `respondJSON` for responses (see `api/handlers.go`)
- All new collectors: wrap work in `defer/recover` panic recovery (see CLAUDE.md pattern)
- All shell commands: use `lib.ExecuteShellCommand()` — never `exec.Command` directly
- All user inputs (paths, URLs, names): validate with `lib.Validate*()` functions
- All new goroutines: respect `ctx.Done()` for graceful shutdown
- Update `CHANGELOG.md` before tagging any release
- Each phase checkpoint is an independently shippable release increment
