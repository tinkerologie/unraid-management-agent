# Claude Integration

Two ways to give Claude knowledge of and control over your Unraid server:

1. **Agent Skill** — a portable knowledge pack that teaches Claude how to use the
   agent's tools effectively. Works in Claude Code, Claude Desktop, and claude.ai.
2. **MCP connection** — the live tool/data connection to your running server.

You typically want **both**: the skill teaches _how_, the MCP connection provides
the _live tools_.

## 1. Install the Agent Skill

The skill lives at [`skills/unraid-management-agent/`](../../../skills/unraid-management-agent/)
and follows the open [Agent Skills standard](https://agentskills.io). Full
install instructions (npx installer, Claude Code plugin, claude.ai zip upload)
are in [`skills/README.md`](../../../skills/README.md).

Quick reference:

```bash
# Claude Code / Cursor / Copilot / Gemini CLI
npx skills add ruaan-deysel/unraid-management-agent
```

```text
# Claude Code plugin
/plugin marketplace add ruaan-deysel/unraid-management-agent
/plugin install unraid-management-agent-skills@unraid-management-agent-skills
```

For **claude.ai / Claude Desktop**, zip the skill folder and upload it under
Settings → Capabilities → Skills (see `skills/README.md`).

## 2. Connect Claude to your server (MCP)

The agent serves a Streamable HTTP MCP endpoint at `/mcp`. How you connect
depends on the client:

### Claude Code

Claude Code accepts the URL directly:

```bash
claude mcp add --transport http unraid http://<unraid-ip>:8043/mcp
```

If you've enabled native TLS (`--tls-cert-file`/`--tls-key-file`), use `https://`
with the hostname the certificate is issued for (e.g.
`https://<server>.myunraid.net:8043/mcp`) — a trusted cert validates against its
hostname, so a raw `https://<ip>` URL only works if the certificate includes
that IP as a Subject Alternative Name.

### Claude Desktop / claude.ai — "Custom Connector"

> [!IMPORTANT]
> A custom connector is reached **from Anthropic's cloud, not from your
> computer.** Anthropic requires the URL to be **HTTPS with a publicly-trusted
> certificate**, and the server must be **reachable from the public internet**.
> A LAN address such as `http://<unraid-ip>:8043/mcp` (or even `https://` with a
> self-signed cert) is rejected — you'll see _"URL must start with https"_ or
> _"our servers cannot reach your local machine."_ This is the cause of
> [#131](https://github.com/ruaan-deysel/unraid-management-agent/issues/131).

To use a custom connector you must expose the agent over **public HTTPS with a
trusted cert**, via either:

- **Native HTTPS** — point the agent at a trusted certificate (e.g. Unraid's
  `myunraid.net` cert) with `--tls-cert-file` / `--tls-key-file` (see
  [configuration guide](../../guides/configuration.md#https--tls)), then
  port-forward or tunnel `https://<public-host>:8043/mcp`.
- **A reverse proxy or tunnel** that terminates a trusted cert in front of the
  agent — e.g. SWAG/Nginx Proxy Manager, Cloudflare Tunnel, or Tailscale Funnel.

Then in **Settings → Connectors → Add custom connector**, enter the public
`https://…/mcp` URL.

### Claude Desktop on a LAN (no public exposure) — `mcp-remote` bridge

If you only use the agent on your local network, the simplest option is the
[`mcp-remote`](https://www.npmjs.com/package/mcp-remote) stdio bridge. Claude
Desktop speaks stdio to `mcp-remote`, which runs on your computer and talks to
the agent over the LAN — so plain HTTP and a LAN IP work fine. Requires
[Node.js](https://nodejs.org). Edit `claude_desktop_config.json` (Settings →
Developer → Edit Config) and add:

```json
{
  "mcpServers": {
    "unraid": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "http://<unraid-ip>:8043/mcp", "--allow-http", "--transport", "http-only"]
    }
  }
}
```

`--allow-http` permits the plain-HTTP LAN connection (use only on a trusted
network) and `--transport http-only` matches the agent's Streamable HTTP
endpoint (it has no SSE half, so this avoids a startup delay). Restart Claude
Desktop after saving.

### Local, on the Unraid box itself

Use STDIO — `unraid-management-agent mcp-stdio`. No URL, no TLS.

Details and per-client config: [`../mcp.md`](../mcp.md) and the skill's
[`references/connection.md`](../../../skills/unraid-management-agent/references/connection.md).

## Claude Projects

For a Claude Project, upload the skill's reference files as project knowledge so
the assistant has the tool catalog and workflows in context:

- `skills/unraid-management-agent/SKILL.md`
- `skills/unraid-management-agent/references/*.md`

Then add the MCP connector (above) so the project can actually call the tools.

## Troubleshooting

See the skill's [`references/connection.md`](../../../skills/unraid-management-agent/references/connection.md#troubleshooting)
for connection issues. The agent has no auth by default — keep it on a trusted
LAN or behind a VPN/reverse proxy.
