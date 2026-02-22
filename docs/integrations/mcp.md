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

## Available Tools (72 total)

### System Monitoring Tools

| Tool                      | Description                                                               |
| ------------------------- | ------------------------------------------------------------------------- |
| `get_system_info`         | System information including hostname, CPU, RAM, temperatures, and uptime |
| `get_array_status`        | Array state, capacity, parity information, and disk assignments           |
| `get_hardware_info`       | Motherboard, CPU, and memory details from DMI/SMBIOS                      |
| `get_registration`        | Unraid license and registration information                               |
| `get_health_status`       | Overall system health status                                              |
| `get_diagnostic_summary`  | Comprehensive diagnostic summary including all subsystems                 |
| `get_network_access_urls` | All available access URLs (LAN, WAN, mDNS, IPv6)                          |

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

| Tool                      | Description                                           |
| ------------------------- | ----------------------------------------------------- |
| `list_containers`         | Docker containers, optionally filtered by state       |
| `get_container_info`      | Detailed information about a specific container       |
| `get_container_logs`      | Container stdout/stderr logs with tail/since opts     |
| `search_containers`       | Search containers by name or state                    |
| `get_docker_settings`     | Docker daemon configuration settings                  |
| `check_container_updates` | Check all containers for available image updates      |
| `check_container_update`  | Check a specific container for an image update        |
| `get_container_size`      | Get disk usage (image size + rw layer) of a container |

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

| Tool                   | Description                                       |
| ---------------------- | ------------------------------------------------- |
| `check_plugin_updates` | Check all installed plugins for available updates |

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

## Tool Safety Annotations

All 70 tools include MCP safety annotations to help AI agents make safe decisions automatically:

### Read-Only Tools (49 tools)

All monitoring and query tools are annotated with `readOnlyHint: true`, signaling to AI agents that these tools are safe to call without side effects:

```
get_system_info, get_array_status, get_hardware_info, get_health_status,
get_diagnostic_summary, get_registration, get_network_info, get_network_access_urls,
get_ups_status, get_nut_status, get_gpu_metrics, list_disks, get_disk_info,
get_disk_settings, list_shares, get_share_config, get_unassigned_devices,
get_zfs_pools, get_zfs_datasets, get_zfs_snapshots, get_zfs_arc_stats,
list_containers, get_container_info, search_containers, get_docker_settings,
check_container_updates, check_container_update, get_container_logs, get_container_size,
list_vms, get_vm_info, search_vms, get_vm_settings, list_vm_snapshots,
check_plugin_updates, get_service_status, list_services, list_processes,
get_notifications, get_notifications_overview, list_log_files, get_log_content,
get_syslog, get_docker_log, get_parity_history, list_user_scripts,
list_collectors, get_collector_status, get_system_settings
```

### Destructive Tools (15 tools) — `destructiveHint: true`

These tools make changes that may be difficult or impossible to reverse:

| Tool                    | Additional Hints       | Confirmation Required |
| ----------------------- | ---------------------- | --------------------- |
| `container_action`      | `idempotentHint: true` | No                    |
| `update_container`      | —                      | Yes (`confirm: true`) |
| `update_all_containers` | —                      | Yes (`confirm: true`) |
| `vm_action`             | `idempotentHint: true` | No                    |
| `create_vm_snapshot`    | —                      | Yes (`confirm: true`) |
| `delete_vm_snapshot`    | —                      | Yes (`confirm: true`) |
| `restore_vm_snapshot`   | —                      | Yes (`confirm: true`) |
| `clone_vm`              | —                      | Yes (`confirm: true`) |
| `array_action`          | `idempotentHint: true` | Yes (`confirm: true`) |
| `update_plugin`         | —                      | Yes (`confirm: true`) |
| `update_all_plugins`    | —                      | Yes (`confirm: true`) |
| `service_action`        | `idempotentHint: true` | Yes (`confirm: true`) |
| `execute_user_script`   | —                      | Yes (`confirm: true`) |
| `system_reboot`         | —                      | Yes (`confirm: true`) |
| `system_shutdown`       | —                      | Yes (`confirm: true`) |

### Non-Destructive Control Tools (8 tools) — `destructiveHint: false`

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

| Prompt                | Description                                            |
| --------------------- | ------------------------------------------------------ |
| `analyze_disk_health` | AI-guided analysis of disk health with recommendations |
| `system_overview`     | Comprehensive summary of system status                 |
| `troubleshoot_issue`  | Interactive troubleshooting assistant                  |

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
