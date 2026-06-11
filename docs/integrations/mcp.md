# Model Context Protocol (MCP) Integration

> **Status: Production-Ready (GA)** — Built on the
> [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) v1.2.0
> with protocol version 2025-06-18.

The Unraid Management Agent includes support for the
[Model Context Protocol (MCP)](https://modelcontextprotocol.io/),
enabling AI agents like Claude, Cursor, GitHub Copilot, Codex,
Windsurf, Gemini CLI, and other LLM-based systems to interact
with your Unraid server programmatically.

## Overview

MCP is an open protocol that standardizes how AI applications can securely connect to external data sources and services. With MCP support, you can:

- **Monitor** your Unraid server using natural language queries
- **Control** Docker containers, VMs, and the array through AI assistants
- **Analyze** disk health, system performance, and troubleshoot issues
- **Automate** routine tasks with AI-powered workflows

> **Related integrations:**
>
> - **Claude (skill):** install the [Agent Skill](claude/README.md) so Claude
>   knows how to use these tools effectively — works in Claude Code, Claude
>   Desktop, claude.ai, Cursor, Copilot, and Gemini CLI.
> - **ChatGPT:** ChatGPT consumes REST **Actions**, not MCP — see the
>   [ChatGPT Custom GPT guide](chatgpt/README.md).
> - See the [integrations index](README.md) for all options (MQTT, Home
>   Assistant, Grafana, …).

## Transports

The MCP server supports two transports — use the one that fits your deployment:

| Transport              | Endpoint / Command                            | Best For                                               |
| ---------------------- | --------------------------------------------- | ------------------------------------------------------ |
| **Streamable HTTP** ⭐ | `POST/GET/DELETE http://<unraid-ip>:8043/mcp` | Remote connections from any machine on the network     |
| **STDIO**              | `unraid-management-agent mcp-stdio`           | Local AI clients running directly on the Unraid server |

> **Which transport should I use?**
>
> - Use **Streamable HTTP** if the AI client (Cursor, VS Code, etc.) runs on a different machine than the Unraid server.
> - Use **STDIO** if the AI client (Claude Desktop, Cursor) runs locally on the Unraid server itself — it has zero network overhead and requires no authentication.

## Available Tools (126 total)

### System Monitoring Tools

| Tool                      | Description                                                                                             |
| ------------------------- | ------------------------------------------------------------------------------------------------------- |
| `get_system_info`         | System information including hostname, CPU, RAM, temperatures, and uptime                               |
| `run_self_test`           | OS-resilience self-test: Unraid version, overall data-source health, capabilities, per-subsystem status |
| `get_array_status`        | Array state, capacity, parity information, and disk assignments                                         |
| `get_hardware_info`       | Motherboard, CPU, and memory details from DMI/SMBIOS                                                    |
| `get_registration`        | Unraid license and registration information                                                             |
| `get_health_status`       | Overall system health status                                                                            |
| `get_diagnostic_summary`  | Comprehensive diagnostic summary including all subsystems                                               |
| `get_network_access_urls` | All available access URLs (LAN, WAN, mDNS, IPv6)                                                        |

### Disk & Storage Tools

| Tool                     | Description                                                |
| ------------------------ | ---------------------------------------------------------- |
| `list_disks`             | All disks with health status, optionally with SMART data   |
| `get_disk_info`          | Detailed information about a specific disk including SMART |
| `list_shares`            | All network shares with settings and usage                 |
| `get_share_config`       | Detailed configuration for a specific share                |
| `get_unassigned_devices` | Unassigned devices (non-array disks, USB drives)           |
| `get_disk_settings`      | Disk configuration settings                                |

### ZFS Tools

| Tool                | Description                                     |
| ------------------- | ----------------------------------------------- |
| `get_zfs_pools`     | ZFS pool information and health status          |
| `get_zfs_datasets`  | ZFS dataset information including quotas/usage  |
| `get_zfs_snapshots` | ZFS snapshot information for all pools/datasets |
| `get_zfs_arc_stats` | ZFS ARC (cache) statistics including hit ratio  |

### Docker Tools

| Tool                        | Description                                                                                                                  |
| --------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `list_containers`           | Docker containers, optionally filtered by state (includes update status)                                                     |
| `get_container_info`        | Detailed information about a specific container (includes update status)                                                     |
| `get_container_logs`        | Container stdout/stderr logs with tail/since opts                                                                            |
| `search_containers`         | Search containers by name or state                                                                                           |
| `get_docker_settings`       | Docker daemon configuration settings                                                                                         |
| `check_container_updates`   | Synchronous on-demand check of all containers for available image updates                                                    |
| `check_container_update`    | Synchronous on-demand check of a specific container for an image update                                                      |
| `refresh_container_updates` | Force an immediate registry digest re-check for all containers and publish the result (updates cache, WebSocket, and alerts) |
| `get_container_size`        | Get disk usage (image size + rw layer) of a container                                                                        |
| `list_docker_networks`      | List all Docker networks with driver, scope, IPAM subnet/gateway, and connected container names (read-only)                  |

> **Update status fields:** `list_containers` and `get_container_info` now include the following fields populated from the cached update check results:
>
> | Field              | Values                                        | Description                                   |
> | ------------------ | --------------------------------------------- | --------------------------------------------- |
> | `update_status`    | `up_to_date`, `update_available`, `unknown`   | Human-readable update state for the container |
> | `update_available` | `true` / `false`                              | Whether a newer image digest is available     |
> | `update_checked`   | RFC 3339 timestamp (omitted if never checked) | When the update check was last performed      |
>
> Use `refresh_container_updates` to force a fresh registry check and push results to the cache, WebSocket hub, and alerting engine. Use `check_container_updates` / `check_container_update` for synchronous on-demand checks that return results directly without publishing.

### VM Tools

| Tool                | Description                              |
| ------------------- | ---------------------------------------- |
| `list_vms`          | Virtual machines, optionally filtered    |
| `get_vm_info`       | Detailed information about a specific VM |
| `search_vms`        | Search VMs by name or state              |
| `get_vm_settings`   | VM manager configuration settings        |
| `list_vm_snapshots` | List snapshots for a specific VM         |

### Network & UPS Tools

| Tool               | Description                                     |
| ------------------ | ----------------------------------------------- |
| `get_network_info` | Network interfaces with IPs and traffic stats   |
| `get_ups_status`   | UPS battery level, load, and runtime            |
| `get_nut_status`   | Detailed NUT (Network UPS Tools) status/metrics |
| `get_gpu_metrics`  | GPU utilization, temperature, and memory        |

### Notifications & Logs Tools

| Tool                         | Description                                 |
| ---------------------------- | ------------------------------------------- |
| `get_notifications`          | System notifications, alerts, and warnings  |
| `get_notifications_overview` | Summary counts of notifications by type     |
| `list_log_files`             | List available log files                    |
| `get_log_content`            | Retrieve content from a specific log file   |
| `get_syslog`                 | System log entries with optional line limit |
| `get_docker_log`             | Docker daemon log entries                   |

### Collector Management Tools

| Tool                   | Description                               |
| ---------------------- | ----------------------------------------- |
| `list_collectors`      | List all data collectors and their status |
| `get_collector_status` | Get status of a specific collector        |

### Plugin Tools

| Tool                     | Description                                                                                               |
| ------------------------ | --------------------------------------------------------------------------------------------------------- |
| `check_plugin_updates`   | Return the cached plugin update status (read-only)                                                        |
| `refresh_plugin_updates` | Force an immediate plugin update check for all installed plugins and publish the result (non-destructive) |

### Service & Process Tools

| Tool                 | Description                                             |
| -------------------- | ------------------------------------------------------- |
| `get_service_status` | Get running/stopped status of a system service          |
| `list_services`      | List all manageable system services with their status   |
| `list_processes`     | List top system processes sorted by CPU, memory, or PID |

### Parity & User Scripts Tools

| Tool                 | Description                                          |
| -------------------- | ---------------------------------------------------- |
| `get_parity_history` | Parity check history with dates, durations, errors   |
| `list_user_scripts`  | List available user scripts from User Scripts plugin |

### OS & Mover Tools

| Tool               | Description                                                                                                                                                               |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `get_os_update`    | Return the cached Unraid OS update availability. Sources local files only — no outbound network calls. Status: `up_to_date`, `update_available`, or `unknown` (read-only) |
| `get_mover_status` | Return the cached mover state (active flag, cron schedule, last-run start/finish timestamps, duration, files moved, bytes moved) (read-only)                              |

### Alerting & Trend Analysis Tools

| Tool                    | Description                                                                                                                                                                                                                                   |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `list_alert_templates`  | List curated, disabled-by-default alert rule templates that use trend/predictive metrics — array fill ETA, disk temp slope, container restart rate, reallocated sectors, disk errors (read-only)                                              |
| `enable_alert_template` | Instantiate and enable an alert rule from a template in one call. Pass `template_id` (e.g. `tmpl-array-fill`) and optional `channels` list (defaults to `["unraid"]`). Idempotent — re-enabling updates the existing rule without duplication |
| `query_metric_history`  | Query the in-memory ring-buffer history for a named metric. Returns all buffered samples plus summary statistics (slope/sec, min, max, avg, last). See below for valid metric names (read-only)                                               |

**`enable_alert_template` — arguments:**

| Argument      | Type             | Required | Description                                                                                       |
| ------------- | ---------------- | -------- | ------------------------------------------------------------------------------------------------- |
| `template_id` | string           | Yes      | Template ID to enable (e.g. `tmpl-array-fill`, `tmpl-disk-temp-climb`, `tmpl-container-flapping`) |
| `channels`    | array of strings | No       | Notification channels (e.g. `["unraid", "email"]`). Defaults to `["unraid"]` when omitted         |

The created rule's `id` matches the `template_id`, so repeated calls are idempotent — the rule is updated in-place rather than duplicated. Returns `503` if the alerting subsystem is not initialised, `404` for an unknown template, `400` for an invalid request body.

**Valid metric names for `query_metric_history`:**

| Metric name      | Scope      | Description                                                           |
| ---------------- | ---------- | --------------------------------------------------------------------- |
| `cpu_temp`       | global     | CPU temperature in °C                                                 |
| `array_used_pct` | global     | Array used percentage                                                 |
| `disk_temp`      | per-entity | Temperature of a specific disk (pass `entity` = disk ID/name)         |
| `disk_used_pct`  | per-entity | Used percentage of a specific disk                                    |
| `disk_errors`    | per-entity | Read/write error count for a specific disk                            |
| `reallocated`    | per-entity | Reallocated sector count for a specific disk                          |
| `pending`        | per-entity | Pending (uncorrectable) sector count for a specific disk              |
| `restart_count`  | per-entity | Restart count for a specific container (pass `entity` = container ID) |

**Example** — query the last hour of array fill percentage:

```bash
curl "http://192.168.20.21:8043/api/v1/metrics/history?metric=array_used_pct"
```

### AI Remediation Toolkit Tools

| Tool                   | Annotation          | Description                                                                                                                                         |
| ---------------------- | ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `system_health_report` | destructive (gated) | Aggregate health signals into prioritised findings with recommended actions. Read-only unless `confirm: true` AND `actions` list are both provided. |
| `list_runbooks`        | read-only           | List all reviewed remediation runbooks with their names, descriptions, and default step shapes.                                                     |
| `run_runbook`          | destructive (gated) | Run a named runbook. Without `confirm: true` it is a dry-run returning planned steps. With `confirm: true` it executes supported-action steps.      |
| `find_root_cause`      | read-only           | Correlate cached system signals (CPU, array, parity, disk temperatures, containers) to surface the most likely root causes of a degraded system.    |

**`system_health_report` — arguments:**

| Argument  | Type             | Description                                                                                                          |
| --------- | ---------------- | -------------------------------------------------------------------------------------------------------------------- |
| `confirm` | bool             | Must be `true` to execute actions. Without it the tool returns the report only.                                      |
| `actions` | array of objects | List of `{action, target}` pairs from a previous report's `recommended_actions`. Executed only when `confirm: true`. |

Supported actions: `start_container`, `stop_container`, `restart_container`, `start_vm`, `stop_vm`, `restart_vm`, `force_stop_vm`.

**`run_runbook` — arguments:**

| Argument  | Type             | Description                                                                                                               |
| --------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `name`    | string           | Runbook name (from `list_runbooks`)                                                                                       |
| `confirm` | bool             | Must be `true` to execute. Without it returns the planned steps (dry-run).                                                |
| `targets` | array of strings | Optional list of container or VM IDs. For `restart_unhealthy_containers`, omit to auto-resolve stopped/exited containers. |

### Settings Tools

| Tool                  | Description                        |
| --------------------- | ---------------------------------- |
| `get_system_settings` | System-wide configuration settings |
| `get_docker_settings` | Docker daemon settings             |
| `get_vm_settings`     | VM manager settings                |
| `get_disk_settings`   | Disk configuration settings        |
| `get_share_config`    | Share-specific configuration       |

### Control Tools (Require Confirmation)

| Tool                        | Description                                       | Actions                                                    |
| --------------------------- | ------------------------------------------------- | ---------------------------------------------------------- |
| `container_action`          | Docker container control                          | start, stop, restart, pause, unpause                       |
| `update_container`          | Pull latest image and recreate a container        | Requires `confirm: true`                                   |
| `update_all_containers`     | Update all containers with available updates      | Requires `confirm: true`                                   |
| `vm_action`                 | Virtual machine control                           | start, stop, restart, pause, resume, hibernate, force-stop |
| `create_vm_snapshot`        | Create a snapshot of a VM                         | Requires `confirm: true`                                   |
| `delete_vm_snapshot`        | Delete a VM snapshot                              | Requires `confirm: true`                                   |
| `restore_vm_snapshot`       | Restore a VM snapshot                             | Requires `confirm: true`                                   |
| `clone_vm`                  | Clone a VM to a new name                          | Requires `confirm: true`                                   |
| `array_action`              | Array control (**use with caution**)              | start, stop                                                |
| `parity_check_action`       | Start parity check                                | correcting or non-correcting                               |
| `parity_check_stop`         | Stop a running parity check                       | -                                                          |
| `parity_check_pause`        | Pause a running parity check                      | -                                                          |
| `parity_check_resume`       | Resume a paused parity check                      | -                                                          |
| `disk_spin_down`            | Spin down a specific disk                         | -                                                          |
| `disk_spin_up`              | Spin up a specific disk                           | -                                                          |
| `update_plugin`             | Update a specific plugin to latest version        | Requires `confirm: true`                                   |
| `update_all_plugins`        | Update all plugins with available updates         | Requires `confirm: true`                                   |
| `service_action`            | Start, stop, or restart a system service          | start, stop, restart — requires `confirm: true`            |
| `execute_user_script`       | Execute a user script (**requires confirmation**) | -                                                          |
| `collector_action`          | Enable or disable a data collector                | enable, disable                                            |
| `update_collector_interval` | Update a collector's polling interval             | -                                                          |
| `system_reboot`             | Reboot the server (**requires confirmation**)     | -                                                          |
| `system_shutdown`           | Shutdown the server (**requires confirmation**)   | -                                                          |

> **⚠️ Warning:** Destructive actions (array stop, reboot, shutdown, user scripts) require explicit confirmation via the `confirm: true` parameter.

## Read-Only Mode

Read-only mode blocks **every state-changing MCP tool** at the server, so AI agents can monitor and diagnose but never modify the system — regardless of which client connects or what it asks for.

Enable it in any of the usual configuration layers:

| Method                     | Setting                                    |
| -------------------------- | ------------------------------------------ |
| Plugin settings page       | **AI Agent Access (MCP) → Read-Only Mode** |
| Config file (`config.cfg`) | `READ_ONLY="true"`                         |
| YAML config (`config.yml`) | `read_only: true`                          |
| CLI flag / env var         | `--read-only` / `READ_ONLY=true`           |

Behaviour while enabled:

- Write tools stay visible in tool listings (less confusing for clients), but every invocation returns: `This operation is blocked: the agent is running in read-only mode`.
- All read-only monitoring tools work normally.
- The dual-mode tools `system_health_report` and `run_runbook` still return their report / dry-run plan, but never execute remediation actions, even with `confirm: true`.
- Blocked attempts are logged with the tool name for auditability.
- The REST API and WebSocket are **not** affected — read-only mode governs MCP access only.

Read-only mode is enforced in addition to the per-tool `confirm: true` gating described below; destructive tools always require explicit confirmation even when read-only mode is off.

## Tool Safety Annotations

Tools include MCP safety annotations to help AI agents make safe decisions automatically:

### Read-Only Tools (77 tools)

All monitoring and query tools are annotated with `readOnlyHint: true`, signaling to AI agents that these tools are safe to call without side effects:

```
get_system_info, get_array_status, get_hardware_info, get_health_status,
get_diagnostic_summary, get_registration, get_network_info, get_network_access_urls,
get_ups_status, get_nut_status, get_gpu_metrics, list_disks, get_disk_info,
get_disk_settings, list_shares, get_share_config, get_unassigned_devices,
get_zfs_pools, get_zfs_datasets, get_zfs_snapshots, get_zfs_arc_stats,
list_containers, get_container_info, search_containers, get_docker_settings,
check_container_updates, check_container_update,
get_container_logs, get_container_size, list_docker_networks,
list_vms, get_vm_info, search_vms, get_vm_settings, list_vm_snapshots,
check_plugin_updates, get_service_status, list_services, list_processes,
get_notifications, get_notifications_overview, list_log_files, get_log_content,
get_syslog, get_docker_log, get_parity_history, list_user_scripts,
list_collectors, get_collector_status, get_system_settings,
get_os_update, get_mover_status,
list_alert_templates, query_metric_history, list_runbooks, find_root_cause
```

### Destructive Tools (17 tools) — `destructiveHint: true`

These tools make changes that may be difficult or impossible to reverse:

| Tool                    | Additional Hints       | Confirmation Required                  |
| ----------------------- | ---------------------- | -------------------------------------- |
| `container_action`      | `idempotentHint: true` | No                                     |
| `update_container`      | —                      | Yes (`confirm: true`)                  |
| `update_all_containers` | —                      | Yes (`confirm: true`)                  |
| `vm_action`             | `idempotentHint: true` | No                                     |
| `create_vm_snapshot`    | —                      | Yes (`confirm: true`)                  |
| `delete_vm_snapshot`    | —                      | Yes (`confirm: true`)                  |
| `restore_vm_snapshot`   | —                      | Yes (`confirm: true`)                  |
| `clone_vm`              | —                      | Yes (`confirm: true`)                  |
| `array_action`          | `idempotentHint: true` | Yes (`confirm: true`)                  |
| `update_plugin`         | —                      | Yes (`confirm: true`)                  |
| `update_all_plugins`    | —                      | Yes (`confirm: true`)                  |
| `service_action`        | `idempotentHint: true` | Yes (`confirm: true`)                  |
| `execute_user_script`   | —                      | Yes (`confirm: true`)                  |
| `system_reboot`         | —                      | Yes (`confirm: true`)                  |
| `system_shutdown`       | —                      | Yes (`confirm: true`)                  |
| `system_health_report`  | —                      | Yes (`confirm: true` + `actions` list) |
| `run_runbook`           | `idempotentHint: true` | Yes (`confirm: true`)                  |

### Non-Destructive Control Tools (10 tools) — `destructiveHint: false`

These tools make changes that are safe and easily reversible:

| Tool                        | Additional Hints       |
| --------------------------- | ---------------------- |
| `parity_check_action`       | `idempotentHint: true` |
| `parity_check_stop`         | `idempotentHint: true` |
| `parity_check_pause`        | `idempotentHint: true` |
| `parity_check_resume`       | `idempotentHint: true` |
| `disk_spin_down`            | `idempotentHint: true` |
| `disk_spin_up`              | `idempotentHint: true` |
| `collector_action`          | `idempotentHint: true` |
| `update_collector_interval` | `idempotentHint: true` |
| `refresh_plugin_updates`    | `idempotentHint: true` |
| `enable_alert_template`     | `idempotentHint: true` |

> **How AI agents use annotations:** When an AI agent receives these annotations,
> it can automatically decide whether to ask for user confirmation before calling
> a tool. Tools with `readOnlyHint: true` can be called freely, while tools with
> `destructiveHint: true` should prompt the user first.

## MCP Resources

Resources provide real-time data streams that AI agents can subscribe to:

| Resource URI          | Description                     |
| --------------------- | ------------------------------- |
| `unraid://system`     | Real-time system information    |
| `unraid://array`      | Real-time array status          |
| `unraid://containers` | Real-time Docker container list |
| `unraid://vms`        | Real-time VM list               |
| `unraid://disks`      | Real-time disk information      |

## MCP Prompts

Prompts provide guided interactions for common tasks:

| Prompt                       | Description                                                                 |
| ---------------------------- | --------------------------------------------------------------------------- |
| `diagnose_disk_health`       | Walks SMART data, temperatures, error rates, and power-on hours per disk    |
| `diagnose_performance_issue` | Correlates CPU, RAM, Docker usage, and VM count to find bottlenecks         |
| `suggest_maintenance`        | Reviews parity history, disk ages, errors, and temps for a maintenance plan |
| `explain_array_state`        | Translates raw array status into plain language with recommended actions    |
| `system_overview`            | Comprehensive summary of system status                                      |
| `troubleshoot_issue`         | Interactive troubleshooting assistant                                       |

## Example Usage

### Using with VS Code / GitHub Copilot

Add to your VS Code workspace (`.vscode/mcp.json`):

```json
{
  "servers": {
    "unraid": {
      "type": "http",
      "url": "http://your-unraid-ip:8043/mcp"
    }
  }
}
```

After configuration, restart the MCP server in VS Code:

1. Open Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`)
2. Run **"MCP: List Servers"** to verify configuration
3. Run **"MCP: Restart Server"** if needed

### Using with Cursor

Add to your Cursor MCP configuration (Settings → MCP → Add Server):

```json
{
  "mcpServers": {
    "unraid": {
      "type": "http",
      "url": "http://your-unraid-ip:8043/mcp"
    }
  }
}
```

### Using with Claude Desktop

**Option A: Remote (Streamable HTTP)** — when Claude Desktop runs on a different machine:

Configure via the Claude.ai web UI:

1. Go to [claude.ai/settings/connectors](https://claude.ai/settings/connectors)
2. Click **"Add Connector"**
3. Enter the URL: `http://your-unraid-ip:8043/mcp`

> **Note:** Claude Desktop does **not** connect to remote servers configured
> directly via `claude_desktop_config.json` — that file is only for local
> servers using the stdio transport. Remote servers must be added via
> Settings → Connectors.

**Option B: Local (STDIO)** — when Claude Desktop runs directly on the Unraid server:

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "unraid": {
      "command": "/usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent",
      "args": ["mcp-stdio"]
    }
  }
}
```

> STDIO is preferred for local use — zero network overhead, no port/firewall
> configuration, and the OS process model provides implicit security.

### Using with Windsurf

Add to your Windsurf MCP configuration (`~/.codeium/windsurf/mcp_config.json`):

```json
{
  "mcpServers": {
    "unraid": {
      "serverUrl": "http://your-unraid-ip:8043/mcp"
    }
  }
}
```

### Using with Codex (OpenAI)

Codex supports HTTP streaming transports. Configure via:

```json
{
  "mcpServers": {
    "unraid": {
      "type": "http",
      "url": "http://your-unraid-ip:8043/mcp"
    }
  }
}
```

### Using with Gemini CLI

Add to your Gemini CLI MCP settings:

```json
{
  "mcpServers": {
    "unraid": {
      "url": "http://your-unraid-ip:8043/mcp"
    }
  }
}
```

### Direct API Calls

The MCP endpoint uses Streamable HTTP transport. All requests require proper
initialization with a session. Here are complete working examples:

**Example 1: Initialize a session and get system info**

```bash
# Step 1: Initialize and capture session ID
curl -s -D /tmp/mcp-headers -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-06-18",
      "capabilities": {},
      "clientInfo": { "name": "my-client", "version": "1.0.0" }
    },
    "id": 1
  }'

# Extract session ID from response headers
SESSION_ID=$(grep -i "Mcp-Session-Id" /tmp/mcp-headers | tr -d '\r' | awk '{print $2}')

# Step 2: Call a tool using the session
curl -s -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "get_system_info",
      "arguments": {}
    },
    "id": 2
  }'
```

**Example 2: List all available tools with annotations**

```bash
curl -s -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 3
  }'
```

Each tool in the response includes an `annotations` object with safety hints:

```json
{
  "name": "get_system_info",
  "annotations": { "readOnlyHint": true },
  "description": "...",
  "inputSchema": { ... }
}
```

**Example 3: Read a resource (real-time system data)**

```bash
curl -s -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": { "uri": "unraid://system" },
    "id": 4
  }'
```

**Example 4: List running Docker containers**

```bash
curl -s -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "list_containers",
      "arguments": { "state": "running" }
    },
    "id": 5
  }'
```

**Example 5: Get a diagnostic summary (comprehensive health check)**

```bash
curl -s -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "get_diagnostic_summary",
      "arguments": {}
    },
    "id": 6
  }'
```

**Example 6: Restart a Docker container (destructive, no confirmation needed)**

```bash
curl -s -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "container_action",
      "arguments": {
        "container_id": "plex",
        "action": "restart"
      }
    },
    "id": 7
  }'
```

**Example 7: Stop the array (destructive, confirmation required)**

```bash
curl -s -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "array_action",
      "arguments": {
        "action": "stop",
        "confirm": true
      }
    },
    "id": 8
  }'
```

**Example 8: Terminate a session**

```bash
curl -s -X DELETE http://your-unraid-ip:8043/mcp \
  -H "Mcp-Session-Id: $SESSION_ID"
```

### Python Client Example

```python
import json
import requests

class UnraidMCPClient:
    """MCP client for the Unraid Management Agent."""

    def __init__(self, base_url: str = "http://your-unraid-ip:8043/mcp"):
        self.base_url = base_url
        self.session_id = None
        self._request_id = 0

    def _next_id(self) -> int:
        self._request_id += 1
        return self._request_id

    def _headers(self) -> dict:
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
        }
        if self.session_id:
            headers["Mcp-Session-Id"] = self.session_id
        return headers

    def _parse_sse(self, text: str) -> dict:
        """Parse SSE response to extract JSON data."""
        for line in text.strip().split("\n"):
            if line.startswith("data: "):
                return json.loads(line[6:])
        return json.loads(text)

    def initialize(self) -> dict:
        """Initialize the MCP session."""
        resp = requests.post(self.base_url, headers=self._headers(), json={
            "jsonrpc": "2.0",
            "method": "initialize",
            "params": {
                "protocolVersion": "2025-06-18",
                "capabilities": {},
                "clientInfo": {"name": "python-client", "version": "1.0.0"},
            },
            "id": self._next_id(),
        })
        self.session_id = resp.headers.get("Mcp-Session-Id")
        return self._parse_sse(resp.text)

    def call_tool(self, name: str, arguments: dict = None) -> dict:
        """Call an MCP tool."""
        resp = requests.post(self.base_url, headers=self._headers(), json={
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {"name": name, "arguments": arguments or {}},
            "id": self._next_id(),
        })
        return self._parse_sse(resp.text)

    def read_resource(self, uri: str) -> dict:
        """Read an MCP resource."""
        resp = requests.post(self.base_url, headers=self._headers(), json={
            "jsonrpc": "2.0",
            "method": "resources/read",
            "params": {"uri": uri},
            "id": self._next_id(),
        })
        return self._parse_sse(resp.text)

    def list_tools(self) -> list:
        """List all available tools."""
        resp = requests.post(self.base_url, headers=self._headers(), json={
            "jsonrpc": "2.0",
            "method": "tools/list",
            "id": self._next_id(),
        })
        result = self._parse_sse(resp.text)
        return result.get("result", {}).get("tools", [])

    def close(self):
        """Terminate the MCP session."""
        if self.session_id:
            requests.delete(self.base_url, headers=self._headers())

# Usage
client = UnraidMCPClient("http://192.168.1.100:8043/mcp")
client.initialize()

# Get system info
system = client.call_tool("get_system_info")
data = json.loads(system["result"]["content"][0]["text"])
print(f"Hostname: {data['hostname']}, CPU: {data['cpu_usage_percent']:.1f}%")

# List running containers
containers = client.call_tool("list_containers", {"state": "running"})
for c in json.loads(containers["result"]["content"][0]["text"]):
    print(f"  {c['name']}: {c['state']} ({c['memory_display']})")

# Read system resource directly
resource = client.read_resource("unraid://system")
print(f"Resource: {resource['result']['contents'][0]['uri']}")

# List tools with safety annotations
tools = client.list_tools()
read_only = [t["name"] for t in tools if t.get("annotations", {}).get("readOnlyHint")]
destructive = [t["name"] for t in tools if t.get("annotations", {}).get("destructiveHint")]
print(f"Read-only tools: {len(read_only)}, Destructive tools: {len(destructive)}")

client.close()
```

## Security Considerations

1. **Network Access**: The MCP endpoint is exposed on the same port as the REST API. Ensure your firewall rules restrict access appropriately.

2. **Destructive Operations**: Array and system power operations require explicit confirmation via the `confirm: true` parameter.

3. **Input Validation**: All inputs are validated using the same security functions as the REST API to prevent command injection.

4. **Logging**: All MCP control actions are logged with timestamps for audit purposes.

## Limitations

- Real-time updates should use WebSocket (`/ws`) in addition to MCP for push notifications
- Some operations may take time to complete; poll status endpoints for updates

## Transport Types

| Transport              | Best For                           | Features                                                |
| ---------------------- | ---------------------------------- | ------------------------------------------------------- |
| **Streamable HTTP** ⭐ | Remote AI clients over the network | Request/response, SSE streaming, session management     |
| **STDIO**              | Local AI clients on the server     | Newline-delimited JSON over stdin/stdout, zero overhead |

### Streamable HTTP Transport Details (MCP Spec 2025-06-18)

Built on the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) v1.2.0,
the Streamable HTTP transport at `/mcp` supports:

- **POST**: Send JSON-RPC requests and notifications
  - Requests return `Content-Type: application/json` responses
  - Notifications return `202 Accepted` with no body
- **GET**: Open an SSE stream for server-initiated messages (requires `Accept: text/event-stream` header)
- **DELETE**: Terminate the session (requires `Mcp-Session-Id` header)
- **OPTIONS**: CORS preflight handling

**Session Management:** The server assigns an `Mcp-Session-Id` on initialization. Clients should include this header in subsequent requests.

### STDIO Transport Details

The STDIO transport runs the MCP server over stdin/stdout using newline-delimited JSON.
It is started via the `mcp-stdio` CLI subcommand:

```bash
/usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent mcp-stdio
```

**Key characteristics:**

- **No HTTP server started** — communicates exclusively via stdin/stdout
- **Logs go to stderr + file** — stdout is reserved for MCP JSON-RPC protocol messages
- **Collectors run internally** — the STDIO process starts its own data collectors so all tools return live data
- **Graceful shutdown** — responds to SIGTERM/SIGINT with full collector cleanup
- **Designed for process spawning** — MCP clients like Claude Desktop launch the process and manage its lifecycle

**When to use STDIO:**

- The AI client runs on the same machine as the Unraid server
- You want zero network overhead and no port/firewall configuration
- The MCP client supports STDIO spawning (Claude Desktop, Cursor local mode)

**Client Compatibility:**

| Client         | Streamable HTTP | STDIO        | Notes                                  |
| -------------- | --------------- | ------------ | -------------------------------------- |
| Cursor         | ✅ Supported    | ✅ Supported | Use HTTP for remote, STDIO for local   |
| Claude Desktop | ✅ Supported    | ✅ Supported | STDIO via `claude_desktop_config.json` |
| GitHub Copilot | ✅ Supported    | —            | HTTP only                              |
| Codex          | ✅ Supported    | ✅ Supported | Supports both transports               |
| Windsurf       | ✅ Supported    | ✅ Supported | Supports both transports               |
| Gemini CLI     | ✅ Supported    | ✅ Supported | Supports both transports               |
| VS Code MCP    | ✅ Supported    | ✅ Supported | HTTP for remote, STDIO for local       |

## Troubleshooting

**"Connection refused":**

- Verify the agent is running: `ps aux | grep unraid-management-agent`
- Check the port is accessible: `netstat -tlnp | grep 8043`

**"Tool not found":**

- List available tools with the `tools/list` method
- Check tool name spelling (use underscores, not hyphens)

**"Action not confirmed":**

- For destructive actions, include `"confirm": true` in the arguments

**VS Code "Waiting for server to respond to initialize request":**

- Ensure the agent is running and accessible from your machine
- Check firewall rules allow connections to port 8043
- Restart the MCP server: Command Palette → "MCP: Restart Server"

**Cursor "No server info found":**

- Use the Streamable HTTP endpoint: `http://your-unraid-ip:8043/mcp`
- Ensure your config uses `"type": "http"` (not `"sse"`)
- Update to the latest version of the agent which supports the MCP 2025-06-18 spec

## Related Documentation

- [REST API Reference](api/API_REFERENCE.md)
- [WebSocket Events](websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md)
- [MCP Protocol Specification](https://modelcontextprotocol.io/docs)
