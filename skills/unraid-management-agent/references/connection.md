# Connecting Clients

The agent serves MCP over two transports. Pick by client.

| Transport | Endpoint / command | Best for |
| --- | --- | --- |
| **Streamable HTTP** | `http://<unraid-ip>:8043/mcp` | Remote/LAN clients (most cases) |
| **STDIO** | `unraid-management-agent mcp-stdio` | Clients running **on the Unraid box** |

Replace `<unraid-ip>` with the server's address (default port `8043`). The
protocol is MCP `2025-06-18` via the official Go SDK.

> **Authentication:** none by default. The agent is designed for a trusted LAN or
> behind a VPN/reverse proxy. Do not assume an API key or auth header exists.

## Streamable HTTP (recommended)

Most MCP clients accept an HTTP URL. Example client config:

```json
{
  "mcpServers": {
    "unraid": {
      "type": "http",
      "url": "http://192.168.1.10:8043/mcp"
    }
  }
}
```

- **Claude Code:** `claude mcp add --transport http unraid http://<unraid-ip>:8043/mcp`
- **Claude Desktop / claude.ai:** add a custom connector with the URL above.
- **Cursor / VS Code (Copilot) / Windsurf / Gemini CLI:** add an HTTP MCP server
  entry pointing at the same URL (see each client's MCP settings).

## STDIO (local only)

When the MCP client runs on the Unraid server itself, spawn the binary:

```json
{
  "mcpServers": {
    "unraid": {
      "command": "unraid-management-agent",
      "args": ["mcp-stdio"]
    }
  }
}
```

In STDIO mode, stdout is reserved for MCP JSON-RPC; logs go to file + stderr.

## ChatGPT and other non-MCP clients

ChatGPT Custom GPTs consume **REST Actions**, not MCP. Use the OpenAPI schema at
`docs/integrations/chatgpt/openapi-actions.yaml` and follow
`docs/integrations/chatgpt/README.md`. See also `rest-api.md`.

## Verifying connectivity

- REST health: `curl http://<unraid-ip>:8043/api/v1/health`
- Swagger UI: `http://<unraid-ip>:8043/swagger/`
- MCP endpoint: `http://<unraid-ip>:8043/mcp` (POST, MCP transport)

## Troubleshooting

| Symptom | Likely cause / fix |
| --- | --- |
| Connection refused | Agent not running, or wrong port — check the plugin status and that port 8043 is reachable |
| Works locally, not from another host | Firewall/subnet — ensure the client's network can reach `<unraid-ip>:8043` |
| Client can't auto-discover the server | The agent advertises mDNS as `_unraid-mgmt-agent._tcp.local.`; the client must support zeroconf discovery, otherwise enter host/port manually |
| 404 on `/mcp` | Older agent build without the MCP endpoint — update the plugin |
| Tools missing for a subsystem | That collector/controller is disabled or unavailable on the host (e.g. no GPU/UPS) |
