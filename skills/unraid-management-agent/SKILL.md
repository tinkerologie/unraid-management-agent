---
name: unraid-management-agent
description: >
  Monitor and control an Unraid server via the Unraid Management Agent's MCP
  server (125 tools, 5 resources, 6 diagnostic prompts) and REST API. Use when:
  connecting an AI client (Claude, ChatGPT, Cursor, Copilot, Gemini CLI) to
  Unraid; checking system, array, disk, Docker, VM, GPU, UPS, ZFS, or network
  status; starting/stopping containers, VMs, the array, or parity checks;
  diagnosing disk health, performance, or array state; managing alerts, health
  checks, fan/CPU/tuning, or updates; or choosing between MCP, REST, and
  WebSocket. Corrects: inventing Unraid tool names or REST paths; running
  destructive actions (stop array, reboot, delete snapshot) without confirm=true;
  polling REST in a loop instead of using WebSocket/MCP; dumping full lists
  instead of search_/get_ tools; and assuming auth the agent lacks by default.
metadata:
  version: 1
  homepage: https://github.com/ruaan-deysel/unraid-management-agent
---

# Unraid Management Agent

The **Unraid Management Agent** is a plugin that runs on an Unraid server and
exposes its full state and control surface over three interfaces:

| Interface | Endpoint | Use for |
| --- | --- | --- |
| **MCP** (Model Context Protocol) | `http://<unraid-ip>:8043/mcp` | AI agents — 125 tools, 5 resources, 6 prompts |
| **REST API** | `http://<unraid-ip>:8043/api/v1` | Scripts, Custom GPT Actions, integrations |
| **WebSocket** | `ws://<unraid-ip>:8043/api/v1/ws` | Live event streaming (near real-time) |

**Core principle:** prefer the **MCP tools** when acting as an AI agent — they
are purpose-built, validated, and named for discovery. Fall back to the REST API
only for clients that cannot speak MCP (e.g. ChatGPT Custom GPTs). Never invent
tool names or paths; the real catalogs are in the reference files below.

## Decision Workflow

### 1. Connect

If the client is not yet connected, read `references/connection.md`. In short:

- **Streamable HTTP (remote/LAN):** point the client at `http://<unraid-ip>:8043/mcp`.
- **STDIO (local on the server):** run `unraid-management-agent mcp-stdio`.
- The agent has **no authentication by default** — it is designed for a trusted
  LAN or behind a VPN/reverse proxy. Do not assume an API key exists unless the
  user says so.

### 2. Read before you write

Always establish current state with a read-only tool before taking action:

- System snapshot → `get_system_info`, or `get_diagnostic_summary` for a broad view
- Array → `get_array_status`; Disks → `list_disks` / `get_disk_info`
- Containers → `list_containers` / `search_containers` / `get_container_info`
- VMs → `list_vms` / `search_vms` / `get_vm_info`

Read-only tools are marked `ReadOnlyHint: true`. The full catalog is in
`references/mcp-tools.md`.

### 3. Pick the narrowest tool

- Looking for one container/VM among many? Use `search_containers` / `search_vms`,
  not a full `list_` dump.
- Want a specific subsystem? Use the dedicated getter (`get_ups_status`,
  `get_gpu_metrics`, `get_zfs_pools`, …) rather than scraping `get_diagnostic_summary`.
- Need history/metrics over time? Use `query_metric_history`, `get_parity_history`,
  `get_alert_history`, `get_health_check_history`.

### 4. Confirm destructive actions

Control tools (`ReadOnlyHint: false`) change the system. The high-risk ones
**require `confirm=true`** and must be confirmed with the user first:

| Tool | Effect |
| --- | --- |
| `array_action` (stop) | Makes **all array data inaccessible** |
| `system_reboot` / `system_shutdown` | Restarts / powers off the whole server |
| `execute_user_script` / `run_runbook` | Runs arbitrary user-defined actions |
| `delete_vm_snapshot` / `restore_vm_snapshot` | Irreversible VM state change |
| `delete_alert_rule` / `delete_health_check` | Removes configuration |

Always state what will happen and wait for explicit user approval before sending
`confirm=true`. See `references/mcp-tools.md` for the full read/write breakdown.

### 5. Use live data correctly

For continuously changing values (CPU, container state), do **not** poll the REST
API in a tight loop. Use the MCP resources (`references/diagnostics.md`) or the
WebSocket stream. One MCP tool call returns the latest cached value already.

### 6. Diagnose with prompts, not ad-hoc chains

The agent ships 6 built-in diagnostic **prompts** that orchestrate the right
tools for common investigations (disk health, performance, maintenance, array
state, overview, troubleshooting). Prefer them over reinventing the analysis.
See `references/diagnostics.md`.

---

## Critical Anti-Patterns

| Anti-pattern | Use instead | Why |
| --- | --- | --- |
| Inventing tool names (`docker_restart`, `getArray`) | The exact names in `references/mcp-tools.md` | Wrong names just fail |
| Stopping the array / rebooting without confirmation | Read state, explain impact, then `confirm=true` | Data loss / downtime |
| Polling `GET /api/v1/system` in a loop | MCP resources or WebSocket `ws://…/api/v1/ws` | The agent already caches; polling wastes CPU |
| `list_containers` then filtering in-context | `search_containers` / `get_container_info` | Less context, faster, more accurate |
| Hand-rolling a disk-health analysis | `diagnose_disk_health` prompt | Built-in prompt covers SMART, temps, wear, errors |
| Assuming an API key / auth header | Treat as unauthenticated LAN/VPN unless told otherwise | Default deploy has no auth |
| Using REST from an MCP-capable client | The MCP tools | Validated args, richer results, fewer round-trips |
| Editing Unraid config files directly | The agent's control tools / settings endpoints | Tools match the WebUI's own logic and validation |

---

## Reference Files

Read these when you need detail. Keep `SKILL.md` itself in context; pull
references on demand.

| File | When to read |
| --- | --- |
| `references/connection.md` | Connecting any client (Claude Code/Desktop, claude.ai, Cursor, Copilot, Gemini CLI, ChatGPT) via MCP HTTP/STDIO; troubleshooting connectivity |
| `references/mcp-tools.md` | The full catalog of all 125 MCP tools grouped by category, with read/write (destructive) flags and arguments |
| `references/diagnostics.md` | The 6 diagnostic prompts and 5 real-time resources — what each analyses and when to use it |
| `references/rest-api.md` | REST API surface for non-MCP clients (Custom GPT Actions, scripts): base path, key endpoints, path/body conventions |
| `references/workflows.md` | Natural-language request → tool/endpoint mappings and end-to-end examples (monitoring, container/VM/array control, maintenance) |

## Capabilities at a Glance

- **Monitoring (read-only):** system, array, disks + SMART, shares, Docker
  (containers, networks, stats, logs, update checks), VMs + snapshots, GPU, UPS,
  NUT, ZFS (pools/datasets/snapshots/ARC), network + access URLs, hardware,
  notifications, parity history, logs, collectors, settings, unassigned devices,
  remote SMB/NFS shares.
- **Control:** container & VM lifecycle, array start/stop, parity check
  start/stop/pause/resume, disk spin up/down, container & plugin updates, VM
  snapshot create/delete/restore, VM clone, service start/stop/restart, user
  scripts, collector enable/disable + intervals, reboot/shutdown.
- **Observability & automation:** alert rules (expr-lang) + templates, health
  checks (HTTP/TCP/container), remediation runbooks, an optional autonomous
  agent, fan control (profiles/curves), CPU governor, and system tuning (turbo
  boost, disk cache, inotify).
