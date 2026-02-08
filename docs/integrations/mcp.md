# Model Context Protocol (MCP) Integration

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

## Endpoints

The MCP server provides two transport options:

| Transport              | Endpoint                                       | Description                                                                  |
| ---------------------- | ---------------------------------------------- | ---------------------------------------------------------------------------- |
| **Streamable HTTP** ⭐ | `POST/GET/DELETE http://<unraid-ip>:8043/mcp`  | Modern MCP transport (spec 2025-03-26) — **recommended for all AI clients**  |
| **SSE (Legacy)**       | `GET/POST http://<unraid-ip>:8043/mcp/sse`     | Deprecated HTTP+SSE transport (spec 2024-11-05) for backward compatibility   |

> **Note:** The Streamable HTTP transport at `/mcp` is the primary endpoint and supports all modern AI clients
> including Cursor, Claude Desktop, GitHub Copilot, Codex, Windsurf, and Gemini CLI.
> The legacy `/mcp/sse` endpoint is maintained for backward compatibility with older clients.

## Available Tools (54 total)

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

| Tool                  | Description                                     |
| --------------------- | ----------------------------------------------- |
| `list_containers`     | Docker containers, optionally filtered by state |
| `get_container_info`  | Detailed information about a specific container |
| `search_containers`   | Search containers by name or state              |
| `get_docker_settings` | Docker daemon configuration settings            |

### VM Tools

| Tool              | Description                              |
| ----------------- | ---------------------------------------- |
| `list_vms`        | Virtual machines, optionally filtered    |
| `get_vm_info`     | Detailed information about a specific VM |
| `search_vms`      | Search VMs by name or state              |
| `get_vm_settings` | VM manager configuration settings        |

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
| `vm_action`                 | Virtual machine control                           | start, stop, restart, pause, resume, hibernate, force-stop |
| `array_action`              | Array control (**use with caution**)              | start, stop                                                |
| `parity_check_action`       | Start parity check                                | correcting or non-correcting                               |
| `parity_check_stop`         | Stop a running parity check                       | -                                                          |
| `parity_check_pause`        | Pause a running parity check                      | -                                                          |
| `parity_check_resume`       | Resume a paused parity check                      | -                                                          |
| `disk_spin_down`            | Spin down a specific disk                         | -                                                          |
| `disk_spin_up`              | Spin up a specific disk                           | -                                                          |
| `execute_user_script`       | Execute a user script (**requires confirmation**) | -                                                          |
| `collector_action`          | Enable or disable a data collector                | enable, disable                                            |
| `update_collector_interval` | Update a collector's polling interval             | -                                                          |
| `system_reboot`             | Reboot the server (**requires confirmation**)     | -                                                          |
| `system_shutdown`           | Shutdown the server (**requires confirmation**)   | -                                                          |

> **⚠️ Warning:** Destructive actions (array stop, reboot, shutdown, user scripts) require explicit confirmation via the `confirm: true` parameter.

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

Since the Unraid Management Agent runs as a **remote** MCP server (over the network),
configure it via the Claude.ai web UI:

1. Go to [claude.ai/settings/connectors](https://claude.ai/settings/connectors)
2. Click **"Add Connector"**
3. Enter the URL: `http://your-unraid-ip:8043/mcp`

> **Note:** Claude Desktop does **not** connect to remote servers configured
> directly via `claude_desktop_config.json` — that file is only for local
> servers using the stdio transport. Remote servers must be added via
> Settings → Connectors.

Claude Desktop supports both Streamable HTTP and SSE transports (Pro, Max, Team, and Enterprise plans).

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

**List all tools:**

```bash
curl -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 1
  }'
```

**Get system information:**

```bash
curl -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "get_system_info",
      "arguments": {}
    },
    "id": 1
  }'
```

**Start a Docker container:**

```bash
curl -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "container_action",
      "arguments": {
        "container_id": "plex",
        "action": "start"
      }
    },
    "id": 1
  }'
```

**Stop the array (requires confirmation):**

```bash
curl -X POST http://your-unraid-ip:8043/mcp \
  -H "Content-Type: application/json" \
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
    "id": 1
  }'
```

### Python Client Example

```python
import json
import requests

def call_mcp_tool(tool_name: str, arguments: dict = None):
    """Call an MCP tool on the Unraid server."""
    response = requests.post(
        "http://your-unraid-ip:8043/mcp",
        json={
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {
                "name": tool_name,
                "arguments": arguments or {}
            },
            "id": 1
        }
    )
    return response.json()

# Get system info
system = call_mcp_tool("get_system_info")
print(f"Hostname: {system['result']['content'][0]['text']}")

# List running containers
containers = call_mcp_tool("list_containers", {"state": "running"})
print(f"Running containers: {containers['result']['content'][0]['text']}")

# Restart a container
result = call_mcp_tool("container_action", {
    "container_id": "plex",
    "action": "restart"
})
print(f"Result: {result['result']['content'][0]['text']}")
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

| Transport              | Best For                          | Features                                                  |
| ---------------------- | --------------------------------- | --------------------------------------------------------- |
| **Streamable HTTP** ⭐ | All modern AI clients             | Request/response, SSE streaming, session management       |
| **SSE (Legacy)**       | Older clients (pre-2025)          | Streaming responses, server push (deprecated)             |
| **Stdio**              | Local CLI tools                   | Direct process communication                              |

### Streamable HTTP Transport Details (MCP Spec 2025-03-26)

The Streamable HTTP transport at `/mcp` supports:

- **POST**: Send JSON-RPC requests and notifications
  - Requests return `Content-Type: application/json` responses
  - Notifications return `202 Accepted` with no body
- **GET**: Open an SSE stream for server-initiated messages (requires `Accept: text/event-stream` header)
- **DELETE**: Terminate the session (requires `Mcp-Session-Id` header)
- **OPTIONS**: CORS preflight handling

**Session Management:** The server assigns an `Mcp-Session-Id` on initialization. Clients should include this header in subsequent requests.

**Client Compatibility:**

| Client         | Transport Used  | Status       |
| -------------- | --------------- | ------------ |
| Cursor         | Streamable HTTP | ✅ Supported |
| Claude Desktop | Streamable HTTP | ✅ Supported |
| GitHub Copilot | HTTP            | ✅ Supported |
| Codex          | HTTP Streaming  | ✅ Supported |
| Windsurf       | Streamable HTTP | ✅ Supported |
| Gemini CLI     | HTTP            | ✅ Supported |
| VS Code MCP    | HTTP / SSE      | ✅ Supported |

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
- Update to the latest version of the agent which supports the MCP 2025-03-26 spec

**404 on /mcp/sse:**

- The SSE endpoint is for legacy clients only. Modern clients should use `/mcp`
- Ensure you're using the latest version of the agent

## Related Documentation

- [REST API Reference](api/API_REFERENCE.md)
- [WebSocket Events](websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md)
- [MCP Protocol Specification](https://modelcontextprotocol.io/docs)
